package config

const (
	kAgentFileName string = "aconfig"
	kKeyAgents     string = "agents"
)

func InitAgentConfig() error {

	if err := initConfig(kAgentFileName); err != nil {
		return err
	}

	// if viper.IsSet(kKeyAgents) {
	// 	agents = viper.Get(kKeyAgents).([]agent)
	// }

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
