package shexec

import (
	"fmt"
	"sync"
)

// defaultCapBytes bounds how much command output is retained in memory. A
// command can produce gigabytes (a noisy build, `yes`); keeping it all would
// let a single call exhaust a shared agent's memory. The caller only needs a
// bounded excerpt, so we keep the first and last halves of this budget and
// discard the middle.
const defaultCapBytes = 512 * 1024

// cappedBuffer is an io.Writer that retains at most capBytes of what is written
// to it — the first capBytes/2 and the last capBytes/2 — while accepting
// (draining) every write so the producer never blocks. When content is dropped
// a one-line marker is inserted between the head and the tail. It is safe for
// concurrent writes (stdout and stderr may share one buffer).
type cappedBuffer struct {
	mu       sync.Mutex
	capBytes int
	head     []byte
	tail     []byte
	total    int64
}

func newCappedBuffer(capBytes int) *cappedBuffer {
	if capBytes < 2 {
		capBytes = 2
	}
	return &cappedBuffer{capBytes: capBytes}
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	b.total += int64(n)

	headCap := b.capBytes / 2
	if len(b.head) < headCap {
		take := headCap - len(b.head)
		if take > len(p) {
			take = len(p)
		}
		b.head = append(b.head, p[:take]...)
		p = p[take:]
	}
	if len(p) == 0 {
		return n, nil
	}
	// keep only the last tailCap bytes across all remaining writes
	tailCap := b.capBytes - headCap
	b.tail = append(b.tail, p...)
	if len(b.tail) > tailCap {
		b.tail = b.tail[len(b.tail)-tailCap:]
	}
	return n, nil
}

// String returns the retained excerpt: head, a drop marker when content was
// discarded, then tail.
func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	kept := int64(len(b.head) + len(b.tail))
	if b.total <= kept {
		return string(b.head) + string(b.tail)
	}
	dropped := b.total - kept
	return fmt.Sprintf("%s\n[... %d bytes omitted ...]\n%s", b.head, dropped, b.tail)
}
