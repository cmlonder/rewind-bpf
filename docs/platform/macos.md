# macOS native backend

The macOS backend is a native staged filesystem transaction for APFS hosts.
It is intentionally separate from the Linux OverlayFS/Landlock/eBPF path.

## What is implemented

- APFS clone-backed workspace staging with `cp -c -R` before the agent starts.
- Seatbelt process launch with writes confined to the staged view.
- User-defined `read.mode: enforce` patterns and bounded PII findings are
  hidden from the view for the duration of the child process.
- Lifecycle commands: `native run`, `native diff`, `native events`,
  `native rollback`, and `native commit --confirm`.
- Manifest-based destination conflict detection before commit.
- Durable JSON run record and `<record>.events.jsonl` sidecar that survive
  rollback/discard.

## Safe invocation

The record must be outside `--runtime-root`, because rollback removes that
dedicated directory:

```bash
rewind native run \
  --workspace ./project \
  --runtime-root /tmp/rewind-runtime \
  --policy ./policy.yaml \
  --record ./run.json \
  --on-success review -- claude

rewind native diff --record ./run.json
rewind native rollback --record ./run.json
# or: rewind native commit --record ./run.json --confirm
```

Run `make mac-native-smoke` to exercise the complete flow against generated
temporary files. It verifies delete isolation, sensitive-read denial, commit,
rollback, and destination drift refusal without touching a real project.

For a guided manual run with the exact commands and expected evidence, see the
[macOS native manual E2E runbook](macos_manual_e2e.md).

## Explicit limitations

This backend does not claim Linux-equivalent kernel telemetry. It currently
refuses `network.mode: enforce`, `write.scope: system`, and all
`resources.*` limits. EndpointSecurity file/process telemetry, network egress
control, process/resource containment, signed helper installation, and
crash/power-loss acceptance on disposable storage remain separate gates.
Seatbelt profile deny rules are retained for helper integration, while sensitive
read enforcement in the current runner is the combination of explicit runtime
root rules and staged-path hide/restore. The runner rejects any configured
runtime root that overlaps the source workspace, so an agent cannot reopen the
source workspace by guessing its absolute path.
