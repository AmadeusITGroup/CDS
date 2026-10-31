package command

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cenv"
	"github.com/amadeusitgroup/cds/internal/cerr"
	cg "github.com/amadeusitgroup/cds/internal/global"
	"gopkg.in/yaml.v3"
)

// serverURLRe matches the scheme+host of a kubeconfig server URL so only the
// host:port is rewritten, preserving any path.
var serverURLRe = regexp.MustCompile(`^(https?://)[^/]+`)

// kubeconfigServerHostPort extracts the HOST:PORT of the first cluster's server
// URL from a kubeconfig (dropping the scheme and any path), i.e. the address to
// forward to.
func kubeconfigServerHostPort(kubeconfig string) (string, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(kubeconfig), &doc); err != nil {
		return "", cerr.AppendError("failed to parse kubeconfig", err)
	}
	clusters, ok := doc["clusters"].([]interface{})
	if !ok || len(clusters) == 0 {
		return "", cerr.NewError("kubeconfig has no clusters")
	}
	cluster, _ := clusters[0].(map[string]interface{})
	spec, _ := cluster["cluster"].(map[string]interface{})
	server, _ := spec["server"].(string)
	if server == "" {
		return "", cerr.NewError("kubeconfig cluster has no server")
	}
	// strip scheme and any trailing path, leaving host:port.
	noScheme := regexp.MustCompile(`^https?://`).ReplaceAllString(server, "")
	if i := strings.IndexByte(noScheme, '/'); i >= 0 {
		noScheme = noScheme[:i]
	}
	return noScheme, nil
}

// rewriteKubeconfigServer rewrites the server host:port of every cluster in the
// kubeconfig to addr (keeping the scheme), so a kubeconfig whose server points
// at an address only reachable on the agent host can be retargeted at a
// client-reachable address (e.g. an SSH-forwarded local port). It returns the
// rewritten YAML.
func rewriteKubeconfigServer(kubeconfig, addr string) ([]byte, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(kubeconfig), &doc); err != nil {
		return nil, cerr.AppendError("failed to parse kubeconfig", err)
	}
	clusters, ok := doc["clusters"].([]interface{})
	if !ok {
		return nil, cerr.NewError("kubeconfig has no clusters")
	}
	for _, c := range clusters {
		cluster, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		spec, ok := cluster["cluster"].(map[string]interface{})
		if !ok {
			continue
		}
		if server, ok := spec["server"].(string); ok {
			spec["server"] = serverURLRe.ReplaceAllString(server, "${1}"+addr)
		}
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, cerr.AppendError("failed to serialize kubeconfig", err)
	}
	return out, nil
}

// localKubeconfigPath returns the user's default kubeconfig path
// (~/.kube/config), honoring KUBECONFIG when it names a single file.
func localKubeconfigPath() string {
	if kc := os.Getenv("KUBECONFIG"); kc != cg.EmptyStr && !filepath.IsAbs(kc) {
		// a relative KUBECONFIG is resolved against the cwd by kubectl; keep it.
		return kc
	} else if kc != cg.EmptyStr {
		return kc
	}
	return filepath.Join(cenv.GetUserHomeDir(), ".kube", "config")
}

// writeKubeconfig writes kubeconfig to path, creating parent dirs. It replaces
// the file; a full merge with an existing kubeconfig is intentionally not done
// here to avoid corrupting an unrelated config — the command warns when it
// overwrites.
func writeKubeconfig(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return cerr.AppendErrorFmt("failed to create %s", err, filepath.Dir(path))
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return cerr.AppendErrorFmt("failed to write kubeconfig %s", err, path)
	}
	return nil
}
