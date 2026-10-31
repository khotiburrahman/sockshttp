//go:build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	mrand "math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

// =====================================================================
// SECTION 1: TYPES & CONFIG
// =====================================================================

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

	TunnelType        string `yaml:"tunnel_type"` // direct | proxy
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

// =====================================================================
// SECTION 2: LOGGER
// =====================================================================

type LogLevel int

const (
	LvlErr  LogLevel = 0
	LvlWarn LogLevel = 1
	LvlInfo LogLevel = 2
	LvlDbg  LogLevel = 3
)

func parseLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return LvlDbg
	case "warn":
		return LvlWarn
	case "error":
		return LvlErr
	}
	return LvlInfo
}

type Logger struct {
	mu         sync.Mutex
	level      LogLevel
	outputs    []io.Writer
	filePath   string
	file       *os.File
	fileSize   int64
	maxSize    int64
	maxBackups int
	listeners  []chan string
}

func NewLogger(level LogLevel) *Logger {
	return &Logger{level: level, outputs: []io.Writer{os.Stdout}}
}

func (l *Logger) SetFile(path string, maxMB, backups int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	l.file = f
	l.filePath = path
	l.fileSize = st.Size()
	l.maxSize = int64(maxMB) * 1024 * 1024
	l.maxBackups = backups
	return nil
}

func (l *Logger) rotateLocked() {
	if l.file == nil {
		return
	}
	l.file.Close()
	for i := l.maxBackups - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", l.filePath, i), fmt.Sprintf("%s.%d", l.filePath, i+1))
	}
	os.Rename(l.filePath, l.filePath+".1")
	l.file, _ = os.OpenFile(l.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	l.fileSize = 0
}

func (l *Logger) Subscribe() chan string {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch := make(chan string, 200)
	l.listeners = append(l.listeners, ch)
	return ch
}

