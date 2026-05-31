package monitor

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/example/cc-panel/internal/ipset"
	"github.com/example/cc-panel/internal/remotedeps"
	"github.com/example/cc-panel/internal/sshx"
)

//go:embed live_inspect.sh
var liveInspectScript string

type BlockedIP struct {
	IP             string `json:"ip"`
	SetName        string `json:"set_name"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	Country        string `json:"country,omitempty"`
	Province       string `json:"province,omitempty"`
	City           string `json:"city,omitempty"`
	ISP            string `json:"isp,omitempty"`
}

type IPConnection struct {
	IP       string `json:"ip"`
	Count    int    `json:"count"`
	Country  string `json:"country,omitempty"`
	Province string `json:"province,omitempty"`
	City     string `json:"city,omitempty"`
	ISP      string `json:"isp,omitempty"`
}

type LiveInsights struct {
	ServerID        int64          `json:"server_id"`
	TCPEstablished  int            `json:"tcp_established"`
	BlockedIPs      []BlockedIP    `json:"blocked_ips"`
	Connections     []IPConnection `json:"connections"`
	CollectedAt     time.Time      `json:"collected_at"`
}

type RegionLookup func(ip string) (country, province, city, isp string, ok bool)

func (s *Service) LiveInsights(ctx context.Context, serverID int64, connLimit int, lookup RegionLookup) (LiveInsights, error) {
	if connLimit <= 0 || connLimit > 500 {
		connLimit = 100
	}
	target, err := s.servers.Get(ctx, serverID)
	if err != nil {
		return LiveInsights{}, err
	}
	password, privateKey, err := s.servers.Credentials(target)
	if err != nil {
		return LiveInsights{}, err
	}
	creds := remotedeps.WithInstallTimeout(sshx.Credentials{
		Host:       target.Host,
		Port:       target.Port,
		Username:   target.Username,
		AuthType:   target.AuthType,
		Password:   password,
		PrivateKey: privateKey,
		Timeout:    s.timeout,
	})
	result, err := s.ssh.Run(ctx, creds, remotedeps.WithFirewallTools(liveInspectScript))
	if err != nil {
		_ = s.servers.MarkOffline(ctx, target.ID)
		return LiveInsights{}, err
	}
	_ = s.servers.MarkOnline(ctx, target.ID)

	estab, blocked, connections, err := parseLiveInspectOutput(result.Stdout, connLimit)
	if err != nil {
		return LiveInsights{}, err
	}
	if lookup != nil {
		for i := range blocked {
			if country, province, city, isp, ok := lookup(blocked[i].IP); ok {
				blocked[i].Country = country
				blocked[i].Province = province
				blocked[i].City = city
				blocked[i].ISP = isp
			}
		}
		for i := range connections {
			if country, province, city, isp, ok := lookup(connections[i].IP); ok {
				connections[i].Country = country
				connections[i].Province = province
				connections[i].City = city
				connections[i].ISP = isp
			}
		}
	}
	return LiveInsights{
		ServerID:       serverID,
		TCPEstablished: estab,
		BlockedIPs:     blocked,
		Connections:    connections,
		CollectedAt:    time.Now().UTC(),
	}, nil
}

func parseLiveInspectOutput(output string, connLimit int) (int, []BlockedIP, []IPConnection, error) {
	section := ""
	estab := 0
	blocked := make([]BlockedIP, 0)
	connections := make([]IPConnection, 0)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch line {
		case "__CC_ESTAB__":
			section = "estab"
			continue
		case "__CC_BLOCKED__":
			section = "blocked"
			continue
		case "__CC_CONN__":
			section = "conn"
			continue
		}
		switch section {
		case "estab":
			value, err := strconv.Atoi(strings.Fields(line)[0])
			if err != nil {
				return 0, nil, nil, fmt.Errorf("parse estab: %w", err)
			}
			estab = value
		case "blocked":
			parts := strings.Split(line, "\t")
			if len(parts) < 2 {
				continue
			}
			ip := ipset.NormalizeIP(parts[1])
			if net.ParseIP(ip) == nil {
				continue
			}
			timeout := 0
			if len(parts) >= 3 {
				timeout, _ = strconv.Atoi(parts[2])
			}
			blocked = append(blocked, BlockedIP{
				SetName:        parts[0],
				IP:             ip,
				TimeoutSeconds: timeout,
			})
		case "conn":
			if len(connections) >= connLimit {
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
			connections = append(connections, IPConnection{IP: ip, Count: count})
		}
	}
	return estab, blocked, connections, nil
}
