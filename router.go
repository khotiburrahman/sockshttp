package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type Connection struct {
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
}

type ConnTracker struct {
	mu      sync.RWMutex
	conns   map[string]*Connection
	order   []string
	maxSize int
}

func NewConnTracker() *ConnTracker {
	return &ConnTracker{
		conns:   map[string]*Connection{},
		maxSize: 300,
	}
}

func (ct *ConnTracker) New(srcIP, domain, dstHost string, dstPort int, rule, chain, proxy string) *Connection {
	nBig, _ := rand.Int(rand.Reader, big.NewInt(1<<31))
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), nBig.Int64())
	c := &Connection{
		ID:        id,
		StartTime: time.Now(),
		SrcIP:     srcIP,
		DstHost:   dstHost,
		DstPort:   dstPort,
		Domain:    domain,
		Rule:      rule,
		Chain:     chain,
		Proxy:     proxy,
	}
	ct.mu.Lock()
	ct.conns[id] = c
	ct.order = append(ct.order, id)
	for len(ct.order) > ct.maxSize {
		old := ct.order[0]
		ct.order = ct.order[1:]
		delete(ct.conns, old)
	}
	ct.mu.Unlock()
	return c
}

func (ct *ConnTracker) Close(id string) {
	ct.mu.Lock()
	if c, ok := ct.conns[id]; ok {
		c.Closed = true
	}
	ct.mu.Unlock()
}

func (ct *ConnTracker) List() []*Connection {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	out := make([]*Connection, 0, len(ct.order))
	for i := len(ct.order) - 1; i >= 0; i-- {
		if c, ok := ct.conns[ct.order[i]]; ok {
			out = append(out, c)
		}
	}
	return out
}

func (ct *ConnTracker) CloseAll() {
	ct.mu.Lock()
	for _, c := range ct.conns {
		c.Closed = true
	}
	ct.mu.Unlock()
}

func getOriginalDst(conn *net.TCPConn) (string, error) {
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
		cfg:      cfg,
		logger:   logger,
		tunnel:   tunnel,
		dns:      dns,
		pool:     pool,
		proxyMgr: pm,
		tracker:  tracker,
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
	defer r.tracker.Close(track.ID)

	display := domain
	if display == "" {
		display = host
	}
	r.logger.Debug("CONN %s → %s:%d [%s] via %s", srcIP, display, port, ruleStr, proxy)

	switch proxy {
	case ProxyReject:
		return

	case ProxyDirect:
		target := dstAddr
		if domain != "" {
			target = net.JoinHostPort(domain, strconv.Itoa(port))
		}
		remote, err := net.DialTimeout("tcp", target, 10*time.Second)
		if err != nil {
			r.logger.Debug("DIRECT dial %s gagal: %v", target, err)
			return
		}
		defer remote.Close()
		r.pipe(raw, remote)

	case ProxyTunnel:
		target := ""
		if domain != "" {
			target = net.JoinHostPort(domain, strconv.Itoa(port))
		} else {
			target = dstAddr
		}
		remote, err := r.tunnel.Dial("tcp", target)
		if err != nil {
			r.logger.Debug("TUNNEL dial %s gagal: %v", target, err)
			return
		}
		defer remote.Close()
		r.pipe(raw, remote)

	default:
		r.logger.Warn("Unknown proxy: %s", proxy)
	}
}

func (r *Router) pipe(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(b, a)
		if tc, ok := b.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		io.Copy(a, b)
		if tc, ok := a.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
	}()
	wg.Wait()
}

func (r *Router) Close() {
	if r.closed.Swap(true) {
		return
	}
	if r.ln != nil {
		r.ln.Close()
	}
	r.wg.Wait()
}