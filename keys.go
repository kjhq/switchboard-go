package main

import (
	"fmt"
	"sync"
	"time"
)

type KeyState string

const (
	KeyUnknown   KeyState = "unknown"
	KeyAvailable KeyState = "available"
	KeyExhausted KeyState = "exhausted"
)

type KeyManager struct {
	mu             sync.Mutex
	keys           []string
	names          []string
	states         []KeyState
	last429        map[int]time.Time
	current        int
	allNotified    bool
	notifiedSwitch map[int]bool
}

func NewKeyManager(keys []string, names []string) *KeyManager {
	states := make([]KeyState, len(keys))
	for i := range states {
		states[i] = KeyUnknown
	}
	if names == nil {
		names = make([]string, len(keys))
	}
	cur := -1
	if len(keys) > 0 {
		cur = 0
	}
	return &KeyManager{
		keys:           append([]string(nil), keys...),
		names:          append([]string(nil), names...),
		states:         states,
		last429:        map[int]time.Time{},
		notifiedSwitch: map[int]bool{},
		current:        cur,
	}
}

func (m *KeyManager) Current() (int, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentLocked()
}

func (m *KeyManager) currentLocked() (int, string, bool) {
	if len(m.keys) == 0 || m.current == -1 || m.allExhaustedLocked() {
		return 0, "", false
	}
	if m.states[m.current] == KeyExhausted {
		m.advanceLocked()
	}
	if m.current == -1 || m.states[m.current] == KeyExhausted {
		return 0, "", false
	}
	return m.current, m.keys[m.current], true
}

func (m *KeyManager) MarkExhausted(i int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.keys) {
		return
	}
	m.states[i] = KeyExhausted
	m.last429[i] = time.Now().UTC()
	if i == m.current {
		m.advanceLocked()
	}
}

func (m *KeyManager) ShouldNotifySwitch(i int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.notifiedSwitch[i] {
		return false
	}
	m.notifiedSwitch[i] = true
	return true
}

func (m *KeyManager) ShouldNotifyAllExhausted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.allNotified {
		return false
	}
	if !m.allExhaustedLocked() {
		return false
	}
	m.allNotified = true
	return true
}

func (m *KeyManager) AdvanceOnExhaustion() (int, string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.keys) == 0 || m.current == -1 {
		return 0, "", false
	}
	m.advanceLocked()
	if m.current == -1 {
		return 0, "", false
	}
	return m.current, m.keys[m.current], true
}

func (m *KeyManager) advanceLocked() {
	if len(m.keys) == 0 || m.current == -1 {
		return
	}
	start := m.current
	for step := 1; step <= len(m.keys); step++ {
		next := (start + step) % len(m.keys)
		if m.states[next] != KeyExhausted {
			m.current = next
			return
		}
	}
	m.current = -1
}

func (m *KeyManager) AllExhausted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.allExhaustedLocked()
}

func (m *KeyManager) allExhaustedLocked() bool {
	for _, st := range m.states {
		if st != KeyExhausted {
			return false
		}
	}
	return true
}

func keyPrefix(k string) string {
	if len(k) > 6 {
		return k[:6]
	}
	return k
}

func keySuffix(k string) string {
	if len(k) > 10 {
		return k[len(k)-4:]
	}
	return ""
}

func (m *KeyManager) Status() StatusResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	states := make([]PerKeyStatus, len(m.keys))
	for i := range m.keys {
		state := m.states[i]
		if i == m.current && state != KeyExhausted {
			state = KeyAvailable
		}
		states[i] = PerKeyStatus{
			Index:       i,
			Name:        m.names[i],
			KeyPrefix:   keyPrefix(m.keys[i]),
			KeySuffix:   keySuffix(m.keys[i]),
			State:       string(state),
			Last429Time: m.last429String(i),
			Current:     i == m.current,
		}
	}
	return StatusResponse{
		CurrentKeyIndex: m.current,
		Keys:            states,
		Note:            "unknown means the key has not yet been validated or used since startup; remaining usage is unavailable from opencode-go API.",
	}
}

func (m *KeyManager) SetState(i int, state KeyState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.states) {
		return
	}
	m.states[i] = state
	if state == KeyExhausted {
		m.last429[i] = time.Now().UTC()
		if i == m.current {
			m.advanceLocked()
		}
	}
	if state != KeyExhausted && m.current == -1 {
		m.current = i
	}
}

