package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type trafficSample struct {
	T    int64 `json:"t"`
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type APIServer struct {
	cfg      *Config
	logger   *Logger
	tunnel   *SSHTunnel
	proxyMgr *ProxyManager
	tracker  *ConnTracker
	dns      *DNSServer
	router   *Router
	limiter  *BandwidthLimiter
	start    time.Time
	srv      *http.Server
	done     chan struct{}

	speedMu   sync.Mutex
	speedUp   int64
	speedDown int64

	trafficMu  sync.Mutex
	traffic    []trafficSample
	maxTraffic int
}

func NewAPIServer(cfg *Config, logger *Logger, tunnel *SSHTunnel, pm *ProxyManager, tr *ConnTracker, dns *DNSServer, router *Router, limiter *BandwidthLimiter) *APIServer {
	a := &APIServer{
		cfg: cfg, logger: logger, tunnel: tunnel,
		proxyMgr: pm, tracker: tr, dns: dns, router: router, limiter: limiter,
		start: time.Now(), done: make(chan struct{}),
		maxTraffic: 120,
	}
	go a.rateSampler()
	return a
}

func (a *APIServer) rateSampler() {
	tk := time.NewTicker(time.Second)
	defer tk.Stop()
	var lastUp, lastDown int64
	for {
		select {
		case <-a.done:
			return
		case <-tk.C:
		}
		totalUp := a.tracker.TotalUp.Load()
		totalDown := a.tracker.TotalDown.Load()
		up := totalUp - lastUp
		down := totalDown - lastDown
		if up < 0 {
			up = 0
		}
		if down < 0 {
			down = 0
		}
		lastUp = totalUp
		lastDown = totalDown
		a.speedMu.Lock()
		a.speedUp = up
		a.speedDown = down
		a.speedMu.Unlock()

		a.trafficMu.Lock()
		a.traffic = append(a.traffic, trafficSample{
			T: time.Now().Unix(), Up: up, Down: down,
		})
		if len(a.traffic) > a.maxTraffic {
			a.traffic = a.traffic[len(a.traffic)-a.maxTraffic:]
		}
		a.trafficMu.Unlock()
	}
}

func (a *APIServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleUI)
	mux.HandleFunc("/api/status", a.auth(a.handleStatus))
	mux.HandleFunc("/api/proxies", a.auth(a.handleProxies))
	mux.HandleFunc("/api/proxies/", a.auth(a.handleProxySwitch))
	mux.HandleFunc("/api/connections", a.auth(a.handleConnections))
	mux.HandleFunc("/api/connections/", a.auth(a.handleConnectionAction))
	mux.HandleFunc("/api/devices", a.auth(a.handleDevices))
	mux.HandleFunc("/api/traffic", a.auth(a.handleTraffic))
	mux.HandleFunc("/api/rules/test", a.auth(a.handleRuleTest))
	mux.HandleFunc("/api/fakeip/save", a.auth(a.handleFakeIPSave))
	mux.HandleFunc("/api/fakeip/stats", a.auth(a.handleFakeIPStats))
	mux.HandleFunc("/api/logs", a.auth(a.handleLogs))
	mux.HandleFunc("/api/config", a.auth(a.handleConfig))
	mux.HandleFunc("/api/limiter", a.auth(a.handleLimiter))
	mux.HandleFunc("/api/limiter/", a.auth(a.handleLimiterAction))

	a.srv = &http.Server{Addr: a.cfg.API.Bind, Handler: mux}
	go func() {
		a.logger.Info("API server: http://%s", a.cfg.API.Bind)
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("API: %v", err)
		}
	}()
	return nil
}

func (a *APIServer) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.API.Secret == "" {
			h(w, r)
			return
		}
		token := r.Header.Get("X-Secret")
		if token == "" {
			token = r.URL.Query().Get("secret")
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(a.cfg.API.Secret)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		h(w, r)
	}
}

func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	lastErr, _ := a.tunnel.lastErr.Load().(string)
	a.speedMu.Lock()
	up := a.speedUp
	down := a.speedDown
	a.speedMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"version":        Version,
		"state":          a.tunnel.State(),
		"uptime_sec":     int(time.Since(a.start).Seconds()),
		"server":         fmt.Sprintf("%s:%d", a.cfg.Server, a.cfg.ServerPort),
		"reconnects":     a.tunnel.reconnect.Load(),
		"last_reconnect": a.tunnel.lastReconnect.Load(), // Waktu reconnect terakhir
		"last_error":     lastErr,
		"dns_queries":    a.dns.queries.Load(),
		"dns_errors":     a.dns.errors.Load(),
		"dns_pool":       a.dns.pool.Stats(),
		"total_up":       a.tracker.TotalUp.Load(),
		"total_down":     a.tracker.TotalDown.Load(),
		"speed_up":       up,
		"speed_down":     down,
		"active_conns":   a.tracker.ActiveCount(),
		"closed_conns":   a.tracker.ClosedCount(),
	})
}

func (a *APIServer) handleProxies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"proxies": a.proxyMgr.List()})
}

