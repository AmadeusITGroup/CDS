# Trying the new (OSS) CDS from scratch

This recipe lets you (and champion users) start the new OSS `cds` from a clean
slate and exercise the full workflow, so you can log what is missing or broken.

The new `cds` now owns `~/.cds` and `~/.cds-agent` (it replaces the enterprise
cds on this machine). **Step 0 backs up your existing `~/.cds` first.**

> Conventions in this doc:
> - `WORKTREE` = the checkout of this branch, e.g.
>   `/Users/bkarfi/Workspace/cds-revival/CDS-worktrees/feat-orchestration-registry`
> - `HOST` = a Linux host reachable over SSH where the agent + containers run
>   (local Linux or remote). On macOS you always target a remote Linux HOST.

---

## 0. Back up the current (enterprise) CDS state

```sh
# keep a timestamped backup of the enterprise cds state, then step aside
mv ~/.cds ~/.cds.enterprise-backup.$(date +%Y%m%d-%H%M%S)
# (optional) the OSS agent dir, if any earlier run created one
[ -d ~/.cds-agent ] && mv ~/.cds-agent ~/.cds-agent.backup.$(date +%Y%m%d-%H%M%S)
```

To restore enterprise cds later: `rm -rf ~/.cds && mv ~/.cds.enterprise-backup.* ~/.cds`.

Also make sure the enterprise `cds` is **not** first on your `PATH` during testing
(e.g. `which cds` should point at the new binary you install below).

---

## 1. Prerequisites

- **Your laptop (client):** Go 1.27.3+, `protoc`, `make`, `gh` (for private repos),
  and an SSH key that can reach the HOST.
- **The HOST (Linux):** `podman >= 4.4`, `kind`, a **systemd user session with
  linger enabled** (`loginctl enable-linger $USER`), and cgroup v2 with cpu
  delegation. IPv6 may be disabled — that is handled (CDS creates the kind
  network IPv4-only).
- A devcontainer image that ships **sshd** (CDS installs authorized_keys but not
  an ssh server). For the orchestration/registry workflow it should also ship a
  push tool (`skopeo` or `podman`) and `kubectl`/`helm` — the CDS `skube-*`
  flavours do.

---

## 2. Build and install the new cds

```sh
cd "$WORKTREE"

# generate protobuf (gitignored) — required once per fresh checkout
make build-pb

# build the client (-> ./cds) and the Linux agent binaries (-> dist/)
make build-client
make build-api-agent-linux

# put the client on your PATH as `cds`
install -m 0755 ./cds /usr/local/bin/cds    # or: cp ./cds ~/bin/cds
cds --help | head -5
```

The client uploads a matching agent binary to the HOST on first use. Point it at
the binaries you just built with the `CDS_AGENT_BINARY` template (os/arch are
substituted):

```sh
export CDS_AGENT_BINARY="$WORKTREE/dist/cds-api-agent-{{os}}-{{arch}}"
```

> Keep this exported for the whole session. Without it the client tries to
> download an agent, which is not wired for OSS yet.

> **Version match:** the client and the agent must report the SAME version
> (the health check is exact). If you rebuild one, rebuild both. Committing
> changes the version string, so build client+agent from the same commit.

---

## 3. Register and bootstrap a HOST

```sh
# first use configures SSH access and installs + starts the agent (systemd user
# service) on the HOST. On macOS, HOST must be a remote Linux box.
cds space host add <HOST>

# confirm the agent is up
cds space host get <HOST>
```

If the HOST is not yet in the CLI db, `space host add` generates an SSH key and
installs it (needs password access the first time). If you already have a key the
HOST trusts, that path is reused.

---

## 4. Exercise the orchestration cluster (kind)

```sh
cds space orc run     -t <HOST>     # build the kind cluster (IPv4-only network)
cds space orc status  -t <HOST>     # running / exited / deleted
cds space orc stop    -t <HOST>
cds space orc start   -t <HOST>
cds space orc kubeconfig -t <HOST>  # fetch kubeconfig (alias: expose)
cds space orc delete  -t <HOST>     # refused if containers depend on it; add -f to force
```

