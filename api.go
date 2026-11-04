package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type APIServer struct {
	cfg      *Config
	logger   *Logger
	tunnel   *SSHTunnel
	proxyMgr *ProxyManager
	tracker  *ConnTracker
	dns      *DNSServer
	start    time.Time
	srv      *http.Server
}

func NewAPIServer(cfg *Config, logger *Logger, tunnel *SSHTunnel, pm *ProxyManager, tr *ConnTracker, dns *DNSServer) *APIServer {
	return &APIServer{
		cfg: cfg, logger: logger, tunnel: tunnel,
		proxyMgr: pm, tracker: tr, dns: dns,
		start: time.Now(),
	}
}

func (a *APIServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleUI)
	mux.HandleFunc("/api/status", a.auth(a.handleStatus))
	mux.HandleFunc("/api/proxies", a.auth(a.handleProxies))
	mux.HandleFunc("/api/proxies/", a.auth(a.handleProxySwitch))
	mux.HandleFunc("/api/connections", a.auth(a.handleConnections))
	mux.HandleFunc("/api/logs", a.auth(a.handleLogs))
	mux.HandleFunc("/api/config", a.auth(a.handleConfig))

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
		if a.cfg.API.Secret != "" {
			token := r.Header.Get("X-Secret")
			if token == "" {
				token = r.URL.Query().Get("secret")
			}
			if token != a.cfg.API.Secret {
				http.Error(w, "unauthorized", 401)
				return
			}
		}
		h(w, r)
	}
}

func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	lastErr, _ := a.tunnel.lastErr.Load().(string)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"state":       a.tunnel.State(),
		"uptime_sec":  int(time.Since(a.start).Seconds()),
		"server":      fmt.Sprintf("%s:%d", a.cfg.Server, a.cfg.ServerPort),
		"reconnects":  a.tunnel.reconnect.Load(),
		"last_error":  lastErr,
		"dns_queries": a.dns.queries.Load(),
	})
}

func (a *APIServer) handleProxies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"proxies": a.proxyMgr.List(),
	})
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
	conns := a.tracker.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"connections": conns,
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
		"server":   fmt.Sprintf("%s:%d", a.cfg.Server, a.cfg.ServerPort),
		"tunnel":   a.cfg.TunnelType,
		"socks5":   fmt.Sprintf("%s:%d", a.cfg.SOCKS5.Bind, a.cfg.SOCKS5.Port),
		"dns":      a.cfg.DNS.Bind,
		"redirect": a.cfg.Redirect.Bind,
		"rules":    a.cfg.Rules,
	})
}

func (a *APIServer) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(uiHTML))
}

func (a *APIServer) Close() {
	if a.srv != nil {
		a.srv.Close()
	}
}