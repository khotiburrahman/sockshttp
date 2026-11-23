package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ProxyTunnel = "TUNNEL"
	ProxyDirect = "DIRECT"
	ProxyReject = "REJECT"

	RuleSrcIPCIDR     = "SRC-IP-CIDR"
	RuleIPCIDR        = "IP-CIDR"
	RuleDomain        = "DOMAIN"
	RuleDomainSuffix  = "DOMAIN-SUFFIX"
	RuleDomainKeyword = "DOMAIN-KEYWORD"
	RuleMatch         = "MATCH"

	LegacyDESKey   = "cinbdf665$4"
	SecurePrefsKey = "fubvx788b46v"
	SOOriginalDst  = 80
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var n int64
	if err := value.Decode(&n); err == nil {
		if n > 0 && n < 1000 {
			fmt.Fprintf(os.Stderr, "WARN: duration %d terlalu kecil, kemungkinan salah (gunakan '2s' bukan '2')\n", n)
		}
		*d = Duration(n)
		return nil
	}
	var s string
	if err := value.Decode(&s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	return fmt.Errorf("cannot parse duration: %q", value.Value)
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

func (d Duration) D() time.Duration { return time.Duration(d) }

// SSHAccount — satu kredensial SSH untuk multi-account rotation.
type SSHAccount struct {
	Name       string `yaml:"name,omitempty"` // label untuk UI (opsional, fallback "Account-N")
	Server     string `yaml:"server"`
	ServerPort int    `yaml:"server_port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	KeyPath    string `yaml:"key_path,omitempty"`
}

type Config struct {
	Server     string `yaml:"server"`
	ServerPort int    `yaml:"server_port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	KeyPath    string `yaml:"key_path,omitempty"`

	// Multi-account SSH
	SSHAccounts        []SSHAccount `yaml:"ssh_accounts"`
	MaxRetryPerAccount int          `yaml:"max_retry_per_account"`

	TunnelType        string   `yaml:"tunnel_type"`
	ProxyHost         string   `yaml:"proxy_host"`
	ProxyHostFallback []string `yaml:"proxy_host_fallback"`
	ProxyPort         int      `yaml:"proxy_port"`
	UseDefaultPayload bool     `yaml:"use_default_payload"`
	CustomPayload     string   `yaml:"custom_payload"`

	DynamicRulesFile string `yaml:"dynamic_rules_file"`

	Pinger        int      `yaml:"pinger"`
	PingerMax     int      `yaml:"pinger_max"`
	PingerTimeout Duration `yaml:"pinger_timeout"`
	NoDelay       bool     `yaml:"no_delay"`

	StartupTimeout Duration `yaml:"startup_timeout"`
	ConnRetention  Duration `yaml:"conn_retention"`

	Blacklist BlacklistConfig `yaml:"blacklist"`
	Dial      DialConfig      `yaml:"dial"`
	Prefetch  PrefetchConfig  `yaml:"prefetch"`

	SOCKS5 struct {
		Bind string `yaml:"bind"`
		Port int    `yaml:"port"`
	} `yaml:"socks5"`

	DNS struct {
		Enabled  bool         `yaml:"enabled"`
		Mode     string       `yaml:"mode"`
		Bind     string       `yaml:"bind"`
		Upstream string       `yaml:"upstream"`
		DoH      DNSDoHConfig `yaml:"doh"`
		DoQ      DNSDoQConfig `yaml:"doq"`
	} `yaml:"dns"`

	Adblock struct {
		Enable bool   `yaml:"enable"`
		Path   string `yaml:"path"`
	} `yaml:"adblock"`

	Redirect struct {
		Bind string `yaml:"bind"`
	} `yaml:"redirect"`

	API struct {
		Bind   string `yaml:"bind"`
		Secret string `yaml:"secret"`
	} `yaml:"api"`

	ProxyGroups []ProxyGroupConfig `yaml:"proxy-groups"`
	Rules       []string           `yaml:"rules"`

	Reconnect struct {
		Enabled      bool     `yaml:"enabled"`
		InitialDelay Duration `yaml:"initial_delay"`
		MaxDelay     Duration `yaml:"max_delay"`
	} `yaml:"reconnect"`

	HealthCheck struct {
		Enabled          bool     `yaml:"enabled"`
		Interval         Duration `yaml:"interval"`
		Timeout          Duration `yaml:"timeout"`
		Target           string   `yaml:"target"`
		FailureThreshold int      `yaml:"failure_threshold"`
	} `yaml:"health_check"`

	FailureHook FailureHookConfig `yaml:"failure_hook"`

	Limiter struct {
		Enabled bool   `yaml:"enabled"`
		File    string `yaml:"file"`
	} `yaml:"limiter"`

	Log struct {
		Level      string `yaml:"level"`
		File       string `yaml:"file"`
		MaxSizeMB  int    `yaml:"max_size_mb"`
		MaxBackups int    `yaml:"max_backups"`
	} `yaml:"log"`

	StateFile string `yaml:"state_file"`
	PIDFile   string `yaml:"pid_file"`

	ValidityDate int64 `yaml:"-"`
}

type ProxyGroupConfig struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Proxies []string `yaml:"proxies"`
	Default string   `yaml:"default"`
}

type FailureHookConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Script     string   `yaml:"script"`
	ScriptArgs []string `yaml:"script_args"`
	Shell      string   `yaml:"shell"`

	CheckInterval   Duration `yaml:"check_interval"`
	MaxReconnects   int      `yaml:"max_reconnects"`
	ReconnectWindow Duration `yaml:"reconnect_window"`
	MaxDowntime     Duration `yaml:"max_downtime"`

	ScriptTimeout Duration `yaml:"script_timeout"`
	Cooldown      Duration `yaml:"cooldown"`
	OncePerOutage bool     `yaml:"once_per_outage"`
}

// ===== TAMBAHAN: struct baru =====

type BlacklistConfig struct {
	Enabled bool     `yaml:"enabled"`
	Mode    string   `yaml:"mode"` // "reject" (NXDOMAIN) | "sinkhole" (0.0.0.0)
	Files   []string `yaml:"files"`
	Inline  []string `yaml:"inline"`
}

type DNSDoHConfig struct {
	URL    string `yaml:"url"`
	Method string `yaml:"method"` // GET | POST (default POST)
}

type DNSDoQConfig struct {
	Server      string `yaml:"server"` // host:port
	TLSInsecure bool   `yaml:"tls_insecure"`
}

type DialConfig struct {
	SocketBuffer   int      `yaml:"socket_buffer"` // bytes; 0 = disable
	KeepAlive      Duration `yaml:"keepalive"`
	ConnectTimeout Duration `yaml:"connect_timeout"`
}

type PrefetchConfig struct {
	Enabled                 bool     `yaml:"enabled"`
	Interval                Duration `yaml:"interval"`
	DomainsFromDynamicRules bool     `yaml:"domains_from_dynamic_rules"`
	ExtraDomains            []string `yaml:"extra_domains"`
	WarmDial                bool     `yaml:"warm_dial"` // TCP 443 warmup
	WarmDialTimeout         Duration `yaml:"warm_dial_timeout"`
}

