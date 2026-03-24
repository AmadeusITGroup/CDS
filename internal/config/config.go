package config

import (
	"io"
	"strings"
)

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

// defaultCLIAgentConfig returns the default content for cliconfig.yaml.
func defaultCLIAgentConfig() io.Reader {
	return strings.NewReader(`apiVersion: v1
agents:
`)
}

// defaultAgentConfig returns the default content for aconfig.yaml.
func defaultAgentConfig() io.Reader {
	return strings.NewReader(`apiVersion: v1
client:
agents:
`)
}
