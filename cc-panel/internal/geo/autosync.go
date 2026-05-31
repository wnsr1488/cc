package geo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"
)

const geoCIDRSyncAdvisoryLockKey int64 = 88001

type AutoSyncConfig struct {
	Enabled       bool       `json:"enabled"`
	ServerIDs     []int64    `json:"server_ids"`
	IntervalHours int        `json:"interval_hours"`
	LastPullAt    *time.Time `json:"last_pull_at,omitempty"`
	LastDeployAt  *time.Time `json:"last_deploy_at,omitempty"`
	LastChanged   bool       `json:"last_changed"`
	LastError     *string    `json:"last_error,omitempty"`
}

type AutoSyncUpdateInput struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	ServerIDs []int64 `json:"server_ids,omitempty"`
}

type ScheduledSyncResult struct {
	Skipped          bool     `json:"skipped"`
	SkipReason       string   `json:"skip_reason,omitempty"`
	ChangedCountries []string `json:"changed_countries,omitempty"`
	DeployedServers  []int64  `json:"deployed_servers,omitempty"`
	Error            string   `json:"error,omitempty"`
}

func (s *Service) GetAutoSyncConfig(ctx context.Context, intervalHours int) (AutoSyncConfig, error) {
	if intervalHours <= 0 {
		intervalHours = 24
	}
	cfg := AutoSyncConfig{IntervalHours: intervalHours}
	serverIDs, err := s.listDefaultWhitelistTargets(ctx)
	if err != nil {
		return cfg, err
	}
	cfg.ServerIDs = serverIDs

	var lastError *string
	err = s.db.QueryRow(ctx, `
		SELECT enabled, last_pull_at, last_deploy_at, last_changed, last_error
		FROM geo_cidr_sync_state
		WHERE id = 1
	`).Scan(&cfg.Enabled, &cfg.LastPullAt, &cfg.LastDeployAt, &cfg.LastChanged, &lastError)
	if err != nil {
		return cfg, err
	}
	cfg.LastError = lastError
	return cfg, nil
}

func (s *Service) UpdateAutoSyncConfig(ctx context.Context, input AutoSyncUpdateInput) (AutoSyncConfig, error) {
	if input.ServerIDs != nil {
		if err := s.saveDefaultWhitelistTargets(ctx, input.ServerIDs); err != nil {
			return AutoSyncConfig{}, err
		}
	}
	if input.Enabled != nil {
		if _, err := s.db.Exec(ctx, `
			UPDATE geo_cidr_sync_state
			SET enabled = $1, updated_at = NOW()
			WHERE id = 1
		`, *input.Enabled); err != nil {
			return AutoSyncConfig{}, err
		}
		if *input.Enabled {
			go s.RunScheduledDefaultWhitelistSync(context.Background())
		}
	}
	return s.GetAutoSyncConfig(ctx, 0)
}

