package firewall

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/example/cc-panel/internal/ipset"
	"github.com/example/cc-panel/internal/iptables"
	"github.com/example/cc-panel/internal/remotedeps"
	serverrepo "github.com/example/cc-panel/internal/server"
	"github.com/example/cc-panel/internal/sshx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	ID             int64     `json:"id"`
	ServerID       int64     `json:"server_id"`
	ServerName     string    `json:"server_name,omitempty"`
	SetName        string    `json:"set_name"`
	IP             string    `json:"ip"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	Reason         *string   `json:"reason,omitempty"`
	CreatedBy      *int64    `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Snapshot struct {
	ID            int64     `json:"id"`
	ServerID      int64     `json:"server_id"`
	IptablesRules string    `json:"iptables_rules,omitempty"`
	IpsetRules    string    `json:"ipset_rules,omitempty"`
	Reason        string    `json:"reason"`
	CreatedBy     *int64    `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type AddEntryInput struct {
	ServerIDs      []int64 `json:"server_ids"`
	IP             string  `json:"ip"`
	TimeoutSeconds int     `json:"timeout"`
	Reason         *string `json:"reason,omitempty"`
}

type BulkAddEntryInput struct {
	ServerIDs      []int64  `json:"server_ids"`
	IPs            []string `json:"ips"`
	TimeoutSeconds int      `json:"timeout"`
	Reason         *string  `json:"reason,omitempty"`
}

type BulkAddResult struct {
	Added   int     `json:"added"`
	Skipped int     `json:"skipped"`
	Entries []Entry `json:"entries"`
}

type Status struct {
	ServerID              int64         `json:"server_id"`
	IPSets                []IPSetStatus `json:"ipsets"`
	IptablesRules         []string      `json:"iptables_rules"`
	IptablesCounts        []string      `json:"iptables_counts"`
	Mounted               bool          `json:"mounted"`
	WhitelistMode         string        `json:"whitelist_mode"`
	StrictWhitelist       bool          `json:"strict_whitelist"`
	StrictWhitelistActive bool          `json:"strict_whitelist_active"`
}

type IPSetStatus struct {
	Name    string `json:"name"`
	Exists  bool   `json:"exists"`
	Entries int    `json:"entries"`
}

type Service struct {
	db      *pgxpool.Pool
	servers *serverrepo.Repository
	ssh     sshx.Executor
	timeout time.Duration
}

func NewService(db *pgxpool.Pool, servers *serverrepo.Repository, ssh sshx.Executor, timeout time.Duration) *Service {
	return &Service{db: db, servers: servers, ssh: ssh, timeout: timeout}
}

func (s *Service) Deploy(ctx context.Context, serverID int64, actorUserID *int64) error {
	target, creds, err := s.sshCredentials(ctx, serverID)
	if err != nil {
		return err
	}
	depsCreds := remotedeps.WithInstallTimeout(creds)
	if _, err := s.ssh.Run(ctx, depsCreds, remotedeps.EnsureFirewallToolsScript()); err != nil {
		_ = s.servers.MarkOffline(ctx, target.ID)
		return err
	}
	snapshot, err := s.CreateSnapshot(ctx, serverID, actorUserID, "before_deploy")
	if err != nil {
		return err
	}
	if _, err := s.ssh.Run(ctx, depsCreds, remotedeps.WithFirewallTools(iptables.DeployScript(serverrepo.UsesStrictWhitelist(target.WhitelistMode)))); err != nil {
		_ = s.restoreSnapshotWithCredentials(ctx, creds, snapshot)
		_ = s.servers.MarkOffline(ctx, target.ID)
		return err
	}
	return s.servers.MarkOnline(ctx, target.ID)
}

func (s *Service) StopRules(ctx context.Context, serverID int64, actorUserID *int64) error {
	target, creds, err := s.sshCredentials(ctx, serverID)
	if err != nil {
		return err
	}
	depsCreds := remotedeps.WithInstallTimeout(creds)
	if _, err := s.ssh.Run(ctx, depsCreds, remotedeps.EnsureFirewallToolsScript()); err != nil {
		_ = s.servers.MarkOffline(ctx, target.ID)
		return err
	}
	if _, err := s.CreateSnapshot(ctx, serverID, actorUserID, "before_stop"); err != nil {
		return err
	}
	if _, err := s.ssh.Run(ctx, depsCreds, remotedeps.WithFirewallTools(iptables.StopScript())); err != nil {
		_ = s.servers.MarkOffline(ctx, target.ID)
		return err
	}
	return s.servers.MarkOnline(ctx, target.ID)
}

func (s *Service) CreateSnapshot(ctx context.Context, serverID int64, actorUserID *int64, reason string) (Snapshot, error) {
	_, creds, err := s.sshCredentials(ctx, serverID)
	if err != nil {
		return Snapshot{}, err
	}
	result, err := s.ssh.Run(ctx, remotedeps.WithInstallTimeout(creds), remotedeps.WithFirewallTools("iptables-save\nprintf '\\n__CC_PANEL_IPSET__\\n'\nipset save"))
	if err != nil {
		return Snapshot{}, err
	}
	parts := strings.SplitN(result.Stdout, "\n__CC_PANEL_IPSET__\n", 2)
	if len(parts) != 2 {
		return Snapshot{}, fmt.Errorf("unexpected snapshot output")
	}
	var snapshot Snapshot
	err = s.db.QueryRow(ctx, `
		INSERT INTO firewall_snapshots (server_id, iptables_rules, ipset_rules, reason, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, server_id, iptables_rules, ipset_rules, reason, created_by, created_at
	`, serverID, parts[0], parts[1], reason, actorUserID).Scan(
		&snapshot.ID, &snapshot.ServerID, &snapshot.IptablesRules, &snapshot.IpsetRules, &snapshot.Reason, &snapshot.CreatedBy, &snapshot.CreatedAt,
	)
	return snapshot, err
}

func (s *Service) RollbackLatest(ctx context.Context, serverID int64) (Snapshot, error) {
	snapshot, err := s.LatestSnapshot(ctx, serverID)
	if err != nil {
		return Snapshot{}, err
	}
	_, creds, err := s.sshCredentials(ctx, serverID)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, s.restoreSnapshotWithCredentials(ctx, creds, snapshot)
}

func (s *Service) LatestSnapshot(ctx context.Context, serverID int64) (Snapshot, error) {
	var snapshot Snapshot
	err := s.db.QueryRow(ctx, `
		SELECT id, server_id, iptables_rules, ipset_rules, reason, created_by, created_at
		FROM firewall_snapshots
		WHERE server_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, serverID).Scan(&snapshot.ID, &snapshot.ServerID, &snapshot.IptablesRules, &snapshot.IpsetRules, &snapshot.Reason, &snapshot.CreatedBy, &snapshot.CreatedAt)
	return snapshot, err
}

