package policy

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/example/cc-panel/internal/ipset"
	"github.com/example/cc-panel/internal/remotedeps"
	serverrepo "github.com/example/cc-panel/internal/server"
	"github.com/example/cc-panel/internal/sshx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

//go:embed connection_scan.sh
var connectionScanScript string

type Policy struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Metric        string    `json:"metric"`
	Threshold     int       `json:"threshold"`
	WindowSeconds int       `json:"window_seconds"`
	BlockSeconds  int       `json:"block_seconds"`
	TargetSet     string    `json:"target_set"`
	Enabled       bool      `json:"enabled"`
	CreatedBy     *int64    `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Event struct {
	ID            int64          `json:"id"`
	PolicyID      int64          `json:"policy_id"`
	ServerID      int64          `json:"server_id"`
	Metric        string         `json:"metric"`
	ObservedValue float64        `json:"observed_value"`
	Threshold     int            `json:"threshold"`
	Action        string         `json:"action"`
	Detail        map[string]any `json:"detail"`
	CreatedAt     time.Time      `json:"created_at"`
}

type EventListResult struct {
	Items    []Event `json:"items"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}

type Input struct {
	Name          string `json:"name"`
	Metric        string `json:"metric"`
	Threshold     int    `json:"threshold"`
	WindowSeconds int    `json:"window_seconds"`
	BlockSeconds  int    `json:"block_seconds"`
	TargetSet     string `json:"target_set"`
	Enabled       bool   `json:"enabled"`
}

type ipConnectionCount struct {
	IP    string
	Count int
}

type defaultWhitelistChecker interface {
	MatchesDefaultWhitelistIP(ip string) (bool, error)
}

type Service struct {
	db      *pgxpool.Pool
	servers *serverrepo.Repository
	ssh     sshx.Executor
	geo     defaultWhitelistChecker
	timeout time.Duration
}

func NewService(db *pgxpool.Pool, servers *serverrepo.Repository, ssh sshx.Executor, geoChecker defaultWhitelistChecker, timeout time.Duration) *Service {
	return &Service{db: db, servers: servers, ssh: ssh, geo: geoChecker, timeout: timeout}
}

