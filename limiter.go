package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type LimiterRule struct {
	IP       string `json:"ip"`
	UpKbps   int    `json:"up_kbps"`
	DownKbps int    `json:"down_kbps"`
	Enabled  bool   `json:"enabled"`
}

type limiterSnapshot struct {
	Version int           `json:"version"`
	Rules   []LimiterRule `json:"rules"`
}

const limiterChunkSize = 16 * 1024

type rateLimiter struct {
	mu     sync.Mutex
	rate   float64
	tokens float64
	burst  float64
	last   time.Time
}

func newRateLimiter(bytesPerSec float64) *rateLimiter {
	burst := bytesPerSec * 0.2
	if burst < float64(limiterChunkSize) { burst = float64(limiterChunkSize) }
	if burst > 512*1024 { burst = 512 * 1024 }
	return &rateLimiter{rate: bytesPerSec, tokens: burst, burst: burst, last: time.Now()}
}

func (r *rateLimiter) Wait(n int) {
	if r.rate <= 0 || n <= 0 { return }
	if n > limiterChunkSize { n = limiterChunkSize }
	for {
		r.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(r.last).Seconds()
		if elapsed > 0 {
			r.tokens += elapsed * r.rate
			if r.tokens > r.burst { r.tokens = r.burst }
			r.last = now
		}
		if r.tokens >= float64(n) {
			r.tokens -= float64(n)
			r.mu.Unlock()
			return
		}
		need := float64(n) - r.tokens
		sleep := time.Duration(need / r.rate * float64(time.Second))
		if sleep < time.Millisecond { sleep = time.Millisecond }
		r.mu.Unlock()
		time.Sleep(sleep)
	}
}

type BandwidthLimiter struct {
	enabled bool
	file    string
	logger  *Logger
	mu      sync.RWMutex
	rules   map[string]*LimiterRule
	up      map[string]*rateLimiter
	down    map[string]*rateLimiter
}

func NewBandwidthLimiter(enabled bool, file string, logger *Logger) *BandwidthLimiter {
	bl := &BandwidthLimiter{
		enabled: enabled, file: file, logger: logger,
		rules: map[string]*LimiterRule{}, up: map[string]*rateLimiter{}, down: map[string]*rateLimiter{},
	}
	if file != "" { bl.load() }
	return bl
}

func (b *BandwidthLimiter) Enabled() bool { return b.enabled }

func (b *BandwidthLimiter) load() {
	data, err := os.ReadFile(b.file)
	if err != nil { return }
	var snap limiterSnapshot
	if err := json.Unmarshal(data, &snap); err != nil { return }
	for _, r := range snap.Rules {
		if net.ParseIP(r.IP) != nil {
			rr := r
			b.rules[r.IP] = &rr
		}
	}
}

func (b *BandwidthLimiter) save() error {
	if b.file == "" { return nil }
	b.mu.RLock()
	snap := limiterSnapshot{Version: 1, Rules: make([]LimiterRule, 0, len(b.rules))}
	for _, r := range b.rules { snap.Rules = append(snap.Rules, *r) }
	b.mu.RUnlock()
	sort.Slice(snap.Rules, func(i, j int) bool { return snap.Rules[i].IP < snap.Rules[j].IP })
	data, _ := json.MarshalIndent(snap, "", "  ")
	os.MkdirAll(filepath.Dir(b.file), 0755)
	return os.WriteFile(b.file, data, 0644)
}

func (b *BandwidthLimiter) List() []LimiterRule {
	b.mu.RLock(); defer b.mu.RUnlock()
	out := make([]LimiterRule, 0, len(b.rules))
	for _, r := range b.rules { out = append(out, *r) }
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out
}

func (b *BandwidthLimiter) Set(rule LimiterRule) error {
	if net.ParseIP(rule.IP) == nil { return fmt.Errorf("IP tidak valid") }
	b.mu.Lock()
	r := rule
	b.rules[rule.IP] = &r
	delete(b.up, rule.IP); delete(b.down, rule.IP)
	b.mu.Unlock()
	return b.save()
}

