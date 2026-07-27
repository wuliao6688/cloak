package tls_client_cffi_src

import (
	"sync"
	"testing"
	"time"

	tls_client "github.com/bogdanfinn/tls-client"
)

func TestConcurrentCreateClientReusesSingleSession(t *testing.T) {
	ClearSessionCache()
	defer ClearSessionCache()

	const workerCount = 32
	sessionID := "concurrent-session"
	input := RequestInput{SessionId: &sessionID}

	clientsByWorker := make([]tls_client.HttpClient, workerCount)
	errorsByWorker := make([]*TLSClientError, workerCount)

	var waitGroup sync.WaitGroup
	for index := 0; index < workerCount; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			client, returnedSessionID, withSession, clientErr := CreateClient(input)
			if returnedSessionID != sessionID || !withSession {
				t.Errorf("unexpected session result: id=%q withSession=%v", returnedSessionID, withSession)
			}
			clientsByWorker[index] = client
			errorsByWorker[index] = clientErr
		}(index)
	}
	waitGroup.Wait()

	firstClient := clientsByWorker[0]
	if firstClient == nil {
		t.Fatal("first worker did not receive a client")
	}
	for index := range clientsByWorker {
		if errorsByWorker[index] != nil {
			t.Fatalf("worker %d failed: %v", index, errorsByWorker[index])
		}
		if clientsByWorker[index] != firstClient {
			t.Fatalf("worker %d received a different client for the same session", index)
		}
	}

	storedClient, err := GetClient(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if storedClient != firstClient {
		t.Fatal("stored session client differs from the client returned to callers")
	}
}

func TestCreateClientDoesNotStoreFailedSession(t *testing.T) {
	ClearSessionCache()
	defer ClearSessionCache()

	sessionID := "invalid-profile-session"
	input := RequestInput{
		SessionId:           &sessionID,
		TLSClientIdentifier: "not-a-profile",
	}

	client, _, _, clientErr := CreateClient(input)
	if clientErr == nil {
		t.Fatal("unknown profile should fail strict CFFI resolution")
	}
	if client != nil {
		t.Fatalf("failed client creation returned a client: %v", client)
	}
	if _, err := GetClient(sessionID); err == nil {
		t.Fatal("failed client creation must not populate the session cache")
	}
}

func TestExistingSessionStillRejectsUnknownProfileIdentifier(t *testing.T) {
	ClearSessionCache()
	defer ClearSessionCache()

	sessionID := "existing-session-invalid-profile"
	client, _, _, clientErr := CreateClient(RequestInput{SessionId: &sessionID})
	if clientErr != nil || client == nil {
		t.Fatalf("failed to create initial session client: client=%v error=%v", client, clientErr)
	}

	client, _, _, clientErr = CreateClient(RequestInput{
		SessionId:           &sessionID,
		TLSClientIdentifier: "not-a-profile",
	})
	if clientErr == nil {
		t.Fatal("existing sessions must not bypass strict profile validation")
	}
	if client != nil {
		t.Fatalf("invalid profile request returned a client: %v", client)
	}
}

func TestCreateClientForRequestLeasesSameSessionUntilRelease(t *testing.T) {
	ClearSessionCache()
	defer ClearSessionCache()

	sessionID := "leased-session"
	input := RequestInput{SessionId: &sessionID}
	firstClient, _, _, releaseFirst, clientErr := CreateClientForRequest(input)
	if clientErr != nil || firstClient == nil || releaseFirst == nil {
		t.Fatalf("failed to acquire first session lease: client=%v release=%v error=%v", firstClient, releaseFirst != nil, clientErr)
	}
	defer releaseFirst()

	type leaseResult struct {
		client  tls_client.HttpClient
		release func()
		err     *TLSClientError
	}
	secondLease := make(chan leaseResult, 1)
	go func() {
		client, _, _, release, clientErr := CreateClientForRequest(input)
		secondLease <- leaseResult{client: client, release: release, err: clientErr}
	}()

	select {
	case result := <-secondLease:
		if result.release != nil {
			result.release()
		}
		t.Fatal("same-session request acquired a lease before the first request released it")
	case <-time.After(100 * time.Millisecond):
	}

	releaseFirst()
	select {
	case result := <-secondLease:
		if result.release != nil {
			defer result.release()
		}
		if result.err != nil {
			t.Fatalf("second lease failed: %v", result.err)
		}
		if result.client != firstClient {
			t.Fatal("same session did not reuse its client after acquiring the next lease")
		}
	case <-time.After(time.Second):
		t.Fatal("same-session request did not acquire the lease after release")
	}
}

