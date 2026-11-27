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
// (~/.kube/config), honoring KUBECONFIG. KUBECONFIG is a path list: its first
// non-empty entry is used, which is where kubectl writes new entries (a
// relative entry is resolved against the cwd, as kubectl does).
func localKubeconfigPath() string {
	for _, kc := range filepath.SplitList(os.Getenv("KUBECONFIG")) {
		if kc != cg.EmptyStr {
			return kc
		}
	}
	return filepath.Join(cenv.GetUserHomeDir(), ".kube", "config")
}

// mergeKubeconfig merges the clusters, users and contexts of add into existing
// (entries keyed by name; add wins on a name clash) and returns the YAML. Every
// other entry of existing, and its current-context, is kept; current-context
// is taken from add only when existing has none.
func mergeKubeconfig(existing, add []byte) ([]byte, error) {
	var base, extra map[string]interface{}
	if err := yaml.Unmarshal(existing, &base); err != nil {
		return nil, cerr.AppendError("failed to parse existing kubeconfig", err)
	}
	if err := yaml.Unmarshal(add, &extra); err != nil {
		return nil, cerr.AppendError("failed to parse kubeconfig", err)
	}
	if base == nil {
		return add, nil
	}
	for _, key := range []string{"clusters", "users", "contexts"} {
		list, _ := base[key].([]interface{})
		add, _ := extra[key].([]interface{})
		for _, a := range add {
			name := kubeEntryName(a)
			replaced := false
			for i, b := range list {
				if name != cg.EmptyStr && kubeEntryName(b) == name {
					list[i], replaced = a, true
					break
				}
			}
			if !replaced {
				list = append(list, a)
			}
		}
		if len(list) > 0 {
			base[key] = list
		}
	}
	if cur, _ := base["current-context"].(string); cur == cg.EmptyStr {
		if v, ok := extra["current-context"]; ok {
			base["current-context"] = v
		}
	}
	for _, k := range []string{"apiVersion", "kind"} {
		if _, ok := base[k]; !ok {
			if v, ok := extra[k]; ok {
				base[k] = v
			}
		}
	}
	out, err := yaml.Marshal(base)
	if err != nil {
		return nil, cerr.AppendError("failed to serialize kubeconfig", err)
	}
	return out, nil
}

func kubeEntryName(e interface{}) string {
	m, _ := e.(map[string]interface{})
	n, _ := m["name"].(string)
	return n
}

// writeKubeconfig merges kubeconfig into the file at path (creating it and its
// parent dirs when missing), so the user's other clusters, users and contexts
// are preserved.
func writeKubeconfig(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return cerr.AppendErrorFmt("failed to create %s", err, filepath.Dir(path))
	}
	if existing, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(existing))) > 0 {
		merged, mErr := mergeKubeconfig(existing, data)
		if mErr != nil {
			return mErr
		}
		data = merged
	} else if err != nil && !os.IsNotExist(err) {
		return cerr.AppendErrorFmt("failed to read kubeconfig %s", err, path)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return cerr.AppendErrorFmt("failed to write kubeconfig %s", err, path)
	}
	return nil
}
