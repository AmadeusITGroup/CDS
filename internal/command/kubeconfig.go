package command

import (
	"fmt"
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

// hostKubeconfigName is the cluster, user and context name of a host's cluster
// in the local kubeconfig. kind names every cluster kind-cds-kind, so the name
// is made host-specific before merging; otherwise one host's entries would
// replace another's (CDS-071).
func hostKubeconfigName(hostName string) string {
	return "cds-" + hostName
}

// renameKubeconfigEntries renames every cluster, user and context of the
// kubeconfig to name (a second entry of a kind gets "-2", ...), rewrites the
// contexts' cluster/user references and the current-context to match, and
// returns the YAML.
func renameKubeconfigEntries(kubeconfig []byte, name string) ([]byte, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal(kubeconfig, &doc); err != nil {
		return nil, cerr.AppendError("failed to parse kubeconfig", err)
	}
	renamed := map[string]map[string]string{}
	for _, key := range []string{"clusters", "users", "contexts"} {
		renamed[key] = map[string]string{}
		list, _ := doc[key].([]interface{})
		for i, e := range list {
			m, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			newName := name
			if i > 0 {
				newName = fmt.Sprintf("%s-%d", name, i+1)
			}
			if old := kubeEntryName(m); old != cg.EmptyStr {
				renamed[key][old] = newName
			}
			m["name"] = newName
		}
	}
	contexts, _ := doc["contexts"].([]interface{})
	for _, c := range contexts {
		m, _ := c.(map[string]interface{})
		spec, ok := m["context"].(map[string]interface{})
		if !ok {
			continue
		}
		for field, key := range map[string]string{"cluster": "clusters", "user": "users"} {
			if ref, _ := spec[field].(string); renamed[key][ref] != cg.EmptyStr {
				spec[field] = renamed[key][ref]
			}
		}
	}
	if cur, _ := doc["current-context"].(string); renamed["contexts"][cur] != cg.EmptyStr {
		doc["current-context"] = renamed["contexts"][cur]
	}
	out, err := yaml.Marshal(doc)
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
	return replaceFileAtomically(path, data)
}

// replaceFileAtomically writes data to a temp file next to path's real target
// (symlinks resolved, so a linked kubeconfig stays linked), keeps the target's
// mode (cg.KPermFile for a new file), fsyncs it and renames it over the target,
// so a failure mid-write never leaves a truncated kubeconfig (CDS-070).
func replaceFileAtomically(path string, data []byte) error {
	target, perm := path, cg.KPermFile
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
		if fi, sErr := os.Stat(target); sErr == nil {
			perm = fi.Mode().Perm()
		}
	} else if !os.IsNotExist(err) {
		return cerr.AppendErrorFmt("failed to resolve kubeconfig %s", err, path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+"-*.tmp")
	if err != nil {
		return cerr.AppendErrorFmt("failed to create a temp file for kubeconfig %s", err, target)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if err := tmp.Chmod(perm); err != nil {
		return cerr.AppendErrorFmt("failed to set the mode of kubeconfig %s", err, target)
	}
	if _, err := tmp.Write(data); err != nil {
		return cerr.AppendErrorFmt("failed to write kubeconfig %s", err, target)
	}
	if err := tmp.Sync(); err != nil {
		return cerr.AppendErrorFmt("failed to sync kubeconfig %s", err, target)
	}
	if err := tmp.Close(); err != nil {
		return cerr.AppendErrorFmt("failed to write kubeconfig %s", err, target)
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return cerr.AppendErrorFmt("failed to replace kubeconfig %s", err, target)
	}
	return nil
}
