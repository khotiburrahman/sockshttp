package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

const tunnelDialTimeout = 5 * time.Second
const warmupDialTimeout = 3 * time.Second

var ErrNoSSHAccounts = errors.New("tidak ada akun SSH terdaftar")

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

func dialSSHWithTimeout(client *ssh.Client, network, addr string, timeout time.Duration) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		conn, err := client.Dial(network, addr)
		resultCh <- result{conn, err}
	}()
	select {
	case r := <-resultCh:
		return r.conn, r.err
	case <-time.After(timeout):
		go func() {
			select {
			case r := <-resultCh:
				if r.conn != nil {
					r.conn.Close()
				}
			case <-time.After(30 * time.Second):
			}
		}()
		return nil, fmt.Errorf("SSH dial timeout setelah %s ke %s", timeout, addr)
	}
}

// isAuthError — deteksi error yang menandakan kredensial salah (deterministik).
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "unable to authenticate") ||
		strings.Contains(s, "no supported methods remain")
}

// AccountInfo — representasi akun SSH untuk API/UI.
type AccountInfo struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Current  bool   `json:"current"`
}

type SSHTunnel struct {
	cfg           *Config
	logger        *Logger
	mu            sync.RWMutex
	client        *ssh.Client
	state         atomic.Value
	disconCh      chan error
	reconnect     atomic.Int64
	lastReconnect atomic.Int64
	lastErr       atomic.Value
	connected     atomic.Bool
	dynamicRules  *DynamicRules

	reconnMu         sync.Mutex
	reconnectHistory []int64

	proxyPortMu sync.Mutex
	currentPort int
	portIndex   int
	connHistory []time.Time

	// Rotasi multi-akun SSH
	rotMu    sync.Mutex
	rotIndex int
	rotFails int

	// Interrupt backoff untuk manual switch
	switchCh chan struct{}

	// Callback dipanggil setelah client ditutup (untuk force-close semua conn user-side)
	onClientClosed atomic.Value
}

// SetOnClientClosed mendaftarkan callback yang dipanggil setiap kali SSH client
// di-close (putus, rotasi akun, manual switch, atau "reject-all" dari UI).
func (t *SSHTunnel) SetOnClientClosed(fn func(reason string)) {
	t.onClientClosed.Store(fn)
}

func (t *SSHTunnel) fireClientClosed(reason string) {
	if v := t.onClientClosed.Load(); v != nil {
		if fn, ok := v.(func(string)); ok && fn != nil {
			fn(reason)
		}
	}
}

func NewSSHTunnel(cfg *Config, logger *Logger, dynRules *DynamicRules) *SSHTunnel {
	t := &SSHTunnel{
		cfg: cfg, logger: logger, disconCh: make(chan error, 1),
		dynamicRules: dynRules, switchCh: make(chan struct{}, 1),
	}
	t.state.Store("disconnected")

	if n := len(cfg.SSHAccounts); n > 1 {
		logger.Info("[ssh] multi-account aktif: %d akun, rotate tiap %d kegagalan per akun",
			n, cfg.MaxRetryPerAccount)
	}

	_, ports, _, _ := dynRules.GetProxyRotate()
	t.proxyPortMu.Lock()
	if len(ports) > 0 {
		t.currentPort = ports[0]
		t.portIndex = 0
	} else {
		t.currentPort = cfg.ProxyPort
	}
	t.proxyPortMu.Unlock()

	return t
}

func (t *SSHTunnel) GetCurrentPort() int {
	t.proxyPortMu.Lock()
	defer t.proxyPortMu.Unlock()
	return t.currentPort
}

