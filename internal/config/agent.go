package config

import (
	"bytes"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/source"
	"gopkg.in/yaml.v3"
)

const (
	kAgentFileName string = "aconfig.yaml"
)

// agentClientData is the typed representation of aconfig.yaml content.
// This config is owned by the agent and tracks the clients registered against it.
type agentClientData struct {
	APIVersion string   `yaml:"apiVersion"`
	Clients    []client `yaml:"clients"`
}

func InitAgentConfig() error {
	if _, err := agentConfigSource(); err != nil {
		return err
	}

	return nil
}

// RegisteredClients returns the clients currently loaded from aconfig.yaml.
func RegisteredClients() ([]client, error) {
	data, err := readAgentData()
	if err != nil {
		return nil, err
	}

	copied := make([]client, len(data.Clients))
	copy(copied, data.Clients)
	return copied, nil
}

// AddClientToConfig appends a client entry to the agent config and persists the change back to the stored source.
func AddClientToConfig(c client) error {
	data, err := readAgentData()
	if err != nil {
		return err
	}
	data.Clients = append(data.Clients, c)
	return writeAgentData(data)
}

func readAgentData() (agentClientData, error) {
	src, err := agentConfigSource()
	if err != nil {
		return agentClientData{}, err
	}
	r, err := src.Read()
	if err != nil {
		return agentClientData{}, cerr.AppendError("failed to read agent config", err)
	}
	var d agentClientData
	if err := yaml.NewDecoder(r).Decode(&d); err != nil {
		return agentClientData{}, cerr.AppendError("failed to parse agent config", err)
	}
	return d, nil
}

func writeAgentData(d agentClientData) error {
	src, err := agentConfigSource()
	if err != nil {
		return err
	}
	if d.APIVersion == cg.EmptyStr {
		d.APIVersion = "v1"
	}
	out, err := yaml.Marshal(d)
	if err != nil {
		return cerr.AppendError("failed to serialize agent config", err)
	}
	return src.Write(bytes.NewReader(out), cg.KPermFile)
}

type client struct {
	Name  string    `yaml:"name"`
	Certs tlssecret `yaml:"tls"`
}

func NewClient(options ...func(*client)) client {
	c := client{}
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithClientName(name string) func(*client) {
	return func(c *client) {
		c.Name = name
	}
}

func WithClientTLS(t tlssecret) func(*client) {
	return func(c *client) {
		c.Certs = t
	}
}

func agentConfigSource() (source.Source, error) {
	src, err := source.NewLocalSource(cenv.ConfigFile(kAgentFileName))
	if err != nil {
		return nil, cerr.AppendError("failed to create agent config source", err)
	}
	if err := EnsureSourceWithDefault(src, defaultAgentConfig(), cg.KPermFile); err != nil {
		return nil, cerr.AppendError("failed to ensure agent config exists", err)
	}
	return src, nil
}
