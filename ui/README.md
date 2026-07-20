# Rewind Control Plane UI

This is the fixture-driven operational UI. It includes the Phase 2 recovery,
release-proof, trust, and action-token surfaces for a safe jury walkthrough.
It is intentionally separate from the public project narrative in `site/`.

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
- Import an Ed25519-signed policy bundle through the connected supervisor; invalid or tampered envelopes are refused and audited.
- Export a persisted signed envelope for review or transfer without exposing private keys.
- Show supervisor connection state and retry an interrupted authenticated event stream with bounded exponential backoff.
- Add or edit workspace assignments and run a fixture boundary test before a future run.
- Edit global runtime settings through revisioned controls; active runs remain unchanged.
- Inspect broker status and request metadata-only credential leases when an opt-in provider is configured.
- Document the opt-in native macOS Keychain/Linux Secret Service provider boundary; the UI still displays lease metadata only.
- Apply keep-latest history pruning and manage expiring reconnect/takeover session leases; fixture mode previews these in memory, while connected mode persists them through the authenticated supervisor.
- Show the optional content PII scanner as audit-only: findings are hashed/redacted and the scanner never changes read permissions.
- Select a generic, Codex, OpenHands, or Claude Code adapter per workspace; the adapter is an auditable launch identity, not an SDK wrapper yet.
- Open notifications, inspect audit event details, and see explicit empty-search feedback.
- Use the Trust & Actions screen to understand one-time action-token challenges,
  edit a pinned HTTPS registry profile, verify signer rotation, and preview a
  signed policy fetch without exposing bearer tokens or private keys. A connected
  supervisor also renders the server-proxied registry package inventory.
- Use all destinations on mobile through a horizontally scrollable bottom navigation.
- Use the dedicated **System Boundaries** screen to distinguish the lower/merged/upper invariant, process scope, network/secret boundary, and platform support from editable defaults.
- Open the small `i` help affordances beside controls for a deeper explanation of authority, retention, session leases, evidence, PII, registry verification, and benchmark caveats.

Fixture mode has no browser authentication because it has no kernel, mount,
process, workspace, or host access. The retention and session dialogs remain
usable there as in-memory previews, so a reviewer can understand the workflow
without a misleading “authenticated user” dead end. When connected to the
explicit loopback HTTP bridge, policy and
workspace forms persist validated local config, signed bundle imports are
verified by the supervisor, rollback/recovery/commit
confirmations POST authenticated intents, and the run detail view follows its
authenticated SSE evidence stream. Credential lease responses contain only an
opaque ID/scope/expiry marker, and the browser still never receives root access
or performs privileged work. Destructive browser intents additionally require
a two-minute, one-time action token. In fixture mode it is browser-bound; on a
connected supervisor the token is issued and consumed server-side, with replay
and action/run mismatches refused. It is a human-intent challenge, not a
replacement for supervisor bearer authentication.

## Authority and authentication boundary

There is no separate browser login in this UI. The demo runs in fixture mode,
where all actions are deterministic in-memory previews and no authentication is
needed. Connected mode uses a bearer token issued by `rewind supervisor`; the
browser keeps it in memory and sends it only to the local supervisor bridge.
The supervisor—not the browser—owns privileged mounts, action-token challenges,
retention pruning, session leases, registry signature verification, and run
mutations. Local authentication beyond that Unix-socket/bearer boundary remains
a post-demo hardening item. “Manage session” means a detachable run lease
(acquire, heartbeat, takeover, release), not a user account login.

## Adapter boundary

`data/fixture.js` remains the safe demo adapter. `data/supervisor-adapter.js`
can connect to the optional loopback HTTP bridge (`--http-listen` plus an exact
`--cors-origin`) and exposes authenticated policy bundle inventory and action
functions. The browser
requests health, capabilities, history, and redacted audit data, then sends only
validated action intents; it must never receive root privileges or raw
credentials. Privileged actions remain an explicit supervisor/CLI boundary.