func (t *SSHTunnel) trackReconnectAndRotate() {
	enabled, ports, maxRetries, window := t.dynamicRules.GetProxyRotate()
	if !enabled || len(ports) <= 1 {
		return
	}
	t.proxyPortMu.Lock()
	defer t.proxyPortMu.Unlock()

	now := time.Now()
	var validHistory []time.Time
	for _, ts := range t.connHistory {
		if now.Sub(ts) <= window {
			validHistory = append(validHistory, ts)
		}
	}
	validHistory = append(validHistory, now)
	t.connHistory = validHistory

	if len(t.connHistory) >= maxRetries {
		oldPort := t.currentPort
		t.portIndex = (t.portIndex + 1) % len(ports)
		t.currentPort = ports[t.portIndex]
		t.logger.Warn("[tunnel] Terdeteksi %d kegagalan dalam %v. Rotasi port proxy: %d -> %d",
			len(t.connHistory), window, oldPort, t.currentPort)
		t.connHistory = nil
	}
}

func (t *SSHTunnel) State() string     { return t.state.Load().(string) }
func (t *SSHTunnel) setState(s string) { t.state.Store(s); t.logger.Info("State: %s", s) }
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
	return dialSSHWithTimeout(c, network, addr, tunnelDialTimeout)
}

// --- Rotator & UI API -----------------------------------------------------

func (t *SSHTunnel) currentAccountWithIndex() (int, SSHAccount) {
	t.rotMu.Lock()
	defer t.rotMu.Unlock()

	if len(t.cfg.SSHAccounts) == 0 {
		return -1, SSHAccount{
			Server: t.cfg.Server, ServerPort: t.cfg.ServerPort,
			Username: t.cfg.Username, Password: t.cfg.Password, KeyPath: t.cfg.KeyPath,
		}
	}
	if t.rotIndex >= len(t.cfg.SSHAccounts) || t.rotIndex < 0 {
		t.rotIndex = 0
	}
	return t.rotIndex, t.cfg.SSHAccounts[t.rotIndex]
}

// ListAccounts mengembalikan snapshot akun-akun SSH untuk UI.
func (t *SSHTunnel) ListAccounts() []AccountInfo {
	t.rotMu.Lock()
	defer t.rotMu.Unlock()

	out := make([]AccountInfo, 0, len(t.cfg.SSHAccounts))
	for i, a := range t.cfg.SSHAccounts {
		name := a.Name
		if name == "" {
			name = fmt.Sprintf("Account-%d", i+1)
		}
		out = append(out, AccountInfo{
			Index:    i,
			Name:     name,
			Server:   a.Server,
			Port:     a.ServerPort,
			Username: a.Username,
			Current:  i == t.rotIndex,
		})
	}
	return out
}

// SwitchToAccount memindahkan akun aktif secara manual, memutus koneksi SSH
// saat ini, dan langsung reconnect ke akun baru (tanpa menunggu backoff).
func (t *SSHTunnel) SwitchToAccount(idx int) error {
	t.rotMu.Lock()
	if len(t.cfg.SSHAccounts) == 0 {
		t.rotMu.Unlock()
		return ErrNoSSHAccounts
	}
	if idx < 0 || idx >= len(t.cfg.SSHAccounts) {
		total := len(t.cfg.SSHAccounts)
		t.rotMu.Unlock()
		return fmt.Errorf("index akun tidak valid: %d (range 0..%d)", idx, total-1)
	}
	oldIdx := t.rotIndex
	if oldIdx == idx {
		t.rotMu.Unlock()
		return nil
	}
	t.rotIndex = idx
	t.rotFails = 0
	acct := t.cfg.SSHAccounts[idx]
	t.rotMu.Unlock()

	t.logger.Warn("[ssh-rotate] MANUAL switch akun %d -> %d (%s@%s:%d)",
		oldIdx+1, idx+1, acct.Username, acct.Server, acct.ServerPort)

	// Interrupt backoff jika sedang tidur
	select {
	case t.switchCh <- struct{}{}:
	default:
	}

	// Paksa putus koneksi saat ini (jika ada) → trigger reconnect
	t.mu.Lock()
	if t.client != nil {
		t.client.Close()
	}
	t.mu.Unlock()

	return nil
}

