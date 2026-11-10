package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// RealIPCache — IP asli → domain, diisi dari DNS forward.
// Dipakai router untuk reverse lookup hostname (bukan Fake-IP).
// ============================================================
type realIPEntry struct {
	domain string
	expire time.Time
}

type RealIPCache struct {
	mu      sync.RWMutex
	data    map[string]realIPEntry
	hits    atomic.Int64
	misses  atomic.Int64
	maxSize int
}

func NewRealIPCache() *RealIPCache {
	return &RealIPCache{data: map[string]realIPEntry{}, maxSize: 20000}
}

func (c *RealIPCache) Set(ip, domain string, ttl time.Duration) {
	if ip == "" || domain == "" {
		return
	}
	if ttl < 30*time.Second {
		ttl = 30 * time.Second
	}
	if ttl > 300*time.Second {
		ttl = 300 * time.Second
	}
	c.mu.Lock()
	c.data[ip] = realIPEntry{domain: domain, expire: time.Now().Add(ttl)}
	if len(c.data) > c.maxSize {
		for k := range c.data {
			delete(c.data, k)
			break
		}
	}
	c.mu.Unlock()
}

func (c *RealIPCache) Lookup(ip string) (string, bool) {
	c.mu.RLock()
	e, ok := c.data[ip]
	c.mu.RUnlock()
	if !ok {
		c.misses.Add(1)
		return "", false
	}
	if time.Now().After(e.expire) {
		c.mu.Lock()
		delete(c.data, ip)
		c.mu.Unlock()
		c.misses.Add(1)
		return "", false
	}
	c.hits.Add(1)
	return e.domain, true
}

func (c *RealIPCache) Stats() map[string]int64 {
	c.mu.RLock()
	size := len(c.data)
	c.mu.RUnlock()
	return map[string]int64{
		"size":   int64(size),
		"hits":   c.hits.Load(),
		"misses": c.misses.Load(),
	}
}

