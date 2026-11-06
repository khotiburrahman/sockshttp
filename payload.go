package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	reRotate = regexp.MustCompile(`\[rotate=(.*?)\]`)
	reRandom = regexp.MustCompile(`\[random=(.*?)\]`)
	rotateMu sync.Mutex
	rotates  = map[int]int{}
	lastUsed string
)

func formatPayload(host string, port int, payload string) string {
	hp := fmt.Sprintf("%s:%d", host, port)
	repl := map[string]string{
		"[method]": "CONNECT", "[host]": host, "[port]": strconv.Itoa(port),
		"[host_port]": hp, "[protocol]": "HTTP/1.0", "[ssh]": hp,
		"[crlf]": "\r\n", "[cr]": "\r", "[lf]": "\n", "[lfcr]": "\n\r",
		"\\n": "\n", "\\r": "\r",
		"[ua]": "Mozilla/5.0 (Windows NT 6.3; WOW64) AppleWebKit/537.36",
	}
	for k, v := range repl {
		payload = strings.ReplaceAll(payload, k, v)
	}
	rotateMu.Lock()
	if payload != lastUsed {
		rotates = map[int]int{}
		lastUsed = payload
	}
	for i, m := range reRotate.FindAllStringSubmatch(payload, -1) {
		opts := strings.Split(m[1], ";")
		if len(opts) == 0 {
			continue
		}
		idx := rotates[i] % len(opts)
		payload = strings.Replace(payload, m[0], opts[idx], 1)
		rotates[i] = idx + 1
	}
	rotateMu.Unlock()
	payload = reRandom.ReplaceAllStringFunc(payload, func(m string) string {
		sub := reRandom.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		opts := strings.Split(sub[1], ";")
		if len(opts) == 0 {
			return m
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(opts))))
		return opts[n.Int64()]
	})
	return payload
}

func injectSplit(payload string, out io.Writer) error {
	parts := strings.Split(payload, "[delay_split")
	delay := time.Second

	for i, p := range parts {
		if i > 0 {
			// p dimulai dengan "]" atau "=NNms]rest"
			if p == "" || p[0] != ']' {
				// coba parse "=duration]"
				if idx := strings.IndexByte(p, ']'); idx >= 0 {
					trimmed := strings.TrimPrefix(p[:idx], "=")
					if trimmed != "" {
						if d, err := time.ParseDuration(trimmed); err == nil {
							delay = d
						}
					}
					p = p[idx+1:]
				}
			} else {
				p = p[1:]
			}
		}
		for _, sp := range strings.Split(p, "[split]") {
			if _, err := io.WriteString(out, sp); err != nil {
				return err
			}
		}
		if i != len(parts)-1 {
			time.Sleep(delay)
		}
	}
	return nil
}