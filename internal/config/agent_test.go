package config

import (
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitAgentConfigCreatesClientsRegistry(t *testing.T) {
	setupConfigTestFS(t)

	require.NoError(t, InitAgentConfig())

	content, err := cos.ReadFile("/tmp/testconfig/.xcds/aconfig.yaml")
	require.NoError(t, err)
	assert.Contains(t, string(content), "clients:")
	assert.NotContains(t, string(content), "agents:")
}

func TestAddClientToConfigPersistsClientsWithoutInit(t *testing.T) {
	setupConfigTestFS(t)

	require.NoError(t, AddClientToConfig(NewClient(
		WithClientName("my-laptop"),
		WithClientTLS(NewTlssecret(
			WithCA("/tmp/ca.pem"),
			WithCert("/tmp/client.pem"),
			WithKey("/tmp/client-key.pem"),
		)),
	)))

	data, err := readAgentData()
	require.NoError(t, err)
	require.Len(t, data.Clients, 1)
	assert.Equal(t, "my-laptop", data.Clients[0].Name)
	assert.Equal(t, "/tmp/client.pem", data.Clients[0].Certs.Cert)
}

func TestRegisteredClientsLoadsClientsWithoutInit(t *testing.T) {
	setupConfigTestFS(t)

	require.NoError(t, cos.Fs.MkdirAll("/tmp/testconfig/.xcds", 0755))
	require.NoError(t, cos.WriteFile("/tmp/testconfig/.xcds/aconfig.yaml", []byte(`apiVersion: v1
clients:
  - name: my-laptop
    tls:
      ca: /tmp/ca.pem
      certificate: /tmp/client.pem
      key: /tmp/client-key.pem
`), 0600))

	clients, err := RegisteredClients()
	require.NoError(t, err)
	require.Len(t, clients, 1)
	assert.Equal(t, "my-laptop", clients[0].Name)
	assert.Equal(t, "/tmp/client.pem", clients[0].Certs.Cert)
}
