package main

import (
	"bufio"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Blacklist — filter domain/IP, di-load sekali saat startup (reload via SIGHUP opsional).
type Blacklist struct {
	mu        sync.RWMutex
	domains   map[string]struct{}
	wildcards []*regexp.Regexp
	cidrs     []*net.IPNet
	logger    *Logger
}

func NewBlacklist(logger *Logger) *Blacklist {
	return &Blacklist{
		domains: make(map[string]struct{}),
		logger:  logger,
	}
}

func (b *Blacklist) Load(cfg *BlacklistConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.domains = make(map[string]struct{})
	b.wildcards = nil
	b.cidrs = nil

	for _, f := range cfg.Files {
		if err := b.loadFile(f); err != nil {
			b.logger.Warn("[blacklist] gagal load %s: %v", f, err)
		}
	}
	for _, e := range cfg.Inline {
		b.addEntry(e)
	}
	b.logger.Info("[blacklist] dimuat: %d domain, %d wildcard, %d CIDR",
		len(b.domains), len(b.wildcards), len(b.cidrs))
	return nil
}

func (b *Blacklist) loadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		// Format hosts: "0.0.0.0 ads.com" / "127.0.0.1 tracker.io"
		fields := strings.Fields(line)
		if len(fields) >= 2 && (fields[0] == "0.0.0.0" || fields[0] == "127.0.0.1") {
			b.addEntry(fields[1])
			continue
		}
		// Format adblock-abp / plain domain: ambil token pertama
		b.addEntry(strings.TrimRight(fields[0], "^"))
	}
	return sc.Err()
}

func (b *Blacklist) addEntry(e string) {
	e = strings.TrimSpace(strings.ToLower(e))
	if e == "" {
		return
	}
	if strings.HasPrefix(e, "*.") {
		base := strings.TrimPrefix(e, "*.")
		pat := `^(.+\.)?` + regexp.QuoteMeta(base) + `$`
		if re, err := regexp.Compile(pat); err == nil {
			b.wildcards = append(b.wildcards, re)
		}
		return
	}
	if _, ipnet, err := net.ParseCIDR(e); err == nil {
		b.cidrs = append(b.cidrs, ipnet)
		return
	}
	b.domains[strings.TrimSuffix(e, ".")] = struct{}{}
}

// MatchDomain — cocokkan exact, suffix (a.b.com block b.com juga), dan wildcard.
func (b *Blacklist) MatchDomain(domain string) bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	if _, ok := b.domains[d]; ok {
		return true
	}
	labels := strings.Split(d, ".")
	for i := 1; i < len(labels); i++ {
		if _, ok := b.domains[strings.Join(labels[i:], ".")]; ok {
			return true
		}
	}
	for _, re := range b.wildcards {
		if re.MatchString(d) {
			return true
		}
	}
	return false
}

func (b *Blacklist) MatchIP(ip net.IP) bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, n := range b.cidrs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
