package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

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