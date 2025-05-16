package config

import (
	"bytes"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"github.com/spf13/viper"
)

const (
	kCLIFileType string = "yaml"
)

func initConfig(configName string) error {
	viper.SetConfigName(configName)
	viper.SetConfigType(kCLIFileType)
	viper.AddConfigPath(cenv.GlobalConfigPath())
	createConfig := false
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			createConfig = true
		} else {
			return cerr.AppendError("Failed to load config", err)
		}
	}

	if createConfig {
		if err := viper.ReadConfig(bytes.NewBuffer(defaultConfig(configName))); err != nil {
			return cerr.AppendError("Failed to create config file using default", err)
		}
		if err := cenv.EnsureFile(cenv.ConfigFile(configName), cg.KPermFile); err != nil {
			return cerr.AppendError("Failed to create config file", err)
		}
		if err := viper.WriteConfig(); err != nil {
			return cerr.AppendError("Failed to write default config to file", err)
		}
		return nil
	}
	return nil
}

type tlssecret struct {
	CA   string `yaml:"ca"`          // Certificate Authority
	Cert string `yaml:"certificate"` // server certificate
	Key  string `yaml:"key"`         // server key
}

func NewTlssecret(options ...func(*tlssecret)) tlssecret {
	t := tlssecret{}
	for _, option := range options {
		option(&t)
	}
	return t
}

func WithCA(ca string) func(*tlssecret) {
	return func(t *tlssecret) {
		t.CA = ca
	}
}

func WithCert(cert string) func(*tlssecret) {
	return func(t *tlssecret) {
		t.Cert = cert
	}
}

func WithKey(key string) func(*tlssecret) {
	return func(t *tlssecret) {
		t.Key = key
	}
}

// cliconfig.yaml example:
// apiVersion: v1
// agents:
//   - agent:
//     certificate-authority-data: HJHJHJHJHJHJHJH
//     client-key-data: CHGHSGSHSGHSGHS
//     server: https://localhost:8087
//   - agent:
//     certificate-authority-data: HJHJHJHJHJHJHJH
//     client-key-data: CHGHSGSHSGHSGHS
//     server: https://my.server.fix.me:1337
//   - agent:
//     server: http://localhost:1337
//     ssh: yes

// aconfig.yaml example:
// apiVersion: v1
// client:
//   certificate-authority-data: HJHJHJHJHJHJHJH
//   client-key-data: CHGHSGSHSGHSGHS
// agents:
//   - agent:
//     certificate-authority-data: HJHJHJHJHJHJHJH
//     client-key-data: CHGHSGSHSGHSGHS
//     server: https://localhost:8087
//   - agent:
//     certificate-authority-data: HJHJHJHJHJHJHJH
//     client-key-data: CHGHSGSHSGHSGHS
//     server: https://my.server.fix.me:1337
//   - agent:
//     server: http://localhost:1337
//     ssh: yes

func defaultConfig(what string) (config []byte) {

	switch what {
	case kCLIFileName:
		config = []byte(`
apiVersion: v1
agents:
`)
	case kAgentFileName:
		config = []byte(`
apiVersion: v1
client:
agents:
`)
	}
	return
}
