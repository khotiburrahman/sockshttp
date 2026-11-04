package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func main() {
	cfgPath  := flag.String("config", "", "path ke file YAML config")
	sksPath  := flag.String("sks", "", "path ke file .sks (legacy Android)")
	server   := flag.String("server", "", "override server SSH")
	port     := flag.Int("port", 0, "override port SSH")
	user     := flag.String("user", "", "override username")
	pass     := flag.String("pass", "", "override password")
	apiBind  := flag.String("api", "", "override API bind")
	logLevel := flag.String("log-level", "", "override log level")
	flag.Parse()

	// =========================================================
	// 1. LOAD CONFIG
	// =========================================================
	cfg := DefaultConfig()
	var err error

	if *cfgPath != "" {
		cfg, err = LoadYAMLConfig(*cfgPath)
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
	} else if *sksPath != "" {
		f, e := os.Open(*sksPath)
		if e != nil {
			log.Fatalf("open sks: %v", e)
		}
		err = LoadSKSConfig(f, cfg)
		f.Close()
		if err != nil {
			log.Fatalf("parse sks: %v", err)
		}
	}

	// CLI overrides
	if *server != "" {
		cfg.Server = *server
	}
	if *port != 0 {
		cfg.ServerPort = *port
	}
	if *user != "" {
		cfg.Username = *user
	}
	if *pass != "" {
		cfg.Password = *pass
	}
	if *apiBind != "" {
		cfg.API.Bind = *apiBind
	}
	if *logLevel != "" {
		cfg.Log.Level = *logLevel
	}

	// Validasi
	if cfg.Server == "" || cfg.ServerPort <= 0 {
		log.Fatal("server/port belum diisi")
	}
	if cfg.ValidityDate > 0 && time.Now().UnixMilli() >= cfg.ValidityDate {
		log.Fatalf("config expired: %s", time.UnixMilli(cfg.ValidityDate))
	}

	// =========================================================
	// 2. INIT LOGGER
	// =========================================================
	logger := NewLogger(parseLevel(cfg.Log.Level))
	if cfg.Log.File != "" {
		if err := logger.SetFile(cfg.Log.File, cfg.Log.MaxSizeMB, cfg.Log.MaxBackups); err != nil {
			logger.Warn("Log file: %v (stdout only)", err)
		}
	}

	logger.Info("=== SocksHTTP-Go v2.0 (integrated router) ===")
	logger.Info("Server=%s:%d Tunnel=%s SOCKS5=%s:%d DNS=%s API=%s",
		cfg.Server, cfg.ServerPort, cfg.TunnelType,
		cfg.SOCKS5.Bind, cfg.SOCKS5.Port,
		cfg.DNS.Bind, cfg.API.Bind)

	// =========================================================
	// 3. PID FILE
	// =========================================================
	if cfg.PIDFile != "" {
		os.MkdirAll(filepath.Dir(cfg.PIDFile), 0755)
		os.WriteFile(cfg.PIDFile, []byte(strconv.Itoa(os.Getpid())), 0644)
		defer os.Remove(cfg.PIDFile)
	}

	// =========================================================
	// 4. CONTEXT & SIGNAL
	// =========================================================
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	// =========================================================
	// 5. INIT SEMUA KOMPONEN
	// =========================================================
	pool := NewFakeIPPool()
	tunnel := NewSSHTunnel(cfg, logger)
	proxyMgr := NewProxyManager(cfg.ProxyGroups, cfg.StateFile)
	tracker := NewConnTracker()
	dns := NewDNSServer(cfg, logger, pool, tunnel)
	socks5 := NewSOCKS5Server(cfg, logger, tunnel, proxyMgr)

	// =========================================================
	// 6. START TUNNEL (background, auto-reconnect)
	// =========================================================
	tmDone := make(chan struct{})
	go func() {
		tunnel.Run(ctx)
		close(tmDone)
	}()
	tunnel.StartHealthLoop(ctx)

	// =========================================================
	// 7. TUNGGU KONEKSI SSH PERTAMA
	// =========================================================
	logger.Info("Menunggu koneksi SSH pertama...")
	for i := 0; i < 120; i++ {
		if tunnel.State() == "connected" {
			break
		}
		select {
		case <-ctx.Done():
			logger.Warn("Dibatalkan sebelum koneksi")
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	if tunnel.State() != "connected" {
		logger.Error("Timeout menunggu koneksi awal (60 detik)")
		cancel()
		os.Exit(1)
	}

	// =========================================================
	// 8. START SOCKS5 (paling penting untuk test)
	// =========================================================
	if err := socks5.Start(); err != nil {
		logger.Error("SOCKS5: %v", err)
		cancel()
		os.Exit(1)
	}

	// =========================================================
	// 9. START DNS (fake-ip)
	// =========================================================
	if cfg.DNS.Enabled {
		if err := dns.Start(); err != nil {
			logger.Error("DNS: %v", err)
			cancel()
			os.Exit(1)
		}
	}

	// =========================================================
	// 10. START ROUTER (REDIRECT listener untuk iptables)
	// =========================================================
	router, err := NewRouter(cfg, logger, tunnel, dns, pool, proxyMgr, tracker)
	if err != nil {
		logger.Error("Router init: %v", err)
		cancel()
		os.Exit(1)
	}
	if err := router.Start(); err != nil {
		logger.Error("Router start: %v", err)
		cancel()
		os.Exit(1)
	}

	// =========================================================
	// 11. START API + WEB UI
	// =========================================================
	api := NewAPIServer(cfg, logger, tunnel, proxyMgr, tracker, dns)
	if err := api.Start(); err != nil {
		logger.Error("API: %v", err)
		cancel()
		os.Exit(1)
	}

	logger.Info("=================================================")
	logger.Info("  SEMUA LAYANAN SIAP")
	logger.Info("  Web UI   : http://%s/", cfg.API.Bind)
	logger.Info("  SOCKS5   : %s:%d", cfg.SOCKS5.Bind, cfg.SOCKS5.Port)
	logger.Info("  DNS      : %s", cfg.DNS.Bind)
	logger.Info("  REDIRECT : %s", cfg.Redirect.Bind)
	logger.Info("=================================================")

	// =========================================================
	// 12. TUNGGU SHUTDOWN
	// =========================================================
	select {
	case <-sig:
		logger.Info("Shutdown signal diterima")
	case <-tmDone:
		logger.Warn("Tunnel loop berhenti tanpa signal")
	}

	// =========================================================
	// 13. CLEANUP (urutan terbalik dari startup)
	// =========================================================
	cancel()
	<-tmDone

	api.Close()
	router.Close()
	if cfg.DNS.Enabled {
		dns.Close()
	}
	socks5.Close()

	tracker.CloseAll()

	logger.Info("Bye 👋")
}