func (a *APIServer) handleProxySwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" && r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	groupName := strings.TrimPrefix(r.URL.Path, "/api/proxies/")
	if groupName == "" {
		http.Error(w, "missing group", 400)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := a.proxyMgr.Set(groupName, body.Name); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	a.logger.Info("Group %s → %s", groupName, body.Name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *APIServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	var list []*ConnectionView
	switch state {
	case "closed":
		list = a.tracker.ListClosed()
	case "all":
		list = a.tracker.ListAll()
	default:
		list = a.tracker.ListActive()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"connections": list})
}

func (a *APIServer) handleConnectionAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/connections/")

	if path == "kill" {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		var body struct {
			IDs   []string `json:"ids"`
			Src   string   `json:"src"`
			Proxy string   `json:"proxy"`
			All   bool     `json:"all"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var n int
		switch {
		case body.All:
			n = a.tracker.ForceCloseAll("killed")
		case body.Src != "":
			n = a.tracker.ForceCloseBySrc(body.Src, "killed")
		case body.Proxy != "":
			n = a.tracker.ForceCloseByProxy(body.Proxy, "killed")
		case len(body.IDs) > 0:
			n = a.tracker.ForceCloseMany(body.IDs, "killed")
		default:
			http.Error(w, "specify all, src, proxy, or ids", 400)
			return
		}
		a.logger.Info("Kill %d connection(s) via API", n)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"closed": n})
		return
	}

	if path == "" {
		http.Error(w, "missing id", 400)
		return
	}
	if r.Method != "DELETE" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if a.tracker.ForceClose(path, "killed") {
		a.logger.Info("Kill connection %s via API", path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	} else {
		http.Error(w, "not found", 404)
	}
}

func (a *APIServer) handleDevices(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"devices": a.tracker.AggregateBySrc()})
}

func (a *APIServer) handleTraffic(w http.ResponseWriter, r *http.Request) {
	a.trafficMu.Lock()
	samples := make([]trafficSample, len(a.traffic))
	copy(samples, a.traffic)
	a.trafficMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"samples": samples})
}

func (a *APIServer) handleRuleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if a.router == nil {
		http.Error(w, "router not available", 503)
		return
	}
	var body struct {
		SrcIP  string `json:"srcIP"`
		DstIP  string `json:"dstIP"`
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	proxy, rule, chain := a.router.Test(body.SrcIP, body.DstIP, body.Domain)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"proxy": proxy, "rule": rule, "chain": chain,
	})
}

func (a *APIServer) handleFakeIPSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if a.cfg.FakeIPFile == "" {
		http.Error(w, "fakeip_file tidak dikonfigurasi", 400)
		return
	}
	if err := a.dns.pool.Save(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	a.logger.Info("fake-IP saved via API ke %s", a.cfg.FakeIPFile)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":   true,
		"file": a.cfg.FakeIPFile,
	})
}

func (a *APIServer) handleFakeIPStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"enabled": a.cfg.DNS.Enabled,
		"file":    a.cfg.FakeIPFile,
		"stats":   a.dns.pool.Stats(),
	})
}

func (a *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	ch := a.logger.Subscribe()
	defer a.logger.Unsubscribe(ch)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	timeout := time.After(30 * time.Second)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return
			}
			w.Write([]byte(line))
			flusher.Flush()
		case <-timeout:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (a *APIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"version":         Version,
		"server":          fmt.Sprintf("%s:%d", a.cfg.Server, a.cfg.ServerPort),
		"tunnel":          a.cfg.TunnelType,
		"socks5":          fmt.Sprintf("%s:%d", a.cfg.SOCKS5.Bind, a.cfg.SOCKS5.Port),
		"dns":             a.cfg.DNS.Bind,
		"dns_enabled":     a.cfg.DNS.Enabled,
		"fakeip_file":     a.cfg.FakeIPFile,
		"redirect":        a.cfg.Redirect.Bind,
		"conn_retention":  a.cfg.ConnRetention.D().String(),
		"startup_timeout": a.cfg.StartupTimeout.D().String(),
		"limiter": map[string]any{
			"enabled": a.cfg.Limiter.Enabled,
			"file":    a.cfg.Limiter.File,
		},
		"rules": a.cfg.Rules,
	})
}

// ============ Limiter API ============

func (a *APIServer) handleLimiter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"enabled": a.limiter.Enabled(),
			"file":    a.cfg.Limiter.File,
			"rules":   a.limiter.List(),
		})
	case "POST":
		var rule LimiterRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if err := a.limiter.Set(rule); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		a.logger.Info("Limiter: set %s up=%dkbps down=%dkbps enabled=%v",
			rule.IP, rule.UpKbps, rule.DownKbps, rule.Enabled)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (a *APIServer) handleLimiterAction(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimPrefix(r.URL.Path, "/api/limiter/")
	if ip == "" {
		http.Error(w, "missing IP", 400)
		return
	}
	if r.Method != "DELETE" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if err := a.limiter.Delete(ip); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	a.logger.Info("Limiter: delete %s", ip)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *APIServer) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(uiHTML))
}

func (a *APIServer) Close() {
	select {
	case <-a.done:
	default:
		close(a.done)
	}
	if a.srv != nil {
		a.srv.Close()
	}
}