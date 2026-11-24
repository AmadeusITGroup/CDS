package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpLockSerializes runs many ops at once on one lock: at most one may be
// inside at any time (two kind creates must never overlap, CDS-029).
func TestOpLockSerializes(t *testing.T) {
	l := newOpLock()
	var inside, maxInside int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, locked(context.Background(), l, "cluster", func() error {
				n := atomic.AddInt32(&inside, 1)
				for {
					m := atomic.LoadInt32(&maxInside)
					if n <= m || atomic.CompareAndSwapInt32(&maxInside, m, n) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt32(&inside, -1)
				return nil
			}))
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), maxInside)
}

// TestOpLockWaitHonoursContext: a request queued behind a long operation gives
// up when its client goes away, and the lock is usable once released.
func TestOpLockWaitHonoursContext(t *testing.T) {
	l := newOpLock()
	release, err := l.acquire(context.Background(), "cluster")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	ran := false
	err = locked(ctx, l, "cluster", func() error { ran = true; return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "another cluster operation")
	assert.False(t, ran)

	release()
	require.NoError(t, locked(context.Background(), l, "cluster", func() error { ran = true; return nil }))
	assert.True(t, ran)
}

// TestClusterAndRegistryLocksAreIndependent: deploy builds both at once (owner
// decision), so holding one must not block the other.
func TestClusterAndRegistryLocksAreIndependent(t *testing.T) {
	assert.NotEqual(t, sClusterLock, sRegistryLock)
	release, err := sClusterLock.acquire(context.Background(), "cluster")
	require.NoError(t, err)
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, locked(ctx, sRegistryLock, "registry", func() error { return nil }))
}
