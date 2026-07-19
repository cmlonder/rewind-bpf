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

These actions deliberately stop at the adapter boundary. They demonstrate the control-plane contract without granting the browser root access or pretending that fixture state is live runtime state.

## Adapter boundary

`data/fixture.js` is the Phase 1 adapter. A future `api-adapter.js` can replace it without changing the view components. The planned connected boundary is a local `rewindd` supervisor over a Unix socket or localhost-only API; the browser must never receive root privileges.
