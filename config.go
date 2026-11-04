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

type Config struct {
	Server     string `yaml:"server"`
	ServerPort int    `yaml:"server_port"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	KeyPath    string `yaml:"key_path,omitempty"`

	TunnelType        string `yaml:"tunnel_type"`
	ProxyHost         string `yaml:"proxy_host"`
	ProxyPort         int    `yaml:"proxy_port"`
	UseDefaultPayload bool   `yaml:"use_default_payload"`
	CustomPayload     string `yaml:"custom_payload"`

	Pinger  int  `yaml:"pinger"`
	NoDelay bool `yaml:"no_delay"`

	SOCKS5 struct {
		Bind string `yaml:"bind"`
		Port int    `yaml:"port"`
	} `yaml:"socks5"`

	DNS struct {
		Enabled  bool   `yaml:"enabled"`
		Bind     string `yaml:"bind"`
		Upstream string `yaml:"upstream"`
	} `yaml:"dns"`

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
		Enabled      bool          `yaml:"enabled"`
		InitialDelay time.Duration `yaml:"initial_delay"`
		MaxDelay     time.Duration `yaml:"max_delay"`
	} `yaml:"reconnect"`

	HealthCheck struct {
		Enabled          bool          `yaml:"enabled"`
		Interval         time.Duration `yaml:"interval"`
		Timeout          time.Duration `yaml:"timeout"`
		Target           string        `yaml:"target"`
		FailureThreshold int           `yaml:"failure_threshold"`
	} `yaml:"health_check"`

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

func DefaultConfig() *Config {
	c := &Config{
		ServerPort: 22,
		TunnelType: "direct",
		Pinger:     3,
		NoDelay:    true,
	}
	c.SOCKS5.Bind = "127.0.0.1"
	c.SOCKS5.Port = 7777
	c.DNS.Enabled = true
	c.DNS.Bind = "0.0.0.0:1053"
	c.DNS.Upstream = "1.1.1.1:53"
	c.Redirect.Bind = "127.0.0.1:7892"
	c.API.Bind = "0.0.0.0:9090"
	c.Reconnect.Enabled = true
	c.Reconnect.InitialDelay = 2 * time.Second
	c.Reconnect.MaxDelay = 60 * time.Second
	c.HealthCheck.Enabled = true
	c.HealthCheck.Interval = 30 * time.Second
	c.HealthCheck.Timeout = 10 * time.Second
	c.HealthCheck.Target = "1.1.1.1:53"
	c.HealthCheck.FailureThreshold = 3
	c.Log.Level = "info"
	c.Log.File = "/var/log/sockshttp.log"
	c.Log.MaxSizeMB = 10
	c.Log.MaxBackups = 3
	c.StateFile = "/etc/qtun/run/sockshttp-state.json"
	c.PIDFile = "/var/run/sockshttp.pid"

	c.ProxyGroups = []ProxyGroupConfig{
		{Name: "DEFAULT", Type: "select", Proxies: []string{ProxyTunnel, ProxyDirect, ProxyReject}, Default: ProxyTunnel},
		{Name: "HP-ISTRI", Type: "select", Proxies: []string{ProxyTunnel, ProxyDirect, ProxyReject}, Default: ProxyTunnel},
		{Name: "LAPTOP-KERJA", Type: "select", Proxies: []string{ProxyTunnel, ProxyDirect, ProxyReject}, Default: ProxyTunnel},
		{Name: "TV-ANDROID", Type: "select", Proxies: []string{ProxyTunnel, ProxyDirect, ProxyReject}, Default: ProxyTunnel},
		{Name: "IOT-CAMERA", Type: "select", Proxies: []string{ProxyTunnel, ProxyDirect, ProxyReject}, Default: ProxyDirect},
	}
	c.Rules = []string{
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"SRC-IP-CIDR,192.168.1.100/32,HP-ISTRI",
		"SRC-IP-CIDR,192.168.1.101/32,LAPTOP-KERJA",
		"SRC-IP-CIDR,192.168.1.150/32,TV-ANDROID",
		"SRC-IP-CIDR,192.168.1.200/32,IOT-CAMERA",
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
		return nil, errors.New("bad format")
	}
	salt, _ := base64.StdEncoding.DecodeString(parts[0])
	iv, _ := base64.StdEncoding.DecodeString(parts[1])
	ct, _ := base64.StdEncoding.DecodeString(parts[2])
	key, err := pbkdf2.Key(sha256.New, password, salt, 1000, 16)
	if err != nil {
		return nil, err
	}
	block, _ := aes.NewCipher(key)
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
	return nil
}