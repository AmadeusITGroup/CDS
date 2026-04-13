package config

import (
	"bytes"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"gopkg.in/yaml.v3"
)

// cliAgentData is the typed representation of cliconfig.yaml content.
// This config is owned by the CLI and tracks the user's registered agents.
type cliAgentData struct {
	APIVersion string  `yaml:"apiVersion"`
	Agents     []agent `yaml:"agents"`
}

// AddAgentToConfig appends an agent entry to the CLI config and persists the change back to the stored source.
func AddAgentToConfig(a agent) error {
	data, err := readCLIAgentData()
	if err != nil {
		return err
	}
	data.Agents = append(data.Agents, a)
	return writeCLIAgentData(data)
}

// AgentAddress returns the gRPC address for the agent running on hostname.
func AgentAddress(hostname string) (string, error) {
	data, err := readCLIAgentData()
	if err != nil {
		return cg.EmptyStr, err
	}

	for _, agent := range data.Agents {
		if strings.Contains(agent.TargetSrv, hostname) {
			return agent.TargetSrv, nil
		}
	}
	if hostname == cg.KLocalhost {
		return ":8087", nil
	}
	return cg.EmptyStr, nil
}

func readCLIAgentData() (cliAgentData, error) {
	src, err := cliConfigSource()
	if err != nil {
		return cliAgentData{}, err
	}
	r, err := src.Read()
	if err != nil {
		return cliAgentData{}, cerr.AppendError("failed to read CLI agent config", err)
	}
	var d cliAgentData
	if err := yaml.NewDecoder(r).Decode(&d); err != nil {
		return cliAgentData{}, cerr.AppendError("failed to parse CLI agent config", err)
	}
	return d, nil
}

func writeCLIAgentData(d cliAgentData) error {
	src, err := cliConfigSource()
	if err != nil {
		return err
	}
	if d.APIVersion == cg.EmptyStr {
		d.APIVersion = "v1"
	}
	out, err := yaml.Marshal(d)
	if err != nil {
		return cerr.AppendError("failed to serialize CLI agent config", err)
	}
	return src.Write(bytes.NewReader(out), cg.KPermFile)
}

type agent struct {
	Certs     tlssecret `yaml:"tls"`
	SshTunnel bool      `yaml:"ssh-tunnel"`   // special case of ssh tunnel
	TargetSrv string    `yaml:"targetServer"` // server address
}

func NewAgent(options ...func(*agent)) agent {
	a := agent{}
	for _, option := range options {
		option(&a)
	}
	return a
}

func WithAgentTLS(t tlssecret) func(*agent) {
	return func(a *agent) {
		a.Certs = t
	}
}

func WithSSHTunnel(use bool) func(*agent) {
	return func(a *agent) {
		a.SshTunnel = use
	}
}

func WithTargetAddress(addr string) func(*agent) {
	return func(a *agent) {
		a.TargetSrv = addr
	}
}
