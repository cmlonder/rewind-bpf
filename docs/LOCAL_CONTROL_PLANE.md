# Local Control Plane Bridge

RewindBPF is not UTM-only. UTM is the privileged Linux acceptance environment;
the operator UI is designed to connect to a local supervisor on the same
computer as the runtime.

## Platform model

```text
Browser UI
   │ authenticated loopback HTTP + one-time action token
   ▼
Local supervisor
   ├── Linux: OverlayFS/FUSE + Landlock + cgroup-v2 + eBPF runtime
   ├── macOS: APFS clone + Seatbelt native transaction records
   └── Windows: capability/read-only bridge until minifilter + VHDX helper
```

The browser never receives root privileges, raw credentials, or a direct
filesystem handle. The supervisor owns the platform-specific mutation and
refuses capabilities that its helper cannot prove.

## Recommended operator flow

The supported local experience is a single launcher command:

```bash
go run ./cmd/rewind dashboard start --workspace /path/to/project
```

The launcher owns the local supervisor, token hand-off, static UI server, and
protected interactive shell. It opens the Control Plane automatically, connects
using a one-time URL fragment, and removes the fragment after the browser has
connected. The shell is the transaction boundary: `rm -rf src` is visible in
the live timeline and staged diff, while the source workspace remains intact.
After the shell exits, the supervisor stays alive for review. The operator
uses the UI to roll back or commit, then stops the launcher with `Ctrl-C`.

This is intentionally different from host-wide monitoring. A terminal started
outside the launcher is not implicitly protected; broad host observation is a
separate signed-helper milestone for each operating system. The launcher is
therefore safe to try on a personal Mac when its workspace points at a
disposable fixture, and it is not an excuse to test destructive commands in a
real repository.

## Linux

Run `rewind supervisor` in the Ubuntu VM and connect the Mac-hosted UI through
an SSH local-forward. Runs must use the same `--history` path as the
supervisor. See the connected example in `README.md`.

## macOS

Build the Darwin binary and start the same supervisor command locally. Native
runs opt into the shared history index:

```bash
GOTOOLCHAIN=local go build -o /tmp/rewind-darwin ./cmd/rewind

/tmp/rewind-darwin supervisor \
  --socket /tmp/rewind-darwin-supervisor.sock \
  --history /Users/Shared/rewind-history.json \
  --token-file /Users/Shared/rewind-darwin-supervisor.token \
  --http-listen 127.0.0.1:8787 \
  --cors-origin http://127.0.0.1:4174
```

Start a native transaction with:

```bash
/tmp/rewind-darwin native run \
  --workspace /Users/Shared/rewind-manual/workspace \
  --runtime-root /Users/Shared/rewind-manual/runtime \
  --policy /Users/Shared/rewind-manual/policy.yaml \
  --record /Users/Shared/rewind-manual/run.json \
  --history /Users/Shared/rewind-history.json \
  --on-success review \
  -- /bin/sh -c 'printf "candidate\n" > generated.txt'
```

The native record is indexed without copying workspace contents. The UI then
shows it through `/v1/history` and `/v1/events`, and commit/rollback actions
reuse the same conflict-checked native backend as the CLI.

The current macOS bridge does not claim EndpointSecurity telemetry, network
enforcement, or resource limits. Those remain signed-helper gates and appear
as degraded capabilities in the UI.

## Windows

The common bridge contract and capability reporting are cross-buildable, but
native mutation is intentionally refused until the signed minifilter and VHDX
transaction helper pass acceptance. A Windows UI connection can therefore show
platform status and policy metadata without implying host protection.

## Fixture mode

When no supervisor is connected, the UI uses deterministic fixture data. It has
no host access and all actions are in-memory previews. The top-right badge must
read `LOCAL RUNTIME` before an operator treats an action as backend-backed.
