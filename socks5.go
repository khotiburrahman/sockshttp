package main

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type SOCKS5Server struct {
	cfg      *Config
	logger   *Logger
	tunnel   *SSHTunnel
	proxyMgr *ProxyManager
	tracker  *ConnTracker
	ln       net.Listener
	closed   atomic.Bool
	wg       sync.WaitGroup
}

func NewSOCKS5Server(cfg *Config, logger *Logger, tunnel *SSHTunnel, pm *ProxyManager, tracker *ConnTracker) *SOCKS5Server {
	return &SOCKS5Server{
		cfg:      cfg,
		logger:   logger,
		tunnel:   tunnel,
		proxyMgr: pm,
		tracker:  tracker,
	}
}

func (s *SOCKS5Server) Start() error {
	addr := net.JoinHostPort(s.cfg.SOCKS5.Bind, strconv.Itoa(s.cfg.SOCKS5.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.ln = ln
	s.logger.Info("SOCKS5 listen di %s", addr)
	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

func (s *SOCKS5Server) acceptLoop() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			if s.closed.Load() {
				return
			}
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(c)
		}()
	}
}

func (s *SOCKS5Server) handleConn(c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(30 * time.Second))

	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil || hdr[0] != 0x05 {
		return
	}
	methods := make([]byte, int(hdr[1]))
	if _, err := io.ReadFull(c, methods); err != nil {
		return
	}
	if _, err := c.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil || req[0] != 0x05 || req[1] != 0x01 {
		return
	}

	var host string
	switch req[3] {
	case 0x01:
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 0x03:
		lb := make([]byte, 1)
		if _, err := io.ReadFull(c, lb); err != nil {
			return
		}
		name := make([]byte, int(lb[0]))
		if _, err := io.ReadFull(c, name); err != nil {
			return
		}
		host = string(name)
	case 0x04:
		b := make([]byte, 16)
		if _, err := io.ReadFull(c, b); err != nil {
			return
		}
		host = net.IP(b).String()
	default:
		return
	}

	pb := make([]byte, 2)
	if _, err := io.ReadFull(c, pb); err != nil {
		return
	}
	port := int(binary.BigEndian.Uint16(pb))

	// SOCKS5 selalu resolve via group DEFAULT.
	// Rule SRC-IP-CIDR tidak berlaku di sini — hanya untuk traffic REDIRECT
	// dari router karena butuh src IP asli client.
	target, chain := s.proxyMgr.Resolve("DEFAULT")
	s.logger.Debug("SOCKS5 %s:%d -> %s (%s)", host, port, target, chain)

	dest := net.JoinHostPort(host, strconv.Itoa(port))

	var remote net.Conn
	var err error

	switch target {
	case ProxyReject:
		c.Write([]byte{0x05, 0x02, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	case ProxyDirect:
		remote, err = net.DialTimeout("tcp", dest, 10*time.Second)
	case ProxyTunnel:
		remote, err = s.tunnel.Dial("tcp", dest)
	default:
		c.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	if err != nil {
		s.logger.Debug("SOCKS5 dial %s gagal: %v", dest, err)
		c.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer remote.Close()

	c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	c.SetDeadline(time.Time{})

	done := make(chan struct{}, 2)
	go func() {
		cw := &countingWriter{w: remote, global: &s.tracker.TotalUp}
		io.Copy(cw, c)
		if tc, ok := remote.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		cw := &countingWriter{w: c, global: &s.tracker.TotalDown}
		io.Copy(cw, remote)
		if tc, ok := c.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	<-done
	c.Close()
	remote.Close()
	<-done
}

func (s *SOCKS5Server) Close() {
	if s.closed.Swap(true) {
		return
	}
	if s.ln != nil {
		s.ln.Close()
	}
	s.wg.Wait()
}