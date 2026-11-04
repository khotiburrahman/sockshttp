package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
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
	return dialSSHWithTimeout(c, network, addr, 15*time.Second)
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

		// Timeout khusus untuk baca header response proxy
		conn.SetReadDeadline(time.Now().Add(10 * time.Second))

		// Baca baris pertama (status line)
		first, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("proxy tidak merespon dalam 10s: %w", err)
		}
		t.logger.Info("Proxy response: %s", strings.TrimRight(first, "\r\n"))

		// Baca sisa header — tapi LENIENT: jangan fail kalau ada error
		// (SocksHttp tidak peduli header lengkap atau tidak, karena server
		//  mungkin sudah langsung masuk mode WebSocket passthrough)
		for i := 0; i < 100; i++ {
			line, err := br.ReadString('\n')
			if err != nil {
				// EOF / timeout — lanjut saja
				t.logger.Debug("Header loop selesai lebih awal: %v", err)
				break
			}
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed == "" {
				break
			}
			t.logger.Debug("Proxy header: %s", trimmed)
		}

		// Reset deadline sebelum handshake SSH
		conn.SetReadDeadline(time.Time{})

		t.logger.Info("Ignoring proxy response, forcing connection..")
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
	maxJitter := int64(*backoff)/2 + 1
	nBig, _ := rand.Int(rand.Reader, big.NewInt(maxJitter))
	jitter := time.Duration(nBig.Int64())
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
			conn, err := dialSSHWithTimeout(c, "tcp", t.cfg.HealthCheck.Target, t.cfg.HealthCheck.Timeout)
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