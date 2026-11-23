package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
)


// DNSResolver — abstraksi upstream DNS.
type DNSResolver interface {
	Resolve(ctx context.Context, query []byte) ([]byte, error)
	Close() error
}

// ---------- TunnelResolver (mode: forward, existing behavior) ----------
type TunnelResolver struct {
	tunnel   *SSHTunnel
	upstream string
}

func NewTunnelResolver(t *SSHTunnel, upstream string) *TunnelResolver {
	return &TunnelResolver{tunnel: t, upstream: upstream}
}

func (r *TunnelResolver) Resolve(ctx context.Context, q []byte) ([]byte, error) {
	if r.tunnel == nil || r.tunnel.State() != "connected" {
		return nil, ErrTunnelNotReady
	}
	conn, err := r.tunnel.Dial("tcp", r.upstream)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	dl, ok := ctx.Deadline()
	if !ok {
		dl = time.Now().Add(5 * time.Second)
	}
	conn.SetDeadline(dl)

	msg := make([]byte, 2+len(q))
	msg[0] = byte(len(q) >> 8)
	msg[1] = byte(len(q))
	copy(msg[2:], q)
	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}
	n := int(lenBuf[0])<<8 | int(lenBuf[1])
	if n == 0 || n > 8192 {
		return nil, fmt.Errorf("bad resp len %d", n)
	}
	resp := make([]byte, n)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (r *TunnelResolver) Close() error { return nil }

// ---------- DoHResolver ----------
type DoHResolver struct {
	url    string
	method string
	client *http.Client
}

func NewDoHResolver(url, method string, tunnel *SSHTunnel, timeout time.Duration) *DoHResolver {
	if method == "" {
		method = "POST"
	}
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if tunnel == nil || tunnel.State() != "connected" {
				return nil, ErrTunnelNotReady
			}
			return tunnel.Dial(network, addr)
		},
	}
	return &DoHResolver{
		url:    url,
		method: strings.ToUpper(method),
		client: &http.Client{Transport: tr, Timeout: timeout},
	}
}

func (r *DoHResolver) Resolve(ctx context.Context, q []byte) ([]byte, error) {
	var req *http.Request
	var err error
	if r.method == "GET" {
		b64 := base64.RawURLEncoding.EncodeToString(q)
		sep := "?"
		if strings.Contains(r.url, "?") {
			sep = "&"
		}
		req, err = http.NewRequestWithContext(ctx, "GET", r.url+sep+"dns="+b64, nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, "POST", r.url, bytes.NewReader(q))
		if err == nil {
			req.Header.Set("Content-Type", "application/dns-message")
		}
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-message")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (r *DoHResolver) Close() error {
	if r.client != nil && r.client.Transport != nil {
		if tr, ok := r.client.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
	return nil
}

// ---------- DoQResolver ----------
// Catatan: DoQ jalan LANGSUNG (tidak via tunnel) karena SSH hanya forward TCP.
type DoQResolver struct {
	server  string
	quicCfg *quic.Config
	tlsCfg  *tls.Config
}

func NewDoQResolver(server string, tlsInsecure bool) *DoQResolver {
	host := server
	if h, _, err := net.SplitHostPort(server); err == nil {
		host = h
	}
	return &DoQResolver{
		server: server,
		quicCfg: &quic.Config{
			HandshakeIdleTimeout: 5 * time.Second,
			MaxIdleTimeout:       30 * time.Second,
		},
		tlsCfg: &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: tlsInsecure,
			NextProtos:         []string{"doq"},
			MinVersion:         tls.VersionTLS13,
		},
	}
}

func (r *DoQResolver) Resolve(ctx context.Context, q []byte) ([]byte, error) {
	conn, err := quic.DialAddr(ctx, r.server, r.tlsCfg, r.quicCfg)
	if err != nil {
		return nil, err
	}
	defer conn.CloseWithError(0, "")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	msg := make([]byte, 2+len(q))
	msg[0] = byte(len(q) >> 8)
	msg[1] = byte(len(q))
	copy(msg[2:], q)
	if _, err := stream.Write(msg); err != nil {
		return nil, err
	}
	if err := stream.Close(); err != nil {
		return nil, err
	}

	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(stream, lenBuf); err != nil {
		return nil, err
	}
	n := int(lenBuf[0])<<8 | int(lenBuf[1])
	if n == 0 || n > 8192 {
		return nil, fmt.Errorf("bad DoQ resp len %d", n)
	}
	resp := make([]byte, n)
	if _, err := io.ReadFull(stream, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (r *DoQResolver) Close() error { return nil }