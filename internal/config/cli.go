package config

import (
	"bytes"
	"fmt"
	"net"
	"net/url"
	"slices"
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

func invokeWithCLIAgentData[T any](f func(cliAgentData) (T, error)) (T, error) {
	var zero T
	data, err := readCLIAgentData()
	if err != nil {
		return zero, err
	}
	return f(data)
}

func updateCLIAgentData(f func(*cliAgentData) error) error {
	data, err := readCLIAgentData()
	if err != nil {
		return err
	}
	if err := f(&data); err != nil {
		return err
	}
	return writeCLIAgentData(data)
}

// AddAgentToConfig appends an agent entry to the CLI config and persists the change back to the stored source.
func AddAgentToConfig(a agent) error {
	return updateCLIAgentData(func(c *cliAgentData) error {
		c.Agents = append(c.Agents, a)
		return nil
	})
}

// RegisteredAgents returns the registered agents from cliconfig.yaml.
func RegisteredAgents() ([]agent, error) {
	return invokeWithCLIAgentData(func(c cliAgentData) ([]agent, error) {
		copied := make([]agent, len(c.Agents))
		copy(copied, c.Agents)
		return copied, nil
	})
}

// RegisteredAgent returns the registered agent matching targetServer.
func RegisteredAgent(targetServer string) (agent, error) {
	normalizedTarget, err := normalizeTargetServer(targetServer)
	if err != nil {
		return agent{}, err
	}

	return invokeWithCLIAgentData(func(c cliAgentData) (agent, error) {
		index := findAgentIndex(c.Agents, normalizedTarget)
		if index < 0 {
			return agent{}, cerr.NewError(fmt.Sprintf("agent %q does not exist", normalizedTarget))
		}

		return c.Agents[index], nil
	})
}

// CreateAgentInConfig creates a new agent entry in the CLI config.
func CreateAgentInConfig(a agent) error {
	normalizedAgent, err := normalizeAgent(a)
	if err != nil {
		return err
	}
	return updateCLIAgentData(func(c *cliAgentData) error {
		if findAgentIndex(c.Agents, normalizedAgent.TargetSrv) >= 0 {
			return cerr.NewError(fmt.Sprintf("agent %q already exists", normalizedAgent.TargetSrv))
		}

		c.Agents = append(c.Agents, normalizedAgent)
		return nil
	})
}

// UpdateAgentInConfig updates an existing agent entry identified by targetServer.
func UpdateAgentInConfig(targetServer string, updated agent) error {
	normalizedTarget, err := normalizeTargetServer(targetServer)
	if err != nil {
		return err
	}

	normalizedAgent, err := normalizeAgent(updated)
	if err != nil {
		return err
	}

	return updateCLIAgentData(func(c *cliAgentData) error {
		index := findAgentIndex(c.Agents, normalizedTarget)
		if index < 0 {
			return cerr.NewError(fmt.Sprintf("agent %q does not exist", normalizedTarget))
		}

		if duplicateIndex := findAgentIndex(c.Agents, normalizedAgent.TargetSrv); duplicateIndex >= 0 && duplicateIndex != index {
			return cerr.NewError(fmt.Sprintf("agent %q already exists", normalizedAgent.TargetSrv))
		}

		c.Agents[index] = normalizedAgent
		return nil
	})
}

// DeleteAgentFromConfig deletes an existing agent entry from the CLI config.
func DeleteAgentFromConfig(targetServer string) error {
	normalizedTarget, err := normalizeTargetServer(targetServer)
	if err != nil {
		return err
	}

	return updateCLIAgentData(func(c *cliAgentData) error {
		index := findAgentIndex(c.Agents, normalizedTarget)
		if index < 0 {
			return cerr.NewError(fmt.Sprintf("agent %q does not exist", normalizedTarget))
		}

		c.Agents = append(c.Agents[:index], c.Agents[index+1:]...)
		return nil
	})
}

// AgentAddress returns the targetServer of an agent by hostname
func AgentAddress(hostname string) (string, error) {
	return invokeWithCLIAgentData(func(c cliAgentData) (string, error) {
		normalizedHostname := strings.TrimSpace(hostname)
		for _, agent := range c.Agents {
			if targetServerHostname(agent.TargetSrv) == normalizedHostname {
				return agent.TargetSrv, nil
			}
		}
		return cg.EmptyStr, cerr.NewError(fmt.Sprintf("No agent found with hostname %q", hostname))
	})
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
	if d.APIVersion == cg.EmptyStr {
		d.APIVersion = "v1"
	}
	if d.Agents == nil {
		d.Agents = []agent{}
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

func normalizeAgent(a agent) (agent, error) {
	targetServer, err := normalizeTargetServer(a.TargetSrv)
	if err != nil {
		return agent{}, err
	}
	a.TargetSrv = targetServer
	return a, nil
}

func normalizeTargetServer(targetServer string) (string, error) {
	normalized := strings.TrimSpace(targetServer)
	if normalized == cg.EmptyStr {
		return cg.EmptyStr, cerr.NewError("target server is required")
	}
	return normalized, nil
}

func findAgentIndex(agents []agent, targetServer string) int {
	return slices.IndexFunc(agents, func(a agent) bool { return strings.TrimSpace(a.TargetSrv) == targetServer })
}

func targetServerHostname(targetServer string) string {
	normalized := strings.TrimSpace(targetServer)
	if normalized == cg.EmptyStr {
		return cg.EmptyStr
	}
	if strings.HasPrefix(normalized, ":") {
		return cg.KLocalhost
	}
	if strings.Contains(normalized, "://") {
		parsedURL, err := url.Parse(normalized)
		if err == nil && parsedURL.Hostname() != cg.EmptyStr {
			return parsedURL.Hostname()
		}
	}
	if host, _, err := net.SplitHostPort(normalized); err == nil {
		if host == cg.EmptyStr {
			return cg.KLocalhost
		}
		return host
	}
	return normalized
}
