package main

import (
	"container/list"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================
// Konstanta tuning untuk stale-while-revalidate.
// ============================================================
const (
	// dnsStaleWindow — berapa lama entry yang sudah expired tetap
	// di-serve (stale) sebelum benar-benar dibuang.
	dnsStaleWindow = 24 * time.Hour

	// dnsRefreshIdleMax — refresh proaktif hanya dilakukan untuk
	// entry yang baru dipakai user dalam rentang ini. Setelah idle
	// melewati batas, refresh berhenti (entry akan jadi stale).
	dnsRefreshIdleMax = 10 * time.Minute

	// realIPStaleWindow — window stale untuk RealIPCache (lebih pendek
	// karena IP bisa berubah lebih cepat dari domain).
	realIPStaleWindow = 1 * time.Hour

	// refreshScanInterval — frekuensi scan cache untuk cari entry
	// yang sudah masuk window 90% TTL.
	refreshScanInterval = 5 * time.Second

	// refreshBatchLimit — maksimal entry yang di-refresh per scan.
	refreshBatchLimit = 32

	// refreshConcurrency — berapa refresh paralel via tunnel.
	refreshConcurrency = 4
)

// ============================================================
// RealIPCache — IP asli → domain, diisi dari DNS forward.
// Dipakai router untuk reverse lookup hostname (bukan Fake-IP).
// Mendukung stale serving: entry yang sudah expired tetap dilayani
// hingga staleUntil habis.
// ============================================================
type realIPEntry struct {
	domain     string
	expire     time.Time
	staleUntil time.Time
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
	now := time.Now()
	c.mu.Lock()
	c.data[ip] = realIPEntry{
		domain:     domain,
		expire:     now.Add(ttl),
		staleUntil: now.Add(ttl + realIPStaleWindow),
	}
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

// Lookup — kembalikan domain untuk IP ini. Entry stale (sudah lewat
// expire tapi belum staleUntil) tetap dilayani agar reverse lookup
// di router tidak miss hanya karena TTL DNS habis.
func (c *RealIPCache) Lookup(ip string) (string, bool) {
	c.mu.RLock()
	e, ok := c.data[ip]
	c.mu.RUnlock()
	if !ok {
		c.misses.Add(1)
		return "", false
	}
	now := time.Now()
	if now.After(e.staleUntil) {
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
				if now.After(e.staleUntil) {
					delete(c.data, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

// ============================================================
// dnsForwardCache — cache response DNS dengan LRU + stale serving.
//
// Siklus hidup entry:
//   Set          → expire = now+ttl, refreshAt = now+0.9*ttl,
//                  staleUntil = now+ttl+dnsStaleWindow
//   Get (fresh)  → now < expire, serve
//   Get (stale)  → expire ≤ now < staleUntil, serve + async refresh
//   Get (dead)   → now ≥ staleUntil, hapus
//   Refresh      → reset semua timestamp, TANPA MoveToFront
//                  (posisi LRU hanya ditentukan query user)
// ============================================================
type cachedDNS struct {
	resp        []byte
	expire      time.Time
	refreshAt   time.Time
	staleUntil  time.Time
	lastQueried time.Time
	refreshing  bool // di akses under dnsForwardCache.mu
	elem        *list.Element
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

// Get — ambil response. fresh=false berarti entry sudah lewat expire
// tapi masih dalam staleUntil; caller boleh serve + trigger refresh.
func (c *dnsForwardCache) Get(key string) (resp []byte, fresh bool, ok bool) {
	c.mu.Lock()
	e, exists := c.data[key]
	if !exists {
		c.misses.Add(1)
		c.mu.Unlock()
		return nil, false, false
	}
	now := time.Now()

	// Sudah lewat staleUntil → benar-benar hilang
	if !now.Before(e.staleUntil) {
		c.lru.Remove(e.elem)
		delete(c.data, key)
		c.misses.Add(1)
		c.mu.Unlock()
		return nil, false, false
	}

	e.lastQueried = now
	c.lru.MoveToFront(e.elem)
	c.hits.Add(1)

	out := append([]byte(nil), e.resp...)
	isFresh := now.Before(e.expire)
	c.mu.Unlock()
	return out, isFresh, true
}

// Set — dipakai saat user miss → forward sukses. Menandai entry baru
// dipakai (lastQueried=now) dan memindahkan ke depan LRU.
func (c *dnsForwardCache) Set(key string, resp []byte, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	refreshAt := now.Add(ttl * 9 / 10)
	staleUntil := now.Add(ttl + dnsStaleWindow)

	// Update entry yang sudah ada
	if e, ok := c.data[key]; ok {
		e.resp = append([]byte(nil), resp...)
		e.expire = now.Add(ttl)
		e.refreshAt = refreshAt
		e.staleUntil = staleUntil
		e.lastQueried = now
		e.refreshing = false
		c.lru.MoveToFront(e.elem)
		return
	}

	// Lazy cleanup entry yang sudah lewat staleUntil
	for k, v := range c.data {
		if now.After(v.staleUntil) {
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
		resp:        append([]byte(nil), resp...),
		expire:      now.Add(ttl),
		refreshAt:   refreshAt,
		staleUntil:  staleUntil,
		lastQueried: now,
		refreshing:  false,
		elem:        elem,
	}
}

// Refresh — dipakai background refresh. Tidak MoveToFront: posisi LRU
// hanya boleh digeser oleh query user. Refresh hanya update payload.
func (c *dnsForwardCache) Refresh(key string, resp []byte, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.data[key]
	if !ok {
		return
	}
	now := time.Now()
	e.resp = append([]byte(nil), resp...)
	e.expire = now.Add(ttl)
	e.refreshAt = now.Add(ttl * 9 / 10)
	e.staleUntil = now.Add(ttl + dnsStaleWindow)
	e.refreshing = false
}

// GetRefreshable — kandidat refresh proaktif:
//   - sudah lewat refreshAt (90% TTL)
//   - belum expire (masih fresh)
//   - baru dipakai user dalam idleMax
//   - tidak sedang di-refresh goroutine lain
// Entry yang di-pick ditandai refreshing=true agar tidak dobel.
func (c *dnsForwardCache) GetRefreshable(now time.Time, idleMax time.Duration, limit int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for k, e := range c.data {
		if e.refreshing {
			continue
		}
		if now.Before(e.refreshAt) || !now.Before(e.expire) {
			continue
		}
		if now.Sub(e.lastQueried) > idleMax {
			continue
		}
		e.refreshing = true
		out = append(out, k)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// RequestAsyncRefresh — dipicu stale hit. Spawn goroutine sekali;
// kalau entry sudah refreshing, no-op.
func (c *dnsForwardCache) RequestAsyncRefresh(key string, fn func(string)) {
	c.mu.Lock()
	e, ok := c.data[key]
	if !ok || e.refreshing {
		c.mu.Unlock()
		return
	}
	e.refreshing = true
	c.mu.Unlock()
	go fn(key)
}

// ClearRefreshing — dipanggil saat refresh selesai / gagal.
func (c *dnsForwardCache) ClearRefreshing(key string) {
	c.mu.Lock()
	if e, ok := c.data[key]; ok {
		e.refreshing = false
	}
	c.mu.Unlock()
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
// DNSServer — forward-only + stale-while-revalidate
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
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewDNSServer(cfg *Config, logger *Logger, realCache *RealIPCache, tunnel *SSHTunnel, ab *Adblocker) *DNSServer {
	ctx, cancel := context.WithCancel(context.Background())
	return &DNSServer{
		cfg:       cfg,
		logger:    logger,
		realCache: realCache,
		tunnel:    tunnel,
		adblocker: ab,
		fwdCache:  newDNSForwardCache(),
		ctx:       ctx,
		cancel:    cancel,
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
	d.wg.Add(3)
	go d.serveUDP()
	go d.serveTCP()
	go d.refreshLoop(d.ctx)
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

	// --- Cache hit (fresh atau stale) ---
	if cached, fresh, ok := d.fwdCache.Get(cacheKey); ok {
		if len(cached) >= 2 {
			cached[0] = q[0]
			cached[1] = q[1]
		}
		// Repopulate RealIPCache dari response cached (agar tidak expired
		// lebih cepat dari fwdCache saat client pakai DNS cache sendiri)
		d.processResponse(cached, domain)

		if !fresh {
			// Stale hit — serve sekarang, refresh di background.
			d.logger.Debug("DNS: stale hit %s → serve + async refresh", domain)
			d.fwdCache.RequestAsyncRefresh(cacheKey, func(k string) {
				d.refreshEntry(d.ctx, k)
			})
		}
		return cached
	}

	// --- Cache miss: forward blocking ---
	resp, err := d.forwardRaw(q)
	if err != nil {
		if err.Error() == "tunnel not ready" {
			d.logger.Debug("DNS forward: tunnel belum siap, skip %s", domain)
		} else {
			d.logger.Warn("DNS forward dial ke %s gagal: %v", d.cfg.DNS.Upstream, err)
		}
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
		domain, qtype, len(resp), rcode, cacheTTL)
	return resp
}

// forwardRaw — forward mentah via tunnel, tanpa cache.
func (d *DNSServer) forwardRaw(q []byte) ([]byte, error) {
	if d.tunnel == nil || d.tunnel.State() != "connected" {
		return nil, fmt.Errorf("tunnel not ready")
	}
	conn, err := d.tunnel.Dial("tcp", d.cfg.DNS.Upstream)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

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
	respLen := int(lenBuf[0])<<8 | int(lenBuf[1])
	if respLen == 0 || respLen > 8192 {
		return nil, fmt.Errorf("bad resp len %d", respLen)
	}
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, err
	}
	return resp, nil
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

// ============================================================
// Background refresh — stale-while-revalidate
// ============================================================

// refreshLoop — scan periodik, refresh entry yang sudah masuk window
// 90% TTL dan masih "hangat" (baru dipakai user).
func (d *DNSServer) refreshLoop(ctx context.Context) {
	defer d.wg.Done()
	tk := time.NewTicker(refreshScanInterval)
	defer tk.Stop()

	sem := make(chan struct{}, refreshConcurrency)
	var batchWG sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			batchWG.Wait()
			return
		case <-tk.C:
			keys := d.fwdCache.GetRefreshable(time.Now(), dnsRefreshIdleMax, refreshBatchLimit)
			for _, k := range keys {
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					batchWG.Wait()
					return
				}
				batchWG.Add(1)
				go func(key string) {
					defer batchWG.Done()
					defer func() { <-sem }()
					d.refreshEntry(ctx, key)
				}(k)
			}
			batchWG.Wait()
		}
	}
}

// refreshEntry — re-resolve satu entry. Di-share oleh refreshLoop dan
// stale-hit trigger. Selalu ClearRefreshing saat selesai.
func (d *DNSServer) refreshEntry(ctx context.Context, key string) {
	defer d.fwdCache.ClearRefreshing(key)

	if ctx.Err() != nil {
		return
	}
	parts := strings.SplitN(key, "|", 3)
	if len(parts) != 3 {
		return
	}
	domain := parts[0]
	qtype64, err := strconv.ParseUint(parts[1], 10, 16)
	if err != nil {
		return
	}
	qclass64, err := strconv.ParseUint(parts[2], 10, 16)
	if err != nil {
		return
	}

	q := buildDNSQuery(domain, uint16(qtype64), uint16(qclass64))
	if q == nil {
		return
	}

	resp, err := d.forwardRaw(q)
	if err != nil {
		d.logger.Debug("DNS refresh: %s gagal: %v", domain, err)
		return // biarkan entry lama sampai staleUntil
	}
	if ctx.Err() != nil {
		return
	}

	minTTL, hasAnswer := d.processResponse(resp, domain)
	rcode := uint8(0)
	if len(resp) >= 4 {
		rcode = resp[3] & 0x0F
	}

	var cacheTTL time.Duration
	switch {
	case rcode == 0 && hasAnswer:
		if minTTL < 60 {
			minTTL = 60
		}
		if minTTL > 3600 {
			minTTL = 3600
		}
		cacheTTL = time.Duration(minTTL) * time.Second
	case rcode == 3, rcode == 5:
		cacheTTL = 30 * time.Second
	case rcode == 0 && !hasAnswer:
		cacheTTL = 60 * time.Second
	}

	if cacheTTL > 0 {
		d.fwdCache.Refresh(key, resp, cacheTTL)
		d.logger.Debug("DNS refresh OK: %s (type %d) → ttl=%v", domain, qtype64, cacheTTL)
	}
}

// buildDNSQuery — query minimal (RD=1) untuk keperluan refresh.
func buildDNSQuery(domain string, qtype, qclass uint16) []byte {
	if len(domain) == 0 || len(domain) > 253 {
		return nil
	}
	q := make([]byte, 12)
	q[0], q[1] = 0x00, 0x01 // ID
	q[2] = 0x01             // RD
	q[5] = 0x01             // QDCOUNT=1

	for _, label := range strings.Split(domain, ".") {
		if label == "" {
			continue
		}
		if len(label) > 63 {
			return nil
		}
		q = append(q, byte(len(label)))
		q = append(q, label...)
	}
	q = append(q, 0x00)
	q = append(q, byte(qtype>>8), byte(qtype))
	q = append(q, byte(qclass>>8), byte(qclass))
	return q
}

// ============================================================
// Response builder
// ============================================================
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
	if d.cancel != nil {
		d.cancel()
	}
	if d.udpConn != nil {
		d.udpConn.Close()
	}
	if d.tcpLn != nil {
		d.tcpLn.Close()
	}
	d.wg.Wait()
}