package main

import (
	"testing"
	"time"
)

func TestRequestLogAddAndGet(t *testing.T) {
	rl := NewRequestLog(10)
	if rl.Len() != 0 {
		t.Fatalf("expected empty log")
	}
	rl.Add(RequestLogEntry{Method: "GET", Path: "/v1/models", KeyIndex: 0, Status: 200, DurationMs: 50, Timestamp: time.Now()})
	if rl.Len() != 1 {
		t.Fatalf("expected 1 entry")
	}
	entries := rl.GetAll()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry from GetAll")
	}
}

func TestRequestLogRingEviction(t *testing.T) {
	rl := NewRequestLog(3)
	for i := 0; i < 5; i++ {
		rl.Add(RequestLogEntry{Method: "POST", Path: "/v1/chat/completions", KeyIndex: 0, Status: 200, DurationMs: 100, Timestamp: time.Now()})
	}
	if rl.Len() != 3 {
		t.Fatalf("expected 3 entries after eviction, got %d", rl.Len())
	}
	entries := rl.GetAll()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries from GetAll, got %d", len(entries))
	}
}

func TestRequestLogOrderPreserved(t *testing.T) {
	rl := NewRequestLog(3)
	rl.Add(RequestLogEntry{Method: "A"})
	rl.Add(RequestLogEntry{Method: "B"})
	rl.Add(RequestLogEntry{Method: "C"})
	rl.Add(RequestLogEntry{Method: "D"})
	entries := rl.GetAll()
	if entries[0].Method != "B" || entries[1].Method != "C" || entries[2].Method != "D" {
		t.Fatalf("expected [B, C, D], got %v", entryMethods(entries))
	}
}

func TestRequestLogPartialBuffer(t *testing.T) {
	rl := NewRequestLog(10)
	rl.Add(RequestLogEntry{Method: "X"})
	rl.Add(RequestLogEntry{Method: "Y"})
	entries := rl.GetAll()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Method != "X" || entries[1].Method != "Y" {
		t.Fatalf("expected [X, Y], got %v", entryMethods(entries))
	}
}

func entryMethods(entries []RequestLogEntry) []string {
	m := make([]string, len(entries))
	for i, e := range entries {
		m[i] = e.Method
	}
	return m
}
