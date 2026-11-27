package command

import (
	"os"
	"path/filepath"
	"strings"
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

func TestKubeconfigServerHostPort(t *testing.T) {
	in := `clusters:
- cluster:
    server: https://127.0.0.1:33505
`
	hp, err := kubeconfigServerHostPort(in)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:33505", hp)
}

func TestKubeconfigServerHostPortNoClusters(t *testing.T) {
	_, err := kubeconfigServerHostPort("apiVersion: v1\n")
	require.Error(t, err)
}

func TestWriteKubeconfigKeepsOtherContexts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	prod := "apiVersion: v1\nkind: Config\ncurrent-context: prod\nclusters:\n- name: prod\n  cluster: {server: 'https://prod:6443'}\nusers:\n- name: prod-u\ncontexts:\n- name: prod\n  context: {cluster: prod, user: prod-u}\n"
	if err := os.WriteFile(path, []byte(prod), 0600); err != nil {
		t.Fatal(err)
	}
	kind := "apiVersion: v1\nkind: Config\ncurrent-context: kind-cds-kind\nclusters:\n- name: kind-cds-kind\n  cluster: {server: 'https://127.0.0.1:1'}\nusers:\n- name: kind-cds-kind\ncontexts:\n- name: kind-cds-kind\n  context: {cluster: kind-cds-kind, user: kind-cds-kind}\n"
	for i := 0; i < 2; i++ { // twice: must stay idempotent
		if err := writeKubeconfig(path, []byte(kind)); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(path)
	got := string(b)
	for _, want := range []string{"name: prod", "name: prod-u", "name: kind-cds-kind", "current-context: prod"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Count(got, "name: kind-cds-kind") != 3 {
		t.Errorf("kind entries duplicated:\n%s", got)
	}
}

// TestLocalKubeconfigPathMultiPath checks that a KUBECONFIG list resolves to
// its first entry, where kubectl writes new entries (CDS-069).
func TestLocalKubeconfigPathMultiPath(t *testing.T) {
	d := t.TempDir()
	first, second := filepath.Join(d, "config"), filepath.Join(d, "other")
	sep := string(os.PathListSeparator)
	for _, tc := range []struct{ env, want string }{
		{first, first},
		{first + sep + second, first},
		{sep + first + sep + second, first},
		{"rel/config" + sep + second, "rel/config"},
	} {
		t.Setenv("KUBECONFIG", tc.env)
		assert.Equal(t, tc.want, localKubeconfigPath(), "KUBECONFIG=%q", tc.env)
	}
	t.Setenv("KUBECONFIG", sep)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".kube", "config"), localKubeconfigPath())
}
