package config

import (
	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/amadeusitgroup/cds/internal/source"
)

const (
	kAgentFileName string = "aconfig.yaml"
)

func InitAgentConfig() error {
	agentPath := cenv.ConfigFile(kAgentFileName)

	src, err := source.NewLocalSource(agentPath)
	if err != nil {
		return cerr.AppendError("failed to create agent config source", err)
	}

	if err := EnsureSourceWithDefault(src, defaultAgentConfig(), cg.KPermFile); err != nil {
		return cerr.AppendError("failed to ensure agent config exists", err)
	}

	return nil
}

// var (
// 	agents []agent
// )

type agent struct {
	Server string    `yaml:"server"` // server address
	Certs  tlssecret `yaml:"tls"`
}

func NewAgent(options ...func(*agent)) agent {
	agent := agent{}
	for _, option := range options {
		option(&agent)
	}
	return agent
}

func WithAddress(addr string) func(*agent) {
	return func(a *agent) {
		a.Server = addr
	}
}

func WithServerTLS(t tlssecret) func(*agent) {
	return func(c *agent) {
		c.Certs = t
	}
}
