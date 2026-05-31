package monitor

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	serverrepo "github.com/example/cc-panel/internal/server"
	"github.com/example/cc-panel/internal/sshx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
)

//go:embed collect_script.sh
var collectScript string

type Metric struct {
	ID                int64     `json:"id"`
	ServerID          int64     `json:"server_id"`
	CPUUsage          float64   `json:"cpu_usage"`
	MemoryUsage       float64   `json:"memory_usage"`
	DiskUsage         float64   `json:"disk_usage"`
	Load1             float64   `json:"load1"`
	Load5             float64   `json:"load5"`
	Load15            float64   `json:"load15"`
	NetInBytes        int64     `json:"net_in_bytes"`
	NetOutBytes       int64     `json:"net_out_bytes"`
	TCPEstablished    int       `json:"tcp_established"`
	TCPTimeWait       int       `json:"tcp_time_wait"`
	BlockedIPCount    int       `json:"blocked_ip_count"`
	IptablesDropHits  int64     `json:"iptables_drop_hits"`
	CreatedAt         time.Time `json:"created_at"`
}

type OverviewItem struct {
	ServerID         int64      `json:"server_id"`
	ServerName       string     `json:"server_name"`
	ServerHost       string     `json:"server_host"`
	ServerStatus     string     `json:"server_status"`
	MetricID         *int64     `json:"metric_id,omitempty"`
	CPUUsage         *float64   `json:"cpu_usage,omitempty"`
	MemoryUsage      *float64   `json:"memory_usage,omitempty"`
	DiskUsage        *float64   `json:"disk_usage,omitempty"`
	Load1            *float64   `json:"load1,omitempty"`
	Load5            *float64   `json:"load5,omitempty"`
	Load15           *float64   `json:"load15,omitempty"`
	NetInBytes       *int64     `json:"net_in_bytes,omitempty"`
	NetOutBytes      *int64     `json:"net_out_bytes,omitempty"`
	TCPEstablished   *int       `json:"tcp_established,omitempty"`
	TCPTimeWait      *int       `json:"tcp_time_wait,omitempty"`
	BlockedIPCount   *int       `json:"blocked_ip_count,omitempty"`
	IptablesDropHits *int64     `json:"iptables_drop_hits,omitempty"`
	CollectedAt      *time.Time `json:"collected_at,omitempty"`
}

type CollectAllResult struct {
	Collected int            `json:"collected"`
	Failed    int            `json:"failed"`
	Errors    map[int64]string `json:"errors,omitempty"`
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

func (s *Service) Collect(ctx context.Context, serverID int64) (Metric, error) {
	target, err := s.servers.Get(ctx, serverID)
	if err != nil {
		return Metric{}, err
	}
	password, privateKey, err := s.servers.Credentials(target)
	if err != nil {
		return Metric{}, err
	}
	result, err := s.ssh.Run(ctx, sshx.Credentials{
		Host:       target.Host,
		Port:       target.Port,
		Username:   target.Username,
		AuthType:   target.AuthType,
		Password:   password,
		PrivateKey: privateKey,
		Timeout:    s.timeout,
	}, collectScript)
	if err != nil {
		_ = s.servers.MarkOffline(ctx, target.ID)
		return Metric{}, err
	}
	metric, err := parseOutput(serverID, result.Stdout)
	if err != nil {
		return Metric{}, err
	}
	inserted, err := s.insert(ctx, metric)
	if err != nil {
		return Metric{}, err
	}
	_ = s.servers.MarkOnline(ctx, target.ID)
	return inserted, nil
}

func (s *Service) CollectAll(ctx context.Context) (CollectAllResult, error) {
	servers, err := s.servers.List(ctx)
	if err != nil {
		return CollectAllResult{}, err
	}
	result := CollectAllResult{Errors: make(map[int64]string)}
	group, groupCtx := errgroup.WithContext(ctx)
	ch := make(chan struct {
		id  int64
		err error
	}, len(servers))
	for _, server := range servers {
		server := server
		group.Go(func() error {
			_, err := s.Collect(groupCtx, server.ID)
			ch <- struct {
				id  int64
				err error
			}{id: server.ID, err: err}
			return nil
		})
	}
	_ = group.Wait()
	close(ch)
	for item := range ch {
		if item.err != nil {
			result.Failed++
			result.Errors[item.id] = item.err.Error()
			continue
		}
		result.Collected++
	}
	return result, nil
}

func (s *Service) List(ctx context.Context, serverID int64, limit int) ([]Metric, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id, cpu_usage, memory_usage, disk_usage, load1, load5, load15,
			net_in_bytes, net_out_bytes, tcp_established, tcp_time_wait, blocked_ip_count,
			iptables_drop_hits, created_at
		FROM server_metrics
		WHERE server_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMetrics(rows)
}

func (s *Service) Overview(ctx context.Context) ([]OverviewItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			s.id, s.name, s.host, s.status,
			m.id, m.cpu_usage, m.memory_usage, m.disk_usage, m.load1, m.load5, m.load15,
			m.net_in_bytes, m.net_out_bytes, m.tcp_established, m.tcp_time_wait,
			m.blocked_ip_count, m.iptables_drop_hits, m.created_at
		FROM servers s
		LEFT JOIN LATERAL (
			SELECT id, cpu_usage, memory_usage, disk_usage, load1, load5, load15,
				net_in_bytes, net_out_bytes, tcp_established, tcp_time_wait,
				blocked_ip_count, iptables_drop_hits, created_at
			FROM server_metrics
			WHERE server_id = s.id
			ORDER BY created_at DESC
			LIMIT 1
		) m ON TRUE
		ORDER BY s.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]OverviewItem, 0)
	for rows.Next() {
		var item OverviewItem
		var metricID *int64
		var cpu, mem, disk, load1, load5, load15 *float64
		var netIn, netOut, dropHits *int64
		var est, tw, blocked *int
		var collectedAt *time.Time
		if err := rows.Scan(
			&item.ServerID, &item.ServerName, &item.ServerHost, &item.ServerStatus,
			&metricID, &cpu, &mem, &disk, &load1, &load5, &load15,
			&netIn, &netOut, &est, &tw, &blocked, &dropHits, &collectedAt,
		); err != nil {
			return nil, err
		}
		item.MetricID = metricID
		item.CPUUsage = cpu
		item.MemoryUsage = mem
		item.DiskUsage = disk
		item.Load1 = load1
		item.Load5 = load5
		item.Load15 = load15
		item.NetInBytes = netIn
		item.NetOutBytes = netOut
		item.TCPEstablished = est
		item.TCPTimeWait = tw
		item.BlockedIPCount = blocked
		item.IptablesDropHits = dropHits
		item.CollectedAt = collectedAt
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) insert(ctx context.Context, metric Metric) (Metric, error) {
	err := s.db.QueryRow(ctx, `
		INSERT INTO server_metrics (
			server_id, cpu_usage, memory_usage, disk_usage, load1, load5, load15,
			net_in_bytes, net_out_bytes, tcp_established, tcp_time_wait,
			blocked_ip_count, iptables_drop_hits
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, created_at
	`, metric.ServerID, metric.CPUUsage, metric.MemoryUsage, metric.DiskUsage,
		metric.Load1, metric.Load5, metric.Load15, metric.NetInBytes, metric.NetOutBytes,
		metric.TCPEstablished, metric.TCPTimeWait, metric.BlockedIPCount, metric.IptablesDropHits,
	).Scan(&metric.ID, &metric.CreatedAt)
	return metric, err
}

