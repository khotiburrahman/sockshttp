package main

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

// Prefetcher — pre-resolve & (opsional) warm-dial domain populer dari
// dynamic_rules.yaml (section `prefetch:`). Domain list hot-reload karena
// diambil dari DynamicRules setiap round.
type Prefetcher struct {
	cfg         *Config
	dns         *DNSServer
	tunnel      *SSHTunnel
	dynRules    *DynamicRules
	logger      *Logger
	interval    time.Duration
	warmDial    bool
	warmTimeout time.Duration
}

func NewPrefetcher(cfg *Config, dns *DNSServer, tunnel *SSHTunnel, dynRules *DynamicRules, logger *Logger) *Prefetcher {
	interval := cfg.Prefetch.Interval.D()
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	warmTimeout := cfg.Prefetch.WarmDialTimeout.D()
	if warmTimeout <= 0 {
		warmTimeout = 5 * time.Second
	}
	return &Prefetcher{
		cfg:         cfg,
		dns:         dns,
		tunnel:      tunnel,
		dynRules:    dynRules,
		logger:      logger,
		interval:    interval,
		warmDial:    cfg.Prefetch.WarmDial,
		warmTimeout: warmTimeout,
	}
}

func (p *Prefetcher) Start(ctx context.Context) {
	if !p.cfg.Prefetch.Enabled || p.dynRules == nil {
		return
	}
	p.logger.Info("[prefetch] aktif, interval=%v, warm_dial=%v", p.interval, p.warmDial)

	// Round pertama setelah tunnel settle
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
		}
		p.runOnce(ctx)
	}()

	// Periodic
	go func() {
		tk := time.NewTicker(p.interval)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				p.runOnce(ctx)
			}
		}
	}()
}

func (p *Prefetcher) runOnce(ctx context.Context) {
	if p.tunnel == nil || p.tunnel.State() != "connected" {
		return
	}
	enabled, domains := p.dynRules.GetPrefetch()
	if !enabled || len(domains) == 0 {
		p.logger.Debug("[prefetch] skip: dynamic_rules.prefetch kosong/nonaktif")
		return
	}

	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, d := range domains {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			defer func() { <-sem }()
			p.prefetchDomain(ctx, domain)
			if p.warmDial {
				p.warmDialDomain(ctx, domain)
			}
		}(d)
	}
	wg.Wait()
	p.logger.Debug("[prefetch] round selesai: %d domain", len(domains))
}

// prefetchDomain — resolve A-record, isi fwdCache + realCache lewat
// processResponse (helper yang sama dipakai handleQuery & refreshEntry).
func (p *Prefetcher) prefetchDomain(ctx context.Context, domain string) {
	q := buildDNSQuery(domain, 1, 1) // A, IN
	if q == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	resp, err := p.dns.resolver.Resolve(cctx, q)
	if err != nil || len(resp) < 12 {
		return
	}
	minTTL, has := p.dns.processResponse(resp, domain)
	if !has {
		return
	}
	if minTTL < 60 {
		minTTL = 60
	}
	if minTTL > 3600 {
		minTTL = 3600
	}
	key := strings.ToLower(domain) + "|1|1"
	p.dns.fwdCache.Set(key, resp, time.Duration(minTTL)*time.Second)
}

// warmDialDomain — dial TCP:443 via tunnel lalu close; memanaskan
// route/NAT/proxy di jalur SSH.
func (p *Prefetcher) warmDialDomain(ctx context.Context, domain string) {
	if p.tunnel == nil {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, p.warmTimeout)
	defer cancel()

	type result struct {
		c   net.Conn
		err error
	}
	ch := make(chan result, 1)
	go func() {
		c, e := p.tunnel.Dial("tcp", domain+":443")
		ch <- result{c, e}
	}()

	select {
	case r := <-ch:
		if r.c != nil {
			r.c.Close()
		}
	case <-cctx.Done():
		go func() {
			select {
			case r := <-ch:
				if r.c != nil {
					r.c.Close()
				}
			case <-time.After(10 * time.Second):
			}
		}()
	}
}