// RejectAllConnections menutup SEMUA koneksi user-side (SOCKS5/DNS/Router)
// tanpa memutus SSH. Bisa dipakai sebagai "panic button" ketika ada koneksi macet.
func (t *SSHTunnel) RejectAllConnections(reason string) {
	if reason == "" {
		reason = "manual-reject-all"
	}
	t.logger.Warn("[tunnel] reject-all dari UI (reason=%s)", reason)
	t.fireClientClosed(reason)
}

func (t *SSHTunnel) onDialSuccess() {
	t.rotMu.Lock()
	defer t.rotMu.Unlock()
	t.rotFails = 0
}

// onDialFailure mengembalikan true jika akun sudah dirotasi.
func (t *SSHTunnel) onDialFailure(acctIdx int, err error) (rotated bool) {
	t.rotMu.Lock()
	defer t.rotMu.Unlock()

	if acctIdx < 0 || len(t.cfg.SSHAccounts) <= 1 {
		return false
	}

	// Manual switch terjadi selama dial — abaikan failure dari akun lama
	if t.rotIndex != acctIdx {
		t.logger.Debug("[ssh-rotate] kegagalan dari akun lama %d diabaikan (rotIndex sekarang %d)",
			acctIdx, t.rotIndex)
		return false
	}

	oldIdx := t.rotIndex
	prev := t.cfg.SSHAccounts[oldIdx]

	// Auth failure = deterministik → rotate sekarang juga
	if isAuthError(err) {
		t.rotFails = 0
		t.rotIndex = (t.rotIndex + 1) % len(t.cfg.SSHAccounts)
		next := t.cfg.SSHAccounts[t.rotIndex]
		t.logger.Warn("[ssh-rotate] AUTH gagal akun %d (%s@%s) → rotate langsung ke akun %d (%s@%s): %v",
			oldIdx+1, prev.Username, prev.Server,
			t.rotIndex+1, next.Username, next.Server, err)
		return true
	}

	max := t.cfg.MaxRetryPerAccount
	if max <= 0 {
		max = 3
	}

	t.rotFails++
	if t.rotFails < max {
		t.logger.Warn("[ssh-rotate] akun %d/%d gagal (%d/%d) – tetap pakai akun ini: %v",
			t.rotIndex+1, len(t.cfg.SSHAccounts), t.rotFails, max, err)
		return false
	}

	t.rotFails = 0
	t.rotIndex = (t.rotIndex + 1) % len(t.cfg.SSHAccounts)
	next := t.cfg.SSHAccounts[t.rotIndex]
	t.logger.Warn("[ssh-rotate] akun %d (%s@%s) gagal %dx → rotate ke akun %d (%s@%s)",
		oldIdx+1, prev.Username, prev.Server, max,
		t.rotIndex+1, next.Username, next.Server)
	return true
}

// --- Autentikasi & dial ---------------------------------------------------

func (t *SSHTunnel) buildAuth(acct SSHAccount) ([]ssh.AuthMethod, error) {
	var auths []ssh.AuthMethod
	if acct.KeyPath != "" {
		data, err := os.ReadFile(acct.KeyPath)
		if err == nil {
			var signer ssh.Signer
			if acct.Password != "" {
				signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(acct.Password))
			} else {
				signer, err = ssh.ParsePrivateKey(data)
			}
			if err == nil {
				auths = append(auths, ssh.PublicKeys(signer))
			}
		}
	}
	if acct.Password != "" {
		auths = append(auths, ssh.Password(acct.Password))
	}
	if len(auths) == 0 {
		return nil, errors.New("tidak ada auth")
	}
	return auths, nil
}

func (t *SSHTunnel) resolveHost(hostname string) []string {
	if net.ParseIP(hostname) != nil {
		return []string{hostname}
	}
	ips, err := net.LookupHost(hostname)
	if err == nil && len(ips) > 0 {
		return ips
	}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, "udp", "1.1.1.1:53")
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err = resolver.LookupHost(ctx, hostname)
	if err == nil && len(ips) > 0 {
		return ips
	}
	if len(t.cfg.ProxyHostFallback) > 0 {
		return t.cfg.ProxyHostFallback
	}
	return []string{hostname}
}

