package shexec

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCappedBufferKeepsHeadAndTailUnderCap(t *testing.T) {
	b := newCappedBuffer(100)
	_, _ = b.Write([]byte("hello"))
	assert.Equal(t, "hello", b.String(), "content under the cap is kept whole")
}

func TestCappedBufferBoundsLargeOutput(t *testing.T) {
	b := newCappedBuffer(100)
	// write far more than the cap in many small writes (simulates streamed output)
	for i := 0; i < 10000; i++ {
		_, _ = b.Write([]byte("0123456789"))
	}
	out := b.String()
	assert.LessOrEqual(t, len(out), 100+64, "retained content is bounded by the cap (plus a short marker)")
	assert.Contains(t, out, "omitted", "the drop is marked")
	// head retained from the very first bytes, tail from the very last
	assert.True(t, strings.HasPrefix(out, "0123456789"), out[:20])
	assert.True(t, strings.HasSuffix(out, "0123456789"), out[len(out)-20:])
}

func TestCappedBufferHeadThenTail(t *testing.T) {
	b := newCappedBuffer(10)             // head=5, tail=5
	_, _ = b.Write([]byte("AAAAA"))      // fills head
	_, _ = b.Write([]byte("BBBBBBBBBB")) // overflows into tail
	_, _ = b.Write([]byte("CCCCC"))      // tail keeps the latest
	out := b.String()
	assert.True(t, strings.HasPrefix(out, "AAAAA"), out)
	assert.True(t, strings.HasSuffix(out, "CCCCC"), out)
	assert.Contains(t, out, "omitted")
}
