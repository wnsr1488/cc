package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/example/cc-panel/internal/audit"
	"github.com/example/cc-panel/internal/auth"
	"github.com/example/cc-panel/internal/config"
	secretcrypto "github.com/example/cc-panel/internal/crypto"
	"github.com/example/cc-panel/internal/firewall"
	"github.com/example/cc-panel/internal/geo"
	"github.com/example/cc-panel/internal/httpx"
	"github.com/example/cc-panel/internal/ipset"
	"github.com/example/cc-panel/internal/monitor"
	"github.com/example/cc-panel/internal/policy"
	serverrepo "github.com/example/cc-panel/internal/server"
	"github.com/example/cc-panel/internal/sshx"
	"github.com/example/cc-panel/internal/webui"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Dependencies struct {
	DB          *pgxpool.Pool
	Config      config.Config
	SecretBox   *secretcrypto.SecretBox
	AuthService *auth.Service
}

type handler struct {
	auth     *auth.Service
	servers  *serverrepo.Repository
	firewall *firewall.Service
	geo      *geo.Service
	policies *policy.Service
	monitor  *monitor.Service
	audit    *audit.Logger
	config   config.Config
}

type App struct {
	Handler http.Handler
	Geo     *geo.Service
	Monitor *monitor.Service
	Policy  *policy.Service
}

func NewApp(deps Dependencies) App {
	serverRepo := serverrepo.NewRepository(deps.DB, deps.SecretBox)
	auditLogger := audit.NewLogger(deps.DB)
	geoService := geo.NewService(deps.DB, serverRepo, sshx.NewSSHExecutor(), deps.Config.SSHTimeout, deps.Config.IP2RegionV4XDB, deps.Config.IP2RegionV6XDB)
	policyService := policy.NewService(deps.DB, serverRepo, sshx.NewSSHExecutor(), geoService, deps.Config.SSHTimeout)
	h := &handler{
		auth:     deps.AuthService,
		servers:  serverRepo,
		firewall: firewall.NewService(deps.DB, serverRepo, sshx.NewSSHExecutor(), deps.Config.SSHTimeout),
		geo:      geoService,
		policies: policyService,
		monitor:  monitor.NewService(deps.DB, serverRepo, sshx.NewSSHExecutor(), deps.Config.SSHTimeout),
		audit:    auditLogger,
		config:   deps.Config,
	}
	return App{Handler: h.buildRouter(deps.AuthService), Geo: geoService, Monitor: h.monitor, Policy: policyService}
}

func NewRouter(deps Dependencies) http.Handler {
	return NewApp(deps).Handler
}

func (h *handler) buildRouter(authService *auth.Service) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", h.login)
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(authService))
			r.Get("/me", h.me)
			r.Get("/servers", h.listServers)
			r.Post("/servers", h.createServer)
			r.Get("/servers/{id}", h.getServer)
			r.Put("/servers/{id}", h.updateServer)
			r.Delete("/servers/{id}", h.deleteServer)
			r.Post("/servers/{id}/test-ssh", h.testSSH)
			r.Post("/servers/{id}/deploy", h.deployServer)
			r.Post("/servers/{id}/stop-rules", h.stopServerRules)
			r.Get("/servers/{id}/firewall/status", h.firewallStatus)
			r.Post("/servers/{id}/rollback", h.rollbackServer)
			r.Get("/servers/{id}/metrics", h.listServerMetrics)
			r.Get("/servers/{id}/metrics/live", h.serverMetricsLive)
			r.Post("/servers/{id}/metrics/collect", h.collectServerMetrics)
			r.Get("/firewall/blacklist", h.listBlacklist)
			r.Post("/firewall/blacklist", h.addBlacklist)
			r.Post("/firewall/blacklist/bulk", h.addBlacklistBulk)
			r.Delete("/firewall/blacklist/{id}", h.deleteFirewallEntry)
			r.Get("/firewall/whitelist", h.listWhitelist)
			r.Post("/firewall/whitelist", h.addWhitelist)
			r.Post("/firewall/whitelist/bulk", h.addWhitelistBulk)
			r.Delete("/firewall/whitelist/{id}", h.deleteFirewallEntry)
			r.Get("/geo/cidrs/summary", h.listGeoCIDRSummaries)
			r.Get("/geo/cidrs", h.listGeoCIDRs)
			r.Post("/geo/cidrs", h.addGeoCIDR)
			r.Post("/geo/cidrs/preview", h.previewGeoCIDRs)
			r.Post("/geo/cidrs/bulk", h.bulkAddGeoCIDRs)
			r.Get("/geo/options", h.geoOptions)
			r.Get("/geo/search", h.searchGeoIP)
			r.Get("/geo/default-whitelist", h.defaultGeoWhitelist)
			r.Get("/geo/default-whitelist/auto-sync", h.getDefaultGeoWhitelistAutoSync)
			r.Put("/geo/default-whitelist/auto-sync", h.updateDefaultGeoWhitelistAutoSync)
			r.Post("/geo/default-whitelist", h.createDefaultGeoWhitelist)
			r.Post("/geo/default-whitelist/sync", h.syncDefaultGeoWhitelist)
			r.Get("/geo/block", h.listGeoRules)
			r.Post("/geo/block", h.createGeoRule)
			r.Get("/policies", h.listPolicies)
			r.Post("/policies", h.createPolicy)
			r.Put("/policies/{id}", h.updatePolicy)
			r.Delete("/policies/{id}", h.deletePolicy)
			r.Get("/policy-events", h.listPolicyEvents)
			r.Post("/servers/{id}/policies/execute", h.executePolicies)
			r.Post("/policies/execute-all", h.executeAllPolicies)
			r.Get("/audit/logs", h.auditLogs)
			r.Get("/metrics/overview", h.metricsOverview)
			r.Post("/metrics/collect-all", h.collectAllMetrics)
		})
	})

	webui.Mount(r, webui.DistDir())

	return r
}