func (s *Service) restoreSnapshotWithCredentials(ctx context.Context, creds sshx.Credentials, snapshot Snapshot) error {
	iptablesEncoded := base64.StdEncoding.EncodeToString([]byte(snapshot.IptablesRules))
	ipsetEncoded := base64.StdEncoding.EncodeToString([]byte(snapshot.IpsetRules))
	command := fmt.Sprintf(`set -e
printf '%s' | base64 -d > /tmp/cc-panel-iptables.rules
printf '%s' | base64 -d > /tmp/cc-panel-ipset.rules
ipset restore -exist < /tmp/cc-panel-ipset.rules
iptables-restore < /tmp/cc-panel-iptables.rules
rm -f /tmp/cc-panel-iptables.rules /tmp/cc-panel-ipset.rules`, iptablesEncoded, ipsetEncoded)
	_, err := s.ssh.Run(ctx, remotedeps.WithInstallTimeout(creds), remotedeps.WithFirewallTools(command))
	return err
}

func (s *Service) TestSSH(ctx context.Context, serverID int64) error {
	target, creds, err := s.sshCredentials(ctx, serverID)
	if err != nil {
		return err
	}
	if _, err := s.ssh.Run(ctx, creds, "echo cc-panel-ok"); err != nil {
		_ = s.servers.MarkOffline(ctx, target.ID)
		return err
	}
	return s.servers.MarkOnline(ctx, target.ID)
}

