// cache_test.go
package cache

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// resetCache safely clears the global cache between tests.
func resetCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	cache = make(map[string]*CacheEntry)
}

func TestCheckCache_Empty(t *testing.T) {
	t.Parallel()
	resetCache()

	if _, ok := CheckCache("https://example.com"); ok {
		t.Fatalf("expected not found for empty cache")
	}
}

func TestAddToCacheAndCheck(t *testing.T) {
	t.Parallel()
	resetCache()

	url := "https://example.com/page"
	now := time.Now()
	want := &CacheEntry{
		Body:      []byte("hello"),
		Timestamp: now,
		Status:    200,
		Err:       nil,
		FinalURL:  url,
	}

	AddToCache(url, want)

	got, ok := CheckCache(url)
	if !ok {
		t.Fatalf("expected to find entry for %q", url)
	}
	if got != want {
		t.Fatalf("expected pointer equality; got %#v want %#v", got, want)
	}
	if string(got.Body) != "hello" || got.Status != 200 || got.FinalURL != url {
		t.Fatalf("unexpected entry fields: %#v", got)
	}
	// Timestamp sanity (allowing small drift if the test machine is fast/slow)
	if got.Timestamp.Before(now.Add(-1*time.Second)) || got.Timestamp.After(now.Add(1*time.Second)) {
		t.Fatalf("timestamp out of expected range: %v vs %v", got.Timestamp, now)
	}
}

func TestCacheSeparateKeys(t *testing.T) {
	t.Parallel()
	resetCache()

	urlA := "https://a.example.com"
	urlB := "https://b.example.com"

	entryA := &CacheEntry{Body: []byte("A"), Status: 201, FinalURL: urlA}
	entryB := &CacheEntry{Body: []byte("B"), Status: 202, FinalURL: urlB}

	AddToCache(urlA, entryA)
	AddToCache(urlB, entryB)

	gotA, okA := CheckCache(urlA)
	if !okA || gotA != entryA {
		t.Fatalf("expected entryA for %q", urlA)
	}
	gotB, okB := CheckCache(urlB)
	if !okB || gotB != entryB {
		t.Fatalf("expected entryB for %q", urlB)
	}
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	resetCache()

	const n = 1000
	var wg sync.WaitGroup
	wg.Add(n * 2)

	// Concurrent writers
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			url := "https://site.example.com/page/" + strconv.Itoa(i)
			AddToCache(url, &CacheEntry{
				Body:      []byte("body-" + strconv.Itoa(i)),
				Timestamp: time.Now(),
				Status:    200 + (i % 10),
				FinalURL:  url,
			})
		}()
	}

	// Concurrent readers (may race with writers; package is expected to be safe)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			url := "https://site.example.com/page/" + strconv.Itoa(i)
			// Busy-wait (with small sleeps) until either found or writers likely done.
			// This is just to exercise the RLock path under contention.
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if _, ok := CheckCache(url); ok {
					return
				}
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Post-condition: all keys should exist.
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()
	if len(cache) != n {
		t.Fatalf("expected %d entries, got %d", n, len(cache))
	}
	for i := 0; i < n; i++ {
		url := "https://site.example.com/page/" + strconv.Itoa(i)
		ce, ok := cache[url]
		if !ok {
			t.Fatalf("missing key %q after concurrent ops", url)
		}
		if ce == nil || ce.FinalURL != url {
			t.Fatalf("corrupted entry for %q: %#v", url, ce)
		}
	}
}
