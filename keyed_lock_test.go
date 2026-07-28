package tls_client

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKeyedLockPool_ReclaimsEntriesAndHonorsCancellation(t *testing.T) {
	var pool keyedLockPool
	release, err := pool.Lock(context.Background(), "same-host")
	require.NoError(t, err)
	require.NotNil(t, release)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = pool.Lock(ctx, "same-host")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded))
	require.NotNil(t, release)

	release()
	// After release, a new Lock should succeed
	release2, err2 := pool.Lock(context.Background(), "same-host")
	require.NoError(t, err2)
	require.NotNil(t, release2)
	release2()
}

func TestKeyedLockPool_DifferentKeysDoNotBlock(t *testing.T) {
	var pool keyedLockPool
	releaseFirst, err := pool.Lock(context.Background(), "first-host")
	require.NoError(t, err)
	defer releaseFirst()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	releaseSecond, err := pool.Lock(ctx, "second-host")
	require.NoError(t, err)
	releaseSecond()
}

func TestKeyedLockPool_DoesNotGrantAlreadyCanceledContext(t *testing.T) {
	var pool keyedLockPool
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pool.Lock(ctx, "available-host")
	require.ErrorIs(t, err, context.Canceled)
}