func (m *KeyManager) MarkAvailable(i int) { m.SetState(i, KeyAvailable) }

func (m *KeyManager) last429String(i int) string {
	if t, ok := m.last429[i]; ok && !t.IsZero() {
		return t.Format(time.RFC3339)
	}
	return ""
}

type PerKeyStatus struct {
	Index       int    `json:"index"`
	Name        string `json:"name,omitempty"`
	KeyPrefix   string `json:"key_prefix,omitempty"`
	KeySuffix   string `json:"key_suffix,omitempty"`
	State       string `json:"state"`
	Last429Time string `json:"last_429_time,omitempty"`
	Current     bool   `json:"current"`
}

type StatusResponse struct {
	CurrentKeyIndex int            `json:"current_key_index"`
	Keys            []PerKeyStatus `json:"keys"`
	Note            string         `json:"note"`
}

type ValidateKeyResult struct {
	Index  int    `json:"index"`
	State  string `json:"state"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

type ValidateKeysResponse struct {
	Results []ValidateKeyResult `json:"results"`
}

func (m *KeyManager) AddKey(key, name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := len(m.keys)
	m.keys = append(m.keys, key)
	m.names = append(m.names, name)
	m.states = append(m.states, KeyUnknown)
	if m.current == -1 {
		m.current = idx
	}
	return idx
}

func (m *KeyManager) RemoveKey(i int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if i < 0 || i >= len(m.keys) {
		return fmt.Errorf("key index %d out of range", i)
	}
	if len(m.keys) == 1 {
		return fmt.Errorf("cannot remove last key")
	}
	wasCurrent := m.current == i
	m.keys = append(m.keys[:i], m.keys[i+1:]...)
	m.names = append(m.names[:i], m.names[i+1:]...)
	m.states = append(m.states[:i], m.states[i+1:]...)
	if wasCurrent {
		m.current = i
		m.advanceLocked()
	} else if m.current > i {
		m.current--
	}
	newLast429 := make(map[int]time.Time, len(m.last429))
	newNotified := make(map[int]bool, len(m.notifiedSwitch))
	for k, v := range m.last429 {
		if k < i {
			newLast429[k] = v
		} else if k > i {
			newLast429[k-1] = v
		}
	}
	for k, v := range m.notifiedSwitch {
		if k < i {
			newNotified[k] = v
		} else if k > i {
			newNotified[k-1] = v
		}
	}
	m.last429 = newLast429
	m.notifiedSwitch = newNotified
	return nil
}

func (m *KeyManager) KeyEntries() []KeyEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := make([]KeyEntry, len(m.keys))
	for i := range m.keys {
		entries[i] = KeyEntry{Key: m.keys[i], Name: m.names[i]}
	}
	return entries
}

func (m *KeyManager) Reorder(indices []int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(indices) != len(m.keys) {
		return fmt.Errorf("permutation length %d != keys %d", len(indices), len(m.keys))
	}
	seen := make(map[int]bool, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(m.keys) {
			return fmt.Errorf("index %d out of range", idx)
		}
		if seen[idx] {
			return fmt.Errorf("duplicate index %d", idx)
		}
		seen[idx] = true
	}
	if len(seen) != len(m.keys) {
		return fmt.Errorf("invalid permutation")
	}
	newKeys := make([]string, len(m.keys))
	newNames := make([]string, len(m.keys))
	newStates := make([]KeyState, len(m.keys))
	newLast429 := make(map[int]time.Time, len(m.last429))
	newNotified := make(map[int]bool, len(m.notifiedSwitch))
	currentKey := -1
	if m.current >= 0 && m.current < len(m.keys) {
		currentKey = m.current
	}
	for newIdx, oldIdx := range indices {
		newKeys[newIdx] = m.keys[oldIdx]
		newNames[newIdx] = m.names[oldIdx]
		newStates[newIdx] = m.states[oldIdx]
		if t, ok := m.last429[oldIdx]; ok {
			newLast429[newIdx] = t
		}
		if n, ok := m.notifiedSwitch[oldIdx]; ok {
			newNotified[newIdx] = n
		}
		if oldIdx == currentKey {
			m.current = newIdx
		}
	}
	m.keys = newKeys
	m.names = newNames
	m.states = newStates
	m.last429 = newLast429
	m.notifiedSwitch = newNotified
	return nil
}