func (s *Service) Status(ctx context.Context, serverID int64) (Status, error) {
	target, creds, err := s.sshCredentials(ctx, serverID)
	if err != nil {
		return Status{}, err
	}
	command := remotedeps.WithFirewallTools(`for set in cc_whitelist cc_geo_whitelist cc_blacklist cc_temp_block cc_geo_block cc_rate_block; do
  if ipset list "$set" >/tmp/cc-panel-ipset-status 2>/dev/null; then
    count=$(awk '/Number of entries:/ {print $4}' /tmp/cc-panel-ipset-status)
    echo "IPSET $set 1 ${count:-0}"
  else
    echo "IPSET $set 0 0"
  fi
done
rm -f /tmp/cc-panel-ipset-status
echo "__CC_PANEL_RULES__"
iptables -S INPUT | grep 'cc_' || true
echo "__CC_PANEL_COUNTS__"
iptables -L INPUT -v -n | grep 'cc_' || true`)
	result, err := s.ssh.Run(ctx, remotedeps.WithInstallTimeout(creds), command)
	if err != nil {
		return Status{}, err
	}
	return parseStatus(serverID, target.WhitelistMode, result.Stdout), nil
}

func parseStatus(serverID int64, whitelistMode string, output string) Status {
	status := Status{
		ServerID:        serverID,
		WhitelistMode:   whitelistMode,
		StrictWhitelist: serverrepo.UsesStrictWhitelist(whitelistMode),
		IPSets:          []IPSetStatus{},
		IptablesRules:   []string{},
		IptablesCounts:  []string{},
	}
	section := "ipsets"
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch line {
		case "__CC_PANEL_RULES__":
			section = "rules"
			continue
		case "__CC_PANEL_COUNTS__":
			section = "counts"
			continue
		}
		if section == "ipsets" && strings.HasPrefix(line, "IPSET ") {
			parts := strings.Fields(line)
			if len(parts) == 4 {
				entries, _ := strconv.Atoi(parts[3])
				status.IPSets = append(status.IPSets, IPSetStatus{Name: parts[1], Exists: parts[2] == "1", Entries: entries})
			}
			continue
		}
		if section == "rules" {
			status.IptablesRules = append(status.IptablesRules, line)
			if strings.Contains(line, "cc_geo_whitelist") || strings.Contains(line, "cc_whitelist") {
				status.Mounted = true
			}
			if strings.Contains(line, iptables.StrictWhitelistComment) {
				status.StrictWhitelistActive = true
			}
			continue
		}
		if section == "counts" {
			status.IptablesCounts = append(status.IptablesCounts, line)
		}
	}
	return status
}