func (l *Logger) Unsubscribe(ch chan string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, c := range l.listeners {
		if c == ch {
			l.listeners = append(l.listeners[:i], l.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

func (l *Logger) write(lvl LogLevel, format string, a ...any) {
	if lvl > l.level {
		return
	}
	tag := "INFO"
	switch lvl {
	case LvlErr:
		tag = "ERR "
	case LvlWarn:
		tag = "WARN"
	case LvlDbg:
		tag = "DBG "
	}
	line := fmt.Sprintf("[%s][%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), tag, fmt.Sprintf(format, a...))

	l.mu.Lock()
	defer l.mu.Unlock()
	for _, w := range l.outputs {
		io.WriteString(w, line)
	}
	if l.file != nil {
		l.file.WriteString(line)
		l.fileSize += int64(len(line))
		if l.maxSize > 0 && l.fileSize >= l.maxSize {
			l.rotateLocked()
		}
	}
	for _, ch := range l.listeners {
		select {
		case ch <- line:
		default:
		}
	}
}

func (l *Logger) Info(f string, a ...any)  { l.write(LvlInfo, f, a...) }
func (l *Logger) Warn(f string, a ...any)  { l.write(LvlWarn, f, a...) }
func (l *Logger) Error(f string, a ...any) { l.write(LvlErr, f, a...) }
func (l *Logger) Debug(f string, a ...any) { l.write(LvlDbg, f, a...) }

// =====================================================================
// SECTION 3: PAYLOAD TEMPLATE
// =====================================================================

var (
	reRotate = regexp.MustCompile(`\[rotate=(.*?)\]`)
	reRandom = regexp.MustCompile(`\[random=(.*?)\]`)
	rotateMu sync.Mutex
	rotates  = map[int]int{}
	lastUsed string
)

func formatPayload(host string, port int, payload string) string {
	hp := fmt.Sprintf("%s:%d", host, port)
	repl := map[string]string{
		"[method]": "CONNECT", "[host]": host, "[port]": strconv.Itoa(port),
		"[host_port]": hp, "[protocol]": "HTTP/1.0", "[ssh]": hp,
		"[crlf]": "\r\n", "[cr]": "\r", "[lf]": "\n", "[lfcr]": "\n\r",
		"\\n": "\n", "\\r": "\r",
		"[ua]": "Mozilla/5.0 (Windows NT 6.3; WOW64) AppleWebKit/537.36",
	}
	for k, v := range repl {
		payload = strings.ReplaceAll(payload, k, v)
	}
	rotateMu.Lock()
	if payload != lastUsed {
		rotates = map[int]int{}
		lastUsed = payload
	}
	for i, m := range reRotate.FindAllStringSubmatch(payload, -1) {
		opts := strings.Split(m[1], ";")
		if len(opts) == 0 {
			continue
		}
		idx := rotates[i] % len(opts)
		payload = strings.Replace(payload, m[0], opts[idx], 1)
		rotates[i] = idx + 1
	}
	rotateMu.Unlock()
	payload = reRandom.ReplaceAllStringFunc(payload, func(m string) string {
		sub := reRandom.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		opts := strings.Split(sub[1], ";")
		if len(opts) == 0 {
			return m
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(opts))))
		return opts[n.Int64()]
	})
	return payload
}

func injectSplit(payload string, out io.Writer) error {
	parts := strings.Split(payload, "[delay_split]")
	for i, p := range parts {
		for _, sp := range strings.Split(p, "[split]") {
			if _, err := io.WriteString(out, sp); err != nil {
				return err
			}
		}
		if i != len(parts)-1 {
			time.Sleep(time.Second)
		}
	}
	return nil
}

// =====================================================================
// SECTION 4: SSH TUNNEL
// =====================================================================

type monitoredConn struct {
	net.Conn
	onClose func(error)
	once    sync.Once
}

func (c *monitoredConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if err != nil {
		c.once.Do(func() { c.onClose(err) })
	}
	return n, err
}
func (c *monitoredConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if err != nil {
		c.once.Do(func() { c.onClose(err) })
	}
	return n, err
}
func (c *monitoredConn) Close() error {
	c.once.Do(func() { c.onClose(net.ErrClosed) })
	return c.Conn.Close()
}

type SSHTunnel struct {
	cfg       *Config
	logger    *Logger
	mu        sync.RWMutex
	client    *ssh.Client
	state     atomic.Value
	disconCh  chan error
	reconnect atomic.Int64
	lastErr   atomic.Value
	connected atomic.Bool
}

func NewSSHTunnel(cfg *Config, logger *Logger) *SSHTunnel {
	t := &SSHTunnel{
		cfg:      cfg,
		logger:   logger,
		disconCh: make(chan error, 1),
	}
	t.state.Store("disconnected")
	return t
}

func (t *SSHTunnel) State() string { return t.state.Load().(string) }

func (t *SSHTunnel) setState(s string) {
	t.state.Store(s)
	t.logger.Info("State: %s", s)
}

func (t *SSHTunnel) GetClient() *ssh.Client {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.client
}

func (t *SSHTunnel) Dial(network, addr string) (net.Conn, error) {
	c := t.GetClient()
	if c == nil {
		return nil, errors.New("tunnel tidak aktif")
	}
	return c.DialTimeout(network, addr, 15*time.Second)
}

func (t *SSHTunnel) buildAuth() ([]ssh.AuthMethod, error) {
	var auths []ssh.AuthMethod
	if t.cfg.KeyPath != "" {
		data, err := os.ReadFile(t.cfg.KeyPath)
		if err == nil {
			var signer ssh.Signer
			if t.cfg.Password != "" {
				signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(t.cfg.Password))
			} else {
				signer, err = ssh.ParsePrivateKey(data)
			}
			if err == nil {
				auths = append(auths, ssh.PublicKeys(signer))
			}
		}
	}
	if t.cfg.Password != "" {
		auths = append(auths, ssh.Password(t.cfg.Password))
	}
	if len(auths) == 0 {
		return nil, errors.New("tidak ada auth")
	}
	return auths, nil
}

func (t *SSHTunnel) openRawConn() (net.Conn, error) {
	var dialAddr, payload string
	readResponse := false

	if t.cfg.TunnelType == "proxy" {
		dialAddr = net.JoinHostPort(t.cfg.ProxyHost, strconv.Itoa(t.cfg.ProxyPort))
		if t.cfg.UseDefaultPayload {
			payload = fmt.Sprintf("CONNECT %s:%d HTTP/1.0\r\n\r\n", t.cfg.Server, t.cfg.ServerPort)
		} else {
			payload = formatPayload(t.cfg.Server, t.cfg.ServerPort, t.cfg.CustomPayload)
		}
		readResponse = true
	} else {
		dialAddr = net.JoinHostPort(t.cfg.Server, strconv.Itoa(t.cfg.ServerPort))
		if !t.cfg.UseDefaultPayload {
			payload = formatPayload(t.cfg.Server, t.cfg.ServerPort, t.cfg.CustomPayload)
		}
	}

	conn, err := net.DialTimeout("tcp", dialAddr, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if t.cfg.NoDelay {
		if tc, ok := conn.(*net.TCPConn); ok {
			tc.SetNoDelay(true)
		}
	}

	if payload != "" {
		t.logger.Info("Injecting payload ke %s", dialAddr)
		if err := injectSplit(payload, conn); err != nil {
			conn.Close()
			return nil, err
		}
	}

	if readResponse {
		br := bufio.NewReader(conn)
		first, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		t.logger.Info("Proxy: %s", strings.TrimRight(first, "\r\n"))
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				conn.Close()
				return nil, err
			}
			if strings.TrimRight(line, "\r\n") == "" {
				break
			}
		}
		return &bufioConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}

type bufioConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufioConn) Read(b []byte) (int, error) { return c.r.Read(b) }

func (t *SSHTunnel) dialOnce() error {
	auths, err := t.buildAuth()
	if err != nil {
		return err
	}
	sshCfg := &ssh.ClientConfig{
		User:            t.cfg.Username,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}
	raw, err := t.openRawConn()
	if err != nil {
		return err
	}
	mon := &monitoredConn{Conn: raw, onClose: func(e error) {
		select {
		case t.disconCh <- e:
		default:
		}
	}}
	addr := net.JoinHostPort(t.cfg.Server, strconv.Itoa(t.cfg.ServerPort))
	sshConn, chans, reqs, err := ssh.NewClientConn(mon, addr, sshCfg)
	if err != nil {
		raw.Close()
		return fmt.Errorf("handshake SSH: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	t.mu.Lock()
	t.client = client
	t.mu.Unlock()
	t.connected.Store(true)
	t.logger.Info("SSH tersambung ke %s", addr)
	if t.cfg.Pinger > 0 {
		go t.pinger(client)
	}
	return nil
}

func (t *SSHTunnel) pinger(client *ssh.Client) {
	tk := time.NewTicker(time.Duration(t.cfg.Pinger) * time.Second)
	defer tk.Stop()
	for range tk.C {
		if !t.connected.Load() {
			return
		}
		if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
			t.logger.Warn("Ping gagal: %v", err)
			return
		}
	}
}

func (t *SSHTunnel) cleanup() {
	t.mu.Lock()
	if t.client != nil {
		t.client.Close()
	}
	t.client = nil
	t.mu.Unlock()
	t.connected.Store(false)
	select {
	case <-t.disconCh:
	default:
	}
}

func (t *SSHTunnel) Run(ctx context.Context) {
	backoff := t.cfg.Reconnect.InitialDelay
	for {
		if ctx.Err() != nil {
			t.cleanup()
			return
		}

		t.setState("connecting")
		if err := t.dialOnce(); err != nil {
			t.logger.Error("Dial gagal: %v", err)
			t.lastErr.Store(err.Error())
			t.waitBackoff(ctx, &backoff)
			continue
		}

		t.setState("connected")
		t.lastErr.Store("")
		backoff = t.cfg.Reconnect.InitialDelay

		select {
		case <-ctx.Done():
			t.cleanup()
			return
		case err := <-t.disconCh:
			t.logger.Warn("Tunnel terputus: %v", err)
			t.lastErr.Store(err.Error())
			t.setState("disconnected")
			t.cleanup()
			t.reconnect.Add(1)
		}

		if !t.cfg.Reconnect.Enabled {
			return
		}
		t.waitBackoff(ctx, &backoff)
	}
}

func (t *SSHTunnel) waitBackoff(ctx context.Context, backoff *time.Duration) {
	jitter := time.Duration(mrand.Int63n(int64(*backoff)/2 + 1))
	wait := *backoff + jitter
	t.logger.Info("Reconnect dalam %s...", wait)
	select {
	case <-ctx.Done():
		return
	case <-time.After(wait):
	}
	*backoff *= 2
	if *backoff > t.cfg.Reconnect.MaxDelay {
		*backoff = t.cfg.Reconnect.MaxDelay
	}
}

func (t *SSHTunnel) StartHealthLoop(ctx context.Context) {
	if !t.cfg.HealthCheck.Enabled {
		return
	}
	go func() {
		failures := 0
		tk := time.NewTicker(t.cfg.HealthCheck.Interval)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
			}
			c := t.GetClient()
			if c == nil {
				continue
			}
			conn, err := c.DialTimeout("tcp", t.cfg.HealthCheck.Target, t.cfg.HealthCheck.Timeout)
			if err != nil {
				failures++
				t.logger.Warn("Health check gagal (%d/%d): %v", failures, t.cfg.HealthCheck.FailureThreshold, err)
				if failures >= t.cfg.HealthCheck.FailureThreshold {
					t.logger.Error("Memaksa reconnect (health threshold)")
					t.mu.Lock()
					if t.client != nil {
						t.client.Close()
					}
					t.mu.Unlock()
					failures = 0
				}
			} else {
				conn.Close()
				failures = 0
			}
		}
	}()
}

