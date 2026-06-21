package main

import (
	"testing"
)

func TestKeyManagerAddKey(t *testing.T) {
	km := NewKeyManager([]string{"a"}, nil)
	idx := km.AddKey("b", "")
	if idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
	if _, key, ok := km.Current(); !ok || key != "a" {
		t.Fatalf("expected current key 'a', got %q", key)
	}
}

func TestKeyManagerAddKeyWithName(t *testing.T) {
	km := NewKeyManager([]string{"a"}, []string{"first"})
	idx := km.AddKey("b", "second")
	if idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
	st := km.Status()
	if st.Keys[0].Name != "first" || st.Keys[1].Name != "second" {
		t.Fatalf("unexpected names: %+v", st.Keys)
	}
}

func TestKeyManagerAddKeyWhenCurrentIsMinusOne(t *testing.T) {
	km := NewKeyManager([]string{"a"}, nil)
	km.MarkExhausted(0)
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected no current key")
	}
	idx := km.AddKey("b", "")
	if idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
	_, key, ok := km.Current()
	if !ok {
		t.Fatal("expected a current key after adding when current was -1")
	}
	if key != "b" {
		t.Fatalf("expected current key 'b', got %q", key)
	}
}

func TestKeyManagerRemoveKeyLastBlocked(t *testing.T) {
	km := NewKeyManager([]string{"a"}, nil)
	if err := km.RemoveKey(0); err == nil {
		t.Fatal("expected error removing last key")
	}
}

func TestKeyManagerRemoveKeyNonCurrent(t *testing.T) {
	km := NewKeyManager([]string{"a", "b", "c"}, nil)
	if err := km.RemoveKey(2); err != nil {
		t.Fatal(err)
	}
	idx, key, ok := km.Current()
	if !ok || idx != 0 || key != "a" {
		t.Fatalf("expected current (0, a), got (%d, %s)", idx, key)
	}
	if km.keys[1] != "b" || len(km.keys) != 2 {
		t.Fatalf("unexpected state after removal")
	}
}

func TestKeyManagerRemoveKeyCurrentAutoAdvances(t *testing.T) {
	km := NewKeyManager([]string{"a", "b"}, nil)
	if err := km.RemoveKey(0); err != nil {
		t.Fatal(err)
	}
	idx, key, ok := km.Current()
	if !ok || idx != 0 || key != "b" {
		t.Fatalf("expected current (0, b), got (%d, %s)", idx, key)
	}
}

func TestKeyManagerRemoveKeyCurrentExhaustedRemaining(t *testing.T) {
	km := NewKeyManager([]string{"a", "b"}, nil)
	km.MarkExhausted(1)
	if err := km.RemoveKey(0); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected no current key")
	}
	if !km.AllExhausted() {
		t.Fatal("expected all exhausted")
	}
}

func TestKeyManagerReorder(t *testing.T) {
	km := NewKeyManager([]string{"a", "b", "c"}, nil)
	if err := km.Reorder([]int{2, 0, 1}); err != nil {
		t.Fatal(err)
	}
	if km.current != 1 {
		t.Fatalf("expected current 1, got %d", km.current)
	}
	_, key, ok := km.Current()
	if !ok || key != "a" {
		t.Fatalf("expected current key 'a', got %q", key)
	}
	if km.keys[0] != "c" || km.keys[1] != "a" || km.keys[2] != "b" {
		t.Fatalf("unexpected order: %v", km.keys)
	}
}

func TestKeyManagerReorderInvalidPermutation(t *testing.T) {
	km := NewKeyManager([]string{"a", "b"}, nil)
	if err := km.Reorder([]int{2, 0}); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
	if err := km.Reorder([]int{0, 1, 2}); err == nil {
		t.Fatal("expected error for wrong length")
	}
	if err := km.Reorder([]int{0, 0}); err == nil {
		t.Fatal("expected error for duplicate index")
	}
}

func TestKeyManagerReorderCurrentMinusOne(t *testing.T) {
	km := NewKeyManager([]string{"a"}, nil)
	km.MarkExhausted(0)
	if err := km.Reorder([]int{0}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := km.Current(); ok {
		t.Fatal("expected no current key after reorder")
	}
}

func TestKeyManagerReorderCurrentTracksKey(t *testing.T) {
	km := NewKeyManager([]string{"a", "b", "c"}, nil)
	// current is 0 ("a"). Exhaust 0, now current advances to 1 ("b").
	km.MarkExhausted(0)
	idx, key, _ := km.Current()
	if idx != 1 || key != "b" {
		t.Fatalf("expected current (1, b), got (%d, %s)", idx, key)
	}
	// Reorder: [0, 2, 1] means new[0]=old[0]="a", new[1]=old[2]="c", new[2]=old[1]="b"
	// Current key "b" was at old index 1, moves to new index 2
	if err := km.Reorder([]int{0, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if km.current != 2 {
		t.Fatalf("expected current 2, got %d", km.current)
	}
	_, key, _ = km.Current()
	if key != "b" {
		t.Fatalf("expected current key 'b', got %q", key)
	}
}

func TestKeyManagerStatusIncludesPrefixSuffix(t *testing.T) {
	km := NewKeyManager([]string{"sk-secret-abcdef123456"}, []string{"primary"})
	st := km.Status()
	if len(st.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(st.Keys))
	}
	if st.Keys[0].KeyPrefix != "sk-sec" {
		t.Fatalf("expected key prefix 'sk-sec', got %q", st.Keys[0].KeyPrefix)
	}
	if st.Keys[0].KeySuffix != "3456" {
		t.Fatalf("expected key suffix '3456', got %q", st.Keys[0].KeySuffix)
	}
	if st.Keys[0].Name != "primary" {
		t.Fatalf("expected name 'primary', got %q", st.Keys[0].Name)
	}
}

func TestKeyPrefixShort(t *testing.T) {
	if got := keyPrefix("ab"); got != "ab" {
		t.Fatalf("expected 'ab', got %q", got)
	}
}

func TestKeySuffixShort(t *testing.T) {
	if got := keySuffix("abc"); got != "" {
		t.Fatalf("expected '', got %q", got)
	}
	if got := keySuffix("abc1234567"); got != "" {
		t.Fatalf("expected '' for 10-char key, got %q", got)
	}
	if got := keySuffix("abc12345678"); got != "5678" {
		t.Fatalf("expected '5678', got %q", got)
	}
}
