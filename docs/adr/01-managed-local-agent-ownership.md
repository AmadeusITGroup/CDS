# ADR 0001: Track Managed Local Agent Ownership

## Status

Accepted

## Context

`cds space host add localhost` can start a local `cds-api-agent` process and
register it as the local DevContainer host. `cds space host delete localhost`
must unregister the host and should stop the local agent only when CDS owns that
agent instance.

The unsafe shortcut is to discover a process by port, for example by looking for
the listener on `8087`, then kill it if the executable name looks like a CDS
agent. That is not a stable ownership contract. A listener on the configured
port may have been started manually, by another CDS checkout, by a service
manager, or by a different registration. Runtime discovery also has races
between process lookup, executable inspection, and signaling.

CDS already stores local host lifecycle state in `db.json`: the local host row
contains runtime detection metadata and is removed when the host is
unregistered. That host row is the natural place to record process ownership for
agents started directly by CDS.

## Decision

Persist a local agent ownership record on the DB host entry when CDS starts an
agent process it owns. The record stores:

- `pid`: process ID returned by `exec.Cmd.Start`.
- `address`: registered agent address.
- `binary`: executable name/path expected for the owned process.
- `manager`: lifecycle manager, currently `process` for direct child processes
  and `systemd` for systemd-managed Linux agents.

`cds space host delete localhost` stops only process-managed agents with a
stored ownership record. Before signaling, CDS verifies that the stored PID still
points to a process whose executable basename matches the stored binary. If the
process is gone or does not match, deletion continues without signaling.

Automatic local agent startup runs after `db.json` is loaded and only when both
stores agree that localhost is registered: `cliconfig.yaml` has a localhost
agent endpoint and `db.json` has a localhost host row. If automatic startup
creates a new process, CDS persists the returned ownership before command
execution. The CLI does not auto-start the local agent immediately before
`space host delete localhost`, because delete is the operation that tears that
registration down.

If CDS starts a process but cannot complete registration, it best-effort stops
the process through the same ownership checks before returning the error.

CDS does not stop arbitrary listeners on the agent port. Port discovery is not
used as proof of ownership.

## Consequences

- Delete semantics are safer: CDS stops only the local process it previously
  recorded as owned by the registration.
- Manual or externally managed agents listening on the same port are not killed
  by unregistering the host.
- Stale ownership is tolerated. If the recorded PID no longer exists, delete
  still removes registration state.
- The first delete after upgrading from a version that did not persist ownership
  may unregister state without stopping an already-running legacy agent. That is
  intentional; the old process has no trusted ownership record.
- Systemd-managed agents are represented distinctly by `manager: systemd`. A
  future change can stop them through `systemctl --user stop` rather than PID
  signaling.
- A partial registration failure should not leave a directly started local agent
  running without an ownership record; rollback uses the recorded process
  ownership for best-effort cleanup.

## Alternatives Considered

### Kill by listening port

Rejected. It can terminate a process that CDS did not start, and it has lookup
and PID reuse races.

### Store ownership in `cliconfig.yaml`

Rejected. `cliconfig.yaml` is the agent endpoint registry. Process ownership is
host lifecycle state and belongs next to the local host runtime metadata in
`db.json`.

### Add a shutdown RPC first

Deferred. A graceful shutdown RPC would be cleaner for live agents, but it still
needs an ownership decision before the CLI decides which agent it is allowed to
stop.
