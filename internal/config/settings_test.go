package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentPort(t *testing.T) {
	for name, tc := range map[string]struct {
		env  string
		want int
	}{
		"default":      {env: "", want: 8087},
		"from env":     {env: "9090", want: 9090},
		"out of range": {env: "70000", want: 8087},
		"not a number": {env: "abc", want: 8087},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.env != "" {
				t.Setenv("CDS_AGENT_PORT", tc.env)
			}
			assert.Equal(t, tc.want, AgentPort())
		})
	}
}

func TestAgentBinaryTemplates(t *testing.T) {
	assert.Empty(t, AgentBinary("linux", "amd64", "1.0.0"))
	assert.Empty(t, AgentDownloadURL("linux", "amd64", "1.0.0"))

	t.Setenv("CDS_AGENT_BINARY", " /dist/cds-api-agent-{{os}}-{{arch}} ")
	t.Setenv("CDS_AGENT_DOWNLOAD_URL", "https://example.com/{{version}}/cds-api-agent-{{os}}-{{arch}}")

	assert.Equal(t, "/dist/cds-api-agent-linux-arm64", AgentBinary("linux", "arm64", "1.0.0"))
	assert.Equal(t, "https://example.com/1.0.0/cds-api-agent-linux-amd64", AgentDownloadURL("linux", "amd64", "1.0.0"))
}
