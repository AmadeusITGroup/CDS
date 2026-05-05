package containerconf

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBytesReturnsIndependentConfigs(t *testing.T) {
	first, err := ParseBytes(strings.NewReader(`{"image":"first"}`))
	require.NoError(t, err)

	second, err := ParseBytes(strings.NewReader(`{"image":"second"}`))
	require.NoError(t, err)

	assert.Equal(t, "first", first.Get(KImage))
	assert.Equal(t, "second", second.Get(KImage))
}

func TestConfigLoadFromBytesReplacesPreviousValues(t *testing.T) {
	config := NewConfig()
	config.Set(KImage, "old-image")

	err := config.LoadFromBytes(strings.NewReader(`{"name":"devbox"}`))
	require.NoError(t, err)

	assert.False(t, config.IsSet(KImage))
	assert.Equal(t, "devbox", config.Get(KName))
}

func TestParseBytesRejectsEmptyConfig(t *testing.T) {
	_, err := ParseBytes(strings.NewReader(" \n // comment only\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "configuration from bytes is empty")
}
