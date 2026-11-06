package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	LvlErr  LogLevel = 0
	LvlWarn LogLevel = 1
	LvlInfo LogLevel = 2
	LvlDbg  LogLevel = 3
)

func parseLevel(s string) LogLevel {
	switch strings.ToLower(s) {
	case "debug":
		return LvlDbg
	case "warn":
		return LvlWarn
	case "error":
		return LvlErr
	}
	return LvlInfo
}

type Logger struct {
	mu         sync.Mutex
	level      LogLevel
	outputs    []io.Writer
	filePath   string
	file       *os.File
	fileSize   int64
	maxSize    int64
	maxBackups int
	listeners  []chan string
}

func NewLogger(level LogLevel) *Logger {
	return &Logger{level: level, outputs: []io.Writer{os.Stdout}}
}

func (l *Logger) SetFile(path string, maxMB, backups int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	l.file = f
	l.filePath = path
	l.fileSize = st.Size()
	l.maxSize = int64(maxMB) * 1024 * 1024
	l.maxBackups = backups
	return nil
}

func (l *Logger) rotateLocked() {
	if l.file == nil {
		return
	}
	l.file.Close()
	for i := l.maxBackups - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", l.filePath, i), fmt.Sprintf("%s.%d", l.filePath, i+1))
	}
	os.Rename(l.filePath, l.filePath+".1")
	l.file, _ = os.OpenFile(l.filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	l.fileSize = 0
}

func (l *Logger) Subscribe() chan string {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch := make(chan string, 200)
	l.listeners = append(l.listeners, ch)
	return ch
}

func (l *Logger) Unsubscribe(ch chan string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, c := range l.listeners {
		if c == ch {
			l.listeners = append(l.listeners[:i], l.listeners[i+1:]...)
			close(ch)
			return
		}
	}
}

func (l *Logger) write(lvl LogLevel, format string, a ...any) {
	if lvl > l.level {
		return
	}
	tag := "INFO"
	switch lvl {
	case LvlErr:
		tag = "ERR "
	case LvlWarn:
		tag = "WARN"
	case LvlDbg:
		tag = "DBG "
	}
	line := fmt.Sprintf("[%s][%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), tag, fmt.Sprintf(format, a...))

	l.mu.Lock()
	defer l.mu.Unlock()
	for _, w := range l.outputs {
		io.WriteString(w, line)
	}
	if l.file != nil {
		l.file.WriteString(line)
		l.fileSize += int64(len(line))
		if l.maxSize > 0 && l.fileSize >= l.maxSize {
			l.rotateLocked()
		}
	}
	for _, ch := range l.listeners {
		select {
		case ch <- line:
		default:
		}
	}
}

func (l *Logger) Info(f string, a ...any)  { l.write(LvlInfo, f, a...) }
func (l *Logger) Warn(f string, a ...any)  { l.write(LvlWarn, f, a...) }
func (l *Logger) Error(f string, a ...any) { l.write(LvlErr, f, a...) }
func (l *Logger) Debug(f string, a ...any) { l.write(LvlDbg, f, a...) }
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Sync()
		l.file.Close()
		l.file = nil
	}
	for _, ch := range l.listeners {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
	l.listeners = nil
}