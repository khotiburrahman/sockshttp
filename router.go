package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// =========================================================
// Connection
// =========================================================
type Connection struct {
	ID        string
	StartTime time.Time
	SrcIP     string
	DstHost   string
	DstPort   int
	Domain    string
	Rule      string
	Chain     string
	Proxy     string
	BytesUp   atomic.Int64
	BytesDown atomic.Int64
	Closed    atomic.Bool
	endTime   atomic.Int64
	endReason atomic.Value

	connsMu   sync.Mutex
	conns     []net.Conn
	closeOnce sync.Once
}

func (c *Connection) AddConn(cn net.Conn) {
	if cn == nil {
		return
	}
	c.connsMu.Lock()
	c.conns = append(c.conns, cn)
	c.connsMu.Unlock()
}

// setEnd menandai koneksi selesai. Return true jika ini yang pertama.
func (c *Connection) setEnd(reason string) bool {
	if c.Closed.CompareAndSwap(false, true) {
		c.endReason.Store(reason)
		c.endTime.Store(time.Now().UnixNano())
		return true
	}
	return false
}

func (c *Connection) ForceClose(reason string) {
	c.setEnd(reason)
	c.closeOnce.Do(func() {
		c.connsMu.Lock()
		for _, cn := range c.conns {
			cn.Close()
		}
		c.connsMu.Unlock()
	})
}

func (c *Connection) EndInfo() (int64, string) {
	return c.endTime.Load(), c.endReason.Load().(string)
}

type ConnectionView struct {
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
	EndTime   time.Time `json:"end,omitempty"`
	EndReason string    `json:"reason,omitempty"`
	RemainSec int       `json:"remain_sec,omitempty"`
}

func (c *Connection) View(retention time.Duration) *ConnectionView {
	v := &ConnectionView{
		ID:        c.ID,
		StartTime: c.StartTime,
		SrcIP:     c.SrcIP,
		DstHost:   c.DstHost,
		DstPort:   c.DstPort,
		Domain:    c.Domain,
		Rule:      c.Rule,
		Chain:     c.Chain,
		Proxy:     c.Proxy,
		BytesUp:   c.BytesUp.Load(),
		BytesDown: c.BytesDown.Load(),
		Closed:    c.Closed.Load(),
	}
	if v.Closed {
		et, er := c.EndInfo()
		if et > 0 {
			v.EndTime = time.Unix(0, et)
			v.EndReason = er
			if retention > 0 {
				remain := retention - time.Since(v.EndTime)
				if remain < 0 {
					remain = 0
				}
				v.RemainSec = int(remain.Seconds())
			}
		}
	}
	return v
}

// =========================================================
// ConnTracker
// =========================================================
type ConnTracker struct {
	mu         sync.RWMutex
	conns      map[string]*Connection
	order      []string
	maxSize    int
	retention  time.Duration
	bytesFile  string
	TotalUp    atomic.Int64
	TotalDown  atomic.Int64
	lastSave   atomic.Int64
	done       chan struct{}
	closedOnce sync.Once
}

func NewConnTracker(stateFile string, retention time.Duration) *ConnTracker {
	ct := &ConnTracker{
		conns:     map[string]*Connection{},
		maxSize:   600,
		retention: retention,
		done:      make(chan struct{}),
	}
	if stateFile != "" {
		ct.bytesFile = stateFile + ".bytes"
		ct.loadTotals()
	}
	go ct.pruneLoop()
	return ct
}

func (ct *ConnTracker) loadTotals() {
	if ct.bytesFile == "" {
		return
	}
	data, err := os.ReadFile(ct.bytesFile)
	if err != nil {
		return
	}
	var m map[string]int64
	if json.Unmarshal(data, &m) == nil {
		ct.TotalUp.Store(m["up"])
		ct.TotalDown.Store(m["down"])
	}
}

func (ct *ConnTracker) saveTotals() {
	if ct.bytesFile == "" {
		return
	}
	data, _ := json.Marshal(map[string]int64{
		"up":   ct.TotalUp.Load(),
		"down": ct.TotalDown.Load(),
	})
	os.MkdirAll(filepath.Dir(ct.bytesFile), 0755)
	os.WriteFile(ct.bytesFile, data, 0644)
	ct.lastSave.Store(time.Now().Unix())
}

func (ct *ConnTracker) pruneLoop() {
	tk := time.NewTicker(5 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ct.done:
			return
		case <-tk.C:
			ct.prune()
			// Save totals tiap 60s
			if time.Now().Unix()-ct.lastSave.Load() >= 60 {
				ct.saveTotals()
			}
		}
	}
}

