package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRewriteKubeconfigServer(t *testing.T) {
	in := `apiVersion: v1
kind: Config
clusters:
- name: kind-cds-kind
  cluster:
    server: https://127.0.0.1:33505
    certificate-authority-data: Zm9v
contexts:
- name: kind-cds-kind
  context:
    cluster: kind-cds-kind
`
	out, err := rewriteKubeconfigServer(in, "10.0.0.5:6443")
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &doc))
	clusters := doc["clusters"].([]interface{})
	spec := clusters[0].(map[string]interface{})["cluster"].(map[string]interface{})
	assert.Equal(t, "https://10.0.0.5:6443", spec["server"])
	// CA data preserved
	assert.Equal(t, "Zm9v", spec["certificate-authority-data"])
}

func TestRewriteKubeconfigServerPreservesScheme(t *testing.T) {
	in := `clusters:
- cluster:
    server: http://1.2.3.4:8080
`
	out, err := rewriteKubeconfigServer(in, "localhost:9090")
	require.NoError(t, err)
	assert.Contains(t, string(out), "http://localhost:9090")
}

func TestRewriteKubeconfigServerErrorsWithoutClusters(t *testing.T) {
	_, err := rewriteKubeconfigServer("apiVersion: v1\nkind: Config\n", "x:1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no clusters")
}