// =====================================================================
// SECTION 5: FAKE-IP DNS SERVER
// =====================================================================

type FakeIPPool struct {
	mu        sync.RWMutex
	domain2IP map[string]string
	ip2domain map[string]string
	nextIP    uint32
}

func NewFakeIPPool() *FakeIPPool {
	return &FakeIPPool{
		domain2IP: map[string]string{},
		ip2domain: map[string]string{},
		nextIP:    (198 << 24) | (18 << 16) | (0 << 8) | 1,
	}
}

func (p *FakeIPPool) Assign(domain string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if ip, ok := p.domain2IP[domain]; ok {
		return ip
	}
	ip := fmt.Sprintf("%d.%d.%d.%d",
		(p.nextIP>>24)&0xFF, (p.nextIP>>16)&0xFF, (p.nextIP>>8)&0xFF, p.nextIP&0xFF)
	p.domain2IP[domain] = ip
	p.ip2domain[ip] = domain
	p.nextIP++
	if (p.nextIP>>16)&0xFF != 18 {
		p.nextIP = (198 << 24) | (18 << 16) | (0 << 8) | 1
	}
	return ip
}

func (p *FakeIPPool) LookupDomain(ip string) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	d, ok := p.ip2domain[ip]
	return d, ok
}

func (p *FakeIPPool) IsFake(ip string) bool {
	parsed := net.ParseIP(ip).To4()
	if parsed == nil {
		return false
	}
	return parsed[0] == 198 && parsed[1] == 18
}

type DNSServer struct {
	cfg     *Config
	logger  *Logger
	pool    *FakeIPPool
	tunnel  *SSHTunnel
	udpConn *net.UDPConn
	tcpLn   net.Listener
	closed  atomic.Bool
	wg      sync.WaitGroup
	queries atomic.Int64
	errors  atomic.Int64
}

func NewDNSServer(cfg *Config, logger *Logger, pool *FakeIPPool, tunnel *SSHTunnel) *DNSServer {
	return &DNSServer{cfg: cfg, logger: logger, pool: pool, tunnel: tunnel}
}

func (d *DNSServer) Start() error {
	ua, _ := net.ResolveUDPAddr("udp", d.cfg.DNS.Bind)
	udp, err := net.ListenUDP("udp", ua)
	if err != nil {
		return err
	}
	tcp, err := net.Listen("tcp", d.cfg.DNS.Bind)
	if err != nil {
		udp.Close()
		return err
	}
	d.udpConn = udp
	d.tcpLn = tcp
	d.logger.Info("Fake-IP DNS listen di %s", d.cfg.DNS.Bind)
	d.wg.Add(2)
	go d.serveUDP()
	go d.serveTCP()
	return nil
}

func (d *DNSServer) serveUDP() {
	defer d.wg.Done()
	buf := make([]byte, 4096)
	for {
		n, addr, err := d.udpConn.ReadFromUDP(buf)
		if err != nil {
			if d.closed.Load() {
				return
			}
			continue
		}
		q := make([]byte, n)
		copy(q, buf[:n])
		go func() {
			r := d.handleQuery(q)
			if r != nil {
				d.udpConn.WriteToUDP(r, addr)
			}
		}()
	}
}

func (d *DNSServer) serveTCP() {
	defer d.wg.Done()
	for {
		c, err := d.tcpLn.Accept()
		if err != nil {
			if d.closed.Load() {
				return
			}
			continue
		}
		go func() {
			defer c.Close()
			c.SetDeadline(time.Now().Add(10 * time.Second))
			h := make([]byte, 2)
			if _, err := io.ReadFull(c, h); err != nil {
				return
			}
			q := make([]byte, int(h[0])<<8|int(h[1]))
			if _, err := io.ReadFull(c, q); err != nil {
				return
			}
			r := d.handleQuery(q)
			if r == nil {
				return
			}
			out := []byte{byte(len(r) >> 8), byte(len(r))}
			c.Write(append(out, r...))
		}()
	}
}

func (d *DNSServer) handleQuery(q []byte) []byte {
	d.queries.Add(1)
	if len(q) < 12 {
		return nil
	}

	pos := 12
	var labels []string
	for pos < len(q) {
		l := int(q[pos])
		if l == 0 {
			pos++
			break
		}
		if l > 63 || pos+l+1 > len(q) {
			return nil
		}
		labels = append(labels, string(q[pos+1:pos+1+l]))
		pos += l + 1
	}
	if pos+4 > len(q) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[pos : pos+2])
	qclass := binary.BigEndian.Uint16(q[pos+2 : pos+4])
	pos += 4
	questionEnd := pos

	domain := strings.Join(labels, ".")

	if qtype != 1 || qclass != 1 {
		return d.buildEmptyResponse(q, questionEnd, 0)
	}

	fakeIP := d.pool.Assign(domain)
	ip := net.ParseIP(fakeIP).To4()
	d.logger.Debug("DNS: %s → %s", domain, fakeIP)

	return d.buildAResponse(q, questionEnd, ip)
}

