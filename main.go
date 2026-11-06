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

var Version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	cfgPath := flag.String("config", "", "path ke file YAML config")
	sksPath := flag.String("sks", "", "path ke file .sks (legacy Android)")
	server := flag.String("server", "", "override server SSH")
	port := flag.Int("port", 0, "override port SSH")
	user := flag.String("user", "", "override username")
	pass := flag.String("pass", "", "override password")
	apiBind := flag.String("api", "", "override API bind")
	logLevel := flag.String("log-level", "", "override log level")
	flag.Parse()

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

	if cfg.Server == "" || cfg.ServerPort <= 0 {
		log.Println("server/port belum diisi")
		return 1
	}
	if cfg.ValidityDate > 0 && time.Now().UnixMilli() >= cfg.ValidityDate {
		log.Printf("config expired: %s", time.UnixMilli(cfg.ValidityDate))
		return 1
	}

	logger := NewLogger(parseLevel(cfg.Log.Level))
	defer logger.Close()
	if cfg.Log.File != "" {
		if err := logger.SetFile(cfg.Log.File, cfg.Log.MaxSizeMB, cfg.Log.MaxBackups); err != nil {
			logger.Warn("Log file: %v (stdout only)", err)
		}
	}

	logger.Info("=== SocksHTTP-Go %s (integrated router) ===", Version)
	logger.Info("Server=%s:%d Tunnel=%s SOCKS5=%s:%d DNS=%s API=%s",
		cfg.Server, cfg.ServerPort, cfg.TunnelType,
		cfg.SOCKS5.Bind, cfg.SOCKS5.Port, cfg.DNS.Bind, cfg.API.Bind)

	if cfg.PIDFile != "" {
		os.MkdirAll(filepath.Dir(cfg.PIDFile), 0755)
		os.WriteFile(cfg.PIDFile, []byte(strconv.Itoa(os.Getpid())), 0644)
		defer os.Remove(cfg.PIDFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewFakeIPPool()
	tunnel := NewSSHTunnel(cfg, logger)
	proxyMgr := NewProxyManager(cfg.ProxyGroups, cfg.StateFile)
	if issues := proxyMgr.ValidateAll(); len(issues) > 0 {
		for _, s := range issues {
			logger.Warn("Proxy config: %s", s)
		}
	}
	tracker := NewConnTracker(cfg.StateFile, cfg.ConnRetention.D())
	defer tracker.Close()

	dns := NewDNSServer(cfg, logger, pool, tunnel)
	socks5 := NewSOCKS5Server(cfg, logger, tunnel, proxyMgr, pool, tracker)

	router, err := NewRouter(cfg, logger, tunnel, dns, pool, proxyMgr, tracker)
	if err != nil {
		logger.Error("Router init: %v", err)
		return 1
	}

	tmDone := make(chan struct{})
	go func() { tunnel.Run(ctx); close(tmDone) }()
	tunnel.StartHealthLoop(ctx)

	startupTimeout := cfg.StartupTimeout.D()
	if startupTimeout <= 0 {
		startupTimeout = 60 * time.Second
	}
	logger.Info("Menunggu koneksi SSH pertama (max %s)...", startupTimeout)
	deadline := time.Now().Add(startupTimeout)
	for {
		if tunnel.State() == "connected" {
			break
		}
		if time.Now().After(deadline) {
			logger.Error("Timeout menunggu koneksi awal")
			return 1
		}
		select {
		case <-ctx.Done():
			return 1
		case <-time.After(500 * time.Millisecond):
		}
	}

	if err := socks5.Start(); err != nil {
		logger.Error("SOCKS5: %v", err)
		return 1
	}
	if cfg.DNS.Enabled {
		if err := dns.Start(); err != nil {
			logger.Error("DNS: %v", err)
			return 1
		}
	}
	if err := router.Start(); err != nil {
		logger.Error("Router start: %v", err)
		return 1
	}

	api := NewAPIServer(cfg, logger, tunnel, proxyMgr, tracker, dns, router)
	if err := api.Start(); err != nil {
		logger.Error("API: %v", err)
		return 1
	}

	logger.Info("=================================================")
	logger.Info("  SEMUA LAYANAN SIAP")
	logger.Info("  Web UI   : http://%s/", cfg.API.Bind)
	logger.Info("  SOCKS5   : %s:%d", cfg.SOCKS5.Bind, cfg.SOCKS5.Port)
	logger.Info("  DNS      : %s", cfg.DNS.Bind)
	logger.Info("  REDIRECT : %s", cfg.Redirect.Bind)
	logger.Info("=================================================")

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sig:
		logger.Info("Shutdown signal diterima")
	case <-tmDone:
		logger.Warn("Tunnel loop berhenti tanpa signal")
	}

	// Reverse shutdown: listener dulu, tunnel terakhir
	logger.Info("Shutdown: menutup listener...")
	api.Close()
	router.Close()
	socks5.Close()
	if cfg.DNS.Enabled {
		dns.Close()
	}

	logger.Info("Shutdown: menutup tunnel...")
	cancel()
	<-tmDone

	tracker.ForceCloseAll("shutdown")

	logger.Info("Bye 👋")
	return 0
}