func DefaultConfig() *Config {
	c := &Config{
		ServerPort:         22,
		TunnelType:         "direct",
		Pinger:             30,
		PingerMax:          45,
		PingerTimeout:      Duration(15 * time.Second),
		NoDelay:            true,
		StartupTimeout:     Duration(60 * time.Second),
		ConnRetention:      Duration(60 * time.Second),
		MaxRetryPerAccount: 3,
	}
	c.Blacklist.Mode = "reject"
	c.Dial.SocketBuffer = 4 * 1024 * 1024 // 4 MB
	c.Dial.KeepAlive = Duration(30 * time.Second)
	c.Dial.ConnectTimeout = Duration(10 * time.Second)
	c.Prefetch.Enabled = false
	c.Prefetch.Interval = Duration(30 * time.Minute)
	c.Prefetch.DomainsFromDynamicRules = true
	c.Prefetch.WarmDial = false
	c.Prefetch.WarmDialTimeout = Duration(5 * time.Second)
	c.SOCKS5.Bind = "127.0.0.1"
	c.SOCKS5.Port = 7777
	c.DNS.Enabled = true
	c.DNS.Mode = "forward"
	c.DNS.Bind = "0.0.0.0:1053"
	c.DNS.Upstream = "1.1.1.1:53"
	c.Adblock.Enable = false
	c.Adblock.Path = "/root/sockshttp-linux-arm64/geosite.dat"
	c.Redirect.Bind = "127.0.0.1:7892"
	c.API.Bind = "0.0.0.0:9090"
	c.Reconnect.Enabled = true
	c.Reconnect.InitialDelay = Duration(10 * time.Second)
	c.Reconnect.MaxDelay = Duration(60 * time.Second)
	c.HealthCheck.Enabled = true
	c.HealthCheck.Interval = Duration(30 * time.Second)
	c.HealthCheck.Timeout = Duration(10 * time.Second)
	c.HealthCheck.Target = "8.8.8.8:443"
	c.HealthCheck.FailureThreshold = 3

	c.FailureHook.Enabled = false
	c.FailureHook.Shell = "/bin/sh"
	c.FailureHook.CheckInterval = Duration(5 * time.Second)
	c.FailureHook.MaxReconnects = 3
	c.FailureHook.ReconnectWindow = Duration(1 * time.Minute)
	c.FailureHook.MaxDowntime = Duration(3 * time.Minute)
	c.FailureHook.ScriptTimeout = Duration(120 * time.Second)
	c.FailureHook.Cooldown = Duration(10 * time.Minute)
	c.FailureHook.OncePerOutage = true

	c.Limiter.Enabled = false
	c.Limiter.File = "/tmp/sockshttp-limiter.json"
	c.Log.Level = "info"
	c.Log.File = "/var/log/sockshttp.log"
	c.Log.MaxSizeMB = 10
	c.Log.MaxBackups = 3
	c.StateFile = "/etc/qtun/run/sockshttp-state.json"
	c.PIDFile = "/var/run/sockshttp.pid"
	c.DynamicRulesFile = "dynamic_rules.yaml"

	c.ProxyGroups = []ProxyGroupConfig{
		{Name: "DEFAULT", Type: "select", Proxies: []string{ProxyTunnel, ProxyDirect, ProxyReject}, Default: ProxyTunnel},
	}
	c.Rules = []string{
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"MATCH,DEFAULT",
	}
	return c
}

func LoadYAMLConfig(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	switch strings.ToLower(cfg.DNS.Mode) {
	case "", "forward":
		cfg.DNS.Mode = "forward"
	case "doh":
		if cfg.DNS.DoH.URL == "" {
			fmt.Fprintf(os.Stderr, "WARN: dns.mode=doh tanpa dns.doh.url → fallback forward\n")
			cfg.DNS.Mode = "forward"
		}
		if cfg.DNS.DoH.Method == "" {
			cfg.DNS.DoH.Method = "POST"
		}
	case "doq":
		if cfg.DNS.DoQ.Server == "" {
			fmt.Fprintf(os.Stderr, "WARN: dns.mode=doq tanpa dns.doq.server → fallback forward\n")
			cfg.DNS.Mode = "forward"
		}
	case "fakeip":
		fmt.Fprintf(os.Stderr, "WARN: dns.mode='fakeip' tidak lagi didukung, dipaksa 'forward'\n")
		cfg.DNS.Mode = "forward"
	default:
		fmt.Fprintf(os.Stderr, "WARN: dns.mode=%q tidak dikenal, dipaksa 'forward'\n", cfg.DNS.Mode)
		cfg.DNS.Mode = "forward"
	}

	if len(cfg.Rules) > 0 {
		last := strings.ToUpper(strings.TrimSpace(strings.SplitN(cfg.Rules[len(cfg.Rules)-1], ",", 2)[0]))
		if last != RuleMatch {
			fmt.Fprintf(os.Stderr, "WARN: rules tidak diakhiri MATCH — semua traffic tak-match akan REJECT\n")
		}
	}
	return cfg, nil
}