func (t *SSHTunnel) openRawConn(acct SSHAccount) (net.Conn, error) {
	var payload string
	var readResponse bool
	var targetHost string
	var targetPort int

	if t.cfg.TunnelType == "proxy" {
		targetHost = t.cfg.ProxyHost
		targetPort = t.GetCurrentPort()
		if t.cfg.UseDefaultPayload {
			payload = fmt.Sprintf("CONNECT %s:%d HTTP/1.0\r\n\r\n", acct.Server, acct.ServerPort)
		} else {
			payload = formatPayload(acct.Server, acct.ServerPort, t.cfg.CustomPayload)
		}
		readResponse = true
	} else {
		targetHost = acct.Server
		targetPort = acct.ServerPort
		if !t.cfg.UseDefaultPayload {
			payload = formatPayload(acct.Server, acct.ServerPort, t.cfg.CustomPayload)
		}
	}

	ips := t.resolveHost(targetHost)
	var conn net.Conn
	var lastErr error

	for _, ip := range ips {
		dialAddr := net.JoinHostPort(ip, strconv.Itoa(targetPort))
		conn, lastErr = net.DialTimeout("tcp", dialAddr, 10*time.Second)
		if lastErr == nil {
			break
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("gagal dial target: %w", lastErr)
	}

	if t.cfg.NoDelay {
		if tc, ok := conn.(*net.TCPConn); ok {
			tc.SetNoDelay(true)
		}
	}

	if payload != "" {
		if err := injectSplit(payload, conn); err != nil {
			conn.Close()
			return nil, err
		}
	}

	if readResponse {
		br := bufio.NewReader(conn)
		conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		for i := 0; i < 10; i++ {
			peek, err := br.Peek(4)
			if err != nil {
				break
			}
			if string(peek) == "SSH-" {
				break
			}
			if string(peek) != "HTTP" {
				break
			}

			first, err := br.ReadString('\n')
			if err != nil {
				conn.Close()
				return nil, fmt.Errorf("proxy tidak merespon: %w", err)
			}
			parts := strings.SplitN(strings.TrimRight(first, "\r\n"), " ", 3)
			if len(parts) < 2 {
				conn.Close()
				return nil, fmt.Errorf("malformed HTTP status")
			}
			statusCode := parts[1]
			is1xx := strings.HasPrefix(statusCode, "1")
			is204or304 := statusCode == "204" || statusCode == "304"
			contentLength := int64(-1)
			isChunked := false
			hasConnectionClose := false

			for j := 0; j < 200; j++ {
				line, err := br.ReadString('\n')
				if err != nil {
					break
				}
				trimmed := strings.TrimRight(line, "\r\n")
				if trimmed == "" {
					break
				}
				lower := strings.ToLower(trimmed)
				if strings.HasPrefix(lower, "content-length:") {
					v := strings.TrimSpace(trimmed[len("content-length:"):])
					if n, e := strconv.ParseInt(v, 10, 64); e == nil {
						contentLength = n
					}
				}
				if strings.HasPrefix(lower, "transfer-encoding:") && strings.Contains(lower, "chunked") {
					isChunked = true
				}
				if strings.HasPrefix(lower, "connection:") && strings.Contains(lower, "close") {
					hasConnectionClose = true
				}
			}

			if is1xx || is204or304 {
				continue
			}
			if contentLength > 0 {
				io.CopyN(io.Discard, br, contentLength)
			} else if isChunked {
				for j := 0; j < 10000; j++ {
					sizeLine, err := br.ReadString('\n')
					if err != nil {
						break
					}
					sizeLine = strings.TrimSpace(sizeLine)
					if idx := strings.IndexByte(sizeLine, ';'); idx >= 0 {
						sizeLine = sizeLine[:idx]
					}
					if sizeLine == "" {
						continue
					}
					chunkSize, err := strconv.ParseInt(sizeLine, 16, 64)
					if err != nil {
						break
					}
					if chunkSize == 0 {
						for k := 0; k < 20; k++ {
							tl, terr := br.ReadString('\n')
							if terr != nil || strings.TrimRight(tl, "\r\n") == "" {
								break
							}
						}
						break
					}
					io.CopyN(io.Discard, br, chunkSize)
					br.ReadString('\n')
				}
			} else if hasConnectionClose {
				io.Copy(io.Discard, br)
			}
		}
		conn.SetReadDeadline(time.Time{})
		return &bufioConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}

type bufioConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufioConn) Read(b []byte) (int, error) { return c.r.Read(b) }

func (t *SSHTunnel) dialOnce() (int, error) {
	acctIdx, acct := t.currentAccountWithIndex()

	auths, err := t.buildAuth(acct)
	if err != nil {
		return acctIdx, err
	}
	sshCfg := &ssh.ClientConfig{
		User:            acct.Username,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}

	raw, err := t.openRawConn(acct)
	if err != nil {
		return acctIdx, err
	}

	mon := &monitoredConn{Conn: raw, onClose: func(e error) {
		select {
		case t.disconCh <- e:
		default:
		}
	}}

	addr := net.JoinHostPort(acct.Server, strconv.Itoa(acct.ServerPort))
	sshConn, chans, reqs, err := ssh.NewClientConn(mon, addr, sshCfg)
	if err != nil {
		raw.Close()
		return acctIdx, fmt.Errorf("handshake SSH (%s@%s): %w", acct.Username, addr, err)
	}

	client := ssh.NewClient(sshConn, chans, reqs)

	warmupTarget := t.cfg.HealthCheck.Target
	if warmupTarget == "" {
		warmupTarget = "1.1.1.1:53"
	}
	var warmupErr error
	for attempt := 1; attempt <= 3; attempt++ {
		wc, werr := dialSSHWithTimeout(client, "tcp", warmupTarget, warmupDialTimeout)
		if werr == nil {
			wc.Close()
			warmupErr = nil
			break
		}
		warmupErr = werr
		if attempt < 3 {
			time.Sleep(300 * time.Millisecond)
		}
	}
	if warmupErr != nil {
		client.Close()
		return acctIdx, fmt.Errorf("warmup gagal 3x: %w", warmupErr)
	}

	t.mu.Lock()
	t.client = client
	t.mu.Unlock()

	// Race: user menekan "Switch" saat dial sedang berjalan?
	t.rotMu.Lock()
	nowIdx := t.rotIndex
	t.rotMu.Unlock()
	if nowIdx != acctIdx {
		t.logger.Info("[ssh-rotate] switch diminta saat dial berlangsung → putuskan koneksi ke akun lama")
		client.Close()
		t.mu.Lock()
		t.client = nil
		t.mu.Unlock()
		return acctIdx, fmt.Errorf("manual switch during dial")
	}

	t.connected.Store(true)
	t.logger.Info("SSH tersambung ke %s (user=%s)", addr, acct.Username)
	if t.cfg.Pinger > 0 {
		go t.pinger(client)
	}
	return acctIdx, nil
}

func (t *SSHTunnel) pinger(client *ssh.Client) {
	for {
		if !t.connected.Load() {
			return
		}
		minSec := t.cfg.Pinger
		maxSec := t.cfg.PingerMax
		if maxSec <= minSec {
			maxSec = minSec
		}

		var waitSec int
		if maxSec > minSec {
			nBig, _ := rand.Int(rand.Reader, big.NewInt(int64(maxSec-minSec)+1))
			waitSec = minSec + int(nBig.Int64())
		} else {
			waitSec = minSec
		}

		time.Sleep(time.Duration(waitSec) * time.Second)
		if !t.connected.Load() {
			return
		}

		done := make(chan error, 1)
		go func() {
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			done <- err
		}()

		select {
		case err := <-done:
			if err != nil {
				t.logger.Warn("Ping gagal → paksa reconnect: %v", err)
				client.Close()
				select {
				case t.disconCh <- err:
				default:
				}
				return
			}
		case <-time.After(5 * time.Second):
			err := fmt.Errorf("ping timeout 5s (koneksi mati diam-diam)")
			t.logger.Warn("%v → paksa reconnect", err)
			client.Close()
			select {
			case t.disconCh <- err:
			default:
			}
			return
		}
	}
}

func (t *SSHTunnel) cleanup() {
	t.mu.Lock()
	had := t.client != nil
	if t.client != nil {
		t.client.Close()
	}
	t.client = nil
	t.mu.Unlock()
	t.connected.Store(false)

	if had {
		t.fireClientClosed("tunnel-closed")
	}

	select {
	case <-t.disconCh:
	default:
	}
}

func (t *SSHTunnel) Run(ctx context.Context) {
	backoff := t.cfg.Reconnect.InitialDelay.D()
	consecutiveRotations := 0

	for {
		if ctx.Err() != nil {
			t.cleanup()
			return
		}

		t.setState("connecting")
		acctIdx, err := t.dialOnce()
		if err != nil {
			t.lastErr.Store(err.Error())
			rotated := t.onDialFailure(acctIdx, err)
			t.trackReconnectAndRotate()

			if rotated {
				t.cleanup()
				backoff = t.cfg.Reconnect.InitialDelay.D()
				consecutiveRotations++

				if consecutiveRotations >= len(t.cfg.SSHAccounts) {
					t.logger.Warn("[ssh-rotate] seluruh %d akun gagal dalam 1 putaran, kembali pakai backoff",
						len(t.cfg.SSHAccounts))
					consecutiveRotations = 0
					t.waitBackoff(ctx, &backoff)
				} else {
					select {
					case <-ctx.Done():
						t.cleanup()
						return
					case <-time.After(500 * time.Millisecond):
					}
				}
				continue
			}

			t.waitBackoff(ctx, &backoff)
			continue
		}
		t.onDialSuccess()
		consecutiveRotations = 0

		select {
		case err := <-t.disconCh:
			t.logger.Debug("[tunnel] drain: %v", err)
		default:
		}

		t.setState("connected")
		t.lastErr.Store("")
		backoff = t.cfg.Reconnect.InitialDelay.D()

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
			now := time.Now().Unix()
			t.lastReconnect.Store(now)

			t.reconnMu.Lock()
			t.reconnectHistory = append(t.reconnectHistory, now)
			if len(t.reconnectHistory) > 50 {
				t.reconnectHistory = t.reconnectHistory[1:]
			}
			t.reconnMu.Unlock()

			t.trackReconnectAndRotate()
		}

		if !t.cfg.Reconnect.Enabled {
			return
		}
		t.waitBackoff(ctx, &backoff)
	}
}

