package geo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/example/cc-panel/internal/ipset"
	"github.com/example/cc-panel/internal/iptables"
	"github.com/example/cc-panel/internal/remotedeps"
	serverrepo "github.com/example/cc-panel/internal/server"
	"github.com/example/cc-panel/internal/sshx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	ip2region "github.com/lionsoul2014/ip2region/binding/golang/service"
	"golang.org/x/sync/errgroup"
)

type CIDR struct {
	ID        int64     `json:"id"`
	Country   string    `json:"country"`
	Province  *string   `json:"province,omitempty"`
	City      *string   `json:"city,omitempty"`
	CIDR      string    `json:"cidr"`
	CreatedAt time.Time `json:"created_at"`
}

type CIDRSummary struct {
	Country            string     `json:"country"`
	Province           *string    `json:"province,omitempty"`
	City               *string    `json:"city,omitempty"`
	CIDRCount          int        `json:"cidr_count"`
	WhitelistRuleCount int        `json:"whitelist_rule_count"`
	BlockRuleCount     int        `json:"block_rule_count"`
	LatestCIDRAt       *time.Time `json:"latest_cidr_at,omitempty"`
}

type CountrySyncResult struct {
	Country   string  `json:"country"`
	CIDRCount int     `json:"cidr_count"`
	RuleID    *int64  `json:"rule_id,omitempty"`
	Error     *string `json:"error,omitempty"`
}

type Rule struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Country   string    `json:"country"`
	Province  *string   `json:"province,omitempty"`
	City      *string   `json:"city,omitempty"`
	Action    string    `json:"action"`
	Enabled   bool      `json:"enabled"`
	CreatedBy *int64    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RegionInfo struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
	ISOCode  string `json:"iso_code"`
	Raw      string `json:"raw"`
}

type AddCIDRInput struct {
	Country  string  `json:"country"`
	Province *string `json:"province,omitempty"`
	City     *string `json:"city,omitempty"`
	CIDR     string  `json:"cidr"`
}

type PreviewCIDRsInput struct {
	CIDRs []string `json:"cidrs"`
}

type CIDRPreview struct {
	CIDR            string      `json:"cidr"`
	StartIP         string      `json:"start_ip,omitempty"`
	Region          *RegionInfo `json:"region,omitempty"`
	SuggestedAction string      `json:"suggested_action,omitempty"`
	Reason          string      `json:"reason,omitempty"`
	Valid           bool        `json:"valid"`
	Error           string      `json:"error,omitempty"`
}

type CreateRuleInput struct {
	Name      string  `json:"name"`
	Country   string  `json:"country"`
	Province  *string `json:"province,omitempty"`
	City      *string `json:"city,omitempty"`
	Action    string  `json:"action"`
	ServerIDs []int64 `json:"server_ids"`
	Enabled   bool    `json:"enabled"`
}

type DefaultWhitelistInput struct {
	ServerIDs       []int64 `json:"server_ids"`
	Enabled         bool    `json:"enabled"`
	Country         string  `json:"country,omitempty"`
	Cleanup         bool    `json:"cleanup,omitempty"`
	Phase           string  `json:"phase,omitempty"`
	ServerID        int64   `json:"server_id,omitempty"`
	StrictWhitelist *bool   `json:"strict_whitelist,omitempty"`
	WhitelistMode   *string `json:"whitelist_mode,omitempty"`
}

type Options struct {
	Countries []string `json:"countries"`
	Provinces []string `json:"provinces"`
	Cities    []string `json:"cities"`
}

type Country struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type Service struct {
	db      *pgxpool.Pool
	servers *serverrepo.Repository
	ssh     sshx.Executor
	timeout time.Duration
	ip2r    *ip2region.Ip2Region
	ip2rErr error
}

func NewService(db *pgxpool.Pool, servers *serverrepo.Repository, ssh sshx.Executor, timeout time.Duration, v4XDBPath, v6XDBPath string) *Service {
	var searcher *ip2region.Ip2Region
	var initErr error
	if v4XDBPath != "" || v6XDBPath != "" {
		searcher, initErr = ip2region.NewIp2RegionWithPath(v4XDBPath, v6XDBPath)
	}
	return &Service{db: db, servers: servers, ssh: ssh, timeout: timeout, ip2r: searcher, ip2rErr: initErr}
}

func (s *Service) SearchIP(ip string) (RegionInfo, error) {
	if net.ParseIP(ip) == nil {
		return RegionInfo{}, fmt.Errorf("invalid ip %q", ip)
	}
	if s.ip2r == nil {
		if s.ip2rErr != nil {
			return RegionInfo{}, fmt.Errorf("ip2region xdb load failed: %w", s.ip2rErr)
		}
		return RegionInfo{}, fmt.Errorf("ip2region xdb is not configured")
	}
	raw, err := s.ip2r.Search(ip)
	if err != nil {
		return RegionInfo{}, err
	}
	parts := strings.Split(raw, "|")
	for len(parts) < 5 {
		parts = append(parts, "")
	}
	return RegionInfo{
		IP:       ip,
		Country:  localizeRegionPart(parts[0]),
		Region:   "",
		Province: localizeRegionPart(parts[1]),
		City:     localizeRegionPart(parts[2]),
		ISP:      normalizeRegionPart(parts[3]),
		ISOCode:  normalizeRegionPart(parts[4]),
		Raw:      raw,
	}, nil
}