func (d *DNSServer) buildAResponse(q []byte, questionEnd int, ip net.IP) []byte {
	resp := make([]byte, questionEnd+16)
	copy(resp, q[:questionEnd])
	resp[2] = 0x81
	resp[3] = 0x80
	resp[6] = 0x00
	resp[7] = 0x01
	resp[8] = 0x00
	resp[9] = 0x00
	resp[10] = 0x00
	resp[11] = 0x00
	off := questionEnd
	resp[off+0] = 0xC0
	resp[off+1] = 0x0C
	resp[off+2] = 0x00
	resp[off+3] = 0x01
	resp[off+4] = 0x00
	resp[off+5] = 0x01
	resp[off+6] = 0x00
	resp[off+7] = 0x00
	resp[off+8] = 0x00
	resp[off+9] = 0x3C
	resp[off+10] = 0x00
	resp[off+11] = 0x04
	copy(resp[off+12:off+16], ip.To4())
	return resp
}

func (d *DNSServer) buildEmptyResponse(q []byte, questionEnd, rcode int) []byte {
	resp := make([]byte, questionEnd)
	copy(resp, q[:questionEnd])
	resp[2] = 0x81
	resp[3] = byte(0x80 | rcode)
	resp[6] = 0x00
	resp[7] = 0x00
	resp[8] = 0x00
	resp[9] = 0x00
	resp[10] = 0x00
	resp[11] = 0x00
	return resp
}

func (d *DNSServer) ResolveViaTunnel(domain string) (net.IP, error) {
	q := d.buildDNSQuery(domain)
	upstream := d.cfg.DNS.Upstream
	if !strings.Contains(upstream, ":") {
		upstream += ":53"
	}
	c, err := d.tunnel.Dial("tcp", upstream)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	c.Write([]byte{byte(len(q) >> 8), byte(len(q))})
	c.Write(q)
	h := make([]byte, 2)
	if _, err := io.ReadFull(c, h); err != nil {
		return nil, err
	}
	resp := make([]byte, int(h[0])<<8|int(h[1]))
	if _, err := io.ReadFull(c, resp); err != nil {
		return nil, err
	}
	return d.parseAResponse(resp)
}

func (d *DNSServer) buildDNSQuery(domain string) []byte {
	txID := make([]byte, 2)
	rand.Read(txID)
	q := []byte{txID[0], txID[1], 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	for _, label := range strings.Split(domain, ".") {
		q = append(q, byte(len(label)))
		q = append(q, []byte(label)...)
	}
	q = append(q, 0x00, 0x00, 0x01, 0x00, 0x01)
	return q
}

func (d *DNSServer) parseAResponse(resp []byte) (net.IP, error) {
	if len(resp) < 12 {
		return nil, errors.New("resp too short")
	}
	ancount := int(binary.BigEndian.Uint16(resp[6:8]))
	if ancount == 0 {
		return nil, errors.New("no answer")
	}
	pos := 12
	for pos < len(resp) {
		b := int(resp[pos])
		if b == 0 {
			pos++
			break
		}
		if b&0xC0 == 0xC0 {
			pos += 2
			break
		}
		pos += b + 1
	}
	pos += 4
	for i := 0; i < ancount && pos < len(resp); i++ {
		if resp[pos]&0xC0 == 0xC0 {
			pos += 2
		} else {
			for pos < len(resp) && resp[pos] != 0 {
				pos += int(resp[pos]) + 1
			}
			pos++
		}
		if pos+10 > len(resp) {
			break
		}
		typ := binary.BigEndian.Uint16(resp[pos : pos+2])
		rdlen := int(binary.BigEndian.Uint16(resp[pos+8 : pos+10]))
		pos += 10
		if typ == 1 && rdlen == 4 && pos+4 <= len(resp) {
			ip := net.IPv4(resp[pos], resp[pos+1], resp[pos+2], resp[pos+3])
			return ip, nil
		}
		pos += rdlen
	}
	return nil, errors.New("no A record found")
}

func (d *DNSServer) Close() {
	if d.closed.Swap(true) {
		return
	}
	if d.udpConn != nil {
		d.udpConn.Close()
	}
	if d.tcpLn != nil {
		d.tcpLn.Close()
	}
	d.wg.Wait()
}

// =====================================================================
// SECTION 6: RULE ENGINE
// =====================================================================

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
		if len(parts) < 2 {
			return nil, fmt.Errorf("MATCH needs target")
		}
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
		return strings.HasSuffix(strings.ToLower(domain), r.Value) ||
			strings.ToLower(domain) == strings.TrimPrefix(r.Value, ".")
	case RuleDomainKeyword:
		return strings.Contains(strings.ToLower(domain), r.Value)
	}
	return false
}

// =====================================================================
// SECTION 7: PROXY MANAGER (groups + state)
// =====================================================================

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

// =====================================================================
// SECTION 8: CONNECTION TRACKER
// =====================================================================

type Connection struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start"`
	SrcIP     string    `json:"srcIP"`
	DstHost   string    `json:"dstHost"`
	DstPort   int       `json:"dstPort"`
	Domain    string    `json:"domain,omitempty"`
	Rule      string    `json:"rule"`
	Chain     string    `json:"chain"`
	Proxy     string    `json:"proxy"`
	BytesUp   int64     `json:"up"`
	BytesDown int64     `json:"down"`
	Closed    bool      `json:"closed"`
}

type ConnTracker struct {
	mu      sync.RWMutex
	conns   map[string]*Connection
	order   []string
	maxSize int
}

func NewConnTracker() *ConnTracker {
	return &ConnTracker{
		conns:   map[string]*Connection{},
		maxSize: 300,
	}
}

func (ct *ConnTracker) New(srcIP, domain, dstHost string, dstPort int, rule, chain, proxy string) *Connection {
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), mrand.Int31())
	c := &Connection{
		ID:        id,
		StartTime: time.Now(),
		SrcIP:     srcIP,
		DstHost:   dstHost,
		DstPort:   dstPort,
		Domain:    domain,
		Rule:      rule,
		Chain:     chain,
		Proxy:     proxy,
	}
	ct.mu.Lock()
	ct.conns[id] = c
	ct.order = append(ct.order, id)
	for len(ct.order) > ct.maxSize {
		old := ct.order[0]
		ct.order = ct.order[1:]
		delete(ct.conns, old)
	}
	ct.mu.Unlock()
	return c
}