func (h *handler) geoAutoSyncIntervalHours() int {
	hours := int(h.config.GeoCIDRSyncInterval / time.Hour)
	if hours <= 0 {
		return 24
	}
	return hours
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	token, user, err := h.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, auth.ErrInvalidCredentials) {
			status = http.StatusUnauthorized
		}
		httpx.WriteError(w, status, err.Error())
		return
	}
	userID := user.ID
	_ = h.audit.Record(r.Context(), &userID, "auth.login", "user", &userID, nil, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	httpx.WriteJSON(w, http.StatusOK, user)
}

func (h *handler) listServers(w http.ResponseWriter, r *http.Request) {
	items, err := h.servers.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *handler) createServer(w http.ResponseWriter, r *http.Request) {
	var req serverrepo.CreateInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := h.servers.Create(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	userID := currentUserID(r)
	_ = h.audit.Record(r.Context(), userID, "server.create", "server", &item.ID, map[string]any{"name": item.Name, "host": item.Host}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h *handler) getServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	item, err := h.servers.Get(r.Context(), id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *handler) updateServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req serverrepo.UpdateInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := h.servers.Update(r.Context(), id, req)
	if err != nil {
		writeDBError(w, err)
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "server.update", "server", &item.ID, map[string]any{"name": item.Name, "host": item.Host}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *handler) deleteServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.servers.Delete(r.Context(), id); err != nil {
		writeDBError(w, err)
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "server.delete", "server", &id, nil, r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) testSSH(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.firewall.TestSSH(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "server.test_ssh", "server", &id, nil, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) deployServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.firewall.Deploy(r.Context(), id, currentUserID(r)); err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "server.deploy_firewall", "server", &id, nil, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "deployed"})
}

func (h *handler) stopServerRules(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.firewall.StopRules(r.Context(), id, currentUserID(r)); err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "server.stop_firewall", "server", &id, nil, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (h *handler) firewallStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	status, err := h.firewall.Status(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, status)
}

func (h *handler) rollbackServer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	snapshot, err := h.firewall.RollbackLatest(r.Context(), id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "server.rollback_firewall", "server", &id, map[string]any{"snapshot_id": snapshot.ID}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "rolled_back", "snapshot_id": snapshot.ID})
}

