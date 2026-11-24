package agent

import (
	"context"

	"github.com/amadeusitgroup/cds/internal/cerr"
)

// opLock serializes the operations that change one host resource (the kind
// cluster, or the registry), so two requests (two `project run`s, or an
// `orc run` racing a deploy) never run two `kind create cluster` or two
// registry unit installs at once (CDS-029). Waiting honours the request
// context, so a cancelled client stops queueing.
//
// shortcut: in-process only; the agent is the single writer on a host (one
// agent per user and port). Upgrade to a file lock if another process ever
// drives kind or the registry Quadlet directly.
type opLock chan struct{}

func newOpLock() opLock { return make(opLock, 1) }

// acquire blocks until the lock is free or ctx is done; call the returned
// release exactly once.
func (l opLock) acquire(ctx context.Context, resource string) (release func(), err error) {
	select {
	case l <- struct{}{}:
		return func() { <-l }, nil
	case <-ctx.Done():
		return nil, cerr.AppendErrorFmt("gave up waiting for another %s operation to finish", ctx.Err(), resource)
	}
}

// one lock per resource: the cluster and the registry are built concurrently
// on deploy, so they must not share a lock.
var (
	sClusterLock  = newOpLock()
	sRegistryLock = newOpLock()
)

// locked runs op while holding l.
func locked(ctx context.Context, l opLock, resource string, op func() error) error {
	release, err := l.acquire(ctx, resource)
	if err != nil {
		return err
	}
	defer release()
	return op()
}
