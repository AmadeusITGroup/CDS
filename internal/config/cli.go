package config

import (
	"bytes"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"gopkg.in/yaml.v3"
)

var (
	clients []client
)

// cliAgentData is the typed representation of cliconfig.yaml content.
type cliAgentData struct {
	APIVersion string   `yaml:"apiVersion"`
	Clients    []client `yaml:"agents"`
}

// ReloadCLIConfig re-reads the CLI agent config from the stored source
// and refreshes the in-memory client list.
func ReloadCLIConfig() error {
	if cliSource == nil {
		return cerr.NewError("config source has not been initialized")
	}
	data, err := readCLIAgentData()
	if err != nil {
		return cerr.AppendError("failed to reload CLI config", err)
	}
	clients = data.Clients
	return nil
}

// AddClientToConfig appends a client entry to the CLI agent config and persists the change back to the stored source.
func AddClientToConfig(c client) error {
	if cliSource == nil {
		return cerr.NewError("config.Init has not been called")
	}
	data, err := readCLIAgentData()
	if err != nil {
		return err
	}
	data.Clients = append(data.Clients, c)
	return writeCLIAgentData(data)
}

// AgentAddress returns the gRPC address for the agent running on hostname.
func AgentAddress(hostname string) string {
	for _, c := range clients {
		if strings.Contains(c.TargetSrv, hostname) {
			return c.TargetSrv
		}
	}
	if hostname == cg.KLocalhost {
		return ":8087"
	}
	return cg.EmptyStr
}

func readCLIAgentData() (cliAgentData, error) {
	r, err := cliSource.Read()
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
	out, err := yaml.Marshal(d)
	if err != nil {
		return cerr.AppendError("failed to serialize CLI agent config", err)
	}
	return cliSource.Write(bytes.NewReader(out), cg.KPermFile)
}

type client struct {
	Certs     tlssecret `yaml:"tls"`
	SshTunnel bool      `yaml:"ssh-tunnel"`   // special case of ssh tunnel
	TargetSrv string    `yaml:"targetServer"` // server address
}

func NewClient(options ...func(*client)) client {
	c := client{}
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithClientTLS(t tlssecret) func(*client) {
	return func(c *client) {
		c.Certs = t
	}
}

func WithSSHTunnel(use bool) func(*client) {
	return func(c *client) {
		c.SshTunnel = use
	}
}

func WithTargetAddress(addr string) func(*client) {
	return func(a *client) {
		a.TargetSrv = addr
	}
}
