package main

import (
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type fakeIPSnapshot struct {
	Version int         `json:"version"`
	NextIP  uint32      `json:"next_ip"`
	Entries [][2]string `json:"entries"`
}

type FakeIPPool struct {
	mu        sync.Mutex
	domain2IP map[string]string
	ip2domain map[string]string
	order     []string
	maxSize   int
	nextIP    uint32
	hits      atomic.Int64
	misses    atomic.Int64
	evictions atomic.Int64

	filePath string
	dirty    atomic.Bool
}

func NewFakeIPPool() *FakeIPPool {
	return &FakeIPPool{
		domain2IP: map[string]string{},
		ip2domain: map[string]string{},
		maxSize:   10000,
		nextIP:    (198 << 24) | (18 << 16) | 1,
	}
}

func (p *FakeIPPool) Load(path string) error {
	p.filePath = path
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	var snap fakeIPSnapshot
	if err := json.NewDecoder(gz).Decode(&snap); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if snap.Version != 1 {
		return fmt.Errorf("unsupported version %d", snap.Version)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.domain2IP = make(map[string]string, len(snap.Entries))
	p.ip2domain = make(map[string]string, len(snap.Entries))
	p.order = p.order[:0]
	for _, e := range snap.Entries {
		if len(e) != 2 || e[0] == "" || e[1] == "" {
			continue
		}
		p.domain2IP[e[0]] = e[1]
		p.ip2domain[e[1]] = e[0]
		p.order = append(p.order, e[0])
	}
	p.nextIP = snap.NextIP
	if (p.nextIP>>24)&0xFF != 198 || (p.nextIP>>16)&0xFF != 18 || p.nextIP == 0 {
		p.nextIP = (198 << 24) | (18 << 16) | 1
		for _, ip := range p.domain2IP {
			if v4 := net.ParseIP(ip).To4(); v4 != nil {
				v := uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
				if v >= p.nextIP {
					p.nextIP = v + 1
				}
			}
		}
	}
	return nil
}

func (p *FakeIPPool) Save() error {
	if p.filePath == "" {
		return nil
	}
	p.mu.Lock()
	snap := fakeIPSnapshot{
		Version: 1,
		NextIP:  p.nextIP,
		Entries: make([][2]string, 0, len(p.order)),
	}
	for _, d := range p.order {
		if ip, ok := p.domain2IP[d]; ok {
			snap.Entries = append(snap.Entries, [2]string{d, ip})
		}
	}
	p.mu.Unlock()

	tmp := p.filePath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(p.filePath), 0755); err != nil {
		return err
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	if err := json.NewEncoder(gz).Encode(snap); err != nil {
		gz.Close()
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("encode: %w", err)
	}
	if err := gz.Close(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("gzip close: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("file close: %w", err)
	}
	if err := os.Rename(tmp, p.filePath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	p.dirty.Store(false)
	return nil
}

func (p *FakeIPPool) DirtyLoop(ctx context.Context, logger *Logger) {
	if p.filePath == "" {
		return
	}
	tk := time.NewTicker(10 * time.Minute)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			if p.dirty.Load() {
				if err := p.Save(); err != nil {
					logger.Warn("fake-IP periodic save: %v", err)
				}
			}
		}
	}
}

func (p *FakeIPPool) Assign(domain string) string {
	domain = strings.ToLower(domain)
	p.mu.Lock()
	defer p.mu.Unlock()
	if ip, ok := p.domain2IP[domain]; ok {
		p.hits.Add(1)
		return ip
	}
	p.misses.Add(1)
	ip := fmt.Sprintf("%d.%d.%d.%d",
		(p.nextIP>>24)&0xFF, (p.nextIP>>16)&0xFF, (p.nextIP>>8)&0xFF, p.nextIP&0xFF)
	p.domain2IP[domain] = ip
	p.ip2domain[ip] = domain
	p.order = append(p.order, domain)
	p.nextIP++
	if (p.nextIP>>24)&0xFF != 198 || (p.nextIP>>16)&0xFF != 18 {
		p.nextIP = (198 << 24) | (18 << 16) | 1
	}
	for len(p.order) > p.maxSize {
		old := p.order[0]
		p.order = p.order[1:]
		if oldIP, ok := p.domain2IP[old]; ok {
			delete(p.ip2domain, oldIP)
			delete(p.domain2IP, old)
			p.evictions.Add(1)
		}
	}
	p.dirty.Store(true)
	return ip
}

func (p *FakeIPPool) LookupDomain(ip string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
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

func (p *FakeIPPool) Stats() map[string]int64 {
	p.mu.Lock()
	size := len(p.domain2IP)
	p.mu.Unlock()
	dirty := int64(0)
	if p.dirty.Load() {
		dirty = 1
	}
	return map[string]int64{
		"size":      int64(size),
		"hits":      p.hits.Load(),
		"misses":    p.misses.Load(),
		"evictions": p.evictions.Load(),
		"dirty":     dirty,
	}
}

// ============================================================
// DNS Forward Cache (untuk mode forward, hindari query berulang)
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
	c.data[key] = cachedDNS{
		resp:   append([]byte(nil), resp...),
		expire: time.Now().Add(ttl),
	}
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
// DNSServer
// ============================================================
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
	fwdCache *dnsForwardCache
}

func NewDNSServer(cfg *Config, logger *Logger, pool *FakeIPPool, tunnel *SSHTunnel) *DNSServer {
	return &DNSServer{
		cfg:      cfg,
		logger:   logger,
		pool:     pool,
		tunnel:   tunnel,
		fwdCache: newDNSForwardCache(),
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
	mode := d.cfg.DNS.Mode
	if mode == "" {
		mode = "fakeip"
	}
	d.logger.Info("DNS listen di %s (mode: %s)", d.cfg.DNS.Bind, mode)
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

	mode := d.cfg.DNS.Mode
	if mode == "" {
		mode = "fakeip"
	}

	// --- MODE FORWARD (DNS over TCP via tunnel) ---
	if mode == "forward" {
		return d.forwardQuery(q, questionEnd, domain, qtype, qclass)
	}

	// --- MODE FAKE-IP (default, existing) ---
	if qtype != 1 || qclass != 1 {
		return d.buildEmptyResponse(q, questionEnd, 0)
	}
	fakeIP := d.pool.Assign(domain)
	ip := net.ParseIP(fakeIP).To4()
	d.logger.Debug("DNS: %s → %s", domain, fakeIP)
	return d.buildAResponse(q, questionEnd, ip)
}

// forwardQuery meneruskan query ke upstream DNS via SSH Tunnel (TCP port 53).
func (d *DNSServer) forwardQuery(q []byte, questionEnd int, domain string, qtype, qclass uint16) []byte {
	// Cek Cache
	cacheKey := fmt.Sprintf("%s|%d|%d", strings.ToLower(domain), qtype, qclass)
	if cached, ok := d.fwdCache.Get(cacheKey); ok {
		// Patch ID agar cocok dengan query saat ini
		if len(cached) >= 2 {
			cached[0] = q[0]
			cached[1] = q[1]
		}
		return cached
	}

	// Pastikan tunnel aktif
	if d.tunnel == nil || d.tunnel.State() != "connected" {
		d.logger.Debug("DNS forward: tunnel belum siap, skip %s", domain)
		d.errors.Add(1)
		return d.buildEmptyResponse(q, questionEnd, 2) // SERVFAIL
	}

	// Buat koneksi TCP DNS ke upstream via tunnel
	conn, err := d.tunnel.Dial("tcp", d.cfg.DNS.Upstream)
	if err != nil {
		d.logger.Warn("DNS forward dial ke %s gagal: %v", d.cfg.DNS.Upstream, err)
		d.errors.Add(1)
		return d.buildEmptyResponse(q, questionEnd, 2)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Kirim query (2-byte length + payload)
	msg := make([]byte, 2+len(q))
	msg[0] = byte(len(q) >> 8)
	msg[1] = byte(len(q))
	copy(msg[2:], q)
	if _, err := conn.Write(msg); err != nil {
		d.logger.Warn("DNS forward write gagal: %v", err)
		d.errors.Add(1)
		return d.buildEmptyResponse(q, questionEnd, 2)
	}

	// Baca response (2-byte length + payload)
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		d.logger.Warn("DNS forward read len gagal: %v", err)
		d.errors.Add(1)
		return d.buildEmptyResponse(q, questionEnd, 2)
	}
	respLen := int(lenBuf[0])<<8 | int(lenBuf[1])
	if respLen == 0 || respLen > 8192 {
		d.errors.Add(1)
		return d.buildEmptyResponse(q, questionEnd, 2)
	}
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(conn, resp); err != nil {
		d.logger.Warn("DNS forward read body gagal: %v", err)
		d.errors.Add(1)
		return d.buildEmptyResponse(q, questionEnd, 2)
	}

	d.logger.Debug("DNS forward: %s → %d bytes", domain, respLen)

	// Cache 5 menit untuk respons sukses
	if len(resp) >= 4 && (resp[3]&0x0F) == 0 {
		d.fwdCache.Set(cacheKey, resp, 5*time.Minute)
	}

	return resp
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