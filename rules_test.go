package main

import "testing"

func TestDomainSuffixBoundary(t *testing.T) {
	r, err := parseRule("DOMAIN-SUFFIX,google.com,DIRECT")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		domain string
		want   bool
	}{
		{"google.com", true},
		{"www.google.com", true},
		{"api.google.com", true},
		{"evilgoogle.com", false},
		{"notgoogle.com", false},
		{"google.com.evil.com", false},
	}
	for _, c := range cases {
		got := r.Match("", "", c.domain)
		if got != c.want {
			t.Errorf("DOMAIN-SUFFIX google.com vs %q = %v, want %v", c.domain, got, c.want)
		}
	}
}

func TestSrcIPCIDR(t *testing.T) {
	r, err := parseRule("SRC-IP-CIDR,192.168.1.100/32,HP-ISTRI")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Match("192.168.1.100", "", "") {
		t.Error("should match 192.168.1.100")
	}
	if r.Match("192.168.1.101", "", "") {
		t.Error("should not match 192.168.1.101")
	}
}

func TestMatchFallback(t *testing.T) {
	r, err := parseRule("MATCH,DEFAULT")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Match("1.2.3.4", "", "") {
		t.Error("MATCH should always return true")
	}
}

func TestIPCIDRNoResolve(t *testing.T) {
	r, err := parseRule("IP-CIDR,10.0.0.0/8,DIRECT,no-resolve")
	if err != nil {
		t.Fatal(err)
	}
	if r.Match("", "", "example.com") {
		t.Error("no-resolve should skip when domain present")
	}
	if !r.Match("", "10.1.2.3", "") {
		t.Error("should match dst IP in range")
	}
}