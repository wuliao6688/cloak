package tls_client_cffi_src

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	tls_client "github.com/bogdanfinn/tls-client"
)

const DefaultMaxSessionCacheEntries = 1024

// A zero TTL preserves sessions until explicit removal or LRU capacity
// eviction. Set TLS_CLIENT_SESSION_CACHE_TTL (for example "30m") to enable
// idle expiration for CFFI deployments.
const DefaultSessionCacheTTL time.Duration = 0

type sessionClientEntry struct {
	client tls_client.HttpClient

	metadataLock sync.Mutex
	lastUsed     time.Time
	sequence     uint64
}

var (
	sessionCachePolicyLock sync.RWMutex
	sessionCacheMaxEntries = DefaultMaxSessionCacheEntries
	sessionCacheTTL        = DefaultSessionCacheTTL
	sessionCacheSequence   atomic.Uint64
)

func init() {
	if value := os.Getenv("TLS_CLIENT_SESSION_CACHE_MAX_ENTRIES"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed >= -1 {
			if parsed == 0 {
				parsed = DefaultMaxSessionCacheEntries
			}
			sessionCacheMaxEntries = parsed
		}
	}
	if value := os.Getenv("TLS_CLIENT_SESSION_CACHE_TTL"); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed >= 0 {
			sessionCacheTTL = parsed
		}
	}
}

// ConfigureSessionCache updates the process-wide CFFI session cache policy.
// maxEntries uses zero for the default, -1 for unlimited, or a positive bound.
// idleTTL zero disables expiration. Existing in-flight sessions are never
// evicted; excess entries are pruned as flights finish.
func ConfigureSessionCache(maxEntries int, idleTTL time.Duration) error {
	if maxEntries < -1 {
		return fmt.Errorf("max session cache entries must be -1, zero, or a positive value")
	}
	if idleTTL < 0 {
		return fmt.Errorf("session cache TTL must not be negative")
	}
	if maxEntries == 0 {
		maxEntries = DefaultMaxSessionCacheEntries
	}

	sessionLifecycleLock.Lock()
	sessionCachePolicyLock.Lock()
	sessionCacheMaxEntries = maxEntries
	sessionCacheTTL = idleTTL
	sessionCachePolicyLock.Unlock()
	evicted := pruneSessionCache(time.Now(), "")
	sessionLifecycleLock.Unlock()
	closeSessionClients(evicted)
	return nil
}

func SessionCacheConfiguration() (maxEntries int, idleTTL time.Duration) {
	sessionCachePolicyLock.RLock()
	defer sessionCachePolicyLock.RUnlock()
	return sessionCacheMaxEntries, sessionCacheTTL
}

// getCachedSessionClient returns and touches a cached client. A caller that
// already owns the session flight may set expireProtected so an idle entry can
// be replaced at the beginning of that exclusive operation. Unleased callers
// never expire a session that is executing or waiting for a flight.
func getCachedSessionClient(sessionID string, expireProtected bool) (tls_client.HttpClient, bool) {
	now := time.Now()
	_, idleTTL := SessionCacheConfiguration()
	registryLocked := !expireProtected
	if registryLocked {
		sessionLocksLock.Lock()
	}
	clientsLock.RLock()
	entry, ok := clients[sessionID]
	canExpire := expireProtected || sessionLocks[sessionID] == nil
	expired := ok && canExpire && sessionEntryExpired(entry, now, idleTTL)
	if ok && !expired {
		touchSessionEntry(entry, now)
	}
	clientsLock.RUnlock()

	var expiredEntry *sessionClientEntry
	if expired {
		clientsLock.Lock()
		entry = clients[sessionID]
		if entry != nil && sessionEntryExpired(entry, now, idleTTL) {
			delete(clients, sessionID)
			expiredEntry = entry
			ok = false
		} else if entry != nil {
			touchSessionEntry(entry, now)
			ok = true
		} else {
			ok = false
		}
		clientsLock.Unlock()
	}
	if registryLocked {
		sessionLocksLock.Unlock()
	}

	if expiredEntry != nil {
		expiredEntry.client.CloseIdleConnections()
	}
	if !ok {
		return nil, false
	}
	return entry.client, true
}