func (s *Service) List(ctx context.Context, limit int) ([]Policy, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, name, metric, threshold, window_seconds, block_seconds, target_set, enabled, created_by, created_at, updated_at
		FROM auto_policies
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Policy, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ListEvents(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	result, err := s.ListEventsPage(ctx, 1, limit)
	if err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (s *Service) ListEventsPage(ctx context.Context, page, pageSize int) (EventListResult, error) {
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * pageSize

	var total int64
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM auto_policy_events`).Scan(&total); err != nil {
		return EventListResult{}, fmt.Errorf("count policy events: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, policy_id, server_id, metric, observed_value, threshold, action, detail, created_at
		FROM auto_policy_events
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2
	`, pageSize, offset)
	if err != nil {
		return EventListResult{}, fmt.Errorf("query policy events: %w", err)
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var detail []byte
		if err := rows.Scan(
			&event.ID, &event.PolicyID, &event.ServerID, &event.Metric, &event.ObservedValue,
			&event.Threshold, &event.Action, &detail, &event.CreatedAt,
		); err != nil {
			return EventListResult{}, fmt.Errorf("scan policy event: %w", err)
		}
		if err := json.Unmarshal(detail, &event.Detail); err != nil {
			return EventListResult{}, fmt.Errorf("unmarshal policy event detail: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return EventListResult{}, err
	}
	return EventListResult{
		Items:    events,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *Service) Execute(ctx context.Context, serverID int64) ([]Event, error) {
	target, err := s.servers.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if target.WhitelistMode != serverrepo.WhitelistModeConnectionCount {
		return nil, nil
	}

	policies, err := s.enabledPolicies(ctx)
	if err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return nil, nil
	}

	var ipCounts []ipConnectionCount
	needsConnections := false
	for _, item := range policies {
		if item.Metric == "connection_count" {
			needsConnections = true
			break
		}
	}
	if needsConnections {
		if err := s.cleanupMappedIPv4Blocks(ctx, serverID); err != nil {
			return nil, err
		}
		if err := s.removeDefaultWhitelistFromRateBlock(ctx, serverID); err != nil {
			return nil, err
		}
		ipCounts, err = s.collectNonWhitelistIPConnections(ctx, serverID)
		if err != nil {
			return nil, err
		}
		ipCounts = s.filterDefaultWhitelistIPs(ipCounts)
	}

	events := make([]Event, 0)
	for _, item := range policies {
		switch item.Metric {
		case "connection_count":
			policyEvents, err := s.executeConnectionCount(ctx, item, serverID, ipCounts)
			if err != nil {
				return nil, err
			}
			events = append(events, policyEvents...)
		default:
			event, err := s.recordEvent(ctx, item, serverID, 0, "skipped", map[string]any{
				"reason": "metric source not connected",
			})
			if err != nil {
				return nil, err
			}
			events = append(events, event)
		}
	}
	return events, nil
}

func (s *Service) ExecuteAll(ctx context.Context) (int, error) {
	servers, err := s.servers.List(ctx)
	if err != nil {
		return 0, err
	}
	totalEvents := 0
	group, groupCtx := errgroup.WithContext(ctx)
	ch := make(chan int, len(servers))
	for _, server := range servers {
		server := server
		group.Go(func() error {
			events, err := s.Execute(groupCtx, server.ID)
			if err != nil {
				ch <- 0
				return nil
			}
			ch <- len(events)
			return nil
		})
	}
	_ = group.Wait()
	close(ch)
	for count := range ch {
		totalEvents += count
	}
	return totalEvents, nil
}

func (s *Service) executeConnectionCount(ctx context.Context, policy Policy, serverID int64, ipCounts []ipConnectionCount) ([]Event, error) {
	events := make([]Event, 0)
	blocked := 0
	for _, item := range ipCounts {
		if item.Count < policy.Threshold {
			continue
		}
		command, err := ipset.AddTimedIPCommand(policy.TargetSet, item.IP, policy.BlockSeconds)
		if err != nil {
			return nil, err
		}
		if err := s.runOnServer(ctx, serverID, command); err != nil {
			return nil, fmt.Errorf("block %s: %w", item.IP, err)
		}
		event, err := s.recordEvent(ctx, policy, serverID, float64(item.Count), "blocked", map[string]any{
			"ip":             item.IP,
			"connections":    item.Count,
			"target_set":     policy.TargetSet,
			"block_seconds":  policy.BlockSeconds,
			"window_seconds": policy.WindowSeconds,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, event)
		blocked++
	}
	if blocked == 0 {
		event, err := s.recordEvent(ctx, policy, serverID, 0, "not_triggered", map[string]any{
			"scanned_ips": len(ipCounts),
			"threshold":   policy.Threshold,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Service) filterDefaultWhitelistIPs(items []ipConnectionCount) []ipConnectionCount {
	if s.geo == nil || len(items) == 0 {
		return items
	}
	filtered := make([]ipConnectionCount, 0, len(items))
	for _, item := range items {
		match, err := s.geo.MatchesDefaultWhitelistIP(item.IP)
		if err != nil || match {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (s *Service) cleanupMappedIPv4Blocks(ctx context.Context, serverID int64) error {
	return s.runOnServer(ctx, serverID, ipset.CleanupMappedIPv4BlockScript())
}

func (s *Service) removeDefaultWhitelistFromRateBlock(ctx context.Context, serverID int64) error {
	if s.geo == nil {
		return nil
	}
	creds, err := s.serverCredentials(ctx, serverID)
	if err != nil {
		return err
	}
	listScript := remotedeps.WithFirewallTools(`ipset list cc_rate_block 2>/dev/null | awk '/^Members:/{m=1;next} m&&NF==0{m=0} m&&$1 ~ /^[0-9a-fA-F:\.*]+$/{print $1}'`)
	result, err := s.ssh.Run(ctx, remotedeps.WithInstallTimeout(creds), listScript)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(result.Stdout, "\n") {
		ip := ipset.NormalizeIP(strings.TrimSpace(line))
		if ip == "" || net.ParseIP(ip) == nil {
			continue
		}
		match, err := s.geo.MatchesDefaultWhitelistIP(ip)
		if err != nil || !match {
			continue
		}
		command, err := ipset.DeleteTimedIPCommand(ipset.RateBlockSet, ip)
		if err != nil {
			return err
		}
		if err := s.runOnServer(ctx, serverID, command); err != nil {
			return fmt.Errorf("unblock %s: %w", ip, err)
		}
	}
	return nil
}

func (s *Service) collectNonWhitelistIPConnections(ctx context.Context, serverID int64) ([]ipConnectionCount, error) {
	creds, err := s.serverCredentials(ctx, serverID)
	if err != nil {
		return nil, err
	}
	result, err := s.ssh.Run(ctx, remotedeps.WithInstallTimeout(creds), remotedeps.WithFirewallTools(connectionScanScript))
	if err != nil {
		return nil, err
	}
	return parseConnectionCounts(result.Stdout)
}

func (s *Service) runOnServer(ctx context.Context, serverID int64, command string) error {
	creds, err := s.serverCredentials(ctx, serverID)
	if err != nil {
		return err
	}
	_, err = s.ssh.Run(ctx, remotedeps.WithInstallTimeout(creds), remotedeps.WithFirewallTools(command))
	return err
}

func (s *Service) serverCredentials(ctx context.Context, serverID int64) (sshx.Credentials, error) {
	target, err := s.servers.Get(ctx, serverID)
	if err != nil {
		return sshx.Credentials{}, err
	}
	password, privateKey, err := s.servers.Credentials(target)
	if err != nil {
		return sshx.Credentials{}, err
	}
	return sshx.Credentials{
		Host:       target.Host,
		Port:       target.Port,
		Username:   target.Username,
		AuthType:   target.AuthType,
		Password:   password,
		PrivateKey: privateKey,
		Timeout:    s.timeout,
	}, nil
}

func parseConnectionCounts(output string) ([]ipConnectionCount, error) {
	items := make([]ipConnectionCount, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		count, err := strconv.Atoi(parts[0])
		if err != nil || count <= 0 {
			continue
		}
		ip := ipset.NormalizeIP(parts[1])
		if net.ParseIP(ip) == nil {
			continue
		}
		items = append(items, ipConnectionCount{IP: ip, Count: count})
	}
	return items, nil
}

func (s *Service) Create(ctx context.Context, input Input, actorUserID *int64) (Policy, error) {
	input = input.withDefaults()
	if err := input.Validate(); err != nil {
		return Policy{}, err
	}
	var item Policy
	err := s.db.QueryRow(ctx, `
		INSERT INTO auto_policies (name, metric, threshold, window_seconds, block_seconds, target_set, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, name, metric, threshold, window_seconds, block_seconds, target_set, enabled, created_by, created_at, updated_at
	`, input.Name, input.Metric, input.Threshold, input.WindowSeconds, input.BlockSeconds, input.TargetSet, input.Enabled, actorUserID).Scan(
		&item.ID, &item.Name, &item.Metric, &item.Threshold, &item.WindowSeconds, &item.BlockSeconds,
		&item.TargetSet, &item.Enabled, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (s *Service) enabledPolicies(ctx context.Context) ([]Policy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, metric, threshold, window_seconds, block_seconds, target_set, enabled, created_by, created_at, updated_at
		FROM auto_policies
		WHERE enabled = TRUE
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Policy, 0)
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) recordEvent(ctx context.Context, item Policy, serverID int64, observed float64, action string, detail map[string]any) (Event, error) {
	payload, err := json.Marshal(detail)
	if err != nil {
		return Event{}, err
	}
	var event Event
	var rawDetail []byte
	err = s.db.QueryRow(ctx, `
		INSERT INTO auto_policy_events (policy_id, server_id, metric, observed_value, threshold, action, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, policy_id, server_id, metric, observed_value, threshold, action, detail, created_at
	`, item.ID, serverID, item.Metric, observed, item.Threshold, action, payload).Scan(
		&event.ID, &event.PolicyID, &event.ServerID, &event.Metric, &event.ObservedValue,
		&event.Threshold, &event.Action, &rawDetail, &event.CreatedAt,
	)
	if err != nil {
		return Event{}, err
	}
	if err := json.Unmarshal(rawDetail, &event.Detail); err != nil {
		return Event{}, err
	}
	return event, nil
}

func (s *Service) Update(ctx context.Context, id int64, input Input) (Policy, error) {
	input = input.withDefaults()
	if err := input.Validate(); err != nil {
		return Policy{}, err
	}
	row := s.db.QueryRow(ctx, `
		UPDATE auto_policies
		SET name = $1, metric = $2, threshold = $3, window_seconds = $4, block_seconds = $5,
			target_set = $6, enabled = $7, updated_at = NOW()
		WHERE id = $8
		RETURNING id, name, metric, threshold, window_seconds, block_seconds, target_set, enabled, created_by, created_at, updated_at
	`, input.Name, input.Metric, input.Threshold, input.WindowSeconds, input.BlockSeconds, input.TargetSet, input.Enabled, id)
	return scan(row)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM auto_policies WHERE id = $1", id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (input Input) Validate() error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch input.Metric {
	case "request_rate", "connection_count", "backend_path":
	default:
		return fmt.Errorf("metric must be request_rate, connection_count, or backend_path")
	}
	if input.Threshold <= 0 || input.WindowSeconds <= 0 || input.BlockSeconds <= 0 {
		return fmt.Errorf("threshold, window_seconds and block_seconds must be greater than 0")
	}
	if input.TargetSet != ipset.RateBlockSet && input.TargetSet != ipset.TempBlockSet {
		return fmt.Errorf("target_set must be %s or %s", ipset.RateBlockSet, ipset.TempBlockSet)
	}
	return nil
}

func (input Input) withDefaults() Input {
	if input.TargetSet == "" {
		input.TargetSet = ipset.RateBlockSet
	}
	return input
}

type scanner interface {
	Scan(dest ...any) error
}

func scan(row scanner) (Policy, error) {
	var item Policy
	err := row.Scan(
		&item.ID, &item.Name, &item.Metric, &item.Threshold, &item.WindowSeconds, &item.BlockSeconds,
		&item.TargetSet, &item.Enabled, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}
