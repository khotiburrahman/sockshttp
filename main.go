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
		log.Fatal("server/port belum diisi")
	}
	if cfg.ValidityDate > 0 && time.Now().UnixMilli() >= cfg.ValidityDate {
		log.Fatalf("config expired: %s", time.UnixMilli(cfg.ValidityDate))
	}

	logger := NewLogger(parseLevel(cfg.Log.Level))
	if cfg.Log.File != "" {
		if err := logger.SetFile(cfg.Log.File, cfg.Log.MaxSizeMB, cfg.Log.MaxBackups); err != nil {
			logger.Warn("Log file: %v (stdout only)", err)
		}
	}

	logger.Info("=== SocksHTTP-Go v2.0 (integrated router) ===")
	logger.Info("Server=%s:%d Tunnel=%s API=%s", cfg.Server, cfg.ServerPort, cfg.TunnelType, cfg.API.Bind)

	if cfg.PIDFile != "" {
		os.MkdirAll(filepath.Dir(cfg.PIDFile), 0755)
		os.WriteFile(cfg.PIDFile, []byte(strconv.Itoa(os.Getpid())), 0644)
		defer os.Remove(cfg.PIDFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	pool := NewFakeIPPool()
	tunnel := NewSSHTunnel(cfg, logger)
	proxyMgr := NewProxyManager(cfg.ProxyGroups, cfg.StateFile)
	tracker := NewConnTracker()
	dns := NewDNSServer(cfg, logger, pool, tunnel)

	tmDone := make(chan struct{})
	go func() {
		tunnel.Run(ctx)
		close(tmDone)
	}()
	tunnel.StartHealthLoop(ctx)

	logger.Info("Menunggu koneksi SSH pertama...")
	for i := 0; i < 120; i++ {
		if tunnel.State() == "connected" {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	if tunnel.State() != "connected" {
		logger.Error("Timeout menunggu koneksi awal")
		cancel()
		os.Exit(1)
	}

	if cfg.DNS.Enabled {
		if err := dns.Start(); err != nil {
			logger.Error("DNS: %v", err)
			cancel()
			os.Exit(1)
		}
	}

	router, err := NewRouter(cfg, logger, tunnel, dns, pool, proxyMgr, tracker)
	if err != nil {
		logger.Error("Router: %v", err)
		cancel()
		os.Exit(1)
	}
	if err := router.Start(); err != nil {
		logger.Error("Router start: %v", err)
		cancel()
		os.Exit(1)
	}

	api := NewAPIServer(cfg, logger, tunnel, proxyMgr, tracker, dns)
	if err := api.Start(); err != nil {
		logger.Error("API: %v", err)
		cancel()
		os.Exit(1)
	}

	logger.Info("Semua layanan siap. Buka http://%s/ di browser.", cfg.API.Bind)

	select {
	case <-sig:
		logger.Info("Shutdown signal diterima")
	case <-tmDone:
		logger.Warn("Tunnel loop berhenti")
	}

	cancel()
	<-tmDone
	router.Close()
	dns.Close()
	api.Close()
	tracker.CloseAll()
	logger.Info("Bye")
}