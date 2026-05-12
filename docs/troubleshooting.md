# CDS troubleshooting guide

This guide lists common developer-experience issues and the fastest way to
diagnose or recover from them.

## Quick diagnostics

Start with:

```sh
make help
go version
protoc --version
which podman || which docker
echo "$CDS_CONFIG_PATH"
```

For an isolated validation run, prefer:

```sh
export CDS_CONFIG_PATH="$HOME/cds-debug"
export PATH="$PWD:$PATH"
```

## Agent is unreachable

Symptoms:

- `space host get` shows `unreachable`.
- gRPC calls fail with `connection refused`.
- CLI reports that an agent target is not configured.

Checks:

```sh
lsof -tiTCP:8087 -sTCP:LISTEN || true
./cds space host get localhost
```

For a remote host:

```sh
REMOTE_HOST=my-remote-host.example.com
ssh "$REMOTE_HOST" 'lsof -tiTCP:8087 -sTCP:LISTEN || true'
ssh "$REMOTE_HOST" 'tail -100 ~/cds-remote-validation/agent.log || true'
```

Common fixes:

- ensure `cds-api-agent` is on `PATH` before running local bootstrap;
- restart the local or remote agent;
- verify the configured host name matches the registered agent address;
- check firewall or remote port access for `8087`.

## TLS handshake fails

Symptoms:

- `x509: certificate signed by unknown authority`
- `x509: ECDSA verification failure`
- `transport: authentication handshake failed`

Common causes:

- CLI and agent are using different CA files;
- certs were copied to `.xcds-agent/certs` while the manually started agent is
  using `.xcds/certs` under its `CDS_CONFIG_PATH`;
- certs were regenerated locally after the remote agent was started.

Fix:

1. Stop the agent.
2. Copy the active CLI certs from `$CDS_CONFIG_PATH/.xcds/certs` to the cert
   directory used by the agent's `CDS_CONFIG_PATH`.
3. Restart the agent.
4. Run `./cds space host get HOST`.

## macOS Podman mount errors

Symptoms:

- deploy fails while creating the container;
- Podman reports that bind mount sources cannot be mounted;
- bind mount paths under `/tmp` fail on macOS.

Cause:

Podman on macOS runs in a VM and may only share selected host paths. Arbitrary
`/tmp` paths are often unavailable inside the VM.

Fix:

Use HOME-based validation paths:

```sh
mkdir -p "$HOME/folder"
```

## SSH into container fails

Symptoms:

- `project ssh` cannot authenticate;
- `project share` prints a connection command but SSH is denied;
- the SSH port is `0`.

Checks:

```sh
./cds project sync PROJECT
./cds project show PROJECT
podman port CONTAINER_NAME 22/tcp
ls -l ~/.ssh/id_rsa.pub ~/.ssh/id_ed25519.pub 2>/dev/null
```

Common fixes:

- run `project sync` to refresh port mappings from the agent;
- ensure the deployed container exposes `22`;
- ensure a default public key exists, preferably `~/.ssh/id_ed25519.pub`;
- redeploy if the container was created before SSH key installation support was
  added.

## `project sync` removes or changes container state

`project sync` trusts the agent as the source of truth. If the container no
longer exists on the agent host, local project state is removed for that
container.

Check the agent directly:

```sh
podman ps -a --format '{{.Names}} {{.Status}}'
```

If the container was removed intentionally, rerun:

```sh
./cds project run PROJECT
```

## Remote host name does not resolve

If the remote host name does not resolve, check DNS:

```sh
host "$REMOTE_HOST"
ssh -o BatchMode=yes "$REMOTE_HOST" 'hostname'
```

## `project run --src-repo` is unavailable

Source-repository deployment is not currently implemented through the agent API.
Use a configured project or path-based deployment:

```sh
./cds project run --path my-flavour/latest --target localhost
```

## `project expose --service` is unavailable

Named service discovery is not currently exposed through the agent API. Use a
direct remote address instead:

```sh
./cds project expose PROJECT --remote localhost:22 --local 127.0.0.1:2222 --timeout 1h
```

## Cleaning validation leftovers

Local:

```sh
./cds project clear latest || true
./cds project delete latest || true
lsof -tiTCP:8087 -sTCP:LISTEN || true
podman ps -a --format '{{.Names}} {{.Status}}' || true
```

Remote:

```sh
REMOTE_HOST=my-remote-host.example.com
ssh "$REMOTE_HOST" 'lsof -tiTCP:8087 -sTCP:LISTEN || true'
ssh "$REMOTE_HOST" 'podman ps -a --format "{{.Names}} {{.Status}}" || true'
ssh "$REMOTE_HOST" 'rm -rf ~/cds-remote-validation'
```

