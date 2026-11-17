package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// FailureHook menjalankan script shell ketika probe (ping/tcp) gagal
// berturut-turut sebanyak PingCount kali.
type FailureHook struct {
	cfg    FailureHookConfig
	logger *Logger

	mu         sync.Mutex
	failStreak int
	inOutage   bool // sudah pernah trigger, belum recovery

	lastRun atomic.Int64 // unix seconds; 0 = belum pernah
	running atomic.Bool
}

func NewFailureHook(cfg FailureHookConfig, logger *Logger) *FailureHook {
	h := &FailureHook{cfg: cfg, logger: logger}
	if !cfg.Enabled {
		return h
	}
	if strings.TrimSpace(cfg.Script) == "" {
		logger.Warn("[hook] enabled tapi script kosong — akan di-skip")
	} else if _, err := os.Stat(cfg.Script); err != nil {
		logger.Warn("[hook] script %s belum ada / tidak bisa diakses: %v", cfg.Script, err)
	}
	return h
}

func (h *FailureHook) Start(ctx context.Context) {
	if !h.cfg.Enabled {
		h.logger.Info("[hook] disabled via config")
		return
	}
	interval := h.cfg.Interval.D()
	if interval < time.Second {
		interval = 10 * time.Second
	}
	need := h.cfg.PingCount
	if need < 1 {
		need = 3
	}

	h.logger.Info("[hook] aktif — probe=%s target=%s count=%d interval=%s",
		h.probeType(), h.target(), need, interval)

	go func() {
		tk := time.NewTicker(interval)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
			}
			h.tick(ctx, need)
		}
	}()
}

func (h *FailureHook) probeType() string {
	p := strings.ToLower(strings.TrimSpace(h.cfg.Probe))
	if p == "" {
		return "ping"
	}
	return p
}

func (h *FailureHook) target() string {
	if h.cfg.Target != "" {
		return h.cfg.Target
	}
	if h.probeType() == "tcp" {
		return "8.8.8.8:443"
	}
	return "8.8.8.8"
}

func (h *FailureHook) tick(ctx context.Context, need int) {
	if h.probe(ctx) {
		h.mu.Lock()
		wasOutage := h.inOutage
		h.failStreak = 0
		h.inOutage = false
		h.mu.Unlock()
		if wasOutage {
			h.logger.Info("[hook] koneksi pulih")
		}
		return
	}

	h.mu.Lock()
	h.failStreak++
	streak := h.failStreak
	skip := h.inOutage && h.cfg.OncePerOutage
	h.mu.Unlock()

	h.logger.Warn("[hook] probe ke %s gagal (%d/%d)", h.target(), streak, need)
	if streak < need || skip {
		return
	}

	h.runScript(ctx, "probe-failed")

	h.mu.Lock()
	h.inOutage = true
	h.failStreak = 0
	h.mu.Unlock()
}

func (h *FailureHook) probe(ctx context.Context) bool {
	timeout := h.cfg.ProbeTimeout.D()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	if h.probeType() == "tcp" {
		d := net.Dialer{Timeout: timeout}
		pctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		conn, err := d.DialContext(pctx, "tcp", h.target())
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}

	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 2
	}
	pctx, cancel := context.WithTimeout(ctx, timeout+2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(pctx, "ping", "-c", "1", "-W", strconv.Itoa(secs), h.target())
	return cmd.Run() == nil
}

func (h *FailureHook) runScript(parent context.Context, reason string) {
	script := strings.TrimSpace(h.cfg.Script)
	if script == "" {
		h.logger.Warn("[hook] script path kosong, skip")
		return
	}

	if cd := h.cfg.Cooldown.D(); cd > 0 {
		if last := h.lastRun.Load(); last > 0 {
			remain := cd - time.Duration(time.Now().Unix()-last)*time.Second
			if remain > 0 {
				h.logger.Info("[hook] cooldown aktif (sisa %s), skip", remain.Round(time.Second))
				return
			}
		}
	}
	if !h.running.CompareAndSwap(false, true) {
		h.logger.Warn("[hook] script sebelumnya masih berjalan, skip")
		return
	}
	defer h.running.Store(false)
	h.lastRun.Store(time.Now().Unix())

	shell := h.cfg.Shell
	if shell == "" {
		shell = "/bin/sh"
	}
	timeout := h.cfg.ScriptTimeout.D()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

argv := make([]string, 0, 1+len(h.cfg.ScriptArgs))
	argv = append(argv, script)
	argv = append(argv, h.cfg.ScriptArgs...)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(cctx, shell, script)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"HOOK_REASON="+reason,
		"HOOK_TARGET="+h.target(),
		"HOOK_PROBE="+h.probeType(),
	)

	h.logger.Warn("[hook] menjalankan %s %s (reason=%s)", shell, strings.Join(argv, " "), reason)
	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	if s := strings.TrimSpace(stdout.String()); s != "" {
		h.logger.Info("[hook] stdout: %s", s)
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		h.logger.Warn("[hook] stderr: %s", s)
	}
	if err != nil {
		if cctx.Err() == context.DeadlineExceeded {
			h.logger.Error("[hook] script timeout setelah %s", dur.Round(time.Millisecond))
		} else {
			h.logger.Error("[hook] script gagal (%s): %v", dur.Round(time.Millisecond), err)
		}
		return
	}
	h.logger.Info("[hook] script selesai dalam %s", dur.Round(time.Millisecond))
}

func (h *FailureHook) String() string {
	return fmt.Sprintf("hook{enabled=%v, probe=%s, target=%s}",
		h.cfg.Enabled, h.probeType(), h.target())
}