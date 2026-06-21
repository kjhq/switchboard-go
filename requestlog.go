package main

import (
	"sync"
	"time"
)

type RequestLogEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	KeyIndex   int       `json:"key_index"`
	Status     int       `json:"status"`
	DurationMs int64     `json:"duration_ms"`
}

type RequestLog struct {
	mu    sync.Mutex
	buf   []RequestLogEntry
	size  int
	pos   int
	count int
}

func NewRequestLog(size int) *RequestLog {
	return &RequestLog{
		buf:  make([]RequestLogEntry, size),
		size: size,
	}
}

func (rl *RequestLog) Add(entry RequestLogEntry) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.buf[rl.pos] = entry
	rl.pos = (rl.pos + 1) % rl.size
	if rl.count < rl.size {
		rl.count++
	}
}

func (rl *RequestLog) GetAll() []RequestLogEntry {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.count == 0 {
		return nil
	}
	entries := make([]RequestLogEntry, rl.count)
	if rl.count < rl.size {
		copy(entries, rl.buf[:rl.count])
		return entries
	}
	n := copy(entries, rl.buf[rl.pos:])
	copy(entries[n:], rl.buf[:rl.pos])
	return entries
}

func (rl *RequestLog) Len() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.count
}