func (ct *ConnTracker) Close(id string) {
	ct.mu.Lock()
	if c, ok := ct.conns[id]; ok {
		c.Closed = true
	}
	ct.mu.Unlock()
}

func (ct *ConnTracker) AddBytes(id string, up, down int64) {
	ct.mu.Lock()
	if c, ok := ct.conns[id]; ok {
		c.BytesUp += up
		c.BytesDown += down
	}
	ct.mu.Unlock()
}

func (ct *ConnTracker) List() []*Connection {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	out := make([]*Connection, 0, len(ct.order))
	for i := len(ct.order) - 1; i >= 0; i-- {
		if c, ok := ct.conns[ct.order[i]]; ok {
			out = append(out, c)
		}
	}
	return out
}

func (ct *ConnTracker) CloseAll() {
	ct.mu.Lock()
	for _, c := range ct.conns {
		c.Closed = true
	}
	ct.mu.Unlock()
}

// =====================================================================
// SECTION 9: REDIRECT LISTENER (SO_ORIGINAL_DST)
// =====================================================================

func getOriginalDst(conn *net.TCPConn) (string, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return "", err
	}
	var dst string
	var innerErr error
	err = raw.Control(func(fd uintptr) {
		var addr unix.RawSockaddrInet4
		size := uint32(unsafe.Sizeof(addr))
		_, _, errno := unix.Syscall6(
			unix.SYS_GETSOCKOPT,
			fd,
			uintptr(unix.SOL_IP),
			uintptr(SOOriginalDst),
			uintptr(unsafe.Pointer(&addr)),
			uintptr(unsafe.Pointer(&size)),
			0,
		)
		if errno != 0 {
			innerErr = errno
			return
		}
		ip := net.IPv4(addr.Addr[0], addr.Addr[1], addr.Addr[2], addr.Addr[3])
		portBytes := (*[2]byte)(unsafe.Pointer(&addr.Port))
		port := int(portBytes[0])<<8 | int(portBytes[1])
		dst = fmt.Sprintf("%s:%d", ip.String(), port)
	})
	if err != nil {
		return "", err
	}
	return dst, innerErr
}

type Router struct {
	cfg      *Config
	logger   *Logger
	tunnel   *SSHTunnel
	dns      *DNSServer
	pool     *FakeIPPool
	proxyMgr *ProxyManager
	tracker  *ConnTracker
	rules    []*Rule

	ln     net.Listener
	closed atomic.Bool
	wg     sync.WaitGroup
}

func NewRouter(cfg *Config, logger *Logger, tunnel *SSHTunnel, dns *DNSServer, pool *FakeIPPool, pm *ProxyManager, tracker *ConnTracker) (*Router, error) {
	r := &Router{
		cfg:      cfg,
		logger:   logger,
		tunnel:   tunnel,
		dns:      dns,
		pool:     pool,
		proxyMgr: pm,
		tracker:  tracker,
	}
	for _, rs := range cfg.Rules {
		rule, err := parseRule(rs)
		if err != nil {
			return nil, err
		}
		r.rules = append(r.rules, rule)
	}
	return r, nil
}

func (r *Router) Start() error {
	ln, err := net.Listen("tcp", r.cfg.Redirect.Bind)
	if err != nil {
		return err
	}
	r.ln = ln
	r.logger.Info("REDIRECT listener di %s", r.cfg.Redirect.Bind)
	r.wg.Add(1)
	go r.acceptLoop()
	return nil
}

func (r *Router) acceptLoop() {
	defer r.wg.Done()
	for {
		c, err := r.ln.Accept()
		if err != nil {
			if r.closed.Load() {
				return
			}
			continue
		}
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			r.handleConn(c)
		}()
	}
}

func (r *Router) dispatch(srcIP, dstIP, domain string) (proxy string, ruleStr string, chainStr string) {
	for _, rule := range r.rules {
		if rule.Match(srcIP, dstIP, domain) {
			ruleStr = rule.raw
			proxy, chainStr = r.proxyMgr.Resolve(rule.Target)
			return proxy, ruleStr, chainStr
		}
	}
	return ProxyReject, "no-match", ""
}

func (r *Router) handleConn(raw net.Conn) {
	defer raw.Close()

	tcpConn, ok := raw.(*net.TCPConn)
	if !ok {
		return
	}

	dstAddr, err := getOriginalDst(tcpConn)
	if err != nil {
		r.logger.Debug("SO_ORIGINAL_DST gagal: %v", err)
		return
	}

	srcIP := ""
	if ra := tcpConn.RemoteAddr(); ra != nil {
		if ta, ok := ra.(*net.TCPAddr); ok {
			srcIP = ta.IP.String()
		}
	}

	host, portStr, _ := net.SplitHostPort(dstAddr)
	port, _ := strconv.Atoi(portStr)

	dstIP := host
	domain := ""
	if r.pool.IsFake(host) {
		if d, ok := r.pool.LookupDomain(host); ok {
			domain = d
			dstIP = ""
		}
	}

	proxy, ruleStr, chainStr := r.dispatch(srcIP, dstIP, domain)

	track := r.tracker.New(srcIP, domain, host, port, ruleStr, chainStr, proxy)
	defer r.tracker.Close(track.ID)

	display := domain
	if display == "" {
		display = host
	}
	r.logger.Debug("CONN %s → %s:%d [%s] via %s", srcIP, display, port, ruleStr, proxy)

	switch proxy {
	case ProxyReject:
		return

	case ProxyDirect:
		target := dstAddr
		if domain != "" {
			target = net.JoinHostPort(domain, strconv.Itoa(port))
		}
		remote, err := net.DialTimeout("tcp", target, 10*time.Second)
		if err != nil {
			r.logger.Debug("DIRECT dial %s gagal: %v", target, err)
			return
		}
		defer remote.Close()
		r.pipe(raw, remote, track)

	case ProxyTunnel:
		target := ""
		if domain != "" {
			target = net.JoinHostPort(domain, strconv.Itoa(port))
		} else {
			target = dstAddr
		}
		remote, err := r.tunnel.Dial("tcp", target)
		if err != nil {
			r.logger.Debug("TUNNEL dial %s gagal: %v", target, err)
			return
		}
		defer remote.Close()
		r.pipe(raw, remote, track)

	default:
		r.logger.Warn("Unknown proxy: %s", proxy)
	}
}

