package config

import (
	"github.com/spf13/viper"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
	cg "github.com/amadeusitgroup/cds/internal/global"
)

const (
	kCLIFileName string = "cliconfig"
	kKeyClients  string = "clients"
)

func InitCLIConfig() error {
	if err := initConfig(kCLIFileName); err != nil {
		return err
	}

	if viper.IsSet(kKeyAgents) {
		clients = viper.Get(kKeyClients).([]client)
	}
	return nil
}

func ReloadCLIConfig() error {
	if err := viper.ReadInConfig(); err != nil {
		return cerr.AppendError("Failed to load config", err)
	}
	return nil
}

func AddClientToConfig(client client, saveToFile bool) error {
	clients = append(clients, client)
	viper.Set(kKeyClients, clients)
	if saveToFile {
		return viper.WriteConfig()
	}
	return nil
}

func AgentAddress(hostname string) string {
	clog.Error("GetLocalAgentAddress is not implemented")
	if hostname == cg.KLocalhost {
		return ":8087"
	}
	return cg.EmptyStr
}

var (
	clients []client
)

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