func storeCachedSessionClient(sessionID string, client tls_client.HttpClient) []tls_client.HttpClient {
	clientsLock.Lock()
	entry := &sessionClientEntry{client: client}
	touchSessionEntry(entry, time.Now())
	clients[sessionID] = entry
	clientsLock.Unlock()
	return pruneSessionCacheIfNeeded(time.Now(), sessionID)
}

func touchCachedSessionClient(sessionID string) {
	clientsLock.RLock()
	if entry := clients[sessionID]; entry != nil {
		touchSessionEntry(entry, time.Now())
	}
	clientsLock.RUnlock()
}

func touchSessionEntry(entry *sessionClientEntry, now time.Time) {
	sequence := sessionCacheSequence.Add(1)
	entry.metadataLock.Lock()
	entry.lastUsed = now
	entry.sequence = sequence
	entry.metadataLock.Unlock()
}

func sessionEntryMetadata(entry *sessionClientEntry) (lastUsed time.Time, sequence uint64) {
	entry.metadataLock.Lock()
	lastUsed = entry.lastUsed
	sequence = entry.sequence
	entry.metadataLock.Unlock()
	return lastUsed, sequence
}

func sessionEntryExpired(entry *sessionClientEntry, now time.Time, idleTTL time.Duration) bool {
	if entry == nil || idleTTL <= 0 {
		return false
	}
	lastUsed, _ := sessionEntryMetadata(entry)
	return now.Sub(lastUsed) >= idleTTL
}

// pruneSessionCacheIfNeeded avoids taking the global session registries on the
// default no-TTL path while the cache remains within its configured capacity.
// Insertions that exceed the capacity still perform the full leased-session-
// aware prune, and enabling TTL keeps the existing expiration behavior.
func pruneSessionCacheIfNeeded(now time.Time, protectedSessionID string) []tls_client.HttpClient {
	maxEntries, idleTTL := SessionCacheConfiguration()
	if idleTTL <= 0 {
		clientsLock.RLock()
		withinCapacity := maxEntries < 0 || len(clients) <= maxEntries
		clientsLock.RUnlock()
		if withinCapacity {
			return nil
		}
	}
	return pruneSessionCache(now, protectedSessionID)
}

// pruneSessionCache removes expired and least-recently-used sessions while
// holding the session-lock registry stable. Registered flights and the
// protected session are skipped, so pruning never changes a live request's
// proxy, cookies, or transport.
func pruneSessionCache(now time.Time, protectedSessionID string) []tls_client.HttpClient {
	maxEntries, idleTTL := SessionCacheConfiguration()
	sessionLocksLock.Lock()
	clientsLock.Lock()

	var evicted []tls_client.HttpClient
	if idleTTL > 0 {
		for sessionID, entry := range clients {
			if sessionID == protectedSessionID || sessionLocks[sessionID] != nil {
				continue
			}
			if sessionEntryExpired(entry, now, idleTTL) {
				delete(clients, sessionID)
				evicted = append(evicted, entry.client)
			}
		}
	}

	for maxEntries >= 0 && len(clients) > maxEntries {
		oldestSessionID := ""
		oldestSequence := ^uint64(0)
		for sessionID, entry := range clients {
			if sessionID == protectedSessionID || sessionLocks[sessionID] != nil {
				continue
			}
			_, sequence := sessionEntryMetadata(entry)
			if sequence < oldestSequence {
				oldestSessionID = sessionID
				oldestSequence = sequence
			}
		}
		if oldestSessionID == "" {
			break
		}
		evicted = append(evicted, clients[oldestSessionID].client)
		delete(clients, oldestSessionID)
	}

	clientsLock.Unlock()
	sessionLocksLock.Unlock()
	return evicted
}

func closeSessionClients(clients []tls_client.HttpClient) {
	for _, client := range clients {
		if client != nil {
			client.CloseIdleConnections()
		}
	}
}