func (ct *ConnTracker) prune() {
	if ct.retention <= 0 {
		return
	}
	cutoff := time.Now().Add(-ct.retention).UnixNano()
	ct.mu.Lock()
	defer ct.mu.Unlock()
	n := 0
	for _, id := range ct.order {
		c := ct.conns[id]
		if c == nil {
			continue
		}
		if c.Closed.Load() {
			et, _ := c.EndInfo()
			if et > 0 && et < cutoff {
				delete(ct.conns, id)
				continue
			}
		}
		ct.order[n] = id
		n++
	}
	ct.order = ct.order[:n]
}

func (ct *ConnTracker) New(srcIP, domain, dstHost string, dstPort int, rule, chain, proxy string) *Connection {
	nBig, _ := rand.Int(rand.Reader, big.NewInt(1<<31))
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), nBig.Int64())
	c := &Connection{
		ID: id, StartTime: time.Now(), SrcIP: srcIP,
		DstHost: dstHost, DstPort: dstPort, Domain: domain,
		Rule: rule, Chain: chain, Proxy: proxy,
	}
	c.endReason.Store("")
	ct.mu.Lock()
	ct.conns[id] = c
	ct.order = append(ct.order, id)
	for len(ct.order) > ct.maxSize {
		// Coba evict oldest closed dulu
		evicted := false
		for i, oid := range ct.order {
			if oc := ct.conns[oid]; oc != nil && oc.Closed.Load() {
				delete(ct.conns, oid)
				ct.order = append(ct.order[:i], ct.order[i+1:]...)
				evicted = true
				break
			}
		}
		if !evicted {
			oid := ct.order[0]
			ct.order = ct.order[1:]
			delete(ct.conns, oid)
		}
	}
	ct.mu.Unlock()
	return c
}

func (ct *ConnTracker) Close(id, reason string) {
	ct.mu.RLock()
	c := ct.conns[id]
	ct.mu.RUnlock()
	if c != nil {
		c.setEnd(reason)
	}
}

func (ct *ConnTracker) ForceClose(id, reason string) bool {
	ct.mu.RLock()
	c, ok := ct.conns[id]
	ct.mu.RUnlock()
	if !ok {
		return false
	}
	c.ForceClose(reason)
	return true
}

func (ct *ConnTracker) ForceCloseMany(ids []string, reason string) int {
	n := 0
	for _, id := range ids {
		if ct.ForceClose(id, reason) {
			n++
		}
	}
	return n
}

func (ct *ConnTracker) ForceCloseAll(reason string) int {
	ct.mu.RLock()
	list := make([]*Connection, 0, len(ct.conns))
	for _, c := range ct.conns {
		if !c.Closed.Load() {
			list = append(list, c)
		}
	}
	ct.mu.RUnlock()
	for _, c := range list {
		c.ForceClose(reason)
	}
	return len(list)
}

func (ct *ConnTracker) ForceCloseBySrc(srcIP, reason string) int {
	ct.mu.RLock()
	var list []*Connection
	for _, c := range ct.conns {
		if c.SrcIP == srcIP && !c.Closed.Load() {
			list = append(list, c)
		}
	}
	ct.mu.RUnlock()
	for _, c := range list {
		c.ForceClose(reason)
	}
	return len(list)
}

func (ct *ConnTracker) ForceCloseByProxy(proxy, reason string) int {
	ct.mu.RLock()
	var list []*Connection
	for _, c := range ct.conns {
		if c.Proxy == proxy && !c.Closed.Load() {
			list = append(list, c)
		}
	}
	ct.mu.RUnlock()
	for _, c := range list {
		c.ForceClose(reason)
	}
	return len(list)
}

func (ct *ConnTracker) ListActive() []*ConnectionView {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	out := make([]*ConnectionView, 0)
	for _, c := range ct.conns {
		if !c.Closed.Load() {
			out = append(out, c.View(ct.retention))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartTime.After(out[j].StartTime) })
	return out
}

func (ct *ConnTracker) ListClosed() []*ConnectionView {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	out := make([]*ConnectionView, 0)
	for _, c := range ct.conns {
		if c.Closed.Load() {
			out = append(out, c.View(ct.retention))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EndTime.After(out[j].EndTime) })
	return out
}

func (ct *ConnTracker) ListAll() []*ConnectionView {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	out := make([]*ConnectionView, 0, len(ct.conns))
	for _, c := range ct.conns {
		out = append(out, c.View(ct.retention))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartTime.After(out[j].StartTime) })
	return out
}

