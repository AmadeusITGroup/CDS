package cos

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failure")
}

func setupCosTest(t *testing.T) {
	t.Helper()
	SetMockedFileSystem()
	t.Cleanup(SetRealFileSystem)
	require.NoError(t, EnsureDir("/tmp/cos", 0700))
}

func TestCreateTempFileWithContent(t *testing.T) {
	setupCosTest(t)

	path, err := CreateTempFileWithContent("/tmp/cos", "tmp-*", strings.NewReader("content"))

	require.NoError(t, err)
	content, err := ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestCreateTempFileWithContentRemovesFileOnFailure(t *testing.T) {
	setupCosTest(t)

	path, err := CreateTempFileWithContent("/tmp/cos", "tmp-*", failingReader{})

	require.Error(t, err)
	assert.Empty(t, path)
	entries, err := ReadDir("/tmp/cos")
	require.NoError(t, err)
	assert.Empty(t, entries, "partially written temp file must be removed")
}
