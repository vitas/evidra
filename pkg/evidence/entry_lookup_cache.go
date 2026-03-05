package evidence

import (
	"path/filepath"
	"sync"
)

type entryLookupCache struct {
	mu      sync.RWMutex
	entries map[string]EvidenceEntry
}

var lookupCaches sync.Map // map[string]*entryLookupCache, keyed by normalized evidence path

func lookupCacheKey(path string) string {
	_, resolved, err := detectStoreMode(path)
	if err == nil && resolved != "" {
		return resolved
	}
	return filepath.Clean(path)
}

func ensureLookupCache(path string) *entryLookupCache {
	key := lookupCacheKey(path)
	if v, ok := lookupCaches.Load(key); ok {
		return v.(*entryLookupCache)
	}
	cache := &entryLookupCache{entries: make(map[string]EvidenceEntry)}
	actual, _ := lookupCaches.LoadOrStore(key, cache)
	return actual.(*entryLookupCache)
}

func cacheEntryByID(path string, entry EvidenceEntry) {
	if entry.EntryID == "" {
		return
	}
	cache := ensureLookupCache(path)
	cache.mu.Lock()
	cache.entries[entry.EntryID] = entry
	cache.mu.Unlock()
}

func lookupCachedEntryByID(path, entryID string) (EvidenceEntry, bool) {
	if entryID == "" {
		return EvidenceEntry{}, false
	}
	cache := ensureLookupCache(path)
	cache.mu.RLock()
	entry, ok := cache.entries[entryID]
	cache.mu.RUnlock()
	return entry, ok
}

func resetLookupCacheForPath(path string) {
	lookupCaches.Delete(lookupCacheKey(path))
}