## 5. Exercise the secure registry

```sh
cds space registry run    -t <HOST>   # TLS + auth, Quadlet unit, host trust
cds space registry status -t <HOST>
cds space registry stop   -t <HOST>
cds space registry start  -t <HOST>
cds space registry delete -t <HOST>   # removes the service; image data is kept
```

---

## 6. The full development workspace (the real test)

Create a project whose devcontainer requests orchestration + ingress + registry.

```sh
mkdir -p ~/labs/mylab/.devcontainer
cat > ~/labs/mylab/.devcontainer/devcontainer.json <<'JSON'
{
  "image": "<an image with sshd + skopeo/podman + kubectl/helm>",
  "orchestration": {
    "engine": "kind",
    "ingress": true,
    "registry": { "secure": true },
    "mirrors": [
      { "registry": "docker.io", "endpoint": "https://registry-1.docker.io" }
    ]
  }
}
JSON

# register the project against this .devcontainer (NOT --flavour; that path is an
# Artifactory stub in OSS) and deploy it
cds project init -n mylab -C ~/labs/mylab
cds project run  mylab -t <HOST>
```

`project run` brings up (concurrently) the cluster + registry + ingress, joins the
devcontainer to the `kind` network, installs the cluster kubeconfig and registry
CA + env into the container, creates the `cdsregistrycred` pull secret, and trusts
the registry CA on the kind nodes.

Inside the devcontainer (`cds project ssh mylab` or `cds project rsh mylab`):

```sh
# the registry connection is pre-exported as CDS_REGISTRY_* and trusted
env | grep CDS_REGISTRY

# build locally and push to the local registry (example with podman)
# image name MUST be $CDS_REGISTRY_NAME:$CDS_REGISTRY_PORT/<name>:<tag>
podman build -t $CDS_REGISTRY_NAME:$CDS_REGISTRY_PORT/myapp:latest .
echo "$CDS_REGISTRY_PWD" | podman login -u "$CDS_REGISTRY_USR" --password-stdin \
  --cert-dir "$(dirname $CDS_REGISTRY_PATH_TO_CA)" $CDS_REGISTRY_NAME:$CDS_REGISTRY_PORT
podman push $CDS_REGISTRY_NAME:$CDS_REGISTRY_PORT/myapp:latest

# deploy a chart that references cds.sec.io:<port>/myapp with:
#   imagePullSecrets: [{ name: cdsregistrycred }]
#   ingress.hosts[].host: cds-kind-control-plane
helm install mylab ./mychart
kubectl get pods            # pod should pull from the local registry and run
```

Then, **from your laptop**, expose the in-cluster service through the ingress and
open it in a browser / Bruno / Postman:

```sh
# forwards localhost:1337 -> cds-kind-control-plane:80 (the ingress) through the
# devcontainer, which routes by Host header to your Service
cds project expose mylab -l localhost:1337 -r cds-kind-control-plane:80
# in another shell:
curl -H 'Host: cds-kind-control-plane' http://localhost:1337/
```

You can also expose a plain service running on the devcontainer itself:

```sh
cds project expose mylab -l localhost:5000 -r 0.0.0.0:8000
```

---

## 7. Tear down

```sh
cds project delete mylab                # removes the container + project config
cds space orc delete      -t <HOST> -f
cds space registry delete -t <HOST>
cds space host delete <HOST>            # tears down the agent on the HOST
```

---

## What to log as issues

Note anything that: errors out, needs a manual step the docs/enterprise did not,
behaves differently from enterprise cds, or is missing a command/flag. Useful
context to capture per issue: the exact command, the `-t`/project used, the full
error (the client logs to `~/.cds/cds.log`), and the HOST's podman/kind versions.

Known not-yet-done (don't re-log these):
- `cds project expose --service kind` is implemented but was not E2E'd; the
  documented in-cluster expose uses `-r cds-kind-control-plane:80`.
- Devcontainers use the `systemd-run --scope` detachment, not Quadlet (deliberate).
- Agent binary auto-download is not wired; use `CDS_AGENT_BINARY`.
- Ingress refactor (ingress-nginx is EOL March 2026) is deferred.