func TestCreateClientForRequestAllowsIndependentSessionsInParallel(t *testing.T) {
	ClearSessionCache()
	defer ClearSessionCache()

	firstSessionID := "parallel-session-one"
	_, _, _, releaseFirst, clientErr := CreateClientForRequest(RequestInput{SessionId: &firstSessionID})
	if clientErr != nil || releaseFirst == nil {
		t.Fatalf("failed to acquire first session lease: %v", clientErr)
	}
	defer releaseFirst()

	secondSessionID := "parallel-session-two"
	secondLease := make(chan func(), 1)
	secondError := make(chan *TLSClientError, 1)
	go func() {
		_, _, _, release, clientErr := CreateClientForRequest(RequestInput{SessionId: &secondSessionID})
		secondError <- clientErr
		secondLease <- release
	}()

	select {
	case releaseSecond := <-secondLease:
		if releaseSecond != nil {
			defer releaseSecond()
		}
		if clientErr := <-secondError; clientErr != nil {
			t.Fatalf("independent session failed: %v", clientErr)
		}
	case <-time.After(time.Second):
		t.Fatal("independent sessions were serialized by a global lock")
	}
}

func TestClearSessionCacheWaitsForActiveSessionBoundary(t *testing.T) {
	ClearSessionCache()
	defer ClearSessionCache()

	sessionID := "clear-boundary-session"
	_, _, _, release, clientErr := CreateClientForRequest(RequestInput{SessionId: &sessionID})
	if clientErr != nil || release == nil {
		t.Fatalf("failed to acquire session lease: %v", clientErr)
	}

	cleared := make(chan struct{})
	go func() {
		ClearSessionCache()
		close(cleared)
	}()
	select {
	case <-cleared:
		release()
		t.Fatal("clear crossed an active session boundary")
	case <-time.After(100 * time.Millisecond):
	}

	release()
	select {
	case <-cleared:
	case <-time.After(time.Second):
		t.Fatal("clear did not finish after the active session released")
	}
	if _, err := GetClient(sessionID); err == nil {
		t.Fatal("session created before clear remained cached after the boundary")
	}
}

func TestSessionCacheEvictsLeastRecentlyUsedSession(t *testing.T) {
	ClearSessionCache()
	oldMax, oldTTL := SessionCacheConfiguration()
	defer func() {
		ClearSessionCache()
		if err := ConfigureSessionCache(oldMax, oldTTL); err != nil {
			t.Errorf("failed to restore session cache configuration: %v", err)
		}
	}()
	if err := ConfigureSessionCache(2, 0); err != nil {
		t.Fatal(err)
	}

	for _, sessionID := range []string{"lru-one", "lru-two"} {
		id := sessionID
		if client, _, _, clientErr := CreateClient(RequestInput{SessionId: &id}); clientErr != nil || client == nil {
			t.Fatalf("failed to create %s: client=%v error=%v", sessionID, client, clientErr)
		}
	}
	if _, err := GetClient("lru-one"); err != nil {
		t.Fatal(err)
	}
	thirdID := "lru-three"
	if client, _, _, clientErr := CreateClient(RequestInput{SessionId: &thirdID}); clientErr != nil || client == nil {
		t.Fatalf("failed to create third session: client=%v error=%v", client, clientErr)
	}

	if _, err := GetClient("lru-two"); err == nil {
		t.Fatal("least-recently-used session was not evicted")
	}
	if _, err := GetClient("lru-one"); err != nil {
		t.Fatalf("recently used session was evicted: %v", err)
	}
	if _, err := GetClient("lru-three"); err != nil {
		t.Fatalf("new session was not cached: %v", err)
	}
}

