package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
	"google.golang.org/protobuf/proto"
)

type Adblocker struct {
	enable  bool
	domains map[string]bool
}

func NewAdblocker(cfg *Config) *Adblocker {
	ab := &Adblocker{
		enable:  cfg.Adblock.Enable,
		domains: make(map[string]bool),
	}

	if !ab.enable {
		return ab
	}

	data, err := os.ReadFile(cfg.Adblock.Path)
	if err != nil {
		fmt.Printf("[adblock] Gagal membaca file %s: %v\n", cfg.Adblock.Path, err)
		return ab
	}

	var geoSite routercommon.GeoSiteList
	if err := proto.Unmarshal(data, &geoSite); err != nil {
		fmt.Printf("[adblock] Gagal decode protobuf: %v\n", err)
		return ab
	}

	count := 0
	for _, entry := range geoSite.Entry {
		// Sesuai script Python Anda, tag-nya adalah "ads"
		if entry.CountryCode == "ads" {
			for _, domain := range entry.Domain {
				// Type 2 = RootDomain (mencakup *.example.com)
				if domain.Type == routercommon.Domain_RootDomain {
					ab.domains[strings.ToLower(domain.Value)] = true
					count++
				}
			}
		}
	}
	fmt.Printf("[adblock] Berhasil memuat %d domain iklan.\n", count)
	return ab
}

func (ab *Adblocker) IsBlocked(domain string) bool {
	if !ab.enable {
		return false
	}

	domain = strings.ToLower(strings.TrimSuffix(domain, "."))

	// Cek domain persis (misal: ads.google.com)
	if ab.domains[domain] {
		return true
	}

	// Cek parent domain (misal: google.com dari ads.google.com)
	parts := strings.Split(domain, ".")
	for i := 1; i < len(parts)-1; i++ {
		parent := strings.Join(parts[i:], ".")
		if ab.domains[parent] {
			return true
		}
	}

	return false
}