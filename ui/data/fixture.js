const baseTime = "2026-07-20T10:42:18.000Z";

export const fixture = {
  environment: "Fixture mode · no kernel or workspace access",
  metrics: {
    activeRuns: 1,
    protectedWorkspaces: 4,
    evidenceComplete: 98,
    upperBytes: "18.4 MiB",
    vmAcceptance: "PASS",
    releaseBundle: "verified",
    storageAmplification: "1.0003×",
    eventBytes: "148.47 B/event",
    lifecycle: "64.34 s",
  },
  runs: [
    {
      id: "run_20260720T104218Z_08e0ef80", shortId: "08e0ef80", state: "running", workspace: "payments-api", workspacePath: "/workspaces/payments-api", command: "agent --task refactor-auth", policy: "strict-agent@1.3.0", backend: "fuse-overlayfs", startedAt: baseTime, elapsed: "00:01:42", upperBytes: 19293798, upperLabel: "18.4 MiB", processCount: 4,
      evidence: { count: 1204, bytes: 186420, dropped: 0, truncated: false, chainValid: true, recordMatch: true, segments: 4 },
      resources: { pids: "4 / 256", memory: "182 / 512 MiB", cpu: "8%" },
      events: [
        { time: "00:00.000", type: "lifecycle", operation: "RUN PREPARED", path: "lower layer captured", decision: "allow", risk: "low", detail: "Manifest SHA-256: 6bd3…a91e" },
        { time: "00:00.081", type: "lifecycle", operation: "SENSOR ATTACHED", path: "eBPF tracepoints", decision: "allow", risk: "low", detail: "Start gate opened before agent exec" },
        { time: "00:00.114", type: "process", operation: "EXECVE", path: "agent --task refactor-auth", decision: "allow", risk: "medium", detail: "PID 14822 admitted to cgroup" },
        { time: "00:00.892", type: "read", operation: "READ", path: ".env", decision: "deny", risk: "high", detail: "Matched **/*.env · Landlock EACCES" },
        { time: "00:01.204", type: "write", operation: "DELETE", path: "src/auth/legacy.go", decision: "allow", risk: "high", detail: "Upper layer only · lower remains intact" },
        { time: "00:01.511", type: "write", operation: "WRITE", path: "src/auth/session.go", decision: "allow", risk: "high", detail: "Copy-on-write: 24 KiB" },
        { time: "00:01.722", type: "process", operation: "SPAWN", path: "go test ./...", decision: "allow", risk: "medium", detail: "Descendant PID 14831" },
        { time: "00:02.008", type: "network", operation: "RAW SOCKET", path: "AF_INET / SOCK_RAW", decision: "deny", risk: "critical", detail: "Seccomp refusal recorded by eBPF socket tracepoint" },
      ],
      diff: [
        { path: "src/auth/legacy.go", kind: "deleted", bytes: "8.2 KiB", note: "upper-layer tombstone" },
        { path: "src/auth/session.go", kind: "modified", bytes: "+24 KiB", note: "copy-on-write" },
        { path: "tmp/agent-plan.md", kind: "created", bytes: "1.4 KiB", note: "new upper-layer file" },
        { path: "config/local.yaml", kind: "denied", bytes: "—", note: "sensitive read blocked" },
      ],
    },
    {
      id: "run_20260720T101107Z_d1b08d60", shortId: "d1b08d60", state: "rolled_back", workspace: "rewind-demo", workspacePath: "/workspaces/rewind-demo", command: "rm -rf src && generate", policy: "hackathon-demo@0.4.0", backend: "fuse-overlayfs", startedAt: "2026-07-20T10:11:07.000Z", elapsed: "00:00:03", upperBytes: 134217728, upperLabel: "128 MiB", processCount: 0,
      evidence: { count: 51, bytes: 17848, dropped: 0, truncated: false, chainValid: true, recordMatch: true, segments: 1 }, resources: { pids: "0 / 256", memory: "0 / 512 MiB", cpu: "0%" },
      events: [
        { time: "00:00.000", type: "lifecycle", operation: "RUN PREPARED", path: "lower marker captured", decision: "allow", risk: "low", detail: "original-source" },
        { time: "00:00.432", type: "write", operation: "DELETE", path: "src/", decision: "allow", risk: "critical", detail: "Agent deletion isolated in upper layer" },
        { time: "00:00.981", type: "write", operation: "WRITE", path: "generated.txt", decision: "allow", risk: "high", detail: "Visible only in merged view" },
        { time: "00:02.104", type: "lifecycle", operation: "ROLLBACK", path: "upper/work discarded", decision: "allow", risk: "low", detail: "Lower marker restored: original-source" },
      ], diff: [{ path: "src/", kind: "deleted", bytes: "128 MiB", note: "discarded at rollback" }, { path: "generated.txt", kind: "created", bytes: "17 B", note: "discarded at rollback" }],
    },
  ],
  policies: [
    { name: "strict-agent", version: "1.3.0", state: "assigned", signed: true, description: "High-safety profile for autonomous coding agents", reads: "enforce", writes: "rollback", network: "audit", assigned: 2, updated: "12 min ago" },
    { name: "developer-safe", version: "0.8.2", state: "available", signed: true, description: "Fast iteration with sensitive-read audit", reads: "audit", writes: "rollback", network: "off", assigned: 1, updated: "Yesterday" },
    { name: "hackathon-demo", version: "0.4.0", state: "available", signed: true, description: "Small, explainable profile for the live jury flow", reads: "enforce", writes: "rollback", network: "audit", assigned: 1, updated: "Jul 18" },
  ],
  workspaces: [
    { name: "payments-api", path: "/workspaces/payments-api", policy: "strict-agent@1.3.0", status: "protected", lastRun: "08e0ef80", agent: "agent --task refactor-auth", adapter: "codex", network: "audit" },
    { name: "rewind-demo", path: "/workspaces/rewind-demo", policy: "hackathon-demo@0.4.0", status: "protected", lastRun: "d1b08d60", agent: "demo-agent --dangerous", adapter: "generic", network: "audit" },
    { name: "docs-site", path: "/workspaces/docs-site", policy: "developer-safe@0.8.2", status: "protected", lastRun: "—", agent: "agent --task docs", adapter: "openhands", network: "off" },
    { name: "scratch", path: "/workspaces/scratch", policy: "none", status: "unassigned", lastRun: "—", agent: "not configured", adapter: "generic", network: "off" },
  ],
  config: {
    revision: 42,
    values: { overlay: "fuse-overlayfs", readMode: "enforce", writeMode: "rollback", network: "audit", eventCap: "unlimited", rotation: "512 KiB", retention: "7 days", truncation: "fail closed", encryption: "AES-256-GCM", trustRotation: "2 pinned keys", remoteRetention: "signed HTTPS", session: "reconnectable", pii: "audit-only" },
  },
  history: [
    { id: "d1b08d60", state: "rolled_back", workspace: "rewind-demo", updated: "2 min ago", size: "128 MiB upper" },
    { id: "08e0ef80", state: "running", workspace: "payments-api", updated: "now", size: "18.4 MiB upper" },
  ],
  effectivePolicy: [["read.mode", "enforce", "strict-agent@1.3.0"], ["read.deny", "**/*.env · **/*.pem · /home/*/.ssh/**", "package"], ["read.pii.mode", "audit", "global default"], ["write.mode", "rollback", "global default"], ["write.scope", "workspace", "workspace override"], ["network.mode", "audit", "package"], ["resources", "256 PIDs · 512 MiB · 50% CPU", "global default"]],
  audit: [["10:42:18", "Run started", "strict-agent@1.3.0", "system"], ["10:41:52", "Policy assigned", "payments-api", "cemal"], ["10:39:10", "Evidence exported", "run_d1b08d60", "cemal"]],
  checkpoints: { nodes: [
    { id: "root", runId: "08e0ef80", state: "succeeded", parents: [] },
    { id: "review-08e0", runId: "08e0ef80", state: "running", parents: ["root"] },
    { id: "rollback-demo", runId: "d1b08d60", state: "rolled_back", parents: ["root"] },
  ] },
  piiFindings: [{ path: "config/generated.env", kind: "github_token", hash: "sha256:2f9a…", replacement: "[REDACTED:github_token]", source: "post-run scan" }],
  remoteRetention: { state: "ready", endpoint: "s3-compatible gateway", digest: "sha256:9d7e…c41", lastRestore: "never · ready to verify" },
  adapterLifecycle: [
    { name: "Codex", kind: "codex", stage: "prepared", status: "identity exported" },
    { name: "OpenHands", kind: "openhands", stage: "hooked", status: "lifecycle callbacks" },
    { name: "Claude", kind: "claude", stage: "validated", status: "command contract" },
  ],
  hardening: {
    namespace: { state: "verified", title: "veth / NAT egress broker", detail: "UTM acceptance passed · atomic DNS refresh and cleanup verified" },
    registry: { state: "verified", title: "Signed policy registry", detail: "HTTPS + pinned Ed25519 keys + retry bound" },
    sessions: { state: "ready", title: "SQLite lease store", detail: "WAL · expiry · takeover semantics" },
    native: [
      { platform: "macOS", state: "manual gate", detail: "Seatbelt + EndpointSecurity + APFS" },
      { platform: "Windows", state: "manual gate", detail: "Job Object + minifilter + VHDX" },
    ],
  },
  registry: {
    state: "verified",
    endpoint: "https://registry.rewind.example/v1",
    lastVerified: "today · 10:39",
    keys: [
      { id: "rewind-prod-2026", state: "current" },
      { id: "rewind-previous-2025", state: "previous" },
    ],
    checks: [
      ["Endpoint transport", "HTTPS + bounded retry", "verified"],
      ["Signer trust", "2 pinned Ed25519 keys", "verified"],
      ["Envelope admission", "signature before persistence", "verified"],
      ["Revocation", "marker-backed · 410 Gone", "verified"],
      ["Browser authority", "metadata only · no root", "ready"],
    ],
  },
};

export function getRun(id) { return fixture.runs.find((run) => run.id === id) || fixture.runs[0]; }
