package config

import (
	"fmt"
	"strings"

	"github.com/amadeusitgroup/cds/internal/clog"
	"github.com/spf13/viper"
)

// Settings tune the behavior of cds. They are read through Viper and can be
// overridden with CDS_<KEY> environment variables, e.g. CDS_AGENT_PORT.
const (
	kSettingsEnvPrefix = "CDS"

	// kAgentPortKey is the port agents listen on.
	kAgentPortKey = "agent_port"
	// kAgentBinaryKey is a local path template of the agent binary, mainly for
	// development, e.g. /src/cds/dist/cds-api-agent-{{os}}-{{arch}}.
	kAgentBinaryKey = "agent_binary"
	// kAgentDownloadURLKey is a URL template the agent binary is downloaded
	// from when not available locally,
	// e.g. https://example.com/cds/{{version}}/cds-api-agent-{{os}}-{{arch}}.
	kAgentDownloadURLKey = "agent_download_url"

	kDefaultAgentPort = 8087
)

var sSettings = newSettings()

func newSettings() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix(kSettingsEnvPrefix)
	v.AutomaticEnv()
	v.SetDefault(kAgentPortKey, kDefaultAgentPort)
	return v
}

// AgentPort returns the port agents listen on.
func AgentPort() int {
	port := sSettings.GetInt(kAgentPortKey)
	if port <= 0 || port > 65535 {
		clog.Warn(fmt.Sprintf("Invalid agent port %q, using %d", sSettings.GetString(kAgentPortKey), kDefaultAgentPort))
		return kDefaultAgentPort
	}
	return port
}

// AgentBinary returns the local path of the agent binary for the given
// platform and version, or an empty string when not configured.
func AgentBinary(goos, goarch, version string) string {
	return expandAgentTemplate(sSettings.GetString(kAgentBinaryKey), goos, goarch, version)
}

// AgentDownloadURL returns the URL of the agent binary for the given platform
// and version, or an empty string when not configured.
func AgentDownloadURL(goos, goarch, version string) string {
	return expandAgentTemplate(sSettings.GetString(kAgentDownloadURLKey), goos, goarch, version)
}

func expandAgentTemplate(template, goos, goarch, version string) string {
	return strings.NewReplacer("{{os}}", goos, "{{arch}}", goarch, "{{version}}", version).Replace(strings.TrimSpace(template))
}