func (h *handler) listServerMetrics(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.monitor.List(r.Context(), id, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *handler) collectServerMetrics(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	item, err := h.monitor.Collect(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "monitor.collect", "server", &id, map[string]any{"metric_id": item.ID}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h *handler) serverMetricsLive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	lookup := monitor.RegionLookup(func(ip string) (string, string, string, string, bool) {
		info, err := h.geo.SearchIP(ip)
		if err != nil {
			return "", "", "", "", false
		}
		return info.Country, info.Province, info.City, info.ISP, true
	})
	item, err := h.monitor.LiveInsights(r.Context(), id, limit, lookup)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	filtered := make([]monitor.IPConnection, 0, len(item.Connections))
	for _, conn := range item.Connections {
		match, err := h.geo.MatchesDefaultWhitelistIP(conn.IP)
		if err != nil || match {
			continue
		}
		filtered = append(filtered, conn)
	}
	item.Connections = filtered
	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *handler) metricsOverview(w http.ResponseWriter, r *http.Request) {
	items, err := h.monitor.Overview(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *handler) collectAllMetrics(w http.ResponseWriter, r *http.Request) {
	result, err := h.monitor.CollectAll(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "monitor.collect_all", "server", nil, map[string]any{
		"collected": result.Collected,
		"failed":    result.Failed,
	}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *handler) addBlacklist(w http.ResponseWriter, r *http.Request) {
	h.addFirewallEntries(w, r, ipset.BlacklistSet, "firewall.blacklist.add")
}

func (h *handler) listBlacklist(w http.ResponseWriter, r *http.Request) {
	h.listFirewallEntries(w, r, ipset.BlacklistSet)
}

func (h *handler) addWhitelist(w http.ResponseWriter, r *http.Request) {
	h.addFirewallEntries(w, r, ipset.WhitelistSet, "firewall.whitelist.add")
}

func (h *handler) addBlacklistBulk(w http.ResponseWriter, r *http.Request) {
	h.addFirewallEntriesBulk(w, r, ipset.BlacklistSet, "firewall.blacklist.bulk_add")
}

func (h *handler) addWhitelistBulk(w http.ResponseWriter, r *http.Request) {
	h.addFirewallEntriesBulk(w, r, ipset.WhitelistSet, "firewall.whitelist.bulk_add")
}

func (h *handler) listWhitelist(w http.ResponseWriter, r *http.Request) {
	h.listFirewallEntries(w, r, ipset.WhitelistSet)
}

func (h *handler) listFirewallEntries(w http.ResponseWriter, r *http.Request, setName string) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := h.firewall.ListEntries(r.Context(), setName, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, entries)
}

func (h *handler) addFirewallEntries(w http.ResponseWriter, r *http.Request, setName, action string) {
	var req firewall.AddEntryInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	entries, err := h.firewall.AddEntries(r.Context(), setName, req, currentUserID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, entry := range entries {
		entryID := entry.ID
		_ = h.audit.Record(r.Context(), currentUserID(r), action, "firewall_entry", &entryID, map[string]any{"server_id": entry.ServerID, "ip": entry.IP}, r.RemoteAddr)
	}
	httpx.WriteJSON(w, http.StatusCreated, entries)
}

func (h *handler) addFirewallEntriesBulk(w http.ResponseWriter, r *http.Request, setName, action string) {
	var req firewall.BulkAddEntryInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	result, err := h.firewall.AddEntriesBulk(r.Context(), setName, req, currentUserID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), action, "firewall_entry", nil, map[string]any{
		"server_ids": req.ServerIDs,
		"ip_count":   len(result.Entries) / max(len(req.ServerIDs), 1),
		"added":      result.Added,
	}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (h *handler) deleteFirewallEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	entry, err := h.firewall.DeleteEntry(r.Context(), id)
	if err != nil {
		writeDBError(w, err)
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "firewall.entry.delete", "firewall_entry", &id, map[string]any{"server_id": entry.ServerID, "ip": entry.IP, "set_name": entry.SetName}, r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) listGeoCIDRs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.geo.ListCIDRs(r.Context(), r.URL.Query().Get("country"), r.URL.Query().Get("province"), r.URL.Query().Get("city"), limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *handler) listGeoCIDRSummaries(w http.ResponseWriter, r *http.Request) {
	items, err := h.geo.ListCIDRSummaries(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *handler) addGeoCIDR(w http.ResponseWriter, r *http.Request) {
	var req geo.AddCIDRInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := h.geo.AddCIDR(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "geo.cidr.add", "geo_cidr", &item.ID, map[string]any{"country": item.Country, "cidr": item.CIDR}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h *handler) previewGeoCIDRs(w http.ResponseWriter, r *http.Request) {
	var req geo.PreviewCIDRsInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, h.geo.PreviewCIDRs(req))
}

func (h *handler) bulkAddGeoCIDRs(w http.ResponseWriter, r *http.Request) {
	var req geo.PreviewCIDRsInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	items, err := h.geo.BulkAddCIDRs(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "geo.cidr.bulk_add", "geo_cidr", nil, map[string]any{"count": len(items)}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, items)
}

func (h *handler) geoOptions(w http.ResponseWriter, r *http.Request) {
	options, err := h.geo.Options(r.Context(), r.URL.Query().Get("country"), r.URL.Query().Get("province"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, options)
}

func (h *handler) searchGeoIP(w http.ResponseWriter, r *http.Request) {
	ip := r.URL.Query().Get("ip")
	info, err := h.geo.SearchIP(ip)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, info)
}

func (h *handler) defaultGeoWhitelist(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"countries": h.geo.DefaultWhitelistCountries()})
}

func (h *handler) getDefaultGeoWhitelistAutoSync(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.geo.GetAutoSyncConfig(r.Context(), h.geoAutoSyncIntervalHours())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cfg)
}

func (h *handler) updateDefaultGeoWhitelistAutoSync(w http.ResponseWriter, r *http.Request) {
	var req geo.AutoSyncUpdateInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	cfg, err := h.geo.UpdateAutoSyncConfig(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err = h.geo.GetAutoSyncConfig(r.Context(), h.geoAutoSyncIntervalHours())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "geo.default_whitelist.auto_sync.update", "geo_cidr", nil, map[string]any{
		"enabled":    cfg.Enabled,
		"server_ids": cfg.ServerIDs,
	}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, cfg)
}

func (h *handler) createDefaultGeoWhitelist(w http.ResponseWriter, r *http.Request) {
	var req geo.DefaultWhitelistInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	rules, err := h.geo.CreateDefaultWhitelist(r.Context(), req, currentUserID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "geo.default_whitelist.create", "geo_block_rule", nil, map[string]any{"countries": h.geo.DefaultWhitelistCountries(), "rules": len(rules)}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, rules)
}

func (h *handler) syncDefaultGeoWhitelist(w http.ResponseWriter, r *http.Request) {
	var req geo.DefaultWhitelistInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	results, err := h.geo.SyncDefaultWhitelist(r.Context(), req, currentUserID(r))
	if err != nil {
		_ = h.audit.Record(r.Context(), currentUserID(r), "geo.default_whitelist.sync_failed", "geo_cidr", nil, map[string]any{"countries": h.geo.DefaultWhitelistCountries(), "server_ids": req.ServerIDs, "results": results, "error": err.Error()}, r.RemoteAddr)
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "geo.default_whitelist.sync", "geo_cidr", nil, map[string]any{"countries": h.geo.DefaultWhitelistCountries(), "server_ids": req.ServerIDs, "results": results}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, results)
}

func (h *handler) listGeoRules(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rules, err := h.geo.ListRules(r.Context(), limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, rules)
}

func (h *handler) createGeoRule(w http.ResponseWriter, r *http.Request) {
	var req geo.CreateRuleInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	rule, err := h.geo.CreateRule(r.Context(), req, currentUserID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "geo.block.create", "geo_block_rule", &rule.ID, map[string]any{"country": rule.Country, "province": rule.Province, "city": rule.City}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, rule)
}

func (h *handler) listPolicies(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.policies.List(r.Context(), limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, items)
}

func (h *handler) createPolicy(w http.ResponseWriter, r *http.Request) {
	var req policy.Input
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := h.policies.Create(r.Context(), req, currentUserID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "policy.create", "auto_policy", &item.ID, map[string]any{"name": item.Name, "metric": item.Metric}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h *handler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var req policy.Input
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	item, err := h.policies.Update(r.Context(), id, req)
	if err != nil {
		writeDBError(w, err)
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "policy.update", "auto_policy", &item.ID, map[string]any{"name": item.Name, "metric": item.Metric}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *handler) deletePolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := h.policies.Delete(r.Context(), id); err != nil {
		writeDBError(w, err)
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "policy.delete", "auto_policy", &id, nil, r.RemoteAddr)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) listPolicyEvents(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	result, err := h.policies.ListEventsPage(r.Context(), page, pageSize)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *handler) executePolicies(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	events, err := h.policies.Execute(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "policy.execute", "server", &id, map[string]any{"events": len(events)}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusCreated, events)
}

func (h *handler) executeAllPolicies(w http.ResponseWriter, r *http.Request) {
	count, err := h.policies.ExecuteAll(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = h.audit.Record(r.Context(), currentUserID(r), "policy.execute_all", "auto_policy", nil, map[string]any{"events": count}, r.RemoteAddr)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"events": count})
}

func (h *handler) auditLogs(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize == 0 {
		pageSize, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	}
	result, err := h.audit.ListPage(r.Context(), page, pageSize)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func currentUserID(r *http.Request) *int64 {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		return nil
	}
	return &user.ID
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func writeDBError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	httpx.WriteError(w, http.StatusInternalServerError, err.Error())
}
