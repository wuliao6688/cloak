package profiles

import "sync"

// registryMu protects the global profile registry (canonicalTLSClients,
// MappedTLSClients, profileMetadata, and related indexes) against
// concurrent reads during hot-reload. All registry mutations must hold
// the write lock; all registry reads must hold the read lock.
//
// Usage:
//
//	registryMu.RLock()
//	profile := MappedTLSClients[key]
//	registryMu.RUnlock()
var registryMu sync.RWMutex

// RLockRegistry acquires a read lock on the profile registry. Callers
// that read canonicalTLSClients, MappedTLSClients, or profileMetadata
// must hold this lock for the duration of the access.
func RLockRegistry() {
	registryMu.RLock()
}

// RUnlockRegistry releases the read lock.
func RUnlockRegistry() {
	registryMu.RUnlock()
}

// LockRegistry acquires the write lock. Internal use only.
func LockRegistry() {
	registryMu.Lock()
}

// UnlockRegistry releases the write lock. Internal use only.
func UnlockRegistry() {
	registryMu.Unlock()
}
