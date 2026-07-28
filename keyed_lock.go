package tls_client

import (
	"context"
	"sync"
)

// keyedLockPool serializes work for one key while allowing unrelated keys to
// proceed independently. Entries are reference counted so high-cardinality
// targets don't remain in memory after the holder and all waiters leave.
type keyedLockPool struct {
	mu      sync.Mutex
	entries map[string]*keyedLockEntry
}

type keyedLockEntry struct {
	token chan struct{}
	refs  int
}

func (p *keyedLockPool) Lock(ctx context.Context, key string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}

	p.mu.Lock()
	if p.entries == nil {
		p.entries = make(map[string]*keyedLockEntry)
	}
	entry := p.entries[key]
	if entry == nil {
		entry = &keyedLockEntry{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		p.entries[key] = entry
	}
	entry.refs++
	p.mu.Unlock()

	select {
	case <-entry.token:
		if err := ctx.Err(); err != nil {
			entry.token <- struct{}{}
			p.releaseReference(key, entry)
			return nil, err
		}
		var once sync.Once
		return func() {
			once.Do(func() {
				entry.token <- struct{}{}
				p.releaseReference(key, entry)
			})
		}, nil
	case <-ctx.Done():
		p.releaseReference(key, entry)
		return nil, ctx.Err()
	}
}

func (p *keyedLockPool) releaseReference(key string, entry *keyedLockEntry) {
	p.mu.Lock()
	entry.refs--
	if entry.refs == 0 && p.entries[key] == entry {
		delete(p.entries, key)
	}
	p.mu.Unlock()
}
