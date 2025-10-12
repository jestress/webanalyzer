package cache

import (
	"sync"
	"time"
)

type CacheEntry struct {
	Body      []byte
	Timestamp time.Time
	Status    int
	Err       error
	FinalURL  string
}

var (
	cache      = make(map[string]*CacheEntry)
	cacheMutex sync.RWMutex
)

func CheckCache(u string) (*CacheEntry, bool) {
	cacheMutex.RLock()
	entry, found := cache[u]
	cacheMutex.RUnlock()
	return entry, found
}

func AddToCache(u string, entry *CacheEntry) {
	cacheMutex.Lock()
	cache[u] = entry
	cacheMutex.Unlock()
}
