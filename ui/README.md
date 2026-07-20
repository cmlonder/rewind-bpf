# Rewind Control Plane UI

This is the fixture-driven Phase 1 operational UI prototype. It is intentionally separate from the public project narrative in `site/`.

## Preview

From the repository root:

```bash
python3 -m http.server 4173 --directory ui
open http://127.0.0.1:4173
```

The prototype never loads eBPF, mounts a filesystem, starts a process, or writes a workspace. Actions mutate only in-memory fixture state so the interaction model can be reviewed safely on a personal host.

## What is interactive today

- Search and filter the run ledger, then open a transaction detail view.
- Review the timeline, upper-layer diff, evidence health, and benchmark context.
- Open a confirmation dialog for rollback, recovery, and export actions.
- Create and select versioned policy packages, copy a policy preview, and simulate deny/allow/audit outcomes.
- Add or edit workspace assignments and run a fixture boundary test before a future run.
- Edit global runtime settings through revisioned controls; active runs remain unchanged.
- Open notifications, inspect audit placeholders, and see explicit empty-search feedback.
- Use all destinations on mobile through a horizontally scrollable bottom navigation.

These actions deliberately stop at the fixture adapter until a supervisor is
connected. When connected to the explicit loopback HTTP bridge, rollback,
recovery, and commit confirmations POST authenticated intents to the supervisor;
the browser still never receives root access or performs privileged work.

## Demo scope decision

Local authentication is intentionally deferred until after the hackathon demo. The demo runs in fixture mode and does not expose a connected runtime, so authentication would add complexity without improving the jury flow. When `rewindd` is connected, authentication and authorization will be introduced at the Unix-socket/API boundary rather than inside browser components.

## Adapter boundary

`data/fixture.js` remains the safe demo adapter. `data/supervisor-adapter.js`
can connect to the optional loopback HTTP bridge (`--http-listen` plus an exact
`--cors-origin`) and exposes an authenticated action function. The browser
requests health, capabilities, history, and redacted audit data, then sends only
validated action intents; it must never receive root privileges or raw
credentials. Privileged actions remain an explicit supervisor/CLI boundary.
