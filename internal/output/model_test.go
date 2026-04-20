package output

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromContextHandlesPointer(t *testing.T) {
	ctx := WithOutputOptions(context.Background(), &OutputOptions{
		mode:    ModeJSON,
		command: "space.host.list",
	})

	got := FromContext(ctx)

	assert.Equal(t, ModeJSON, got.mode)
	assert.Equal(t, "space.host.list", got.command)
}

func TestRenderJSONProducesValidJSON(t *testing.T) {
	var buf bytes.Buffer

	err := Render(OutputOptions{
		mode:   ModeJSON,
		writer: &buf,
	}, SimpleResult{Message: "hello"})
	require.NoError(t, err)

	var payload struct {
		Data SimpleResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	assert.Equal(t, "hello", payload.Data.Message)
}