func (c *RealIPCache) PruneLoop(ctx context.Context) {
	tk := time.NewTicker(60 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			now := time.Now()
			c.mu.Lock()
			for k, e := range c.data {
				if now.After(e.expire) {
					delete(c.data, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

// ============================================================
// dnsForwardCache — cache response yang sudah di-strip AAAA
// ============================================================
type cachedDNS struct {
	resp   []byte
	expire time.Time
}

type dnsForwardCache struct {
	mu   sync.Mutex
	data map[string]cachedDNS
}

func newDNSForwardCache() *dnsForwardCache {
	return &dnsForwardCache{data: map[string]cachedDNS{}}
}

func (c *dnsForwardCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expire) {
		delete(c.data, key)
		return nil, false
	}
	out := make([]byte, len(e.resp))
	copy(out, e.resp)
	return out, true
}

func (c *dnsForwardCache) Set(key string, resp []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = cachedDNS{resp: append([]byte(nil), resp...), expire: time.Now().Add(ttl)}
	if len(c.data) > 2000 {
		for k := range c.data {
			delete(c.data, k)
			if len(c.data) < 1000 {
				break
			}
		}
	}
}

// ============================================================
// DNSServer — forward-only
// ============================================================
type DNSServer struct {
	cfg       *Config
	logger    *Logger
	realCache *RealIPCache
	tunnel    *SSHTunnel
	udpConn   *net.UDPConn
	tcpLn     net.Listener
	closed    atomic.Bool
	wg        sync.WaitGroup
	queries   atomic.Int64
	errors    atomic.Int64
	fwdCache  *dnsForwardCache
}

func NewDNSServer(cfg *Config, logger *Logger, realCache *RealIPCache, tunnel *SSHTunnel) *DNSServer {
	return &DNSServer{
		cfg:       cfg,
		logger:    logger,
		realCache: realCache,
		tunnel:    tunnel,
		fwdCache:  newDNSForwardCache(),
	}
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
	d.logger.Info("DNS listen di %s (forward via tunnel, upstream %s)", d.cfg.DNS.Bind, d.cfg.DNS.Upstream)
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
			if r := d.handleQuery(q); r != nil {
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
		d.errors.Add(1)
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
		if l&0xC0 == 0xC0 {
			pos += 2
			break
		}
		if l > 63 || pos+l+1 > len(q) {
			d.errors.Add(1)
			return nil
		}
		labels = append(labels, string(q[pos+1:pos+1+l]))
		pos += l + 1
	}
	if pos+4 > len(q) {
		d.errors.Add(1)
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[pos : pos+2])
	qclass := binary.BigEndian.Uint16(q[pos+2 : pos+4])
	pos += 4
	questionEnd := pos
	domain := strings.Join(labels, ".")

	// AAAA: tidak ada IPv6 di tunnel ini → balas empty NOERROR tanpa forward.
	// Ini memaksa client Happy Eyeballs fallback ke IPv4.
	if qtype == 28 {
		d.logger.Debug("DNS: %s AAAA → empty (IPv4-only tunnel)", domain)
		return d.buildEmptyResponse(q, questionEnd)
	}

	return d.forwardQuery(q, questionEnd, domain, qtype, qclass)
}

func (d *DNSServer) forwardQuery(q []byte, questionEnd int, domain string, qtype, qclass uint16) []byte {
	cacheKey := fmt.Sprintf("%s|%d|%d", strings.ToLower(domain), qtype, qclass)
	if cached, ok := d.fwdCache.Get(cacheKey); ok {
		if len(cached) >= 2 {
			cached[0] = q[0]
			cached[1] = q[1]
		}
		return cached
	}

	if d.tunnel == nil || d.tunnel.State() != "connected" {
		d.logger.Debug("DNS forward: tunnel belum siap, skip %s", domain)
		d.errors.Add(1)
		return d.buildErrorResponse(q, questionEnd, 2) // SERVFAIL
	}

	conn, err := d.tunnel.Dial("tcp", d.cfg.DNS.Upstream)
	if err != nil {
		d.logger.Warn("DNS forward dial ke %s gagal: %v", d.cfg.DNS.Upstream, err)
		d.errors.Add(1)
		return d.buildErrorResponse(q, questionEnd, 2)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	msg := make([]byte, 2+len(q))
	msg[0] = byte(len(q) >> 8)
	msg[1] = byte(len(q))
	copy(msg[2:], q)
	if _, err := conn.Write(msg); err != nil {
		d.errors.Add(1)
		return d.buildErrorResponse(q, questionEnd, 2)
	}
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		d.errors.Add(1)
		return d.buildErrorResponse(q, questionEnd, 2)
	}
	respLen := int(lenBuf[0])<<8 | int(lenBuf[1])
	if respLen == 0 || respLen > 8192 {
		d.errors.Add(1)
		return d.buildErrorResponse(q, questionEnd, 2)
	}
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(conn, resp); err != nil {
		d.errors.Add(1)
		return d.buildErrorResponse(q, questionEnd, 2)
	}

	// Extract A records dari answer section, isi RealIPCache
	minTTL := d.extractACache(resp, domain)

	if len(resp) >= 4 && (resp[3]&0x0F) == 0 {
		ttl := minTTL
		if ttl < 30 {
			ttl = 30
		}
		if ttl > 300 {
			ttl = 300
		}
		d.fwdCache.Set(cacheKey, resp, time.Duration(ttl)*time.Second)
	}

	d.logger.Debug("DNS forward: %s (type %d) → %d bytes", domain, qtype, respLen)
	return resp
}

// extractACache parse answer section saja, ambil A record, isi RealIPCache.
// Return minTTL. Tidak memodifikasi resp (AAAA tidak ada di response A).
func (d *DNSServer) extractACache(resp []byte, domain string) uint32 {
	if len(resp) < 12 {
		return 0
	}
	ancount := int(binary.BigEndian.Uint16(resp[6:8]))
	if ancount == 0 {
		return 0
	}

	pos := 12
	for pos < len(resp) {
		l := int(resp[pos])
		if l == 0 {
			pos++
			break
		}
		if l&0xC0 == 0xC0 {
			pos += 2
			break
		}
		pos += l + 1
	}
	if pos+4 > len(resp) {
		return 0
	}
	pos += 4 // qtype + qclass
	cur := pos

	var minTTL uint32
	for i := 0; i < ancount && cur < len(resp); i++ {
		// skip name (compressed or plain)
		for cur < len(resp) {
			l := int(resp[cur])
			if l == 0 {
				cur++
				break
			}
			if l&0xC0 == 0xC0 {
				cur += 2
				break
			}
			cur += l + 1
		}
		if cur+10 > len(resp) {
			break
		}
		rtype := binary.BigEndian.Uint16(resp[cur : cur+2])
		ttl := binary.BigEndian.Uint32(resp[cur+4 : cur+8])
		rdlen := int(binary.BigEndian.Uint16(resp[cur+8 : cur+10]))
		rdataStart := cur + 10
		rdataEnd := rdataStart + rdlen
		if rdataEnd > len(resp) {
			break
		}
		if rtype == 1 && rdlen == 4 {
			ip := net.IPv4(resp[rdataStart], resp[rdataStart+1], resp[rdataStart+2], resp[rdataStart+3]).String()
			d.realCache.Set(ip, domain, time.Duration(ttl)*time.Second)
			if minTTL == 0 || ttl < minTTL {
				minTTL = ttl
			}
		}
		cur = rdataEnd
	}
	return minTTL
}

func (d *DNSServer) buildEmptyResponse(q []byte, questionEnd int) []byte {
	resp := make([]byte, questionEnd)
	copy(resp, q[:questionEnd])
	resp[2] = 0x81 // QR=1, RD=1
	resp[3] = 0x80 // RA=1, RCODE=0
	for i := 6; i < 12; i++ {
		resp[i] = 0
	}
	return resp
}

func (d *DNSServer) buildErrorResponse(q []byte, questionEnd, rcode int) []byte {
	resp := make([]byte, questionEnd)
	copy(resp, q[:questionEnd])
	resp[2] = 0x81
	resp[3] = byte(0x80 | rcode)
	for i := 6; i < 12; i++ {
		resp[i] = 0
	}
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