func (s *Service) ListEntries(ctx context.Context, setName string, limit int) ([]Entry, error) {
	if setName != ipset.BlacklistSet && setName != ipset.WhitelistSet {
		return nil, fmt.Errorf("unsupported ipset %q", setName)
	}
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := s.db.Query(ctx, `
		SELECT fe.id, fe.server_id, s.name, fe.set_name, fe.ip, fe.timeout_seconds, fe.reason, fe.created_by, fe.created_at
		FROM firewall_entries fe
		JOIN servers s ON s.id = fe.server_id
		WHERE fe.set_name = $1
		ORDER BY fe.created_at DESC
		LIMIT $2
	`, setName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]Entry, 0)
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(
			&entry.ID,
			&entry.ServerID,
			&entry.ServerName,
			&entry.SetName,
			&entry.IP,
			&entry.TimeoutSeconds,
			&entry.Reason,
			&entry.CreatedBy,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Service) AddEntries(ctx context.Context, setName string, input AddEntryInput, actorUserID *int64) ([]Entry, error) {
	if len(input.ServerIDs) == 0 {
		return nil, fmt.Errorf("server_ids is required")
	}
	if input.TimeoutSeconds < 0 {
		return nil, fmt.Errorf("timeout must be greater than or equal to 0")
	}
	command, err := ipset.AddIPCommand(setName, input.IP, input.TimeoutSeconds)
	if err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(input.ServerIDs))
	for _, serverID := range input.ServerIDs {
		if err := s.runOnServer(ctx, serverID, command); err != nil {
			return nil, fmt.Errorf("server %d: %w", serverID, err)
		}
		entry, err := s.upsertEntry(ctx, serverID, setName, input, actorUserID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (s *Service) AddEntriesBulk(ctx context.Context, setName string, input BulkAddEntryInput, actorUserID *int64) (BulkAddResult, error) {
	if len(input.ServerIDs) == 0 {
		return BulkAddResult{}, fmt.Errorf("server_ids is required")
	}
	if input.TimeoutSeconds < 0 {
		return BulkAddResult{}, fmt.Errorf("timeout must be greater than or equal to 0")
	}
	ips, err := normalizeBulkIPs(input.IPs)
	if err != nil {
		return BulkAddResult{}, err
	}
	command, err := ipset.BulkAddIPScript(setName, ips, input.TimeoutSeconds)
	if err != nil {
		return BulkAddResult{}, err
	}

	entries := make([]Entry, 0)
	addedTotal := 0
	skippedTotal := 0
	for _, serverID := range input.ServerIDs {
		addedIPs, skipped, err := s.runBulkAddOnServer(ctx, serverID, command)
		if err != nil {
			return BulkAddResult{}, fmt.Errorf("server %d: %w", serverID, err)
		}
		skippedTotal += skipped
		for _, ip := range addedIPs {
			entry, err := s.upsertEntry(ctx, serverID, setName, AddEntryInput{
				ServerIDs:      input.ServerIDs,
				IP:             ip,
				TimeoutSeconds: input.TimeoutSeconds,
				Reason:         input.Reason,
			}, actorUserID)
			if err != nil {
				return BulkAddResult{}, err
			}
			entries = append(entries, entry)
			addedTotal++
		}
	}
	return BulkAddResult{Added: addedTotal, Skipped: skippedTotal, Entries: entries}, nil
}

func parseBulkAddOutput(output string) (added []string, skipped int) {
	added = make([]string, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ADDED ") {
			ip := strings.TrimSpace(strings.TrimPrefix(line, "ADDED "))
			if net.ParseIP(ip) != nil {
				added = append(added, ip)
			}
			continue
		}
		if strings.HasPrefix(line, "SKIP ") {
			skipped++
		}
	}
	return added, skipped
}

func (s *Service) runBulkAddOnServer(ctx context.Context, serverID int64, command string) ([]string, int, error) {
	_, creds, err := s.sshCredentials(ctx, serverID)
	if err != nil {
		return nil, 0, err
	}
	body := strings.Join([]string{
		remotedeps.EnsureFirewallToolsBody(),
		command,
	}, "\n")
	result, err := s.ssh.RunScript(ctx, remotedeps.WithInstallTimeout(creds), remotedeps.ScriptBody(body))
	if err != nil {
		return nil, 0, fmt.Errorf("%w%s", err, formatRemoteOutput(result))
	}
	added, skipped := parseBulkAddOutput(result.Stdout)
	return added, skipped, nil
}

func formatRemoteOutput(result sshx.Result) string {
	stderr := strings.TrimSpace(result.Stderr)
	stdout := strings.TrimSpace(result.Stdout)
	if stderr == "" && stdout == "" {
		return ""
	}
	if stderr != "" {
		return ": " + stderr
	}
	return ": " + stdout
}

func normalizeBulkIPs(ips []string) ([]string, error) {
	if len(ips) == 0 {
		return nil, fmt.Errorf("ips is required")
	}
	if len(ips) > 500 {
		return nil, fmt.Errorf("too many ips, maximum 500 per request")
	}
	seen := make(map[string]struct{}, len(ips))
	normalized := make([]string, 0, len(ips))
	for _, raw := range ips {
		ip := ipset.NormalizeIP(raw)
		if ip == "" {
			continue
		}
		if net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("invalid ip %q", ip)
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		normalized = append(normalized, ip)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("ips is required")
	}
	return normalized, nil
}

func (s *Service) DeleteEntry(ctx context.Context, id int64) (Entry, error) {
	entry, err := s.GetEntry(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	command, err := ipset.DeleteIPCommand(entry.SetName, entry.IP)
	if err != nil {
		return Entry{}, err
	}
	if err := s.runOnServer(ctx, entry.ServerID, command); err != nil {
		return Entry{}, err
	}
	tag, err := s.db.Exec(ctx, "DELETE FROM firewall_entries WHERE id = $1", id)
	if err != nil {
		return Entry{}, err
	}
	if tag.RowsAffected() == 0 {
		return Entry{}, pgx.ErrNoRows
	}
	return entry, nil
}

func (s *Service) GetEntry(ctx context.Context, id int64) (Entry, error) {
	var entry Entry
	err := s.db.QueryRow(ctx, `
		SELECT id, server_id, set_name, ip, timeout_seconds, reason, created_by, created_at
		FROM firewall_entries
		WHERE id = $1
	`, id).Scan(&entry.ID, &entry.ServerID, &entry.SetName, &entry.IP, &entry.TimeoutSeconds, &entry.Reason, &entry.CreatedBy, &entry.CreatedAt)
	return entry, err
}

func (s *Service) upsertEntry(ctx context.Context, serverID int64, setName string, input AddEntryInput, actorUserID *int64) (Entry, error) {
	var entry Entry
	err := s.db.QueryRow(ctx, `
		INSERT INTO firewall_entries (server_id, set_name, ip, timeout_seconds, reason, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (server_id, set_name, ip)
		DO UPDATE SET timeout_seconds = EXCLUDED.timeout_seconds, reason = EXCLUDED.reason, created_by = EXCLUDED.created_by
		RETURNING id, server_id, set_name, ip, timeout_seconds, reason, created_by, created_at
	`, serverID, setName, input.IP, input.TimeoutSeconds, input.Reason, actorUserID).Scan(
		&entry.ID, &entry.ServerID, &entry.SetName, &entry.IP, &entry.TimeoutSeconds, &entry.Reason, &entry.CreatedBy, &entry.CreatedAt,
	)
	return entry, err
}

func (s *Service) runOnServer(ctx context.Context, serverID int64, command string) error {
	_, creds, err := s.sshCredentials(ctx, serverID)
	if err != nil {
		return err
	}
	_, err = s.ssh.Run(ctx, remotedeps.WithInstallTimeout(creds), remotedeps.WithFirewallTools(command))
	return err
}

func (s *Service) sshCredentials(ctx context.Context, serverID int64) (serverrepo.Server, sshx.Credentials, error) {
	target, err := s.servers.Get(ctx, serverID)
	if err != nil {
		return serverrepo.Server{}, sshx.Credentials{}, err
	}
	password, privateKey, err := s.servers.Credentials(target)
	if err != nil {
		return serverrepo.Server{}, sshx.Credentials{}, err
	}
	return target, sshx.Credentials{
		Host:       target.Host,
		Port:       target.Port,
		Username:   target.Username,
		AuthType:   target.AuthType,
		Password:   password,
		PrivateKey: privateKey,
		Timeout:    s.timeout,
	}, nil
}