func (s *Service) saveDefaultWhitelistTargets(ctx context.Context, serverIDs []int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM geo_default_whitelist_targets`); err != nil {
		return err
	}
	for _, serverID := range serverIDs {
		if serverID <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo_default_whitelist_targets (server_id)
			VALUES ($1)
			ON CONFLICT (server_id) DO NOTHING
		`, serverID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) listDefaultWhitelistTargets(ctx context.Context) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT server_id
		FROM geo_default_whitelist_targets
		ORDER BY server_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) RunScheduledDefaultWhitelistSync(ctx context.Context) ScheduledSyncResult {
	result := ScheduledSyncResult{}

	conn, err := s.db.Acquire(ctx)
	if err != nil {
		result.Error = err.Error()
		s.recordAutoSyncError(ctx, err.Error())
		return result
	}
	defer conn.Release()

	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, geoCIDRSyncAdvisoryLockKey).Scan(&locked); err != nil {
		result.Error = err.Error()
		s.recordAutoSyncError(ctx, err.Error())
		return result
	}
	if !locked {
		result.Skipped = true
		result.SkipReason = "another sync is running"
		return result
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, geoCIDRSyncAdvisoryLockKey)
	}()

	cfg, err := s.GetAutoSyncConfig(ctx, 0)
	if err != nil {
		result.Error = err.Error()
		s.recordAutoSyncError(ctx, err.Error())
		return result
	}
	if !cfg.Enabled {
		result.Skipped = true
		result.SkipReason = "auto sync disabled"
		return result
	}
	if len(cfg.ServerIDs) == 0 {
		result.Skipped = true
		result.SkipReason = "no target servers configured"
		return result
	}

	changedCountries := make([]string, 0)
	group, groupCtx := errgroup.WithContext(ctx)
	ch := make(chan string, len(defaultWhitelistCountries))
	for _, country := range defaultWhitelistCountries {
		country := country
		group.Go(func() error {
			changed, err := s.refreshCountryCIDRsIfChanged(groupCtx, country)
			if err != nil {
				return fmt.Errorf("%s: %w", country, err)
			}
			if changed {
				ch <- country
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		close(ch)
		result.Error = err.Error()
		s.recordAutoSyncError(ctx, err.Error())
		return result
	}
	close(ch)
	for country := range ch {
		changedCountries = append(changedCountries, country)
	}
	sort.Strings(changedCountries)

	now := time.Now()
	if _, err := s.db.Exec(ctx, `
		UPDATE geo_cidr_sync_state
		SET last_pull_at = $1, last_changed = $2, last_error = NULL, updated_at = NOW()
		WHERE id = 1
	`, now, len(changedCountries) > 0); err != nil {
		result.Error = err.Error()
		s.recordAutoSyncError(ctx, err.Error())
		return result
	}

	if len(changedCountries) == 0 {
		result.Skipped = true
		result.SkipReason = "no cidr changes detected"
		log.Printf("geo auto sync: no cidr changes, skip deploy")
		return result
	}

	result.ChangedCountries = changedCountries
	deployGroup, deployCtx := errgroup.WithContext(ctx)
	for _, serverID := range cfg.ServerIDs {
		serverID := serverID
		deployGroup.Go(func() error {
			return s.deployWhitelistSnapshot(deployCtx, serverID, defaultWhitelistCountries)
		})
	}
	if err := deployGroup.Wait(); err != nil {
		result.Error = err.Error()
		s.recordAutoSyncError(ctx, err.Error())
		return result
	}

	for _, country := range defaultWhitelistCountries {
		if _, err := s.ensureDefaultWhitelistRule(ctx, country, true, nil); err != nil {
			result.Error = err.Error()
			s.recordAutoSyncError(ctx, err.Error())
			return result
		}
	}

	deployAt := time.Now()
	if _, err := s.db.Exec(ctx, `
		UPDATE geo_cidr_sync_state
		SET last_deploy_at = $1, last_error = NULL, updated_at = NOW()
		WHERE id = 1
	`, deployAt); err != nil {
		result.Error = err.Error()
		s.recordAutoSyncError(ctx, err.Error())
		return result
	}

	result.DeployedServers = append([]int64(nil), cfg.ServerIDs...)
	log.Printf("geo auto sync: changed countries=%v deployed servers=%v", changedCountries, cfg.ServerIDs)
	return result
}

func (s *Service) refreshCountryCIDRsIfChanged(ctx context.Context, country string) (bool, error) {
	code, ok := countryCodeByName[strings.TrimSpace(country)]
	if !ok {
		return false, fmt.Errorf("unsupported country %q", country)
	}
	remoteCIDRs, err := fetchCountryCIDRs(code)
	if err != nil {
		return false, err
	}
	existingCIDRs, err := s.matchCIDRs(ctx, country, nil, nil)
	if err != nil {
		return false, err
	}
	if cidrContentHash(remoteCIDRs) == cidrContentHash(existingCIDRs) {
		return false, nil
	}
	if _, err := s.replaceCountryCIDRsFromList(ctx, country, remoteCIDRs); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) replaceCountryCIDRsFromList(ctx context.Context, country string, cidrs []string) ([]string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM geo_cidrs WHERE country = $1`, country); err != nil {
		return nil, err
	}
	for _, cidr := range cidrs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO geo_cidrs (country, cidr)
			VALUES ($1, $2)
			ON CONFLICT (cidr)
			DO UPDATE SET country = EXCLUDED.country, province = NULL, city = NULL
		`, country, cidr); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return cidrs, nil
}

func cidrContentHash(cidrs []string) string {
	if len(cidrs) == 0 {
		return ""
	}
	sorted := append([]string(nil), cidrs...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])
}

func (s *Service) recordAutoSyncError(ctx context.Context, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	_, _ = s.db.Exec(ctx, `
		UPDATE geo_cidr_sync_state
		SET last_error = $1, updated_at = NOW()
		WHERE id = 1
	`, message)
}

func (s *Service) ShouldRunAutoSync(ctx context.Context, interval time.Duration) (bool, error) {
	if interval <= 0 {
		return false, nil
	}
	var enabled bool
	var lastPullAt *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT enabled, last_pull_at
		FROM geo_cidr_sync_state
		WHERE id = 1
	`).Scan(&enabled, &lastPullAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if !enabled {
		return false, nil
	}
	if lastPullAt == nil {
		return true, nil
	}
	return time.Since(*lastPullAt) >= interval, nil
}
