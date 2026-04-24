package containerconf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		want       string
		wantErr    string
	}{
		{
			name:       "keeps normalized identifier",
			identifier: "resource/dockerfile/Dockerfile",
			want:       "resource/dockerfile/Dockerfile",
		},
		{
			name:       "keeps flat logical identifier",
			identifier: "dockerfile",
			want:       "dockerfile",
		},
		{
			name:       "keeps escaped logical names stable",
			identifier: "resource/config/..%2Fbashrc",
			want:       "resource/config/..%2Fbashrc",
		},
		{
			name:       "rejects empty identifier",
			identifier: "",
			wantErr:    "artifact identifier is required",
		},
		{
			name:       "rejects absolute identifier",
			identifier: "/tmp/file.txt",
			wantErr:    "must be relative",
		},
		{
			name:       "rejects traversal",
			identifier: "../tmp/file.txt",
			wantErr:    "escapes the staging directory",
		},
		{
			name:       "rejects malformed resource identifier",
			identifier: "resource/dockerfile/../Dockerfile",
			wantErr:    "must use resource/<kind>/<logical-name>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeIdentifier(tt.identifier)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResourceIdentifierEscapesLogicalName(t *testing.T) {
	got, err := ResourceIdentifier("config", "../bashrc")
	require.NoError(t, err)
	assert.Equal(t, "resource/config/..%2Fbashrc", got)
}

func TestSingletonIdentifierUsesKindAsLogicalName(t *testing.T) {
	got, err := SingletonIdentifier(KindAuthFile)
	require.NoError(t, err)
	assert.Equal(t, "resource/auth/auth", got)
}

func TestDockerfileIdentifierUsesFileNameOnly(t *testing.T) {
	config := NewConfig()
	config.Set(KBuild+"."+KBuildDockerfile, "../Dockerfile")

	got, err := DockerfileIdentifier(config)
	require.NoError(t, err)
	assert.Equal(t, "resource/dockerfile/Dockerfile", got)
}