func (b *BandwidthLimiter) Delete(ip string) error {
	b.mu.Lock()
	delete(b.rules, ip); delete(b.up, ip); delete(b.down, ip)
	b.mu.Unlock()
	return b.save()
}

func (b *BandwidthLimiter) wait(ip string, n int, up bool) {
	if !b.enabled || ip == "" || n <= 0 { return }
	b.mu.RLock()
	rule := b.rules[ip]
	var l *rateLimiter
	if up { l = b.up[ip] } else { l = b.down[ip] }
	b.mu.RUnlock()

	if rule == nil || !rule.Enabled { return }
	kbps := rule.UpKbps
	if !up { kbps = rule.DownKbps }
	if kbps <= 0 { return }
	target := float64(kbps) * 1024 / 8

	if l != nil && l.rate == target { l.Wait(n); return }

	b.mu.Lock()
	rule = b.rules[ip]
	if rule == nil || !rule.Enabled { b.mu.Unlock(); return }
	kbps = rule.UpKbps
	if !up { kbps = rule.DownKbps }
	if kbps <= 0 { b.mu.Unlock(); return }
	target = float64(kbps) * 1024 / 8
	if up { l = b.up[ip] } else { l = b.down[ip] }
	if l == nil || l.rate != target {
		l = newRateLimiter(target)
		if up { b.up[ip] = l } else { b.down[ip] = l }
	}
	b.mu.Unlock()
	l.Wait(n)
}

func (b *BandwidthLimiter) WaitUp(ip string, n int)   { b.wait(ip, n, true) }
func (b *BandwidthLimiter) WaitDown(ip string, n int) { b.wait(ip, n, false) }

type limitedWriter struct {
	w       io.Writer
	limiter *BandwidthLimiter
	ip      string
	up      bool
}

func newLimitedWriter(w io.Writer, l *BandwidthLimiter, ip string, up bool) *limitedWriter {
	return &limitedWriter{w: w, limiter: l, ip: ip, up: up}
}

func (lw *limitedWriter) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		n := len(b)
		if n > limiterChunkSize { n = limiterChunkSize }
		if lw.up { lw.limiter.WaitUp(lw.ip, n) } else { lw.limiter.WaitDown(lw.ip, n) }
		wrote, err := lw.w.Write(b[:n])
		total += wrote
		b = b[wrote:]
		if err != nil { return total, err }
	}
	return total, nil
}

// connLimitedWriter membatasi bandwidth per koneksi (untuk Destination Limiter)
type connLimitedWriter struct {
	w       io.Writer
	limiter *rateLimiter
}

func (cw *connLimitedWriter) Write(b []byte) (int, error) {
	total := 0
	for len(b) > 0 {
		n := len(b)
		if n > limiterChunkSize { n = limiterChunkSize }
		cw.limiter.Wait(n)
		wrote, err := cw.w.Write(b[:n])
		total += wrote
		b = b[wrote:]
		if err != nil { return total, err }
	}
	return total, nil
}

// =========================================================
// Destination Limiter Cache (Per-Device + Per-Rule)
// =========================================================

type RateLimiterCache struct {
	mu      sync.Mutex
	buckets map[string]*rateLimiter
}

func NewRateLimiterCache() *RateLimiterCache {
	return &RateLimiterCache{
		buckets: make(map[string]*rateLimiter),
	}
}

func (c *RateLimiterCache) Get(key string, rate float64) *rateLimiter {
	c.mu.Lock()
	defer c.mu.Unlock()

	if l, ok := c.buckets[key]; ok {
		// Jika rate berubah (karena edit YAML), buat bucket baru
		if l.rate != rate {
			nl := newRateLimiter(rate)
			c.buckets[key] = nl
			return nl
		}
		return l
	}

	l := newRateLimiter(rate)
	c.buckets[key] = l
	return l
}