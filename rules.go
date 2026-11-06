package main

import (
	"fmt"
	"net"
	"strings"
)

type Rule struct {
	Type      string
	Value     string
	Target    string
	NoResolve bool
	cidr      *net.IPNet
	raw       string
}

func parseRule(s string) (*Rule, error) {
	parts := strings.Split(s, ",")
	if len(parts) < 2 {
		return nil, fmt.Errorf("bad rule: %s", s)
	}
	r := &Rule{Type: strings.ToUpper(strings.TrimSpace(parts[0])), raw: s}
	switch r.Type {
	case RuleMatch:
		r.Target = strings.TrimSpace(parts[1])
		r.Value = ""
	case RuleSrcIPCIDR, RuleIPCIDR:
		if len(parts) < 3 {
			return nil, fmt.Errorf("%s needs 3 parts", r.Type)
		}
		r.Value = strings.TrimSpace(parts[1])
		r.Target = strings.TrimSpace(parts[2])
		_, cidr, err := net.ParseCIDR(r.Value)
		if err != nil {
			return nil, fmt.Errorf("bad CIDR %s: %w", r.Value, err)
		}
		r.cidr = cidr
		if len(parts) >= 4 && strings.EqualFold(parts[3], "no-resolve") {
			r.NoResolve = true
		}
	case RuleDomain, RuleDomainSuffix, RuleDomainKeyword:
		if len(parts) < 3 {
			return nil, fmt.Errorf("%s needs 3 parts", r.Type)
		}
		r.Value = strings.ToLower(strings.TrimSpace(parts[1]))
		r.Target = strings.TrimSpace(parts[2])
	default:
		return nil, fmt.Errorf("unknown rule type: %s", r.Type)
	}
	return r, nil
}

func (r *Rule) Match(srcIP, dstIP, domain string) bool {
	switch r.Type {
	case RuleMatch:
		return true
	case RuleSrcIPCIDR:
		ip := net.ParseIP(srcIP)
		return ip != nil && r.cidr.Contains(ip)
	case RuleIPCIDR:
		if r.NoResolve && domain != "" {
			return false
		}
		if dstIP == "" {
			return false
		}
		ip := net.ParseIP(dstIP)
		return ip != nil && r.cidr.Contains(ip)
	case RuleDomain:
		return strings.EqualFold(domain, r.Value)
	case RuleDomainSuffix:
		d := strings.ToLower(domain)
		v := strings.ToLower(r.Value)
		v = strings.TrimPrefix(v, ".")
		return d == v || strings.HasSuffix(d, "."+v)
	case RuleDomainKeyword:
		return strings.Contains(strings.ToLower(domain), r.Value)
	}
	return false
}

// Explain menjelaskan apakah rule ini match dan kenapa.
func (r *Rule) Explain(srcIP, dstIP, domain string) string {
	if !r.Match(srcIP, dstIP, domain) {
		return ""
	}
	return r.raw
}