type xmlPropEntry struct {
	Key   string `xml:"key,attr"`
	Value string `xml:",chardata"`
}
type xmlProperties struct {
	XMLName xml.Name       `xml:"properties"`
	Entries []xmlPropEntry `xml:"entry"`
}

func decryptLegacyDES(key string, in []byte) ([]byte, error) {
	if len(in) == 0 || len(in)%des.BlockSize != 0 {
		return nil, errors.New("invalid DES data")
	}
	block, err := des.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(in))
	for i := 0; i < len(in); i += block.BlockSize() {
		block.Decrypt(out[i:], in[i:])
	}
	pad := int(out[len(out)-1])
	if pad < 1 || pad > des.BlockSize {
		return nil, errors.New("bad padding")
	}
	return out[:len(out)-pad], nil
}

func decryptSecurePrefs(password, b64 string) ([]byte, error) {
	parts := strings.Split(b64, ".")
	if len(parts) != 3 {
		return nil, errors.New("format .sks tidak valid (butuh 3 bagian dipisah titik)")
	}
	salt, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode salt: %w", err)
	}
	iv, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode iv: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, 1000, 16)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, iv, ct, nil)
}

func LoadSKSConfig(r io.Reader, cfg *Config) error {
	raw, _ := io.ReadAll(r)
	plain, err := decryptSecurePrefs(SecurePrefsKey, string(bytes.TrimSpace(raw)))
	if err != nil {
		plain, err = decryptLegacyDES(LegacyDESKey, raw)
		if err != nil {
			return fmt.Errorf("dekripsi gagal: %w", err)
		}
	}
	var xp xmlProperties
	if err := xml.Unmarshal(plain, &xp); err != nil {
		return err
	}
	m := map[string]string{}
	for _, e := range xp.Entries {
		m[e.Key] = e.Value
	}
	getStr := func(k, d string) string {
		if v, ok := m[k]; ok && v != "" {
			return v
		}
		return d
	}
	getInt := func(k string, d int) int {
		if n, e := strconv.Atoi(m[k]); e == nil {
			return n
		}
		return d
	}
	getBool := func(k string, d bool) bool {
		v, ok := m[k]
		if !ok {
			return d
		}
		return strings.TrimSpace(v) == "1" || v == "true"
	}

	cfg.Server = getStr("sshServer", cfg.Server)
	cfg.ServerPort = getInt("sshPort", cfg.ServerPort)
	cfg.Username = getStr("sshUser", cfg.Username)
	cfg.Password = getStr("sshPass", cfg.Password)
	if getInt("tunnelType", 1) == 2 {
		cfg.TunnelType = "proxy"
		cfg.ProxyHost = getStr("proxyRemoto", "")
		cfg.ProxyPort = getInt("proxyRemotoPorta", 0)
	}
	cfg.UseDefaultPayload = getBool("usarDefaultPayload", true)
	if v := getStr("proxyPayload", ""); v != "" {
		cfg.CustomPayload = v
	}
	cfg.NoDelay = getBool("disableDelaySSH", cfg.NoDelay)
	if v, ok := m["file.validade"]; ok {
		if n, e := strconv.ParseInt(strings.TrimSpace(v), 10, 64); e == nil {
			cfg.ValidityDate = n
		}
	}
	cfg.PingerMax = cfg.Pinger

	return nil
}