func (ct *ConnTracker) ActiveCount() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	n := 0
	for _, c := range ct.conns {
		if !c.Closed.Load() {
			n++
		}
	}
	return n
}

func (ct *ConnTracker) ClosedCount() int {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	n := 0
	for _, c := range ct.conns {
		if c.Closed.Load() {
			n++
		}
	}
	return n
}

type SrcAggregate struct {
	SrcIP       string `json:"srcIP"`
	ActiveConns int    `json:"active_conns"`
	TotalConns  int    `json:"total_conns"`
	BytesUp     int64  `json:"up"`
	BytesDown   int64  `json:"down"`
}

func (ct *ConnTracker) AggregateBySrc() []SrcAggregate {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	m := map[string]*SrcAggregate{}
	for _, c := range ct.conns {
		a, ok := m[c.SrcIP]
		if !ok {
			a = &SrcAggregate{SrcIP: c.SrcIP}
			m[c.SrcIP] = a
		}
		a.TotalConns++
		if !c.Closed.Load() {
			a.ActiveConns++
		}
		a.BytesUp += c.BytesUp.Load()
		a.BytesDown += c.BytesDown.Load()
	}
	out := make([]SrcAggregate, 0, len(m))
	for _, a := range m {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ActiveConns != out[j].ActiveConns {
			return out[i].ActiveConns > out[j].ActiveConns
		}
		return out[i].SrcIP < out[j].SrcIP
	})
	return out
}

// Shutdown menghentikan background prune loop dan menyimpan total byte.
// (Sebelumnya bernama Close, yang bentrok dengan Close(id, reason).)
func (ct *ConnTracker) Shutdown() {
	ct.closedOnce.Do(func() {
		close(ct.done)
		ct.saveTotals()
	})
}

// =========================================================
// countingWriter
// =========================================================
type countingWriter struct {
	w      io.Writer
	local  *atomic.Int64
	global *atomic.Int64
}

func (cw *countingWriter) Write(b []byte) (int, error) {
	n, err := cw.w.Write(b)
	if n > 0 {
		if cw.local != nil {
			cw.local.Add(int64(n))
		}
		if cw.global != nil {
			cw.global.Add(int64(n))
		}
	}
	return n, err
}

// =========================================================
// getOriginalDst
// =========================================================
func getOriginalDst(conn *net.TCPConn) (string, error) {
	ra := conn.RemoteAddr()
	if ta, ok := ra.(*net.TCPAddr); ok && ta.IP.To4() == nil {
		return "", fmt.Errorf("IPv6 original dst tidak didukung")
	}
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

// =========================================================
// Router
// =========================================================
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
		cfg: cfg, logger: logger, tunnel: tunnel, dns: dns,
		pool: pool, proxyMgr: pm, tracker: tracker,
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

// Test dipakai API untuk rule tester.
func (r *Router) Test(srcIP, dstIP, domain string) (string, string, string) {
	return r.dispatch(srcIP, dstIP, domain)
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
	defaultReason := "eof"
	defer func() { r.tracker.Close(track.ID, defaultReason) }()

	display := domain
	if display == "" {
		display = host
	}
	r.logger.Debug("CONN %s -> %s:%d [%s] via %s", srcIP, display, port, ruleStr, proxy)

	switch proxy {
	case ProxyReject:
		defaultReason = "rejected"
		return

	case ProxyDirect:
		target := dstAddr
		if domain != "" {
			target = net.JoinHostPort(domain, strconv.Itoa(port))
		}
		remote, err := net.DialTimeout("tcp", target, 10*time.Second)
		if err != nil {
			defaultReason = "dial-failed"
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
			defaultReason = "dial-failed"
			return
		}
		defer remote.Close()
		r.pipe(raw, remote, track)
	}
}

func (r *Router) pipe(a, b net.Conn, track *Connection) {
	track.AddConn(a)
	track.AddConn(b)

	done := make(chan struct{}, 2)
	go func() {
		cw := &countingWriter{w: b, local: &track.BytesUp, global: &r.tracker.TotalUp}
		io.Copy(cw, a)
		if tc, ok := b.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		cw := &countingWriter{w: a, local: &track.BytesDown, global: &r.tracker.TotalDown}
		io.Copy(cw, b)
		if tc, ok := a.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	a.Close()
	b.Close()
	<-done
}

func (r *Router) Shutdown() {
	if r.closed.Swap(true) {
		return
	}
	if r.ln != nil {
		r.ln.Close()
	}
	r.wg.Wait()
}