func (s *Service) AddCIDR(ctx context.Context, input AddCIDRInput) (CIDR, error) {
	ip, _, err := net.ParseCIDR(input.CIDR)
	if err != nil {
		return CIDR{}, fmt.Errorf("invalid cidr: %w", err)
	}
	if strings.TrimSpace(input.Country) == "" {
		info, err := s.SearchIP(ip.String())
		if err != nil {
			return CIDR{}, fmt.Errorf("country is required when ip2region lookup fails: %w", err)
		}
		input.Country = info.Country
		input.Province = optionalString(info.Province)
		input.City = optionalString(info.City)
	}
	var item CIDR
	err = s.db.QueryRow(ctx, `
		INSERT INTO geo_cidrs (country, province, city, cidr)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (cidr)
		DO UPDATE SET country = EXCLUDED.country, province = EXCLUDED.province, city = EXCLUDED.city
		RETURNING id, country, province, city, cidr, created_at
	`, input.Country, input.Province, input.City, input.CIDR).Scan(
		&item.ID, &item.Country, &item.Province, &item.City, &item.CIDR, &item.CreatedAt,
	)
	return item, err
}

func (s *Service) PreviewCIDRs(input PreviewCIDRsInput) []CIDRPreview {
	previews := make([]CIDRPreview, 0, len(input.CIDRs))
	for _, rawCIDR := range input.CIDRs {
		cidr := strings.TrimSpace(rawCIDR)
		if cidr == "" {
			continue
		}
		preview := CIDRPreview{CIDR: cidr}
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			preview.Error = fmt.Sprintf("CIDR 格式错误: %v", err)
			previews = append(previews, preview)
			continue
		}
		preview.StartIP = ip.String()
		info, err := s.SearchIP(ip.String())
		if err != nil {
			preview.Error = fmt.Sprintf("ip2region 查询失败: %v", err)
			previews = append(previews, preview)
			continue
		}
		preview.Valid = true
		preview.Region = &info
		preview.SuggestedAction, preview.Reason = suggestAction(info)
		previews = append(previews, preview)
	}
	return previews
}

