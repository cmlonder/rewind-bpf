# Rewind Control Plane UI

This is the fixture-driven Phase 1 operational UI prototype. It is intentionally separate from the public project narrative in `site/`.

## Preview

From the repository root:

```bash
python3 -m http.server 4173 --directory ui
open http://127.0.0.1:4173
```

The prototype never loads eBPF, mounts a filesystem, starts a process, or writes a workspace. Actions mutate only in-memory fixture state so the interaction model can be reviewed safely on a personal host.

## Adapter boundary

`data/fixture.js` is the Phase 1 adapter. A future `api-adapter.js` can replace it without changing the view components. The planned connected boundary is a local `rewindd` supervisor over a Unix socket or localhost-only API; the browser must never receive root privileges.
