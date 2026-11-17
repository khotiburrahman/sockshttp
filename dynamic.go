package main

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type DestRule struct {
	Name     string   `yaml:"name"`
	Targets  []string `yaml:"targets"`
	UpKbps   int      `yaml:"up_kbps"`
	DownKbps int      `yaml:"down_kbps"`
}

type destTargetParsed struct {
	target   string
	cidr     *net.IPNet
	isDomain bool
}

type destRuleParsed struct {
	name           string
	upBucketRate   float64
	downBucketRate float64
	targets        []destTargetParsed
}

type DynamicRules struct {
	mu      sync.RWMutex
	file    string
	logger  *Logger
	lastMod time.Time

	// Proxy Rotate
	rotateEnabled  bool
	rotatePorts    []int
	rotateMaxRetry int
	rotateWindow   time.Duration

	// Dest Limit
	destEnabled bool
	destRules   []destRuleParsed
}

func NewDynamicRules(file string, logger *Logger) *DynamicRules {
	return &DynamicRules{
		file:   file,
		logger: logger,
	}
}

type dynamicSnapshot struct {
	ProxyRotate struct {
		Enabled    bool     `yaml:"enabled"`
		Ports      []int    `yaml:"ports"`
		MaxRetries int      `yaml:"max_retries"`
		Window     Duration `yaml:"window"`
	} `yaml:"proxy_rotate"`

	DestLimiter struct {
		Enabled bool       `yaml:"enabled"`
		Rules   []DestRule `yaml:"rules"`
	} `yaml:"dest_limiter"`
}

func (d *DynamicRules) Reload() {
	if d.file == "" {
		return
	}
	data, err := os.ReadFile(d.file)
	if err != nil {
		d.logger.Warn("DynamicRules read: %v", err)
		return
	}
	var snap dynamicSnapshot
	if err := yaml.Unmarshal(data, &snap); err != nil {
		d.logger.Warn("DynamicRules parse error (menggunakan aturan lama): %v", err)
		return
	}

	var newRules []destRuleParsed
	for _, r := range snap.DestLimiter.Rules {
		parsed := destRuleParsed{
			name:           r.Name,
			upBucketRate:   float64(r.UpKbps) * 1024 / 8,
			downBucketRate: float64(r.DownKbps) * 1024 / 8,
		}
		for _, t := range r.Targets {
			target := strings.ToLower(strings.TrimSpace(t))
			if target == "" {
				continue
			}
			dt := destTargetParsed{target: target}
			if _, cidr, err := net.ParseCIDR(target); err == nil {
				dt.cidr = cidr
			} else if ip := net.ParseIP(target); ip != nil {
				dt.cidr = &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}
				if ip.To4() == nil {
					dt.cidr.Mask = net.CIDRMask(128, 128)
				}
			} else {
				dt.isDomain = true
			}
			parsed.targets = append(parsed.targets, dt)
		}
		newRules = append(newRules, parsed)
	}

	d.mu.Lock()
	d.rotateEnabled = snap.ProxyRotate.Enabled
	d.rotatePorts = snap.ProxyRotate.Ports
	d.rotateMaxRetry = snap.ProxyRotate.MaxRetries
	d.rotateWindow = snap.ProxyRotate.Window.D()
	if d.rotateWindow <= 0 {
		d.rotateWindow = 2 * time.Minute
	}
	if d.rotateMaxRetry <= 0 {
		d.rotateMaxRetry = 4
	}

	d.destEnabled = snap.DestLimiter.Enabled
	d.destRules = newRules
	d.mu.Unlock()

	d.logger.Info("DynamicRules: memuat ulang (rotate: %v, dest: %v, %d rules)",
		d.rotateEnabled, d.destEnabled, len(newRules))
}

func (d *DynamicRules) StartWatcher(ctx context.Context) {
	if d.file == "" {
		return
	}
	d.Reload()
	if info, err := os.Stat(d.file); err == nil {
		d.lastMod = info.ModTime()
	}

	go func() {
		tk := time.NewTicker(2 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
				info, err := os.Stat(d.file)
				if err != nil {
					continue
				}
				if !info.ModTime().Equal(d.lastMod) {
					d.lastMod = info.ModTime()
					d.Reload()
				}
			}
		}
	}()
}

// Ubah signature function menjadi seperti ini
func (d *DynamicRules) GetDestLimits(ip, domain string) (ruleID string, upRate, downRate float64) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if !d.destEnabled {
		return "", 0, 0
	}

	dl := strings.ToLower(domain)
	var parsedIP net.IP
	if ip != "" {
		parsedIP = net.ParseIP(ip)
	}

	for _, r := range d.destRules {
		for _, t := range r.targets {
			match := false
			if t.isDomain {
				if dl != "" && strings.Contains(dl, t.target) {
					match = true
				}
			} else if t.cidr != nil && parsedIP != nil {
				if t.cidr.Contains(parsedIP) {
					match = true
				}
			}
			if match {
				// Kembalikan nama aturan (ruleID) beserta limitnya
				return r.name, r.upBucketRate, r.downBucketRate
			}
		}
	}
	return "", 0, 0
}

func (d *DynamicRules) GetProxyRotate() (bool, []int, int, time.Duration) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.rotateEnabled, d.rotatePorts, d.rotateMaxRetry, d.rotateWindow
}