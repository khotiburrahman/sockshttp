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

func main() { os.Exit(run()) }

func run() int {
	cfgPath := flag.String("config", "", "path ke file YAML config")
	sksPath := flag.String("sks", "", "path ke file .sks")
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

	// Normalisasi multi-account SSH:
	// kalau ssh_accounts tidak diisi, isi otomatis dari field top-level
	// (supaya flag CLI & config lama tetap berfungsi).
	if len(cfg.SSHAccounts) == 0 && cfg.Server != "" && cfg.ServerPort > 0 {
		cfg.SSHAccounts = []SSHAccount{{
			Server:     cfg.Server,
			ServerPort: cfg.ServerPort,
			Username:   cfg.Username,
			Password:   cfg.Password,
			KeyPath:    cfg.KeyPath,
		}}
	}
	if cfg.MaxRetryPerAccount <= 0 {
		cfg.MaxRetryPerAccount = 3
	}

	// Validasi minimal: harus ada minimal 1 akun valid
	hasAccount := len(cfg.SSHAccounts) > 0 &&
		cfg.SSHAccounts[0].Server != "" && cfg.SSHAccounts[0].ServerPort > 0
	if !hasAccount {
		log.Println("server/port belum diisi (set di ssh_accounts, atau server/server_port)")
		return 1
	}

	logger := NewLogger(parseLevel(cfg.Log.Level))
	defer logger.Close()
	if cfg.Log.File != "" {
		if err := logger.SetFile(cfg.Log.File, cfg.Log.MaxSizeMB, cfg.Log.MaxBackups); err != nil {
			logger.Warn("Log file: %v (stdout only)", err)
		}
	}

	logger.Info("=== SocksHTTP-Go %s (forward-only router) ===", Version)
	logger.Info("Tunnel=%s SOCKS5=%s:%d DNS=%s API=%s",
		cfg.TunnelType, cfg.SOCKS5.Bind, cfg.SOCKS5.Port, cfg.DNS.Bind, cfg.API.Bind)

	if n := len(cfg.SSHAccounts); n > 1 {
		logger.Info("Multi-account SSH aktif: %d akun (rotate tiap %d kegagalan per akun)",
			n, cfg.MaxRetryPerAccount)
		for i, a := range cfg.SSHAccounts {
			logger.Info("  [%d] %s@%s:%d", i+1, a.Username, a.Server, a.ServerPort)
		}
	} else if n == 1 {
		a := cfg.SSHAccounts[0]
		logger.Info("SSH: %s@%s:%d", a.Username, a.Server, a.ServerPort)
	}

	if cfg.PIDFile != "" {
		os.MkdirAll(filepath.Dir(cfg.PIDFile), 0755)
		os.WriteFile(cfg.PIDFile, []byte(strconv.Itoa(os.Getpid())), 0644)
		defer os.Remove(cfg.PIDFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	realCache := NewRealIPCache()
	go realCache.PruneLoop(ctx)

	dynamicRules := NewDynamicRules(cfg.DynamicRulesFile, logger)
	dynamicRules.StartWatcher(ctx)

	tunnel := NewSSHTunnel(cfg, logger, dynamicRules)
	proxyMgr := NewProxyManager(cfg.ProxyGroups, cfg.StateFile)
	tracker := NewConnTracker(cfg.StateFile, cfg.ConnRetention.D())
	defer tracker.Shutdown()

	limiter := NewBandwidthLimiter(cfg.Limiter.Enabled, cfg.Limiter.File, logger)
	adblock := NewAdblocker(cfg)

	dns := NewDNSServer(cfg, logger, realCache, tunnel, adblock)
	socks5 := NewSOCKS5Server(cfg, logger, tunnel, proxyMgr, tracker)

	router, err := NewRouter(cfg, logger, tunnel, dns, realCache, proxyMgr, tracker, limiter, dynamicRules)
	if err != nil {
		logger.Error("Router init: %v", err)
		return 1
	}

	// Putus koneksi jika grup diubah (REJECT/TUNNEL/DIRECT)
	proxyMgr.OnProxyChange = func(group, newProxy string) {
		router.ForceCloseGroup(group, "proxy-switched-to-"+newProxy)
	}

	tmDone := make(chan struct{})
	go func() { tunnel.Run(ctx); close(tmDone) }()
	tunnel.StartHealthLoop(ctx)

	// Failure Hook
	hook := NewFailureHook(cfg.FailureHook, logger, tunnel)
	hook.Start(ctx)

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

	api := NewAPIServer(cfg, logger, tunnel, proxyMgr, tracker, dns, router, limiter, realCache, dynamicRules)
	if err := api.Start(); err != nil {
		logger.Error("API: %v", err)
		return 1
	}

	logger.Info("=================================================")
	logger.Info("  SEMUA LAYANAN SIAP")
	logger.Info("  Web UI   : http://%s/", cfg.API.Bind)
	logger.Info("=================================================")

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sig:
		logger.Info("Shutdown signal diterima")
	case <-tmDone:
		logger.Warn("Tunnel loop berhenti tanpa signal")
	}

	logger.Info("Shutdown: menutup listener...")
	api.Close()
	router.Shutdown()
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