func scanMetrics(rows pgx.Rows) ([]Metric, error) {
	items := make([]Metric, 0)
	for rows.Next() {
		var item Metric
		if err := rows.Scan(
			&item.ID, &item.ServerID, &item.CPUUsage, &item.MemoryUsage, &item.DiskUsage,
			&item.Load1, &item.Load5, &item.Load15, &item.NetInBytes, &item.NetOutBytes,
			&item.TCPEstablished, &item.TCPTimeWait, &item.BlockedIPCount, &item.IptablesDropHits,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func parseOutput(serverID int64, output string) (Metric, error) {
	line := strings.TrimSpace(output)
	if idx := strings.LastIndex(line, "\n"); idx >= 0 {
		line = strings.TrimSpace(line[idx+1:])
	}
	fields := strings.Fields(line)
	if len(fields) < 12 {
		return Metric{}, fmt.Errorf("unexpected monitor output %q", line)
	}
	cpu, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return Metric{}, err
	}
	mem, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return Metric{}, err
	}
	disk, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return Metric{}, err
	}
	load1, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return Metric{}, err
	}
	load5, err := strconv.ParseFloat(fields[4], 64)
	if err != nil {
		return Metric{}, err
	}
	load15, err := strconv.ParseFloat(fields[5], 64)
	if err != nil {
		return Metric{}, err
	}
	est, err := strconv.Atoi(fields[6])
	if err != nil {
		return Metric{}, err
	}
	tw, err := strconv.Atoi(fields[7])
	if err != nil {
		return Metric{}, err
	}
	netIn, err := strconv.ParseInt(fields[8], 10, 64)
	if err != nil {
		return Metric{}, err
	}
	netOut, err := strconv.ParseInt(fields[9], 10, 64)
	if err != nil {
		return Metric{}, err
	}
	blocked, err := strconv.Atoi(fields[10])
	if err != nil {
		return Metric{}, err
	}
	drops, err := strconv.ParseInt(fields[11], 10, 64)
	if err != nil {
		return Metric{}, err
	}
	return Metric{
		ServerID:         serverID,
		CPUUsage:         cpu,
		MemoryUsage:      mem,
		DiskUsage:        disk,
		Load1:            load1,
		Load5:            load5,
		Load15:           load15,
		NetInBytes:       netIn,
		NetOutBytes:      netOut,
		TCPEstablished:   est,
		TCPTimeWait:      tw,
		BlockedIPCount:   blocked,
		IptablesDropHits: drops,
	}, nil
}
