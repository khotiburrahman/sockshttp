package main

import (
	"container/list"
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
	// Clamp ke [60s, 3600s] — selaras dengan fwdCache TTL
	if ttl < 60*time.Second {
		ttl = 60 * time.Second
	}
	if ttl > 3600*time.Second {
		ttl = 3600 * time.Second
	}
	c.mu.Lock()
	c.data[ip] = realIPEntry{domain: domain, expire: time.Now().Add(ttl)}
	if len(c.data) > c.maxSize {
		// Buang satu entry acak (RealIPCache hanya diakses hot path,
		// tidak pakai LRU agar RLock Lookup tetap murah).
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
// dnsForwardCache — cache response DNS dengan LRU.
// ============================================================
type cachedDNS struct {
	resp   []byte
	expire time.Time
	elem   *list.Element
}

type dnsForwardCache struct {
	mu      sync.Mutex
	data    map[string]*cachedDNS
	lru     *list.List
	maxSize int
	hits    atomic.Int64
	misses  atomic.Int64
}

func newDNSForwardCache() *dnsForwardCache {
	return &dnsForwardCache{
		data:    map[string]*cachedDNS{},
		lru:     list.New(),
		maxSize: 20000,
	}
}

func (c *dnsForwardCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok {
		c.misses.Add(1)
		return nil, false
	}
	if time.Now().After(e.expire) {
		c.lru.Remove(e.elem)
		delete(c.data, key)
		c.misses.Add(1)
		return nil, false
	}
	c.lru.MoveToFront(e.elem)
	c.hits.Add(1)
	out := make([]byte, len(e.resp))
	copy(out, e.resp)
	return out, true
}

func (c *dnsForwardCache) Set(key string, resp []byte, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update entry yang sudah ada
	if e, ok := c.data[key]; ok {
		e.resp = append([]byte(nil), resp...)
		e.expire = time.Now().Add(ttl)
		c.lru.MoveToFront(e.elem)
		return
	}

	// Lazy cleanup entry expired
	now := time.Now()
	for k, v := range c.data {
		if now.After(v.expire) {
			c.lru.Remove(v.elem)
			delete(c.data, k)
		}
	}

	// Evict LRU (paling belakang) kalau penuh
	for c.lru.Len() >= c.maxSize {
		back := c.lru.Back()
		if back == nil {
			break
		}
		k := back.Value.(string)
		c.lru.Remove(back)
		delete(c.data, k)
	}

	elem := c.lru.PushFront(key)
	c.data[key] = &cachedDNS{
		resp:   append([]byte(nil), resp...),
		expire: time.Now().Add(ttl),
		elem:   elem,
	}
}

func (c *dnsForwardCache) Stats() map[string]int64 {
	c.mu.Lock()
	size := len(c.data)
	c.mu.Unlock()
	return map[string]int64{
		"size":   int64(size),
		"hits":   c.hits.Load(),
		"misses": c.misses.Load(),
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
	adblocker *Adblocker
	udpConn   *net.UDPConn
	tcpLn     net.Listener
	closed    atomic.Bool
	wg        sync.WaitGroup
	queries   atomic.Int64
	errors    atomic.Int64
	fwdCache  *dnsForwardCache
}

func NewDNSServer(cfg *Config, logger *Logger, realCache *RealIPCache, tunnel *SSHTunnel, ab *Adblocker) *DNSServer {
	return &DNSServer{
		cfg:       cfg,
		logger:    logger,
		realCache: realCache,
		tunnel:    tunnel,
		adblocker: ab,
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

	// Adblock
	if d.adblocker != nil && d.adblocker.IsBlocked(domain) {
		d.logger.Debug("DNS: %s diblokir oleh Adblock", domain)
		return d.buildBlockedResponse(q, questionEnd, qtype)
	}

	// AAAA → empty NOERROR (IPv4-only tunnel)
	if qtype == 28 {
		d.logger.Debug("DNS: %s AAAA → empty (IPv4-only tunnel)", domain)
		return d.buildEmptyResponse(q, questionEnd)
	}

	return d.forwardQuery(q, questionEnd, domain, qtype, qclass)
}

func (d *DNSServer) forwardQuery(q []byte, questionEnd int, domain string, qtype, qclass uint16) []byte {
	cacheKey := fmt.Sprintf("%s|%d|%d", strings.ToLower(domain), qtype, qclass)

	// --- Cache hit: repopulate RealIPCache ---
	if cached, ok := d.fwdCache.Get(cacheKey); ok {
		if len(cached) >= 2 {
			cached[0] = q[0]
			cached[1] = q[1]
		}
		// Repopulate RealIPCache dari response cached (agar tidak expired
		// lebih cepat dari fwdCache saat client pakai DNS cache sendiri)
		d.processResponse(cached, domain)
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

	// Proses response: isi RealIPCache + ambil minTTL
	minTTL, hasAnswer := d.processResponse(resp, domain)

	rcode := uint8(0)
	if len(resp) >= 4 {
		rcode = resp[3] & 0x0F
	}

	// Tentukan TTL cache berdasarkan rcode
	var cacheTTL time.Duration
	switch {
	case rcode == 0 && hasAnswer:
		// Positive answer — pakai minTTL dari upstream, clamp [60s, 3600s]
		if minTTL < 60 {
			minTTL = 60
		}
		if minTTL > 3600 {
			minTTL = 3600
		}
		cacheTTL = time.Duration(minTTL) * time.Second
	case rcode == 3:
		// NXDOMAIN — negative cache 30 detik
		cacheTTL = 30 * time.Second
	case rcode == 5:
		// REFUSED — cache pendek
		cacheTTL = 30 * time.Second
	case rcode == 0 && !hasAnswer:
		// NODATA (NOERROR, 0 answer) — cache 60s
		cacheTTL = 60 * time.Second
	default:
		// SERVFAIL dan lainnya — jangan cache
		cacheTTL = 0
	}

	if cacheTTL > 0 {
		d.fwdCache.Set(cacheKey, resp, cacheTTL)
	}

	d.logger.Debug("DNS forward: %s (type %d) → %d bytes, rcode=%d, ttl=%v",
		domain, qtype, respLen, rcode, cacheTTL)
	return resp
}

// processResponse parse answer section: isi RealIPCache untuk A record,
// dan kembalikan (minTTL, hasAnswer).
// minTTL dihitung dari A dan CNAME (chain validity).
func (d *DNSServer) processResponse(resp []byte, domain string) (uint32, bool) {
	if len(resp) < 12 {
		return 0, false
	}
	ancount := int(binary.BigEndian.Uint16(resp[6:8]))
	if ancount == 0 {
		return 0, false
	}

	// Skip question section
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
		return 0, false
	}
	pos += 4 // qtype + qclass
	cur := pos

	var minTTL uint32
	var aIPs []string
	hasAnswer := false

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

		// Catat TTL minimum dari A dan CNAME
		if rtype == 1 || rtype == 5 {
			if minTTL == 0 || ttl < minTTL {
				minTTL = ttl
			}
			hasAnswer = true
		}

		// Kumpulkan A record untuk RealIPCache
		if rtype == 1 && rdlen == 4 {
			ip := net.IPv4(resp[rdataStart], resp[rdataStart+1], resp[rdataStart+2], resp[rdataStart+3]).String()
			aIPs = append(aIPs, ip)
		}

		cur = rdataEnd
	}

	// Set RealIPCache dengan minTTL agar sinkron dengan fwdCache
	ttlDur := time.Duration(minTTL) * time.Second
	for _, ip := range aIPs {
		d.realCache.Set(ip, domain, ttlDur)
	}

	return minTTL, hasAnswer
}

func (d *DNSServer) buildEmptyResponse(q []byte, questionEnd int) []byte {
	resp := make([]byte, questionEnd)
	copy(resp, q[:questionEnd])
	resp[2] = 0x81
	resp[3] = 0x80
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

// buildBlockedResponse — balas 0.0.0.0 untuk A, NXDOMAIN untuk tipe lain
func (d *DNSServer) buildBlockedResponse(q []byte, questionEnd int, qtype uint16) []byte {
	if qtype != 1 {
		return d.buildErrorResponse(q, questionEnd, 3)
	}

	answer := []byte{
		0xC0, 0x0C, // name pointer
		0x00, 0x01, // type A
		0x00, 0x01, // class IN
		0x00, 0x00, 0x00, 0x3C, // TTL 60s
		0x00, 0x04, // rdlength
		0x00, 0x00, 0x00, 0x00, // 0.0.0.0
	}

	resp := make([]byte, len(q[:questionEnd])+len(answer))
	copy(resp, q[:questionEnd])
	resp[2] = 0x81
	resp[3] = 0x80
	resp[6] = 0x00
	resp[7] = 0x01
	resp[8] = 0x00
	resp[9] = 0x00
	resp[10] = 0x00
	resp[11] = 0x00
	copy(resp[questionEnd:], answer)
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