func (t *SSHTunnel) waitBackoff(ctx context.Context, backoff *time.Duration) {
	maxJitter := int64(*backoff)/2 + 1
	nBig, _ := rand.Int(rand.Reader, big.NewInt(maxJitter))
	wait := *backoff + time.Duration(nBig.Int64())
	t.logger.Info("Reconnect dalam %s...", wait)
	select {
	case <-ctx.Done():
		return
	case <-t.switchCh:
		// Interupsi manual switch
		*backoff = t.cfg.Reconnect.InitialDelay.D()
		t.logger.Info("[ssh-rotate] backoff diinterupsi oleh manual switch")
		return
	case <-time.After(wait):
	}
	*backoff *= 2
	if *backoff > t.cfg.Reconnect.MaxDelay.D() {
		*backoff = t.cfg.Reconnect.MaxDelay.D()
	}
}

func (t *SSHTunnel) StartHealthLoop(ctx context.Context) {
	if !t.cfg.HealthCheck.Enabled {
		return
	}
	go func() {
		failures := 0
		tk := time.NewTicker(t.cfg.HealthCheck.Interval.D())
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
			conn, err := dialSSHWithTimeout(c, "tcp", t.cfg.HealthCheck.Target, t.cfg.HealthCheck.Timeout.D())
			if err != nil {
				failures++
				t.logger.Warn("Health check gagal (%d/%d): %v", failures, t.cfg.HealthCheck.FailureThreshold, err)
				if failures >= t.cfg.HealthCheck.FailureThreshold {
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