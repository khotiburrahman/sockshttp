package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type ProxyGroup struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Proxies []string `json:"all"`
	Now     string   `json:"now"`
}

type ProxyManager struct {
	mu          sync.RWMutex
	groups      map[string]*ProxyGroup
	order       []string
	persistFile string
}

func NewProxyManager(cfgs []ProxyGroupConfig, persistFile string) *ProxyManager {
	pm := &ProxyManager{
		groups:      map[string]*ProxyGroup{},
		persistFile: persistFile,
	}
	for _, c := range cfgs {
		g := &ProxyGroup{
			Name:    c.Name,
			Type:    "select",
			Proxies: c.Proxies,
			Now:     c.Default,
		}
		if g.Now == "" && len(c.Proxies) > 0 {
			g.Now = c.Proxies[0]
		}
		pm.groups[c.Name] = g
		pm.order = append(pm.order, c.Name)
	}
	pm.loadState()
	return pm
}

func (pm *ProxyManager) loadState() {
	if pm.persistFile == "" {
		return
	}
	data, err := os.ReadFile(pm.persistFile)
	if err != nil {
		return
	}
	var state map[string]string
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}
	for name, sel := range state {
		if g, ok := pm.groups[name]; ok {
			for _, p := range g.Proxies {
				if p == sel {
					g.Now = sel
					break
				}
			}
		}
	}
}

func (pm *ProxyManager) saveState() {
	if pm.persistFile == "" {
		return
	}
	pm.mu.RLock()
	state := map[string]string{}
	for name, g := range pm.groups {
		state[name] = g.Now
	}
	pm.mu.RUnlock()
	data, _ := json.MarshalIndent(state, "", "  ")
	os.MkdirAll(filepath.Dir(pm.persistFile), 0755)
	os.WriteFile(pm.persistFile, data, 0644)
}

func (pm *ProxyManager) Get(name string) (*ProxyGroup, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	g, ok := pm.groups[name]
	return g, ok
}

func (pm *ProxyManager) List() []*ProxyGroup {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	out := make([]*ProxyGroup, 0, len(pm.order))
	for _, name := range pm.order {
		out = append(out, pm.groups[name])
	}
	return out
}

func (pm *ProxyManager) Set(groupName, proxyName string) error {
	pm.mu.Lock()
	g, ok := pm.groups[groupName]
	if !ok {
		pm.mu.Unlock()
		return fmt.Errorf("unknown group: %s", groupName)
	}
	valid := false
	for _, p := range g.Proxies {
		if p == proxyName {
			valid = true
			break
		}
	}
	if !valid {
		pm.mu.Unlock()
		return fmt.Errorf("%s not in group %s", proxyName, groupName)
	}
	g.Now = proxyName
	pm.mu.Unlock()
	pm.saveState()
	return nil
}

func (pm *ProxyManager) Resolve(target string) (string, string) {
	chain := []string{target}
	for i := 0; i < 10; i++ {
		if target == ProxyTunnel || target == ProxyDirect || target == ProxyReject {
			return target, strings.Join(chain, " → ")
		}
		g, ok := pm.Get(target)
		if !ok {
			return ProxyReject, strings.Join(chain, " → ") + " (unknown)"
		}
		target = g.Now
		chain = append(chain, target)
	}
	return ProxyReject, strings.Join(chain, " → ") + " (loop)"
}