func TestSessionCacheExpiresIdleSessions(t *testing.T) {
	ClearSessionCache()
	oldMax, oldTTL := SessionCacheConfiguration()
	defer func() {
		ClearSessionCache()
		if err := ConfigureSessionCache(oldMax, oldTTL); err != nil {
			t.Errorf("failed to restore session cache configuration: %v", err)
		}
	}()
	if err := ConfigureSessionCache(-1, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	sessionID := "ttl-session"
	if client, _, _, clientErr := CreateClient(RequestInput{SessionId: &sessionID}); clientErr != nil || client == nil {
		t.Fatalf("failed to create TTL session: client=%v error=%v", client, clientErr)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := GetClient(sessionID); err == nil {
		t.Fatal("idle session did not expire")
	}
}

func TestSessionCacheTTLPruningIsThrottledUntilDeadline(t *testing.T) {
	ClearSessionCache()
	oldMax, oldTTL := SessionCacheConfiguration()
	defer func() {
		ClearSessionCache()
		if err := ConfigureSessionCache(oldMax, oldTTL); err != nil {
			t.Errorf("failed to restore session cache configuration: %v", err)
		}
	}()
	if err := ConfigureSessionCache(-1, time.Hour); err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1_000_000, 0)
	sessionCacheNextPrune.Store(now.Add(time.Hour).UnixNano())
	before := sessionCachePruneScans.Load()
	for i := 0; i < 1000; i++ {
		if evicted := pruneSessionCacheIfNeeded(now, ""); len(evicted) != 0 {
			t.Fatal("empty cache unexpectedly evicted a client")
		}
	}
	if after := sessionCachePruneScans.Load(); after != before {
		t.Fatalf("TTL cache scanned before its deadline: before=%d after=%d", before, after)
	}

	sessionCacheNextPrune.Store(now.Add(-time.Nanosecond).UnixNano())
	_ = pruneSessionCacheIfNeeded(now, "")
	if after := sessionCachePruneScans.Load(); after != before+1 {
		t.Fatalf("due TTL cache should scan exactly once: before=%d after=%d", before, after)
	}
}

func TestSessionCacheCapacityDoesNotEvictLeasedSession(t *testing.T) {
	ClearSessionCache()
	oldMax, oldTTL := SessionCacheConfiguration()
	defer func() {
		ClearSessionCache()
		if err := ConfigureSessionCache(oldMax, oldTTL); err != nil {
			t.Errorf("failed to restore session cache configuration: %v", err)
		}
	}()
	if err := ConfigureSessionCache(1, 0); err != nil {
		t.Fatal(err)
	}

	leasedID := "leased-cache-entry"
	leasedClient, _, _, release, clientErr := CreateClientForRequest(RequestInput{SessionId: &leasedID})
	if clientErr != nil || leasedClient == nil || release == nil {
		t.Fatalf("failed to create leased session: client=%v release=%v error=%v", leasedClient, release != nil, clientErr)
	}
	defer release()

	otherID := "unleased-cache-entry"
	if client, _, _, clientErr := CreateClient(RequestInput{SessionId: &otherID}); clientErr != nil || client == nil {
		t.Fatalf("failed to create competing session: client=%v error=%v", client, clientErr)
	}
	if cached, err := GetClient(leasedID); err != nil || cached != leasedClient {
		t.Fatalf("active leased session was evicted: client=%v error=%v", cached, err)
	}
	if _, err := GetClient(otherID); err == nil {
		t.Fatal("unleased session should be evicted while the older entry is protected")
	}
}

func TestSessionCacheTTLDoesNotCloseActiveLease(t *testing.T) {
	ClearSessionCache()
	oldMax, oldTTL := SessionCacheConfiguration()
	defer func() {
		ClearSessionCache()
		if err := ConfigureSessionCache(oldMax, oldTTL); err != nil {
			t.Errorf("failed to restore session cache configuration: %v", err)
		}
	}()
	if err := ConfigureSessionCache(-1, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	sessionID := "active-ttl-entry"
	leasedClient, _, _, release, clientErr := CreateClientForRequest(RequestInput{SessionId: &sessionID})
	if clientErr != nil || leasedClient == nil || release == nil {
		t.Fatalf("failed to create leased session: client=%v release=%v error=%v", leasedClient, release != nil, clientErr)
	}
	defer release()

	time.Sleep(30 * time.Millisecond)
	if cached, err := GetClient(sessionID); err != nil || cached != leasedClient {
		t.Fatalf("TTL eviction closed an active session: client=%v error=%v", cached, err)
	}
}

func TestConfigureSessionCacheRejectsInvalidPolicy(t *testing.T) {
	oldMax, oldTTL := SessionCacheConfiguration()
	if err := ConfigureSessionCache(-2, 0); err == nil {
		t.Fatal("capacity below -1 should be rejected")
	}
	if err := ConfigureSessionCache(1, -time.Second); err == nil {
		t.Fatal("negative idle TTL should be rejected")
	}
	maxEntries, idleTTL := SessionCacheConfiguration()
	if maxEntries != oldMax || idleTTL != oldTTL {
		t.Fatalf("invalid policy changed configuration: got (%d, %v), want (%d, %v)", maxEntries, idleTTL, oldMax, oldTTL)
	}
}