func (s *Service) BulkAddCIDRs(ctx context.Context, input PreviewCIDRsInput) ([]CIDR, error) {
	previews := s.PreviewCIDRs(input)
	items := make([]CIDR, 0, len(previews))
	for _, preview := range previews {
		if !preview.Valid || preview.Region == nil {
			return nil, fmt.Errorf("%s: %s", preview.CIDR, preview.Error)
		}
		item, err := s.AddCIDR(ctx, AddCIDRInput{
			CIDR:     preview.CIDR,
			Country:  preview.Region.Country,
			Province: optionalString(preview.Region.Province),
			City:     optionalString(preview.Region.City),
		})
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) Countries() []Country {
	items := make([]Country, 0, len(countryCodeByName))
	for name, code := range countryCodeByName {
		items = append(items, Country{Name: name, Code: code})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func (s *Service) DefaultWhitelistCountries() []string {
	return append([]string(nil), defaultWhitelistCountries...)
}

func (s *Service) ImportCountryCIDRs(ctx context.Context, country string) ([]CIDR, error) {
	code, ok := countryCodeByName[strings.TrimSpace(country)]
	if !ok {
		return nil, fmt.Errorf("unsupported country %q", country)
	}
	cidrs, err := fetchCountryCIDRs(code)
	if err != nil {
		return nil, err
	}
	items := make([]CIDR, 0, len(cidrs))
	for _, cidr := range cidrs {
		item, err := s.AddCIDR(ctx, AddCIDRInput{Country: country, CIDR: cidr})
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) countriesForDefaultWhitelist(country string) ([]string, error) {
	if country == "" {
		return defaultWhitelistCountries, nil
	}
	for _, item := range defaultWhitelistCountries {
		if item == country {
			return []string{item}, nil
		}
	}
	return nil, fmt.Errorf("unsupported default whitelist country %q", country)
}

func (s *Service) applyWhitelistModeSetting(ctx context.Context, input DefaultWhitelistInput) error {
	mode := resolveWhitelistMode(input)
	if mode == "" {
		return nil
	}
	for _, serverID := range input.ServerIDs {
		if err := s.servers.SetWhitelistMode(ctx, serverID, mode); err != nil {
			return fmt.Errorf("server %d: %w", serverID, err)
		}
	}
	return nil
}

func resolveWhitelistMode(input DefaultWhitelistInput) string {
	if input.WhitelistMode != nil {
		mode := strings.TrimSpace(*input.WhitelistMode)
		if mode == "" {
			return ""
		}
		if !serverrepo.ValidWhitelistMode(mode) {
			return ""
		}
		return mode
	}
	if input.StrictWhitelist == nil {
		return ""
	}
	if *input.StrictWhitelist {
		return serverrepo.WhitelistModeStrict
	}
	return serverrepo.WhitelistModeOff
}

func (s *Service) applyStrictWhitelistSetting(ctx context.Context, input DefaultWhitelistInput) error {
	return s.applyWhitelistModeSetting(ctx, input)
}

func (s *Service) CreateDefaultWhitelist(ctx context.Context, input DefaultWhitelistInput, actorUserID *int64) ([]Rule, error) {
	if len(input.ServerIDs) == 0 {
		return nil, fmt.Errorf("server_ids is required")
	}
	if err := s.saveDefaultWhitelistTargets(ctx, input.ServerIDs); err != nil {
		return nil, fmt.Errorf("save default whitelist targets: %w", err)
	}
	if err := s.applyStrictWhitelistSetting(ctx, input); err != nil {
		return nil, err
	}
	if input.Cleanup || input.Country == "" {
		if err := s.cleanupDeprecatedDefaultWhitelist(ctx); err != nil {
			return nil, err
		}
	}
	countries, err := s.countriesForDefaultWhitelist(input.Country)
	if err != nil {
		return nil, err
	}
	rules := make([]Rule, 0, len(countries))
	for _, country := range countries {
		if _, err := s.importCountryCIDRsIfMissing(ctx, country); err != nil {
			return nil, fmt.Errorf("%s: %w", country, err)
		}
		rule, err := s.ensureDefaultWhitelistRule(ctx, country, input.Enabled, actorUserID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", country, err)
		}
		rules = append(rules, rule)
	}
	deployCountries := countries
	if input.Country == "" {
		deployCountries = defaultWhitelistCountries
	}
	group, groupCtx := errgroup.WithContext(ctx)
	for _, serverID := range input.ServerIDs {
		serverID := serverID
		countriesCopy := append([]string(nil), deployCountries...)
		group.Go(func() error {
			return s.deployWhitelistSnapshot(groupCtx, serverID, countriesCopy)
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return rules, nil
}

func (s *Service) SyncDefaultWhitelist(ctx context.Context, input DefaultWhitelistInput, actorUserID *int64) ([]CountrySyncResult, error) {
	phase := strings.TrimSpace(input.Phase)
	if phase == "" {
		phase = "all"
	}
	if phase != "cidr" && len(input.ServerIDs) == 0 {
		return nil, fmt.Errorf("server_ids is required")
	}
	if len(input.ServerIDs) > 0 {
		if err := s.saveDefaultWhitelistTargets(ctx, input.ServerIDs); err != nil {
			return nil, fmt.Errorf("save default whitelist targets: %w", err)
		}
	}
	if err := s.applyStrictWhitelistSetting(ctx, input); err != nil {
		return nil, err
	}
	if input.Cleanup || input.Country == "" {
		if err := s.cleanupDeprecatedDefaultWhitelist(ctx); err != nil {
			return nil, err
		}
	}
	countries, err := s.countriesForDefaultWhitelist(input.Country)
	if err != nil {
		return nil, err
	}
	if input.Country == "" {
		return s.syncDefaultWhitelistBulk(ctx, input, countries, phase, actorUserID)
	}
	results := make([]CountrySyncResult, 0, len(countries))
	for _, country := range countries {
		result, err := s.syncDefaultWhitelistCountry(ctx, country, input, phase, actorUserID)
		if err != nil {
			results = append(results, result)
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) syncDefaultWhitelistBulk(ctx context.Context, input DefaultWhitelistInput, countries []string, phase string, actorUserID *int64) ([]CountrySyncResult, error) {
	type countryResult struct {
		result CountrySyncResult
		err    error
	}
	results := make([]CountrySyncResult, 0, len(countries))
	if phase == "all" || phase == "cidr" {
		group, groupCtx := errgroup.WithContext(ctx)
		ch := make(chan countryResult, len(countries))
		for _, country := range countries {
			country := country
			group.Go(func() error {
				cidrs, err := s.replaceCountryCIDRs(groupCtx, country)
				result := CountrySyncResult{Country: country, CIDRCount: len(cidrs)}
				if err != nil {
					message := err.Error()
					result.Error = &message
				}
				ch <- countryResult{result: result, err: err}
				return err
			})
		}
		if err := group.Wait(); err != nil {
			close(ch)
			for item := range ch {
				results = append(results, item.result)
			}
			return results, err
		}
		close(ch)
		for item := range ch {
			results = append(results, item.result)
		}
		sort.Slice(results, func(i, j int) bool { return results[i].Country < results[j].Country })
		if phase == "cidr" {
			return results, nil
		}
	}
	if phase == "all" || phase == "deploy" {
		group, groupCtx := errgroup.WithContext(ctx)
		for _, serverID := range input.ServerIDs {
			serverID := serverID
			group.Go(func() error {
				return s.deployWhitelistSnapshot(groupCtx, serverID, defaultWhitelistCountries)
			})
		}
		if err := group.Wait(); err != nil {
			return results, err
		}
		if phase == "deploy" {
			return results, nil
		}
	}
	if phase == "all" || phase == "rule" {
		if len(results) == 0 {
			for _, country := range countries {
				results = append(results, CountrySyncResult{Country: country})
			}
		}
		for i, country := range countries {
			rule, err := s.ensureDefaultWhitelistRule(ctx, country, input.Enabled, actorUserID)
			if err != nil {
				message := err.Error()
				if i < len(results) {
					results[i].Error = &message
				}
				return results, fmt.Errorf("%s: %w", country, err)
			}
			if i < len(results) {
				results[i].RuleID = &rule.ID
			} else {
				results = append(results, CountrySyncResult{Country: country, RuleID: &rule.ID})
			}
		}
	}
	return results, nil
}

func (s *Service) syncDefaultWhitelistCountry(ctx context.Context, country string, input DefaultWhitelistInput, phase string, actorUserID *int64) (CountrySyncResult, error) {
	result := CountrySyncResult{Country: country}

	var cidrs []string
	var err error

	switch phase {
	case "cidr", "all":
		cidrs, err = s.replaceCountryCIDRs(ctx, country)
		if err != nil {
			message := err.Error()
			result.Error = &message
			return result, fmt.Errorf("%s: %w", country, err)
		}
		result.CIDRCount = len(cidrs)
		if phase == "cidr" {
			return result, nil
		}
	case "deploy", "rule":
		cidrs, err = s.matchCIDRs(ctx, country, nil, nil)
		if err != nil {
			message := err.Error()
			result.Error = &message
			return result, fmt.Errorf("%s: %w", country, err)
		}
		result.CIDRCount = len(cidrs)
	default:
		return result, fmt.Errorf("unsupported sync phase %q", phase)
	}

	if phase == "all" || phase == "deploy" {
		serverIDs := input.ServerIDs
		if input.ServerID > 0 {
			serverIDs = []int64{input.ServerID}
		}
		group, groupCtx := errgroup.WithContext(ctx)
		for _, serverID := range serverIDs {
			serverID := serverID
			group.Go(func() error {
				return s.deployCIDRs(groupCtx, serverID, cidrs, "ACCEPT", false)
			})
		}
		if err := group.Wait(); err != nil {
			message := err.Error()
			result.Error = &message
			return result, err
		}
		if phase == "deploy" {
			return result, nil
		}
	}

	if phase == "all" || phase == "rule" {
		rule, err := s.ensureDefaultWhitelistRule(ctx, country, input.Enabled, actorUserID)
		if err != nil {
			message := err.Error()
			result.Error = &message
			return result, fmt.Errorf("%s: %w", country, err)
		}
		result.RuleID = &rule.ID
	}
	return result, nil
}

func (s *Service) Options(ctx context.Context, country, province string) (Options, error) {
	options := Options{
		Countries: []string{},
		Provinces: []string{},
		Cities:    []string{},
	}
	countries, err := s.distinctValues(ctx, "country", "")
	if err != nil {
		return Options{}, err
	}
	options.Countries = mergeCountries(countries, s.Countries())

	if country != "" {
		provinces, err := s.distinctValues(ctx, "province", "country = $1", country)
		if err != nil {
			return Options{}, err
		}
		options.Provinces = provinces
	}
	if country != "" && province != "" {
		cities, err := s.distinctValues(ctx, "city", "country = $1 AND province = $2", country, province)
		if err != nil {
			return Options{}, err
		}
		options.Cities = cities
	}
	return options, nil
}

func (s *Service) ListCIDRs(ctx context.Context, country, province, city string, limit int) ([]CIDR, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, country, province, city, cidr, created_at
		FROM geo_cidrs
		WHERE ($1 = '' OR country = $1)
			AND ($2 = '' OR province = $2)
			AND ($3 = '' OR city = $3)
		ORDER BY created_at DESC
		LIMIT $4
	`, country, province, city, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CIDR, 0)
	for rows.Next() {
		var item CIDR
		if err := rows.Scan(&item.ID, &item.Country, &item.Province, &item.City, &item.CIDR, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) ListCIDRSummaries(ctx context.Context) ([]CIDRSummary, error) {
	rows, err := s.db.Query(ctx, `
		WITH cidr_summary AS (
			SELECT country, province, city, COUNT(*)::int AS cidr_count, MAX(created_at) AS latest_cidr_at
			FROM geo_cidrs
			GROUP BY country, province, city
		),
		rule_summary AS (
			SELECT country, province, city,
				COUNT(*) FILTER (WHERE action = 'ACCEPT')::int AS whitelist_rule_count,
				COUNT(*) FILTER (WHERE action = 'DROP')::int AS block_rule_count
			FROM geo_block_rules
			GROUP BY country, province, city
		)
		SELECT c.country, c.province, c.city, c.cidr_count,
			COALESCE(r.whitelist_rule_count, 0), COALESCE(r.block_rule_count, 0), c.latest_cidr_at
		FROM cidr_summary c
		LEFT JOIN rule_summary r ON r.country = c.country
			AND r.province IS NOT DISTINCT FROM c.province
			AND r.city IS NOT DISTINCT FROM c.city
		ORDER BY c.country ASC, c.province ASC NULLS FIRST, c.city ASC NULLS FIRST
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CIDRSummary, 0)
	for rows.Next() {
		var item CIDRSummary
		if err := rows.Scan(
			&item.Country,
			&item.Province,
			&item.City,
			&item.CIDRCount,
			&item.WhitelistRuleCount,
			&item.BlockRuleCount,
			&item.LatestCIDRAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) replaceCountryCIDRs(ctx context.Context, country string) ([]string, error) {
	code, ok := countryCodeByName[strings.TrimSpace(country)]
	if !ok {
		return nil, fmt.Errorf("unsupported country %q", country)
	}
	cidrs, err := fetchCountryCIDRs(code)
	if err != nil {
		return nil, err
	}
	return s.replaceCountryCIDRsFromList(ctx, country, cidrs)
}

func (s *Service) ensureDefaultWhitelistRule(ctx context.Context, country string, enabled bool, actorUserID *int64) (Rule, error) {
	var rule Rule
	err := s.db.QueryRow(ctx, `
		SELECT id, name, country, province, city, action, enabled, created_by, created_at, updated_at
		FROM geo_block_rules
		WHERE country = $1 AND province IS NULL AND city IS NULL AND action = 'ACCEPT'
		ORDER BY created_at ASC
		LIMIT 1
	`, country).Scan(
		&rule.ID, &rule.Name, &rule.Country, &rule.Province, &rule.City, &rule.Action, &rule.Enabled,
		&rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if err == nil {
		err = s.db.QueryRow(ctx, `
			UPDATE geo_block_rules
			SET enabled = $1, updated_at = NOW()
			WHERE id = $2
			RETURNING id, name, country, province, city, action, enabled, created_by, created_at, updated_at
		`, enabled, rule.ID).Scan(
			&rule.ID, &rule.Name, &rule.Country, &rule.Province, &rule.City, &rule.Action, &rule.Enabled,
			&rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt,
		)
		return rule, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Rule{}, err
	}
	var created Rule
	err = s.db.QueryRow(ctx, `
		INSERT INTO geo_block_rules (name, country, action, enabled, created_by)
		VALUES ($1, $2, 'ACCEPT', $3, $4)
		RETURNING id, name, country, province, city, action, enabled, created_by, created_at, updated_at
	`, country+"默认白名单", country, enabled, actorUserID).Scan(
		&created.ID, &created.Name, &created.Country, &created.Province, &created.City, &created.Action, &created.Enabled,
		&created.CreatedBy, &created.CreatedAt, &created.UpdatedAt,
	)
	return created, err
}

func (s *Service) cleanupDeprecatedDefaultWhitelist(ctx context.Context) error {
	for _, country := range deprecatedDefaultWhitelistCountries {
		if _, err := s.db.Exec(ctx, `DELETE FROM geo_cidrs WHERE country = $1`, country); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, `
			DELETE FROM geo_block_rules
			WHERE country = $1 AND province IS NULL AND city IS NULL AND action = 'ACCEPT'
		`, country); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CreateRule(ctx context.Context, input CreateRuleInput, actorUserID *int64) (Rule, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Rule{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(input.Country) == "" {
		return Rule{}, fmt.Errorf("country is required")
	}
	if len(input.ServerIDs) == 0 {
		return Rule{}, fmt.Errorf("server_ids is required")
	}
	if input.Action == "" {
		input.Action = "DROP"
	}
	if input.Action != "DROP" && input.Action != "ACCEPT" {
		return Rule{}, fmt.Errorf("action must be DROP or ACCEPT")
	}
	matchedCIDRs, err := s.matchCIDRs(ctx, input.Country, input.Province, input.City)
	if err != nil {
		return Rule{}, err
	}
	if len(matchedCIDRs) == 0 && input.Province == nil && input.City == nil {
		if _, err := s.ImportCountryCIDRs(ctx, input.Country); err != nil {
			return Rule{}, fmt.Errorf("auto import country cidrs failed: %w", err)
		}
		matchedCIDRs, err = s.matchCIDRs(ctx, input.Country, input.Province, input.City)
		if err != nil {
			return Rule{}, err
		}
	}
	if len(matchedCIDRs) == 0 {
		return Rule{}, fmt.Errorf("no cidr data matched this region")
	}

	group, groupCtx := errgroup.WithContext(ctx)
	for _, serverID := range input.ServerIDs {
		serverID := serverID
		group.Go(func() error {
			return s.deployCIDRs(groupCtx, serverID, matchedCIDRs, input.Action, false)
		})
	}
	if err := group.Wait(); err != nil {
		return Rule{}, err
	}

	var rule Rule
	err = s.db.QueryRow(ctx, `
		INSERT INTO geo_block_rules (name, country, province, city, action, enabled, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, country, province, city, action, enabled, created_by, created_at, updated_at
	`, input.Name, input.Country, input.Province, input.City, input.Action, input.Enabled, actorUserID).Scan(
		&rule.ID, &rule.Name, &rule.Country, &rule.Province, &rule.City, &rule.Action, &rule.Enabled,
		&rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt,
	)
	return rule, err
}

func (s *Service) ListRules(ctx context.Context, limit int) ([]Rule, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, name, country, province, city, action, enabled, created_by, created_at, updated_at
		FROM geo_block_rules
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]Rule, 0)
	for rows.Next() {
		var rule Rule
		if err := rows.Scan(
			&rule.ID, &rule.Name, &rule.Country, &rule.Province, &rule.City, &rule.Action, &rule.Enabled,
			&rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *Service) matchCIDRs(ctx context.Context, country string, province, city *string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT cidr
		FROM geo_cidrs
		WHERE country = $1
			AND ($2::text IS NULL OR province = $2)
			AND ($3::text IS NULL OR city = $3)
	`, country, province, city)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cidrs := make([]string, 0)
	for rows.Next() {
		var cidr string
		if err := rows.Scan(&cidr); err != nil {
			return nil, err
		}
		cidrs = append(cidrs, cidr)
	}
	return cidrs, rows.Err()
}

func (s *Service) deployCIDRs(ctx context.Context, serverID int64, cidrs []string, action string, skipIptables bool) error {
	target, creds, err := s.serverCredentials(ctx, serverID)
	if err != nil {
		return err
	}
	setName := ipset.GeoBlockSet
	if action == "ACCEPT" {
		setName = ipset.GeoWhitelistSet
	}
	body := strings.Join([]string{
		remotedeps.EnsureFirewallToolsBody(),
		ipset.IncrementalAddScript(setName, cidrs),
	}, "\n")
	if !skipIptables {
		body += "\n" + strings.TrimPrefix(iptables.DeployScript(serverrepo.UsesStrictWhitelist(target.WhitelistMode)), "set -e\n")
	}
	result, err := s.ssh.RunScript(ctx, creds, remotedeps.ScriptBody(body))
	if err != nil {
		return fmt.Errorf("deploy cidrs: %w%s", err, remoteOutput(result))
	}
	return nil
}

func (s *Service) deployWhitelistSnapshot(ctx context.Context, serverID int64, countries []string) error {
	target, creds, err := s.serverCredentials(ctx, serverID)
	if err != nil {
		return err
	}
	cidrs, err := s.listCIDRsForCountries(ctx, countries)
	if err != nil {
		return err
	}
	body := strings.Join([]string{
		remotedeps.EnsureFirewallToolsBody(),
		ipset.SnapshotScript(ipset.GeoWhitelistSet, cidrs),
		strings.TrimPrefix(iptables.DeployScript(serverrepo.UsesStrictWhitelist(target.WhitelistMode)), "set -e\n"),
	}, "\n")
	result, err := s.ssh.RunScript(ctx, creds, remotedeps.ScriptBody(body))
	if err != nil {
		return fmt.Errorf("deploy whitelist snapshot: %w%s", err, remoteOutput(result))
	}
	return nil
}

func (s *Service) serverCredentials(ctx context.Context, serverID int64) (serverrepo.Server, sshx.Credentials, error) {
	target, err := s.servers.Get(ctx, serverID)
	if err != nil {
		return serverrepo.Server{}, sshx.Credentials{}, err
	}
	password, privateKey, err := s.servers.Credentials(target)
	if err != nil {
		return serverrepo.Server{}, sshx.Credentials{}, err
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
	return target, creds, nil
}

func (s *Service) importCountryCIDRsIfMissing(ctx context.Context, country string) ([]CIDR, error) {
	cidrs, err := s.matchCIDRs(ctx, country, nil, nil)
	if err != nil {
		return nil, err
	}
	if len(cidrs) > 0 {
		return nil, nil
	}
	return s.ImportCountryCIDRs(ctx, country)
}

func (s *Service) listCIDRsForCountries(ctx context.Context, countries []string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT cidr
		FROM geo_cidrs
		WHERE country = ANY($1)
		ORDER BY cidr ASC
	`, countries)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]string, 0)
	for rows.Next() {
		var cidr string
		if err := rows.Scan(&cidr); err != nil {
			return nil, err
		}
		items = append(items, cidr)
	}
	return items, rows.Err()
}

func remoteOutput(result sshx.Result) string {
	output := strings.TrimSpace(result.Stderr)
	if output == "" {
		output = strings.TrimSpace(result.Stdout)
	}
	if output == "" {
		return ""
	}
	return ": " + output
}

func (s *Service) distinctValues(ctx context.Context, column, where string, args ...any) ([]string, error) {
	query := fmt.Sprintf("SELECT DISTINCT %s FROM geo_cidrs", column)
	if where != "" {
		query += " WHERE " + where
	}
	query += fmt.Sprintf(" AND %s IS NOT NULL AND %s <> ''", column, column)
	if where == "" {
		query = fmt.Sprintf("SELECT DISTINCT %s FROM geo_cidrs WHERE %s IS NOT NULL AND %s <> ''", column, column, column)
	}
	query += fmt.Sprintf(" ORDER BY %s ASC", column)
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func fetchCountryCIDRs(code string) ([]string, error) {
	normalizedCode := strings.ToUpper(code)
	url := fmt.Sprintf("https://metowolf.github.io/iplist/data/country/%s.txt", normalizedCode)
	if normalizedCode == "CN-MAINLAND" {
		url = "https://metowolf.github.io/iplist/data/special/china.txt"
	}
	client := http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("country cidr source returned %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(body), "\n")
	cidrs := make([]string, 0, len(lines))
	for _, line := range lines {
		cidr := strings.TrimSpace(line)
		if cidr == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			continue
		}
		cidrs = append(cidrs, cidr)
	}
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("country cidr source returned no cidrs")
	}
	return cidrs, nil
}

func mergeCountries(imported []string, catalog []Country) []string {
	seen := map[string]struct{}{}
	for _, country := range imported {
		if country != "" {
			seen[country] = struct{}{}
		}
	}
	for _, country := range catalog {
		seen[country.Name] = struct{}{}
	}
	countries := make([]string, 0, len(seen))
	for country := range seen {
		countries = append(countries, country)
	}
	sort.Strings(countries)
	return countries
}

func normalizeRegionPart(value string) string {
	if value == "0" {
		return ""
	}
	return value
}

func localizeRegionPart(value string) string {
	value = normalizeRegionPart(value)
	if translated, ok := regionNameTranslations[value]; ok {
		return translated
	}
	return value
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func suggestAction(info RegionInfo) (string, string) {
	switch info.Country {
	case "中国", "香港", "台湾":
		return "ACCEPT", "常见可信中文地区，建议按业务需要加入地区白名单"
	case "":
		return "DROP", "无法识别国家/地区，建议谨慎封禁或人工确认"
	default:
		return "DROP", "非默认可信地区，建议按访问情况加入地区封禁"
	}
}

var regionNameTranslations = map[string]string{
	"United States":   "美国",
	"California":      "加利福尼亚",
	"Washington":      "华盛顿",
	"Oregon":          "俄勒冈",
	"Virginia":        "弗吉尼亚",
	"New York":        "纽约",
	"Texas":           "得克萨斯",
	"China":           "中国",
	"Hong Kong":       "香港",
	"Taiwan":          "台湾",
	"Japan":           "日本",
	"Tokyo":           "东京",
	"Singapore":       "新加坡",
	"South Korea":     "韩国",
	"Germany":         "德国",
	"France":          "法国",
	"United Kingdom":  "英国",
	"England":         "英格兰",
	"London":          "伦敦",
	"Russia":          "俄罗斯",
	"Canada":          "加拿大",
	"Australia":       "澳大利亚",
	"Queensland":      "昆士兰",
	"Brisbane":        "布里斯班",
	"New South Wales": "新南威尔士",
	"Sydney":          "悉尼",
	"Victoria":        "维多利亚",
	"Melbourne":       "墨尔本",
	"India":           "印度",
	"Brazil":          "巴西",
	"Netherlands":     "荷兰",
}

var defaultWhitelistCountries = []string{
	"柬埔寨",
	"中国内地",
	"中国香港",
	"缅甸",
	"菲律宾",
	"中国台湾",
	"泰国",
	"越南",
	"老挝",
}

var deprecatedDefaultWhitelistCountries = []string{
	"中国",
}

var countryCodeByName = map[string]string{
	"阿富汗":         "af",
	"阿尔巴尼亚":       "al",
	"阿尔及利亚":       "dz",
	"美属萨摩亚":       "as",
	"安道尔":         "ad",
	"安哥拉":         "ao",
	"安圭拉":         "ai",
	"南极洲":         "aq",
	"安提瓜和巴布达":     "ag",
	"阿根廷":         "ar",
	"亚美尼亚":        "am",
	"阿鲁巴":         "aw",
	"澳大利亚":        "au",
	"奥地利":         "at",
	"阿塞拜疆":        "az",
	"巴哈马":         "bs",
	"巴林":          "bh",
	"孟加拉国":        "bd",
	"巴巴多斯":        "bb",
	"白俄罗斯":        "by",
	"比利时":         "be",
	"伯利兹":         "bz",
	"贝宁":          "bj",
	"百慕大":         "bm",
	"不丹":          "bt",
	"玻利维亚":        "bo",
	"波黑":          "ba",
	"博茨瓦纳":        "bw",
	"布韦岛":         "bv",
	"巴西":          "br",
	"英属印度洋领地":     "io",
	"文莱":          "bn",
	"保加利亚":        "bg",
	"布基纳法索":       "bf",
	"布隆迪":         "bi",
	"柬埔寨":         "kh",
	"喀麦隆":         "cm",
	"加拿大":         "ca",
	"佛得角":         "cv",
	"开曼群岛":        "ky",
	"中非":          "cf",
	"乍得":          "td",
	"智利":          "cl",
	"中国":          "cn",
	"中国内地":        "cn-mainland",
	"圣诞岛":         "cx",
	"科科斯群岛":       "cc",
	"哥伦比亚":        "co",
	"科摩罗":         "km",
	"刚果共和国":       "cg",
	"刚果民主共和国":     "cd",
	"库克群岛":        "ck",
	"哥斯达黎加":       "cr",
	"科特迪瓦":        "ci",
	"克罗地亚":        "hr",
	"古巴":          "cu",
	"塞浦路斯":        "cy",
	"捷克":          "cz",
	"丹麦":          "dk",
	"吉布提":         "dj",
	"多米尼克":        "dm",
	"多米尼加":        "do",
	"厄瓜多尔":        "ec",
	"埃及":          "eg",
	"萨尔瓦多":        "sv",
	"赤道几内亚":       "gq",
	"厄立特里亚":       "er",
	"爱沙尼亚":        "ee",
	"埃塞俄比亚":       "et",
	"福克兰群岛":       "fk",
	"法罗群岛":        "fo",
	"斐济":          "fj",
	"芬兰":          "fi",
	"法国":          "fr",
	"法属圭亚那":       "gf",
	"法属波利尼西亚":     "pf",
	"法属南部领地":      "tf",
	"加蓬":          "ga",
	"冈比亚":         "gm",
	"格鲁吉亚":        "ge",
	"德国":          "de",
	"加纳":          "gh",
	"直布罗陀":        "gi",
	"希腊":          "gr",
	"格陵兰":         "gl",
	"格林纳达":        "gd",
	"瓜德罗普":        "gp",
	"关岛":          "gu",
	"危地马拉":        "gt",
	"根西":          "gg",
	"几内亚":         "gn",
	"几内亚比绍":       "gw",
	"圭亚那":         "gy",
	"海地":          "ht",
	"赫德岛和麦克唐纳群岛":  "hm",
	"梵蒂冈":         "va",
	"洪都拉斯":        "hn",
	"香港":          "hk",
	"中国香港":        "hk",
	"匈牙利":         "hu",
	"冰岛":          "is",
	"印度":          "in",
	"印尼":          "id",
	"伊朗":          "ir",
	"伊拉克":         "iq",
	"爱尔兰":         "ie",
	"马恩岛":         "im",
	"以色列":         "il",
	"意大利":         "it",
	"牙买加":         "jm",
	"日本":          "jp",
	"泽西":          "je",
	"约旦":          "jo",
	"哈萨克斯坦":       "kz",
	"肯尼亚":         "ke",
	"基里巴斯":        "ki",
	"朝鲜":          "kp",
	"韩国":          "kr",
	"科威特":         "kw",
	"吉尔吉斯斯坦":      "kg",
	"老挝":          "la",
	"拉脱维亚":        "lv",
	"黎巴嫩":         "lb",
	"莱索托":         "ls",
	"利比里亚":        "lr",
	"利比亚":         "ly",
	"列支敦士登":       "li",
	"立陶宛":         "lt",
	"卢森堡":         "lu",
	"澳门":          "mo",
	"北马其顿":        "mk",
	"马达加斯加":       "mg",
	"马拉维":         "mw",
	"马来西亚":        "my",
	"马尔代夫":        "mv",
	"马里":          "ml",
	"马耳他":         "mt",
	"马绍尔群岛":       "mh",
	"马提尼克":        "mq",
	"毛里塔尼亚":       "mr",
	"毛里求斯":        "mu",
	"马约特":         "yt",
	"墨西哥":         "mx",
	"密克罗尼西亚":      "fm",
	"摩尔多瓦":        "md",
	"摩纳哥":         "mc",
	"蒙古":          "mn",
	"黑山":          "me",
	"蒙特塞拉特":       "ms",
	"摩洛哥":         "ma",
	"莫桑比克":        "mz",
	"缅甸":          "mm",
	"纳米比亚":        "na",
	"瑙鲁":          "nr",
	"尼泊尔":         "np",
	"荷兰":          "nl",
	"荷属安的列斯":      "an",
	"新喀里多尼亚":      "nc",
	"新西兰":         "nz",
	"尼加拉瓜":        "ni",
	"尼日尔":         "ne",
	"尼日利亚":        "ng",
	"纽埃":          "nu",
	"诺福克岛":        "nf",
	"北马里亚纳群岛":     "mp",
	"挪威":          "no",
	"阿曼":          "om",
	"巴基斯坦":        "pk",
	"帕劳":          "pw",
	"巴勒斯坦":        "ps",
	"巴拿马":         "pa",
	"巴布亚新几内亚":     "pg",
	"巴拉圭":         "py",
	"秘鲁":          "pe",
	"菲律宾":         "ph",
	"皮特凯恩群岛":      "pn",
	"波兰":          "pl",
	"葡萄牙":         "pt",
	"波多黎各":        "pr",
	"卡塔尔":         "qa",
	"留尼汪":         "re",
	"罗马尼亚":        "ro",
	"俄罗斯":         "ru",
	"卢旺达":         "rw",
	"圣赫勒拿":        "sh",
	"圣基茨和尼维斯":     "kn",
	"圣卢西亚":        "lc",
	"圣皮埃尔和密克隆":    "pm",
	"圣文森特和格林纳丁斯":  "vc",
	"萨摩亚":         "ws",
	"圣马力诺":        "sm",
	"圣多美和普林西比":    "st",
	"沙特阿拉伯":       "sa",
	"塞内加尔":        "sn",
	"塞尔维亚":        "rs",
	"塞舌尔":         "sc",
	"塞拉利昂":        "sl",
	"新加坡":         "sg",
	"斯洛伐克":        "sk",
	"斯洛文尼亚":       "si",
	"所罗门群岛":       "sb",
	"索马里":         "so",
	"南非":          "za",
	"南乔治亚和南桑威奇群岛": "gs",
	"南苏丹":         "ss",
	"西班牙":         "es",
	"斯里兰卡":        "lk",
	"苏丹":          "sd",
	"苏里南":         "sr",
	"斯瓦尔巴和扬马延":    "sj",
	"斯威士兰":        "sz",
	"瑞典":          "se",
	"瑞士":          "ch",
	"叙利亚":         "sy",
	"台湾":          "tw",
	"中国台湾":        "tw",
	"塔吉克斯坦":       "tj",
	"坦桑尼亚":        "tz",
	"泰国":          "th",
	"东帝汶":         "tl",
	"多哥":          "tg",
	"托克劳":         "tk",
	"汤加":          "to",
	"特立尼达和多巴哥":    "tt",
	"突尼斯":         "tn",
	"土耳其":         "tr",
	"土库曼斯坦":       "tm",
	"特克斯和凯科斯群岛":   "tc",
	"图瓦卢":         "tv",
	"乌干达":         "ug",
	"乌克兰":         "ua",
	"阿联酋":         "ae",
	"英国":          "gb",
	"美国":          "us",
	"美国本土外小岛屿":    "um",
	"乌拉圭":         "uy",
	"乌兹别克斯坦":      "uz",
	"瓦努阿图":        "vu",
	"委内瑞拉":        "ve",
	"越南":          "vn",
	"英属维尔京群岛":     "vg",
	"美属维尔京群岛":     "vi",
	"瓦利斯和富图纳":     "wf",
	"西撒哈拉":        "eh",
	"也门":          "ye",
	"赞比亚":         "zm",
	"津巴布韦":        "zw",
}