func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

func (r *Router) pipe(a, b net.Conn, track *Connection) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(b, a)
		atomic.AddInt64(&track.BytesUp, n)
		closeWrite(b)
	}()
	go func() {
		defer wg.Done()
		n, _ := io.Copy(a, b)
		atomic.AddInt64(&track.BytesDown, n)
		closeWrite(a)
	}()
	wg.Wait()
}

func (r *Router) Close() {
	if r.closed.Swap(true) {
		return
	}
	if r.ln != nil {
		r.ln.Close()
	}
	r.wg.Wait()
}

// =====================================================================
// SECTION 10: HTTP API
// =====================================================================

type APIServer struct {
	cfg      *Config
	logger   *Logger
	tunnel   *SSHTunnel
	proxyMgr *ProxyManager
	tracker  *ConnTracker
	dns      *DNSServer
	start    time.Time
	srv      *http.Server
}

func NewAPIServer(cfg *Config, logger *Logger, tunnel *SSHTunnel, pm *ProxyManager, tr *ConnTracker, dns *DNSServer) *APIServer {
	return &APIServer{
		cfg: cfg, logger: logger, tunnel: tunnel,
		proxyMgr: pm, tracker: tr, dns: dns,
		start: time.Now(),
	}
}

func (a *APIServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleUI)
	mux.HandleFunc("/api/status", a.auth(a.handleStatus))
	mux.HandleFunc("/api/proxies", a.auth(a.handleProxies))
	mux.HandleFunc("/api/proxies/", a.auth(a.handleProxySwitch))
	mux.HandleFunc("/api/connections", a.auth(a.handleConnections))
	mux.HandleFunc("/api/logs", a.auth(a.handleLogs))
	mux.HandleFunc("/api/config", a.auth(a.handleConfig))

	a.srv = &http.Server{Addr: a.cfg.API.Bind, Handler: mux}
	go func() {
		a.logger.Info("API server: http://%s", a.cfg.API.Bind)
		if err := a.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("API: %v", err)
		}
	}()
	return nil
}

func (a *APIServer) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cfg.API.Secret != "" {
			token := r.Header.Get("X-Secret")
			if token == "" {
				token = r.URL.Query().Get("secret")
			}
			if token != a.cfg.API.Secret {
				http.Error(w, "unauthorized", 401)
				return
			}
		}
		h(w, r)
	}
}

func (a *APIServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	lastErr, _ := a.tunnel.lastErr.Load().(string)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"state":       a.tunnel.State(),
		"uptime_sec":  int(time.Since(a.start).Seconds()),
		"server":      fmt.Sprintf("%s:%d", a.cfg.Server, a.cfg.ServerPort),
		"reconnects":  a.tunnel.reconnect.Load(),
		"last_error":  lastErr,
		"dns_queries": a.dns.queries.Load(),
	})
}

func (a *APIServer) handleProxies(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"proxies": a.proxyMgr.List(),
	})
}

func (a *APIServer) handleProxySwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" && r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	groupName := strings.TrimPrefix(r.URL.Path, "/api/proxies/")
	if groupName == "" {
		http.Error(w, "missing group", 400)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := a.proxyMgr.Set(groupName, body.Name); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	a.logger.Info("Group %s → %s", groupName, body.Name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *APIServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	conns := a.tracker.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"connections": conns,
	})
}

func (a *APIServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	ch := a.logger.Subscribe()
	defer a.logger.Unsubscribe(ch)
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	timeout := time.After(30 * time.Second)
	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return
			}
			w.Write([]byte(line))
			flusher.Flush()
		case <-timeout:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (a *APIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"server":   fmt.Sprintf("%s:%d", a.cfg.Server, a.cfg.ServerPort),
		"tunnel":   a.cfg.TunnelType,
		"socks5":   fmt.Sprintf("%s:%d", a.cfg.SOCKS5.Bind, a.cfg.SOCKS5.Port),
		"dns":      a.cfg.DNS.Bind,
		"redirect": a.cfg.Redirect.Bind,
		"rules":    a.cfg.Rules,
	})
}

func (a *APIServer) handleUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(uiHTML))
}

func (a *APIServer) Close() {
	if a.srv != nil {
		a.srv.Close()
	}
}

// =====================================================================
// SECTION 11: EMBEDDED WEB UI
// =====================================================================

const uiHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>SocksHTTP Dashboard</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;background:#0d1117;color:#c9d1d9;font-size:14px}
header{background:#161b22;border-bottom:1px solid #30363d;padding:14px 20px;display:flex;align-items:center;gap:20px}
header h1{font-size:18px;font-weight:600;color:#58a6ff}
.state-badge{padding:3px 10px;border-radius:12px;font-size:11px;font-weight:600;text-transform:uppercase}
.state-connected{background:#1f6feb;color:#fff}
.state-connecting{background:#bf8700;color:#fff}
.state-disconnected{background:#da3633;color:#fff}
nav{background:#161b22;border-bottom:1px solid #30363d;padding:0 20px;display:flex;gap:0}
nav button{background:transparent;border:none;color:#8b949e;padding:12px 20px;cursor:pointer;font-size:14px;border-bottom:2px solid transparent;transition:all .2s}
nav button:hover{color:#c9d1d9}
nav button.active{color:#58a6ff;border-bottom-color:#58a6ff}
main{padding:20px;max-width:1400px;margin:0 auto}
.tab{display:none}
.tab.active{display:block}
.group-card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;margin-bottom:12px}
.group-name{font-weight:600;color:#58a6ff;margin-bottom:10px;font-size:15px}
.group-buttons{display:flex;gap:8px;flex-wrap:wrap}
.group-btn{background:#21262d;border:1px solid #30363d;color:#c9d1d9;padding:8px 16px;border-radius:6px;cursor:pointer;font-size:13px;font-weight:500;transition:all .15s}
.group-btn:hover{background:#30363d}
.group-btn.active{background:#1f6feb;border-color:#1f6feb;color:#fff}
.group-btn.reject.active{background:#da3633;border-color:#da3633}
.group-btn.direct.active{background:#238636;border-color:#238636}
table{width:100%;border-collapse:collapse;background:#161b22;border:1px solid #30363d;border-radius:8px;overflow:hidden}
th{background:#21262d;padding:10px 12px;text-align:left;font-weight:600;color:#8b949e;font-size:12px;text-transform:uppercase;letter-spacing:.5px}
td{padding:10px 12px;border-top:1px solid #21262d;font-family:ui-monospace,monospace;font-size:12px}
td.mono{font-family:ui-monospace,monospace}
.tag{padding:2px 8px;border-radius:4px;font-size:11px;font-weight:600}
.tag-TUNNEL{background:#1f6feb22;color:#58a6ff}
.tag-DIRECT{background:#23863622;color:#3fb950}
.tag-REJECT{background:#da363322;color:#f85149}
#logbox{background:#0d1117;border:1px solid #30363d;border-radius:8px;padding:12px;height:600px;overflow-y:auto;font-family:ui-monospace,monospace;font-size:12px;line-height:1.5;white-space:pre-wrap;word-break:break-all}
.stat{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px;margin-bottom:20px}
.stat-card{background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px}
.stat-label{color:#8b949e;font-size:12px;text-transform:uppercase;letter-spacing:.5px;margin-bottom:6px}
.stat-value{font-size:22px;font-weight:600;color:#58a6ff}
.bytes{color:#8b949e;font-size:11px}
.empty{text-align:center;color:#8b949e;padding:40px;font-style:italic}
</style>
</head>
<body>

<header>
  <h1>⚡ SocksHTTP</h1>
  <span id="stateBadge" class="state-badge state-disconnected">disconnected</span>
  <span style="color:#8b949e;font-size:13px" id="serverInfo"></span>
</header>

<nav>
  <button class="active" onclick="showTab('proxies',this)">Proxies</button>
  <button onclick="showTab('connections',this)">Connections</button>
  <button onclick="showTab('logs',this)">Logs</button>
  <button onclick="showTab('config',this)">Config</button>
</nav>

<main>

<div id="tab-proxies" class="tab active">
  <div class="stat" id="statsRow"></div>
  <div id="groupsContainer"></div>
</div>

<div id="tab-connections" class="tab">
  <table>
    <thead><tr>
      <th>Time</th><th>Src IP</th><th>Destination</th><th>Rule</th><th>Proxy</th><th>Up</th><th>Down</th>
    </tr></thead>
    <tbody id="connBody"><tr><td colspan="7" class="empty">Loading...</td></tr></tbody>
  </table>
</div>

<div id="tab-logs" class="tab">
  <div id="logbox">Connecting to log stream...</div>
</div>

<div id="tab-config" class="tab">
  <pre id="configBox" style="background:#161b22;border:1px solid #30363d;border-radius:8px;padding:16px;overflow-x:auto">Loading...</pre>
</div>

</main>

<script>
function fmtBytes(n){
  if(n<1024)return n+" B";
  if(n<1048576)return (n/1024).toFixed(1)+" KB";
  if(n<1073741824)return (n/1048576).toFixed(1)+" MB";
  return (n/1073741824).toFixed(2)+" GB";
}

function showTab(name,btn){
  document.querySelectorAll('.tab').forEach(function(t){t.classList.remove('active')});
  document.querySelectorAll('nav button').forEach(function(b){b.classList.remove('active')});
  document.getElementById('tab-'+name).classList.add('active');
  btn.classList.add('active');
  if(name==='logs')connectLogs();
  if(name==='config')loadConfig();
}

async function loadStatus(){
  try{
    var r=await fetch('/api/status');
    var d=await r.json();
    var badge=document.getElementById('stateBadge');
    badge.textContent=d.state;
    badge.className='state-badge state-'+d.state;
    document.getElementById('serverInfo').textContent=d.server||'';
    var color = (d.state==='connected') ? '#3fb950' : '#f85149';
    var html = '';
    html += '<div class="stat-card"><div class="stat-label">State</div><div class="stat-value" style="color:'+color+'">'+d.state+'</div></div>';
    html += '<div class="stat-card"><div class="stat-label">Uptime</div><div class="stat-value">'+Math.floor(d.uptime_sec/60)+'m</div></div>';
    html += '<div class="stat-card"><div class="stat-label">Reconnects</div><div class="stat-value">'+d.reconnects+'</div></div>';
    html += '<div class="stat-card"><div class="stat-label">DNS Queries</div><div class="stat-value">'+d.dns_queries+'</div></div>';
    document.getElementById('statsRow').innerHTML=html;
  }catch(e){console.error(e)}
}

async function loadProxies(){
  try{
    var r=await fetch('/api/proxies');
    var d=await r.json();
    var container=document.getElementById('groupsContainer');
    if(!d.proxies||d.proxies.length===0){
      container.innerHTML='<div class="empty">No proxy groups</div>';
      return;
    }
    var html='';
    d.proxies.forEach(function(g){
      var btns='';
      g.all.forEach(function(p){
        var cls='group-btn '+p.toLowerCase().replace(/_/g,'-')+(g.now===p?' active':'');
        btns += '<button class="'+cls+'" onclick="switchProxy(\''+g.name+'\',\''+p+'\')">'+p+'</button>';
      });
      html += '<div class="group-card"><div class="group-name">'+g.name+'</div><div class="group-buttons">'+btns+'</div></div>';
    });
    container.innerHTML=html;
  }catch(e){console.error(e)}
}

async function switchProxy(group,name){
  try{
    var r=await fetch('/api/proxies/'+encodeURIComponent(group),{
      method:'PUT',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({name:name})
    });
    if(!r.ok)throw new Error(await r.text());
    loadProxies();
  }catch(e){alert('Error: '+e.message)}
}

async function loadConnections(){
  try{
    var r=await fetch('/api/connections');
    var d=await r.json();
    var body=document.getElementById('connBody');
    if(!d.connections||d.connections.length===0){
      body.innerHTML='<tr><td colspan="7" class="empty">No active connections</td></tr>';
      return;
    }
    var html='';
    d.connections.forEach(function(c){
      var t=new Date(c.start);
      var ts=t.toTimeString().slice(0,8);
      var dst=c.domain||c.dstHost;
      var proxyCls='tag tag-'+c.proxy;
      html += '<tr>';
      html += '<td>'+ts+'</td>';
      html += '<td>'+c.srcIP+'</td>';
      html += '<td>'+dst+':'+c.dstPort+'</td>';
      html += '<td style="color:#8b949e">'+c.rule+'</td>';
      html += '<td><span class="'+proxyCls+'">'+c.proxy+'</span></td>';
      html += '<td class="bytes">'+fmtBytes(c.up)+'</td>';
      html += '<td class="bytes">'+fmtBytes(c.down)+'</td>';
      html += '</tr>';
    });
    body.innerHTML=html;
  }catch(e){console.error(e)}
}

var logSource=null;
function connectLogs(){
  if(logSource){logSource.close()}
  var box=document.getElementById('logbox');
  box.textContent='';
  fetch('/api/logs').then(function(r){
    var reader=r.body.getReader();
    var dec=new TextDecoder();
    function read(){
      reader.read().then(function(res){
        if(res.done){setTimeout(connectLogs,2000);return}
        box.textContent+=dec.decode(res.value);
        box.scrollTop=box.scrollHeight;
        if(box.textContent.length>200000)box.textContent=box.textContent.slice(-150000);
        read();
      });
    }
    read();
  });
}

async function loadConfig(){
  try{
    var r=await fetch('/api/config');
    var d=await r.json();
    document.getElementById('configBox').textContent=JSON.stringify(d,null,2);
  }catch(e){}
}

setInterval(loadStatus,3000);
setInterval(loadProxies,5000);
setInterval(loadConnections,2000);
loadStatus();
loadProxies();
loadConnections();
</script>
</body>
</html>`

// =====================================================================
// SECTION 12: MAIN
// =====================================================================

func main() {
	cfgPath := flag.String("config", "", "path ke file YAML config")
	sksPath := flag.String("sks", "", "path ke file .sks (legacy Android)")
	server := flag.String("server", "", "override server SSH")
	port := flag.Int("port", 0, "override port SSH")
	user := flag.String("user", "", "override username")
	pass := flag.String("pass", "", "override password")
	apiBind := flag.String("api", "", "override API bind")
	logLevel := flag.String("log-level", "", "override log level")
	flag.Parse()

	cfg := DefaultConfig()
	var err error
	if *cfgPath != "" {
		cfg, err = LoadYAMLConfig(*cfgPath)
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
	} else if *sksPath != "" {
		f, e := os.Open(*sksPath)
		if e != nil {
			log.Fatalf("open sks: %v", e)
		}
		err = LoadSKSConfig(f, cfg)
		f.Close()
		if err != nil {
			log.Fatalf("parse sks: %v", err)
		}
	}
	if *server != "" {
		cfg.Server = *server
	}
	if *port != 0 {
		cfg.ServerPort = *port
	}
	if *user != "" {
		cfg.Username = *user
	}
	if *pass != "" {
		cfg.Password = *pass
	}
	if *apiBind != "" {
		cfg.API.Bind = *apiBind
	}
	if *logLevel != "" {
		cfg.Log.Level = *logLevel
	}

	if cfg.Server == "" || cfg.ServerPort <= 0 {
		log.Fatal("server/port belum diisi")
	}
	if cfg.ValidityDate > 0 && time.Now().UnixMilli() >= cfg.ValidityDate {
		log.Fatalf("config expired: %s", time.UnixMilli(cfg.ValidityDate))
	}

	logger := NewLogger(parseLevel(cfg.Log.Level))
	if cfg.Log.File != "" {
		if err := logger.SetFile(cfg.Log.File, cfg.Log.MaxSizeMB, cfg.Log.MaxBackups); err != nil {
			logger.Warn("Log file: %v (stdout only)", err)
		}
	}

	logger.Info("=== SocksHTTP-Go v2.0 (integrated router) ===")
	logger.Info("Server=%s:%d Tunnel=%s API=%s", cfg.Server, cfg.ServerPort, cfg.TunnelType, cfg.API.Bind)

	if cfg.PIDFile != "" {
		os.MkdirAll(filepath.Dir(cfg.PIDFile), 0755)
		os.WriteFile(cfg.PIDFile, []byte(strconv.Itoa(os.Getpid())), 0644)
		defer os.Remove(cfg.PIDFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	pool := NewFakeIPPool()
	tunnel := NewSSHTunnel(cfg, logger)
	proxyMgr := NewProxyManager(cfg.ProxyGroups, cfg.StateFile)
	tracker := NewConnTracker()
	dns := NewDNSServer(cfg, logger, pool, tunnel)

	tmDone := make(chan struct{})
	go func() {
		tunnel.Run(ctx)
		close(tmDone)
	}()
	tunnel.StartHealthLoop(ctx)

	logger.Info("Menunggu koneksi SSH pertama...")
	for i := 0; i < 120; i++ {
		if tunnel.State() == "connected" {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	if tunnel.State() != "connected" {
		logger.Error("Timeout menunggu koneksi awal")
		cancel()
		os.Exit(1)
	}

	if cfg.DNS.Enabled {
		if err := dns.Start(); err != nil {
			logger.Error("DNS: %v", err)
			cancel()
			os.Exit(1)
		}
	}

	router, err := NewRouter(cfg, logger, tunnel, dns, pool, proxyMgr, tracker)
	if err != nil {
		logger.Error("Router: %v", err)
		cancel()
		os.Exit(1)
	}
	if err := router.Start(); err != nil {
		logger.Error("Router start: %v", err)
		cancel()
		os.Exit(1)
	}

	api := NewAPIServer(cfg, logger, tunnel, proxyMgr, tracker, dns)
	if err := api.Start(); err != nil {
		logger.Error("API: %v", err)
		cancel()
		os.Exit(1)
	}

	logger.Info("Semua layanan siap. Buka http://%s/ di browser.", cfg.API.Bind)

	select {
	case <-sig:
		logger.Info("Shutdown signal diterima")
	case <-tmDone:
		logger.Warn("Tunnel loop berhenti")
	}

	cancel()
	<-tmDone
	router.Close()
	dns.Close()
	api.Close()
	tracker.CloseAll()
	logger.Info("Bye")
}