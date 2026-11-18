package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type FailureHook struct {
	cfg    FailureHookConfig
	logger *Logger
	tunnel *SSHTunnel

	mu        sync.Mutex
	downSince time.Time
	inOutage  bool

	lastRun atomic.Int64
	running atomic.Bool
}

func NewFailureHook(cfg FailureHookConfig, logger *Logger, tunnel *SSHTunnel) *FailureHook {
	h := &FailureHook{cfg: cfg, logger: logger, tunnel: tunnel}
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
	interval := h.cfg.CheckInterval.D()
	if interval < time.Second {
		interval = 5 * time.Second
	}

	h.logger.Info("[hook] aktif — monitor tunnel (cek=%s, max_down=%s, max_reconn=%dx/%s)",
		interval, h.cfg.MaxDowntime.D(), h.cfg.MaxReconnects, h.cfg.ReconnectWindow.D())

	go func() {
		tk := time.NewTicker(interval)
		defer tk.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tk.C:
			}
			h.tick(ctx)
		}
	}()
}

func (h *FailureHook) tick(ctx context.Context) {
	state := h.tunnel.State()

	h.mu.Lock()
	defer h.mu.Unlock()

	// 1. Jika tunnel sudah tersambung, reset status
	if state == "connected" {
		if h.inOutage {
			h.logger.Info("[hook] koneksi tunnel pulih")
		}
		h.inOutage = false
		h.downSince = time.Time{}
		return
	}

	// Jika sudah pernah trigger dan mode OncePerOutage, diam saja sampai pulih
	if h.inOutage && h.cfg.OncePerOutage {
		return
	}

	// 2. Catat mulai kapan tunnel "down" (connecting/disconnected)
	if h.downSince.IsZero() {
		h.downSince = time.Now()
	}

	downtime := time.Since(h.downSince)

	// 3. Cek Kondisi A: Terlalu lama tidak tersambung (Misal: > 3 Menit)
	if h.cfg.MaxDowntime.D() > 0 && downtime >= h.cfg.MaxDowntime.D() {
		h.logger.Warn("[hook] Tunnel sudah %s tidak tersambung (max %s)", downtime.Round(time.Second), h.cfg.MaxDowntime.D())
		h.execute(ctx, fmt.Sprintf("downtime-%s", downtime.Round(time.Second)))
		h.inOutage = true
		return
	}

	// 4. Cek Kondisi B: Terlalu banyak reconnect dalam rentang waktu (Misal: 3x dalam 1 Menit)
	if h.cfg.MaxReconnects > 0 && h.cfg.ReconnectWindow.D() > 0 {
		h.tunnel.reconnMu.Lock()
		// Copy slice agar aman dari race condition saat diakses di luar lock
		hist := make([]int64, len(h.tunnel.reconnectHistory))
		copy(hist, h.tunnel.reconnectHistory)
		h.tunnel.reconnMu.Unlock()

		now := time.Now()
		recent := 0
		// Baca dari belakang (waktu paling baru)
		for i := len(hist) - 1; i >= 0; i-- {
			if now.Sub(time.Unix(hist[i], 0)) <= h.cfg.ReconnectWindow.D() {
				recent++
			} else {
				break
			}
		}

		if recent >= h.cfg.MaxReconnects {
			h.logger.Warn("[hook] Terdeteksi %d reconnect dalam %s terakhir", recent, h.cfg.ReconnectWindow.D())
			h.execute(ctx, fmt.Sprintf("reconnect-spam-%dx", recent))
			h.inOutage = true
			return
		}
	}
}

func (h *FailureHook) execute(parent context.Context, reason string) {
	script := strings.TrimSpace(h.cfg.Script)
	if script == "" {
		h.logger.Warn("[hook] script path kosong, skip")
		return
	}

	if cd := h.cfg.Cooldown.D(); cd > 0 {
		if last := h.lastRun.Load(); last > 0 {
			remain := cd - time.Duration(time.Now().Unix()-last)*time.Second
			if remain > 0 {
				h.logger.Info("[hook] cooldown aktif (sisa %s), skip eksekusi", remain.Round(time.Second))
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
		timeout = 120 * time.Second
	}
	cctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	// Bentuk perintah persis seperti di terminal: login.sh ganti-ip
	var cmdStr string
	if len(h.cfg.ScriptArgs) > 0 {
		cmdStr = script + " " + strings.Join(h.cfg.ScriptArgs, " ")
	} else {
		cmdStr = script
	}

	cmdDir := filepath.Dir(script)
	if cmdDir == "" {
		cmdDir = "."
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(cctx, shell, "-c", cmdStr)
	cmd.Dir = cmdDir

	// Redirect stdin ke /dev/null agar script tidak hang menunggu input
	devNull, _ := os.Open(os.DevNull)
	cmd.Stdin = devNull
	defer devNull.Close()

	cmd.Env = append(os.Environ(),
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"HOOK_REASON="+reason,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	h.logger.Warn("[hook] menjalankan: %s -c '%s' (reason=%s)", shell, cmdStr, reason)
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