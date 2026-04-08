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
//   - targetServer: https://localhost:8087
//     tls:
//       ca: /Users/me/.xcds/certs/ca.pem
//       certificate: /Users/me/.xcds/certs/client.pem
//       key: /Users/me/.xcds/certs/client-key.pem
//   - targetServer: localhost:1337
//     ssh-tunnel: true

// aconfig.yaml example:
// apiVersion: v1
// clients:
//   - name: my-laptop
//     tls:
//       ca: /Users/me/.xcds/certs/ca.pem
//       certificate: /Users/me/.xcds/certs/client.pem
//       key: /Users/me/.xcds/certs/client-key.pem

// defaultCLIAgentConfig returns the default content for cliconfig.yaml.
func defaultCLIAgentConfig() io.Reader {
	return strings.NewReader(`apiVersion: v1
agents:
`)
}

// defaultAgentConfig returns the default content for aconfig.yaml.
func defaultAgentConfig() io.Reader {
	return strings.NewReader(`apiVersion: v1
clients:
`)
}
