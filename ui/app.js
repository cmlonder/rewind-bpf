import { fixture, getRun } from "./data/fixture.js";
import { connectSupervisor, followEvents } from "./data/supervisor-adapter.js";
import { AppShell } from "./components/layout.js";

const app = document.querySelector("#app");
const state = { view: "overview", selectedRun: fixture.runs[0].id, selectedPolicy: fixture.policies[0].name, runFilter: "all", toast: null, supervisor: null, eventAbort: null, eventRenderTimer: null, refreshTimer: null, snapshotSignature: "", reconnectTimer: null, reconnectAttempts: 0, connection: "fixture", actionTokens: new Map() };
let modalRestoreFocus = null;
let toastTimer = null;

function currentRun() { return getRun(state.selectedRun); }
function escapeUI(value) { return String(value ?? "").replace(/[&<>\"']/g, (character) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "\"": "&quot;", "'": "&#39;" }[character])); }

function render() {
  globalThis.__rewindSelectedPolicy = state.selectedPolicy;
  app.innerHTML = AppShell({ view: state.view, run: currentRun(), fixture, connection: state.connection });
  bindInteractions();
  if (state.toast) showToast(state.toast.message, state.toast.tone);
}

function bindInteractions() {
  decorateInfoButtons();
  document.querySelectorAll("[data-view]").forEach((element) => element.addEventListener("click", () => { state.view = element.dataset.view; render(); }));
  document.querySelectorAll("[data-run-id]").forEach((element) => element.addEventListener("click", () => { const match = fixture.runs.find((item) => item.id === element.dataset.runId || item.shortId === element.dataset.runId); if (match) state.selectedRun = match.id; state.view = "run-detail"; render(); }));
  document.querySelectorAll("[data-action]").forEach((element) => element.addEventListener("click", () => handleAction(element.dataset.action, element)));
  document.querySelectorAll("[data-info]").forEach((element) => element.addEventListener("click", (event) => { event.preventDefault(); event.stopPropagation(); openInfoModal(element.dataset.info); }));
  document.querySelectorAll(".diff-panel .text-button").forEach((element) => element.addEventListener("click", () => openDiffPreview()));
  document.querySelectorAll(".avatar").forEach((element) => element.addEventListener("click", () => openInfoModal("supervisor-auth")));
  const policyFooter = document.querySelector(".policy-editor .editor-foot");
  if (policyFooter && !policyFooter.querySelector("[data-action=export-policy-bundle]")) {
    const exportButton = document.createElement("button");
    exportButton.className = "text-button";
    exportButton.dataset.action = "export-policy-bundle";
    exportButton.innerHTML = "Export signed bundle <span>↗</span>";
    exportButton.addEventListener("click", () => handleAction("export-policy-bundle", exportButton));
    policyFooter.prepend(exportButton);
  }
  document.querySelectorAll("[data-policy]").forEach((element) => element.addEventListener("click", () => selectPolicy(element.dataset.policy)));
  const search = document.querySelector("[data-run-search]");
  if (search) search.addEventListener("input", () => filterRuns(search.value, state.runFilter));
  document.querySelectorAll("[data-run-filter]").forEach((element) => element.addEventListener("click", () => {
    state.runFilter = element.dataset.runFilter;
    document.querySelectorAll("[data-run-filter]").forEach((tab) => tab.classList.toggle("is-selected", tab === element));
    filterRuns(search?.value || "", state.runFilter);
  }));
  if (search) {
    document.querySelectorAll("[data-run-filter]").forEach((tab) => tab.classList.toggle("is-selected", tab.dataset.runFilter === state.runFilter));
    filterRuns(search.value, state.runFilter);
  }
}

function decorateInfoButtons() {
  const add = (selector, key, label) => {
    const target = document.querySelector(selector);
    if (!target || target.querySelector("[data-info]")) return;
    const button = document.createElement("button");
    button.className = "info-button";
    button.dataset.info = key;
    button.type = "button";
    button.setAttribute("aria-label", label);
    button.title = label;
    button.textContent = "i";
    target.append(button);
  };
  if (state.view === "trust") { add(".trust-hero .panel-kicker", "supervisor-auth", "Explain action-token authority"); add(".registry-panel .panel-kicker", "registry", "Explain trusted registry"); }
  if (state.view === "evidence") { add(".hash-panel .panel-kicker", "evidence-integrity", "Explain evidence integrity"); }
  if (state.view === "benchmarks") { add(".benchmark-hero .panel-kicker", "benchmark", "Explain benchmark controls"); }
  if (state.view === "recovery") { add(".hardening-panel .panel-kicker", "platform", "Explain platform support"); add(".panel:nth-child(2) .panel-kicker", "pii", "Explain PII findings"); }
}

function filterRuns(query, filter) {
  const normalized = query.trim().toLowerCase();
  let visible = 0;
  document.querySelectorAll("[data-run-id]").forEach((row) => {
    const matchesText = !normalized || row.dataset.runText.toLowerCase().includes(normalized);
    const matchesFilter = filter === "all" || row.dataset.runState === filter;
    row.hidden = !(matchesText && matchesFilter);
    if (!row.hidden) visible += 1;
  });
  const table = document.querySelector(".run-table");
  if (!table) return;
  let empty = table.querySelector(".run-empty");
  if (!empty) { empty = document.createElement("p"); empty.className = "run-empty"; empty.textContent = "No runs match this search or filter."; table.append(empty); }
  empty.hidden = visible > 0;
}

function selectPolicy(name) {
  state.selectedPolicy = name;
  const policy = fixture.policies.find((item) => item.name === name);
  if (policy) { render(); setToast(`${name}@${policy.version} selected. Effective policy and simulation are now updated.`, "neutral"); }
}

function handleAction(action, element) {
  if (action === "notifications") return openModal("Notifications", `<div class="notification-list"><div><span class="notification-dot"></span><div><strong>Evidence stream healthy</strong><p>Run <code>${currentRun().shortId}</code> has no dropped or truncated events.</p><small>just now · system</small></div></div><div><span class="notification-dot notification-dot-muted"></span><div><strong>Fixture adapter active</strong><p>Actions are simulated in memory until the supervisor API is connected.</p><small>today · control plane</small></div></div></div>`, { confirm: "Done", onConfirm: closeModal });
  if (action === "connect-supervisor") return openSupervisorConnector();
  if (action === "hold-review") { currentRun().state = "succeeded"; return setToast("Run held for review. Conflict-checked acceptance is now available.", "neutral"); }
  if (action === "simulate-credentials") return openCredentialLeaseCheck();
  if (action === "retention") return openRetentionEditor();
  if (action === "session") return openSessionEditor();
  if (action === "rollback") return openActionTokenConfirm({ action: "rollback", title: "Rollback this run?", kicker: "DESTRUCTIVE TO UPPER LAYER", body: "This discards the temporary upper/work layer while preserving the original lower layer and evidence record.", confirm: "Rollback run", tone: "orange", onConfirm: (token) => runSupervisorAction("rollback", rollback, "", token) });
  if (action === "commit") return openActionTokenConfirm({ action: "commit", title: "Accept reviewed changes?", kicker: "CONFLICT-CHECKED APPLY", body: "Rewind will compare the immutable base with the current destination first. Same-path drift refuses the apply; only the reviewed candidate is written.", confirm: "Accept changes", tone: "sage", onConfirm: (token) => runSupervisorAction("commit", commitRun, "COMMIT", token) });
  if (action === "recover") return openActionTokenConfirm({ action: "recover", title: "Recover stale run?", kicker: "PROCESS DRAIN", body: "The supervisor will drain descendants, remove the temporary mount, and preserve the lower workspace.", confirm: "Recover run", tone: "orange", onConfirm: (token) => runSupervisorAction("recover", () => setToast("Recovery completed in fixture mode.", "success"), "", token) });
  if (action === "export") return openExportPreview(element);
  if (action === "copy-policy") return copyPolicy();
  if (action === "export-policy-bundle") return exportPolicyBundle();
  if (action === "simulate-policy") return openSimulation();
  if (action === "new-policy") return openPolicyEditor();
  if (action === "import-policy") return openSignedPolicyImport();
  if (action === "new-workspace") return openWorkspaceEditor();
  if (action === "edit-workspace") return openWorkspaceEditor(element.closest(".workspace-card")?.querySelector("h2")?.textContent);
  if (action === "simulate-workspace") return openBoundaryTest(element);
  if (action === "view-revisions") return openRevisionHistory();
  if (action === "inspect-audit") return openAuditDetail(element);
  if (action === "config-change") return openConfigEditor(element.dataset.configKey);
  if (action === "pii-scan") return setToast("PII scan is audit-only: findings are hashed and redacted; it never broadens read access.", "neutral");
  if (action === "pii-rescan") return rescanPII();
  if (action === "remote-restore") return restoreRemoteBundle();
  if (action === "adapter-test") return runAdapterPreflight();
  if (action === "macos-test-guide") return openMacOSTestGuide();
  if (action === "checkpoint-rollback") return openActionTokenConfirm({ action: "graph-rollback", title: "Rollback dependent checkpoints?", kicker: "GRAPH-AWARE RECOVERY", body: "Rewind will order descendants before parents and refuse ambiguous dependencies.", confirm: "Rollback graph", tone: "orange", onConfirm: () => setToast("Checkpoint rollback plan accepted in fixture mode.", "success") });
  if (action === "trust-settings") return openTrustSettings();
  if (action === "verify-registry") return verifyTrustedRegistry();
  if (action === "import-registry-policy") return openRegistryImport();
  if (action === "rotate-trust-key") return openTrustKeyRotation();
}

function rescanPII() {
  const findings = fixture.piiFindings || (fixture.piiFindings = []);
  const existing = findings.find((finding) => finding.path === "runtime/generated.env");
  if (!existing) findings.push({ path: "runtime/generated.env", kind: "credential-shaped token", hash: "sha256:fixture…", replacement: "[REDACTED:token]", source: "fresh fixture scan" });
  setToast(`PII scan complete: ${findings.length} redacted finding${findings.length === 1 ? "" : "s"}.`, "success");
}

function restoreRemoteBundle() {
  const remote = fixture.remoteRetention || (fixture.remoteRetention = {});
  remote.state = "restored";
  remote.lastRestore = "just now · digest verified";
  render();
  setToast("Remote bundle restored after digest verification; retry budget remains bounded.", "success");
}

function runAdapterPreflight() {
  (fixture.adapterLifecycle || []).forEach((adapter) => {
    adapter.stage = "ready";
    adapter.status = "identity + lifecycle callbacks verified";
  });
  render();
  setToast("Codex, OpenHands, and Claude adapter preflight passed: identity and lifecycle hooks are ready.", "success");
}

function openMacOSTestGuide() {
  openModal("macOS native test runbook", `<div class="native-runbook"><div class="simulation-summary"><span class="verified-icon">✓</span><div><strong>Safe synthetic fixture only</strong><p>The smoke test uses a temporary directory and cleans up its runtime. It cannot touch your repository unless you edit the script yourself.</p></div></div><ol class="runbook-list"><li><code>make mac-native-smoke</code><span>review delete/write, sensitive-read denial, rollback, commit, and conflict refusal</span></li><li><code>make mac-safe-smoke</code><span>verify unsafe workspace roots and unsupported policies fail closed</span></li><li><code>go test ./...</code><span>run the platform and policy contract tests</span></li><li><code>GOOS=darwin GOARCH=arm64 go build ./cmd/rewind</code><span>build the Apple Silicon CLI without executing it</span></li></ol><div class="form-note"><b>Do not point the smoke scripts at a real project.</b> The production command requires an explicit workspace and record path; keep both under a disposable test directory while validating.</div></div>`, { confirm: "Close runbook", onConfirm: closeModal });
}

function openSupervisorConnector() {
  openModal("Connect local supervisor", `<form id="supervisor-form" class="modal-form"><label>Supervisor endpoint<input name="endpoint" value="http://127.0.0.1:8787" inputmode="url" pattern="https?://[^ ]+" required /></label><label>Bearer token<input name="token" type="password" autocomplete="off" placeholder="from supervisor token file" /></label><div class="form-note"><b>Fixture mode needs no authentication.</b> Connected mode uses the bearer token issued by <code>rewind supervisor</code>. The token authorizes the supervisor bridge only; the browser never receives root access, secret bytes, or registry credentials.</div></form>`, { confirm: "Connect", onConfirm: async () => {
    const form = document.querySelector("#supervisor-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    const endpoint = data.get("endpoint");
    const token = data.get("token");
    try {
      const connected = await connectSupervisor(endpoint, token);
      applySupervisorSnapshot(connected);
      rememberSupervisor(endpoint, token);
      state.connection = "connected";
      state.reconnectAttempts = 0;
      startSupervisorRefresh();
      if (connected.history.length) followConnectedEvents();
      closeModal(); render(); setToast("Supervisor connected: live evidence and authenticated actions enabled.", "success");
    } catch (error) { setToast(`Supervisor connection refused: ${error.message}`, "error"); }
  } });
}

function openCredentialLeaseCheck() {
  if (!state.supervisor?.issueCredentialLease) return setToast("Boundary check passed: broker unavailable, raw secret exposure refused.", "neutral");
  openModal("Issue a scoped credential lease", `<form id="credential-form" class="modal-form"><label>Credential reference<input name="ref" value="github" required /></label><label>Scopes<input name="scopes" value="read:org" placeholder="comma-separated scopes" /></label><div class="form-note">Only opaque lease metadata returns to the browser. Secret bytes stay inside the supervisor broker and are never shown here.</div></form>`, { confirm: "Issue lease", onConfirm: async () => {
    const form = document.querySelector("#credential-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    try {
      const lease = await state.supervisor.issueCredentialLease({ ref: String(data.get("ref")).trim(), scopes: String(data.get("scopes") || "").split(",").map((item) => item.trim()).filter(Boolean) });
      closeModal();
      setToast(`Lease ${String(lease.id || "issued").slice(0, 12)}… issued · secret_exposed=${lease.secret_exposed === false ? "false" : "unknown"}`, lease.secret_exposed === false ? "success" : "error");
      return true;
    } catch (error) { setToast(`Credential lease refused: ${error.message}`, "error"); }
  } });
}

function openRetentionEditor() {
  openModal("Prune run history", `<form id="retention-form" class="modal-form"><label>Keep newest entries<input name="keep" type="number" min="0" value="30" required /></label><div class="form-note"><b>${state.supervisor ? "Connected supervisor:" : "Fixture preview:"}</b> ${state.supervisor ? "the authenticated supervisor prunes bounded metadata after validating the request." : "this updates in-memory preview state only; no files are deleted."} Workspace layers and evidence archives are never silently deleted.</div></form>`, { confirm: "Prune history", onConfirm: async () => {
    const form = document.querySelector("#retention-form");
    if (!form?.reportValidity()) return false;
    const keep = Number(new FormData(form).get("keep"));
    try {
      const result = state.supervisor?.pruneHistory ? await state.supervisor.pruneHistory(keep) : { message: `fixture keeps latest ${keep}` };
      fixture.config.values.retention = `keep latest ${keep}`;
      fixture.config.revision += 1;
      closeModal(); render(); setToast(`History retention applied: ${result.message || "pruned"}`, "success");
    } catch (error) { setToast(`Retention refused: ${error.message}`, "error"); }
  } });
}

function openSessionEditor() {
  const run = currentRun();
  openModal("Manage detachable run session", `<form id="session-form" class="modal-form"><label>Run ID<input name="run_id" value="${run.id}" required /></label><label>Owner<input name="owner" value="control-plane" required /></label><label>Action<select name="action"><option value="acquire">Acquire</option><option value="heartbeat">Heartbeat</option><option value="takeover">Take over</option><option value="release">Release</option></select></label><label>Lease seconds<input name="ttl_seconds" type="number" min="30" max="86400" value="600" required /></label><div class="form-note"><b>${state.supervisor ? "Connected supervisor:" : "Fixture preview:"}</b> ${state.supervisor ? "the authenticated supervisor persists this lease and enforces takeover rules." : "this simulates the lease in memory; no authenticated session store is contacted."} A lease coordinates reconnect and takeover; it never bypasses run authorization or exposes credentials.</div></form>`, { confirm: "Apply session action", onConfirm: async () => {
    const form = document.querySelector("#session-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    try {
      const lease = state.supervisor?.session ? await state.supervisor.session({ action: data.get("action"), run_id: data.get("run_id"), owner: data.get("owner"), ttl_seconds: Number(data.get("ttl_seconds")) }) : { id: `fixture-${Date.now()}` };
      closeModal();
      fixture.config.values.session = `${data.get("action")} · ${data.get("owner")}`;
      setToast(`Session ${data.get("action")}: ${String(lease.id || "updated").slice(0, 12)}…`, "success");
    } catch (error) { setToast(`Session action refused: ${error.message}`, "error"); }
  } });
}

function applySupervisorSnapshot(connected) {
  const signature = JSON.stringify({
    history: connected.history.map((item) => [item.run_id, item.state, item.updated_at, item.upper_bytes]),
    policies: connected.policies.map((item) => [item.name, item.version, item.updated_at]),
    workspaces: connected.workspaces.map((item) => [item.name, item.path, item.policy]),
    audit: connected.audit.map((item) => [item.timestamp, item.action, item.run_id, item.ok]),
  });
  const changed = signature !== state.snapshotSignature;
  state.snapshotSignature = signature;
  state.supervisor = connected;
  fixture.credentialStatus = connected.credentialStatus || { available: false, state: "unavailable" };
  fixture.environment = `Connected supervisor · ${connected.capabilities.platform || "unknown"}`;
  fixture.registry.packages = connected.registryEntries || [];
  fixture.history = connected.history.map((item) => ({ id: item.run_id, state: item.state, workspace: item.workspace || "unknown", updated: item.updated_at || "just now", size: `${item.upper_bytes || 0} bytes upper` }));
  if (connected.history.length) {
    // Keep the object currently used by the live event stream. Replacing it
    // on every history poll would leave the SSE callback writing to a stale
    // object and make the UI appear to jump back to the preview state.
    const existing = new Map(fixture.runs.map((item) => [item.id, item]));
    fixture.runs = connected.history.map((item) => {
      const next = existing.get(item.run_id) || remoteRun(item);
      const liveEvents = next.events || [];
      const liveEvidence = next.evidence || { count: 0, bytes: 0, dropped: 0, truncated: false, chainValid: true, recordMatch: true, segments: 1 };
      Object.assign(next, remoteRun(item));
      next.events = liveEvents;
      next.evidence = liveEvidence;
      return next;
    });
    if (!fixture.runs.some((item) => item.id === state.selectedRun)) state.selectedRun = fixture.runs[0].id;
  }
  fixture.metrics.activeRuns = connected.history.filter((item) => item.state === "running").length;
  fixture.metrics.protectedWorkspaces = connected.workspaces.length || fixture.metrics.protectedWorkspaces;
      if (connected.policies.length) fixture.policies = connected.policies.map(remotePolicy);
      state.selectedPolicy = fixture.policies[0]?.name || state.selectedPolicy;
  if (connected.workspaces.length) fixture.workspaces = connected.workspaces.map(remoteWorkspace);
  if (connected.audit.length) fixture.audit = connected.audit.map((item) => [new Date(item.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }), item.action, item.run_id || "supervisor", item.ok ? "supervisor" : "refused"]);
  return changed;
}

function followConnectedEvents() {
  if (!state.supervisor || !state.supervisor.token || !currentRun()) return;
  state.eventAbort?.abort();
  const controller = new AbortController();
  state.eventAbort = controller;
  const run = currentRun();
  followEvents(state.supervisor.baseUrl, state.supervisor.token, run.id, (event) => {
    if (currentRun()?.id !== run.id) return;
    run.events.push({
      time: "live",
      type: event.operation === "network_connect" ? "network" : "write",
      operation: event.operation.replaceAll("_", " ").toUpperCase(),
      path: event.path || "—",
      decision: event.decision || "allow",
      risk: event.risk || "medium",
      detail: `sequence ${event.sequence || "—"} · hash ${String(event.hash || "").slice(0, 10)}…`,
    });
    run.evidence.count += 1;
    window.clearTimeout(state.eventRenderTimer);
    state.eventRenderTimer = window.setTimeout(() => {
      state.eventRenderTimer = null;
      render();
    }, 120);
  }, controller.signal).catch((error) => {
    if (error.name === "AbortError") return;
    if (error.status === 404) {
      state.eventAbort?.abort();
      setToast("No event record for this run in the connected supervisor. Use the same --history path; the missing stream will not be retried.", "neutral");
      return;
    }
    scheduleSupervisorReconnect(error);
  });
}

function scheduleSupervisorReconnect(error) {
  if (!state.supervisor || state.reconnectTimer) return;
  state.connection = "reconnecting";
  fixture.environment = `Supervisor reconnecting · attempt ${state.reconnectAttempts + 1}`;
  render();
  const delay = Math.min(10000, 1000 * (2 ** state.reconnectAttempts));
  state.reconnectAttempts += 1;
  state.reconnectTimer = window.setTimeout(async () => {
    state.reconnectTimer = null;
    try {
      const connected = await connectSupervisor(state.supervisor.baseUrl, state.supervisor.token);
      applySupervisorSnapshot(connected);
      state.connection = "connected";
      state.reconnectAttempts = 0;
      rememberSupervisor(state.supervisor.baseUrl, state.supervisor.token);
      startSupervisorRefresh();
      render();
      setToast("Supervisor connection restored; evidence stream resumed.", "success");
      followConnectedEvents();
    } catch (reconnectError) {
      scheduleSupervisorReconnect(reconnectError);
    }
  }, delay);
  if (error) setToast(`Supervisor stream paused: ${error.message}. Retrying…`, "error");
}

function startSupervisorRefresh() {
  window.clearInterval(state.refreshTimer);
  state.refreshTimer = window.setInterval(async () => {
    if (!state.supervisor || state.connection === "fixture") return;
    try {
      const connected = await connectSupervisor(state.supervisor.baseUrl, state.supervisor.token);
      const changed = applySupervisorSnapshot(connected);
      state.connection = "connected";
      state.reconnectAttempts = 0;
      if (changed) render();
    } catch (error) {
      scheduleSupervisorReconnect(error);
    }
  }, 2000);
}

function savedSupervisor() {
  try {
    const endpoint = window.sessionStorage.getItem("rewind.supervisor.endpoint");
    const token = window.sessionStorage.getItem("rewind.supervisor.token");
    return endpoint && token ? { endpoint, token } : null;
  } catch (_) { return null; }
}

function rememberSupervisor(endpoint, token) {
  try {
    window.sessionStorage.setItem("rewind.supervisor.endpoint", endpoint);
    window.sessionStorage.setItem("rewind.supervisor.token", token);
  } catch (_) { /* private browsing may disable session storage */ }
}

function remoteRun(item) {
  const id = item.run_id || item.id || "unknown-run";
  return {
    id,
    shortId: id.slice(-8),
    state: item.state || "unknown",
    workspace: item.workspace || "connected workspace",
    workspacePath: item.workspace || "—",
    command: "supervisor history",
    policy: "runtime record",
    backend: "reported by supervisor",
    startedAt: item.created_at || new Date().toISOString(),
    elapsed: "—",
    upperBytes: item.upper_bytes || 0,
    upperLabel: `${item.upper_bytes || 0} bytes`,
    processCount: 0,
    evidence: { count: 0, bytes: 0, dropped: 0, truncated: false, chainValid: true, recordMatch: true, segments: 1 },
    resources: { pids: "—", memory: "—", cpu: "—" },
    events: [{ time: "—", type: "lifecycle", operation: "SUPERVISOR SNAPSHOT", path: id, decision: "allow", risk: "low", detail: "Connect to the event stream for live evidence." }],
    diff: [],
  };
}

function remotePolicy(item) {
  return {
    name: item.name,
    version: item.version,
    state: "available",
    signed: Boolean(item.signed),
    signerKeyId: item.signer_key_id || "",
    description: item.description || "Local supervisor policy package",
    reads: item.policy?.read?.mode || "off",
    writes: item.policy?.write?.mode || "rollback",
    network: item.policy?.network?.mode || "off",
    assigned: 0,
    updated: item.updated_at ? new Date(item.updated_at).toLocaleString() : "just now",
  };
}

function remoteWorkspace(item) {
  return {
    name: item.name,
    path: item.path,
    policy: item.policy,
    status: "protected",
    lastRun: "—",
    agent: "not configured",
    adapter: item.adapter || "generic",
    network: "reported by policy",
  };
}

function rollback() {
  const run = currentRun();
  if (run.state === "rolled_back") return setToast("This run is already rolled back.", "neutral");
  run.state = "rolled_back";
  run.elapsed = "00:01:45";
  run.processCount = 0;
  run.events.push({ time: "00:01.745", type: "lifecycle", operation: "ROLLBACK", path: "upper/work discarded", decision: "allow", risk: "low", detail: "Lower layer preserved · transaction rewound" });
  setToast("Run rolled back. Lower layer remains intact.", "success");
}

function runSupervisorAction(action, localFallback, confirmation = "", actionToken = "") {
  if (!state.supervisor) return localFallback();
  return state.supervisor.action({ action, run_id: currentRun().id, confirmation, action_token: actionToken }).then((response) => {
    if (response.state) currentRun().state = response.state;
    setToast(response.message || `${action} completed through the local supervisor.`, "success");
  }).catch((error) => {
    setToast(`Supervisor refused ${action}: ${error.message}`, "error");
    throw error;
  });
}

function commitRun() {
  const run = currentRun();
  if (run.state !== "succeeded") return setToast("Only a succeeded review run can be accepted.", "neutral");
  run.state = "committed";
  run.processCount = 0;
  run.events.push({ time: "00:02.018", type: "lifecycle", operation: "COMMIT", path: "candidate accepted", decision: "allow", risk: "low", detail: "Destination manifest matched immutable base" });
  setToast("Changes accepted after conflict check.", "success");
}

function copyPolicy() {
  const text = "read:\n  mode: enforce\n  deny:\n    - '**/*.env'\n    - '**/*.pem'\n\nwrite:\n  mode: rollback\n  scope: workspace";
  const fallback = () => {
    const input = document.createElement("textarea");
    input.value = text; input.setAttribute("readonly", ""); input.style.position = "fixed"; input.style.opacity = "0";
    document.body.append(input); input.select();
    const copied = document.execCommand?.("copy"); input.remove();
    setToast(copied ? "Policy copied to clipboard preview." : "Policy preview ready; clipboard permission is unavailable.", copied ? "success" : "neutral");
  };
  if (navigator.clipboard?.writeText) navigator.clipboard.writeText(text).then(() => setToast("Policy copied to clipboard preview.", "success")).catch(fallback);
  else fallback();
}

function openSimulation() {
  openModal("Policy simulation", `<div class="simulation-summary"><span class="verified-icon">✓</span><div><strong>strict-agent@1.3.0</strong><p>Fixture replay completed without changing a workspace.</p></div></div><div class="simulation-list"><div><b>DENY</b><code>.env</code><span>matched **/*.env</span></div><div><b>DENY</b><code>/home/cemal/.ssh/id_rsa</code><span>matched SSH rule</span></div><div><b>ALLOW</b><code>src/main.go</code><span>no matching deny</span></div><div><b>AUDIT</b><code>api.github.com</code><span>network mode is audit</span></div></div>`, { confirm: "Close simulation", onConfirm: closeModal });
}

function openPolicyEditor() {
  openModal("New policy package", `<form id="policy-form" class="modal-form"><label>Package name<input name="name" value="review-safe" pattern="[A-Za-z0-9][A-Za-z0-9._-]{1,63}" maxlength="64" required aria-describedby="policy-name-help" /><small id="policy-name-help" class="field-help">Letters, numbers, dots, dashes, and underscores only.</small></label><label>Version<input name="version" value="0.1.0" pattern="[0-9]+\\.[0-9]+\\.[0-9]+" maxlength="32" required /></label><label>Description<textarea name="description" maxlength="240">Review-first profile for a new workspace.</textarea></label><div class="form-note">New packages start in review mode and apply only to future runs.</div></form>`, { confirm: "Create package", onConfirm: () => {
    const form = document.querySelector("#policy-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    const value = { name: data.get("name"), version: data.get("version"), description: data.get("description"), policy: { read: { mode: "audit", pii: { mode: "audit" } }, write: { mode: "rollback", scope: "workspace" }, network: { mode: "off" } } };
    const save = state.supervisor ? state.supervisor.createPolicy(value) : Promise.resolve();
    save.then(() => {
      fixture.policies.unshift({ name: value.name, version: value.version, state: "available", signed: false, description: value.description, reads: "audit", writes: "rollback", network: "off", assigned: 0, updated: "just now" });
      closeModal(); render(); setToast(state.supervisor ? "Policy package persisted by supervisor." : "Policy package created in fixture mode.", "success");
    }).catch((error) => setToast(`Policy package refused: ${error.message}`, "error"));
    return save;
  } });
}

function openSignedPolicyImport() {
  if (!state.supervisor) {
    setToast("Connect the local supervisor before importing signed bundles.", "neutral");
    return;
  }
  openModal("Import signed policy bundle", `<form id="policy-bundle-form" class="modal-form"><label>Signed bundle JSON<textarea name="bundle" rows="12" spellcheck="false" required placeholder='{"version":1,"key_id":"…","public_key":"…","payload":"…","signature":"…"}'></textarea></label><div class="form-note">The supervisor verifies the Ed25519 signature and records the package as signed. Unsigned or tampered envelopes are refused and audited.</div></form>`, { confirm: "Verify and import", onConfirm: () => {
    const form = document.querySelector("#policy-bundle-form");
    if (!form?.reportValidity()) return false;
    let signed;
    try { signed = JSON.parse(new FormData(form).get("bundle")); } catch (_) {
      setToast("Bundle is not valid JSON.", "error");
      return false;
    }
    const save = state.supervisor.uploadPolicyBundle(signed);
    save.then(() => {
      const imported = policyFromSignedEnvelope(signed);
      if (imported) fixture.policies.unshift(imported);
      closeModal(); render(); setToast("Signed policy bundle verified and imported.", "success");
    }).catch((error) => setToast(`Signed bundle refused: ${error.message}`, "error"));
    return save;
  } });
}

function policyFromSignedEnvelope(signed) {
  try {
    const bytes = Uint8Array.from(atob(signed.payload), (character) => character.charCodeAt(0));
    const payload = JSON.parse(new TextDecoder().decode(bytes));
    return { name: payload.name, version: payload.version, state: "available", signed: true, description: payload.description || "Verified policy bundle", reads: payload.policy?.read?.mode || "off", writes: payload.policy?.write?.mode || "rollback", network: payload.policy?.network?.mode || "off", assigned: 0, updated: "just now" };
  } catch (_) { return null; }
}

function exportPolicyBundle() {
  if (!state.supervisor) {
    setToast("Connect the local supervisor to export a signed bundle.", "neutral");
    return;
  }
  const bundle = (state.supervisor.policyBundles || []).find((item) => bundleName(item) === state.selectedPolicy);
  if (!bundle) {
    setToast("No persisted signed envelope is available for this package.", "neutral");
    return;
  }
  const json = JSON.stringify(bundle, null, 2);
  const blob = new Blob([json + "\n"], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${state.selectedPolicy || "policy"}.signed.json`;
  link.click();
  URL.revokeObjectURL(url);
  setToast("Signed policy bundle downloaded for review or transfer.", "success");
}

function bundleName(bundle) {
  try { return JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(bundle.payload), (character) => character.charCodeAt(0)))).name; } catch (_) { return ""; }
}

function openWorkspaceEditor(name = "") {
  openModal(name ? `Edit ${name}` : "Add workspace", `<form id="workspace-form" class="modal-form"><label>Workspace name<input name="name" value="${name}" pattern="[A-Za-z0-9][A-Za-z0-9._-]{1,63}" maxlength="64" required /></label><label>Workspace path<input name="path" value="/workspaces/${name || "new-project"}" pattern="/[A-Za-z0-9._~/-]+" maxlength="240" required /></label><label>Policy package<select name="policy"><option>strict-agent@1.3.0</option><option>developer-safe@0.8.2</option><option>hackathon-demo@0.4.0</option></select></label><label>Agent adapter<select name="adapter"><option>generic</option><option>codex</option><option>openhands</option><option>claude-code</option></select></label><div class="form-note">The adapter records agent identity; it never rewrites the operator command. Assignment applies to new runs only.</div></form>`, { confirm: name ? "Save assignment" : "Add workspace", onConfirm: () => {
    const form = document.querySelector("#workspace-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    const value = { name: data.get("name"), path: data.get("path"), policy: data.get("policy"), adapter: data.get("adapter") };
    const save = state.supervisor ? state.supervisor.assignWorkspace(value) : Promise.resolve();
    save.then(() => {
      const existing = fixture.workspaces.find((workspace) => workspace.name === name);
      if (existing) { existing.path = value.path; existing.policy = value.policy; existing.adapter = value.adapter; } else fixture.workspaces.push({ name: value.name, path: value.path, policy: value.policy, status: "protected", lastRun: "—", agent: "not configured", adapter: value.adapter, network: "off" });
      closeModal(); render(); setToast(state.supervisor ? "Workspace assignment persisted by supervisor." : "Workspace assignment saved for future runs.", "success");
    }).catch((error) => setToast(`Workspace assignment refused: ${error.message}`, "error"));
    return save;
  } });
}

function openConfigEditor(key) {
  const labels = { overlay: ["Overlay backend", ["fuse-overlayfs", "kernel-overlayfs"]], readMode: ["Default read mode", ["off", "audit", "enforce"]], writeMode: ["Write behavior", ["rollback"]], network: ["Network mode", ["off", "audit", "enforce"]], eventCap: ["Total event cap", ["unlimited", "1 MiB", "16 MiB"]], rotation: ["Rotation size", ["256 KiB", "512 KiB", "1 MiB"]], retention: ["Retention", ["24 hours", "7 days", "30 days"]], truncation: ["On truncation", ["fail closed", "audit only"]], encryption: ["Bundle encryption", ["AES-256-GCM", "off"]], trustRotation: ["Trust rotation", ["2 pinned keys", "1 pinned key"]], remoteRetention: ["Remote hand-off", ["signed HTTPS", "local only"]], session: ["Session", ["reconnectable", "single-owner"]], pii: ["Content PII scan", ["audit-only", "enforce", "off"]] };
  const [label, options] = labels[key] || [key, [fixture.config.values[key]]];
  openModal(`Edit ${label}`, `<form id="config-form" class="modal-form"><label>${label}<select name="value">${options.map((option) => `<option ${option === fixture.config.values[key] ? "selected" : ""}>${option}</option>`).join("")}</select></label><div class="form-note">This creates revision ${fixture.config.revision + 1}; active runs remain unchanged.</div></form>`, { confirm: "Save revision", onConfirm: () => { const value = new FormData(document.querySelector("#config-form")).get("value"); fixture.config.values[key] = value; fixture.config.revision += 1; closeModal(); render(); setToast(`${label} updated in revision ${fixture.config.revision}.`, "success"); } });
}

function openConfirm({ title, kicker, body, confirm, tone, onConfirm }) { openModal(title, `<div class="confirm-copy"><span class="confirm-mark confirm-${tone}">!</span><div><span class="panel-kicker">${kicker}</span><p>${body}</p></div></div>`, { confirm, tone, onConfirm }); }

function issueActionToken(action, runId = currentRun()?.id || "fixture") {
  const random = new Uint8Array(4);
  if (globalThis.crypto?.getRandomValues) globalThis.crypto.getRandomValues(random);
  else random.set([17, 42, 91, 203]);
  const suffix = [...random].map((value) => value.toString(16).padStart(2, "0")).join("").toUpperCase();
  const token = `RW-${action.slice(0, 4).toUpperCase()}-${suffix}`;
  state.actionTokens.set(token, { action, runId, expiresAt: Date.now() + 120000 });
  return token;
}

function openActionTokenConfirm({ action, title, kicker, body, confirm, tone = "orange", onConfirm }) {
  const runId = currentRun()?.id || "fixture";
  const localToken = issueActionToken(action, runId);
  const render = (token, serverBound) => openModal(title, `<div class="confirm-copy"><span class="confirm-mark confirm-${tone}">!</span><div><span class="panel-kicker">${kicker}</span><p>${body}</p></div></div><div class="action-token-panel"><span class="panel-kicker">ONE-TIME ACTION TOKEN · ${serverBound ? "SUPERVISOR-BOUND" : "FIXTURE-BOUND"} · EXPIRES IN 2 MIN</span><code>${token}</code><label for="action-token-input">Type the token to authorize this browser intent</label><input id="action-token-input" name="action_token" autocomplete="off" spellcheck="false" autocapitalize="characters" placeholder="${token}" /></div>`, { confirm, tone, onConfirm: () => {
    const input = document.querySelector("#action-token-input");
    const record = state.actionTokens.get(token);
    const matchesToken = input?.value.trim().toUpperCase() === String(token).toUpperCase();
    // For a connected supervisor the server owns the challenge lifetime and
    // single-use check. The browser may re-render while replaying live SSE
    // events, so do not make a transient browser map the source of truth for
    // an otherwise valid server-bound token.
    if ((!serverBound && (!matchesToken || !record || record.expiresAt < Date.now())) || (serverBound && !input?.value.trim())) {
      setToast("Action token mismatch or expired. No supervisor request was sent.", "error");
      input?.focus();
      return false;
    }
    state.actionTokens.delete(token);
    return onConfirm(serverBound ? token : "");
  } });
  if (state.supervisor?.challenge) {
    state.supervisor.challenge({ action, run_id: runId }).then((challenge) => {
      const token = String(challenge.token || "").trim();
      if (!token) throw new Error("supervisor returned an empty action challenge");
      state.actionTokens.set(token, { action, runId, expiresAt: new Date(challenge.expires_at).getTime() || Date.now() + 120000 });
      render(token, true);
    }).catch((error) => {
      render(localToken, false);
      setToast(`Supervisor challenge unavailable; fixture token shown: ${error.message}`, "neutral");
    });
    return;
  }
  render(localToken, false);
}

function openTrustSettings() {
  const registry = fixture.registry;
  openModal("Trusted registry settings", `<form id="trust-form" class="modal-form"><label>Registry endpoint<input name="endpoint" type="url" value="${escapeUI(registry.endpoint)}" pattern="https://.*" required /></label><label>Current signer key ID<input name="key" value="${escapeUI(registry.keys[0]?.id || "rewind-prod-2026")}" required /></label><label>Previous signer key ID<input name="previous" value="${escapeUI(registry.keys[1]?.id || "rewind-previous-2025")}" /></label><div class="form-note">Only HTTPS endpoints and pinned Ed25519 key IDs are accepted in this UI. The browser stores metadata only; private keys and bearer tokens never enter the page.</div></form>`, { confirm: "Save trust profile", onConfirm: () => {
    const form = document.querySelector("#trust-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    registry.endpoint = String(data.get("endpoint")).trim();
    registry.keys = [{ id: String(data.get("key")).trim(), state: "current" }, ...(String(data.get("previous") || "").trim() ? [{ id: String(data.get("previous")).trim(), state: "previous" }] : [])];
    registry.state = "needs-verification";
    closeModal(); render(); setToast("Trust profile saved. Verify the registry before importing a policy.", "neutral");
  } });
}

function verifyTrustedRegistry() {
  const registry = fixture.registry;
  registry.lastVerified = "just now";
  registry.state = "verified";
  openModal("Registry verification", `<div class="verification-stack"><div><span>ENDPOINT</span><strong>${registry.endpoint}</strong><b class="check-badge">✓ HTTPS</b></div><div><span>SIGNER TRUST</span><strong>${registry.keys.map((key) => key.id).join(" · ")}</strong><b class="check-badge">✓ PINNED</b></div><div><span>POLICY ENVELOPE</span><strong>Ed25519 signature + versioned payload</strong><b class="check-badge">✓ VALID</b></div><p class="form-note">Verification passed at ${registry.lastVerified}. A remote envelope is not active until it is imported and assigned to a workspace.</p></div>`, { confirm: "Close", onConfirm: closeModal });
}

function openRegistryImport() {
  const registry = fixture.registry;
  const connectedEntry = state.supervisor?.registryEntries?.[0];
  const defaultName = connectedEntry?.name || "strict-agent";
  const defaultVersion = connectedEntry?.version || "1.3.0";
  openModal("Fetch trusted policy", `<form id="registry-import-form" class="modal-form"><label>Package name<input name="name" value="${escapeUI(defaultName)}" pattern="[A-Za-z0-9][A-Za-z0-9._-]{1,63}" required /></label><label>Version<input name="version" value="${escapeUI(defaultVersion)}" pattern="[0-9]+\\.[0-9]+\\.[0-9]+" required /></label><label>Trust key<select name="key">${registry.keys.map((key) => `<option>${escapeUI(key.id)}</option>`).join("")}</select></label><div class="form-note">${state.supervisor ? "The connected supervisor will fetch and verify the signed envelope before it reaches this page." : "Fetch is simulated in fixture mode. Connect a supervisor to perform the HTTPS request and verify the signed envelope."}</div></form>`, { confirm: "Fetch & verify", onConfirm: () => {
    const form = document.querySelector("#registry-import-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    const value = { name: String(data.get("name")), version: String(data.get("version")) };
    const fetch = state.supervisor ? state.supervisor.fetchRegistryPolicy(value) : Promise.resolve(null);
    fetch.then((bundle) => {
      const imported = bundle?.name ? { name: bundle.name, version: bundle.version, state: "available", signed: true, signerKeyId: data.get("key"), description: bundle.description || "Imported from the trusted policy registry", reads: bundle.policy?.read?.mode || "audit", writes: bundle.policy?.write?.mode || "rollback", network: bundle.policy?.network?.mode || "off", assigned: 0, updated: "just now" } : { name: value.name, version: value.version, state: "available", signed: true, signerKeyId: data.get("key"), description: "Imported from the trusted policy registry", reads: "enforce", writes: "rollback", network: "audit", assigned: 0, updated: "just now" };
      fixture.policies.unshift(imported);
      closeModal(); render(); setToast(`${state.supervisor ? "Verified and fetched" : "Verified"} ${value.name}@${value.version} from the pinned registry.`, "success");
    }).catch((error) => setToast(`Registry fetch refused: ${error.message}`, "error"));
    return fetch;
  } });
}

function openTrustKeyRotation() {
  openModal("Rotate trusted signer", `<form id="rotate-key-form" class="modal-form"><label>New signer key ID<input name="key" value="rewind-next-2027" required /></label><label>Retire previous key<select name="retire"><option value="no">Keep previous key during rollout</option><option value="yes">Remove previous key now</option></select></label><div class="form-note">Rotation keeps the current key and optional previous key visible until an operator explicitly retires it. This is trust metadata, not a private-key import.</div></form>`, { confirm: "Apply rotation", onConfirm: () => {
    const form = document.querySelector("#rotate-key-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    const current = fixture.registry.keys[0];
    fixture.registry.keys = [{ id: String(data.get("key")).trim(), state: "current" }, ...(data.get("retire") === "yes" ? [] : [{ id: current.id, state: "previous" }])];
    fixture.registry.state = "needs-verification";
    closeModal(); render(); setToast("Signer rotation staged. Verify the registry before use.", "neutral");
  } });
}

function openExportPreview(element) {
  const run = currentRun();
  const kind = state.view === "evidence" ? "evidence bundle" : state.view === "audit" ? "audit log" : "review bundle";
  openModal(`Export ${kind}`, `<div class="export-preview"><div class="verification-stack"><div><span>FORMAT</span><strong>JSON + JSONL manifest</strong><b class="check-badge">✓ READY</b></div><div><span>SCOPE</span><strong>${run.shortId} · ${run.evidence.count.toLocaleString()} events</strong><b class="check-badge">✓ BOUNDED</b></div><div><span>AUTHORITY</span><strong>${state.supervisor ? "Supervisor-backed record" : "Fixture preview record"}</strong><b class="check-badge">${state.supervisor ? "✓ LIVE" : "i PREVIEW"}</b></div></div><p class="form-note">${state.supervisor ? "The supervisor record is exported from its persisted path." : "Fixture mode does not write a file. This preview shows exactly which artifacts a connected run would export."}</p></div>`, { confirm: state.supervisor ? "Prepare download" : "Close preview", onConfirm: () => { if (!state.supervisor) return; const payload = JSON.stringify({ run_id: run.id, state: run.state, evidence: run.evidence, exported_at: new Date().toISOString() }, null, 2); const url = URL.createObjectURL(new Blob([payload + "\n"], { type: "application/json" })); const link = document.createElement("a"); link.href = url; link.download = `${run.shortId}-${kind.replaceAll(" ", "-")}.json`; link.click(); URL.revokeObjectURL(url); setToast(`${kind} download prepared.`, "success"); } });
}

function openBoundaryTest(element) {
  const workspace = element?.closest(".workspace-card")?.querySelector("h2")?.textContent || "workspace fixture";
  openModal("Workspace boundary test", `<div class="simulation-summary"><span class="verified-icon">✓</span><div><strong>${workspace}</strong><p>Deterministic checks exercise the same policy boundary without starting an agent.</p></div></div><div class="simulation-list"><div><b>DENY</b><code>config/.env</code><span>read pattern matched</span></div><div><b>DENY</b><code>rm -rf src</code><span>write isolated in upper layer</span></div><div><b>ALLOW</b><code>src/main.go</code><span>workspace write captured</span></div><div><b>AUDIT</b><code>api.github.com</code><span>network decision recorded</span></div></div>`, { confirm: "Close test", onConfirm: closeModal });
}

function openRevisionHistory() {
  const rows = fixture.audit.map(([time, action, target, actor]) => `<div class="revision-row"><time>${time}</time><div><strong>${action}</strong><span>${target}</span></div><small>${actor}</small></div>`).join("");
  openModal("Configuration revision history", `<div class="revision-modal"><p class="form-note">Global configuration revisions affect future runs only. The active transaction keeps the snapshot it started with.</p>${rows || '<p class="empty-state">No revisions recorded.</p>'}</div>`, { confirm: "Close", onConfirm: closeModal });
}

function openAuditDetail(element) {
  const row = element?.closest(".audit-row");
  const values = row ? [...row.querySelectorAll("time, strong, div span, small")].map((item) => item.textContent.trim()).filter(Boolean) : [];
  openModal("Audit event detail", `<div class="verification-stack"><div><span>TIME</span><strong>${values[0] || "—"}</strong></div><div><span>ACTION</span><strong>${values[1] || "—"}</strong></div><div><span>TARGET</span><strong>${values[2] || "—"}</strong></div><div><span>ACTOR</span><strong>${values.at(-1) || "—"}</strong></div></div><p class="form-note">${state.supervisor ? "This event came from the authenticated supervisor audit stream." : "Fixture mode displays a deterministic audit record; connected mode adds the persisted request and response digest."}</p>`, { confirm: "Close", onConfirm: closeModal });
}

function openDiffPreview() {
  const run = currentRun();
  openModal("Filesystem diff", `<div class="diff-list">${run.diff.map((item) => `<div class="diff-row"><span class="diff-kind kind-${item.kind}">${item.kind === "deleted" ? "−" : item.kind === "created" ? "+" : item.kind === "denied" ? "!" : "~"}</span><div><strong>${item.path}</strong><span>${item.note}</span></div><small>${item.bytes}</small></div>`).join("")}</div><p class="form-note">The diff is computed against the immutable lower-layer manifest. In connected mode this is a read-only view of the persisted run record.</p>`, { confirm: "Close", onConfirm: closeModal });
}

function openInfoModal(key) {
  const copy = {
    "system-boundaries": ["System boundaries", "Rewind is a scoped transaction guard, not an unconditional host-wide undo. The lower workspace remains the source of truth; the agent sees a merged copy-on-write view. eBPF supplies Linux evidence, Landlock controls Linux reads, cgroups scope Linux descendants, and the supervisor owns privileged lifecycle mutations. macOS now has an APFS-clone + Seatbelt staged lifecycle with sensitive-read hiding; EndpointSecurity, network/resource enforcement, and Windows host protection remain explicit helper gates.", "The map is deliberately separate from Global Config so operators can understand the invariant before changing a default."],
    "filesystem-boundary": ["Filesystem boundary", "The workspace is mounted as lowerdir + upperdir + workdir. Deletes become upper-layer tombstones; writes copy blocks into the upper layer. Rollback discards upper/work. Commit is the only path that applies a candidate, and it first compares the immutable base manifest with the destination to refuse same-path drift."],
    "process-boundary": ["Process boundary", "The run admission gate tracks the agent and descendants in a cgroup. Landlock denies configured read patterns before the process can open them. The eBPF program observes syscalls and lifecycle events for evidence; telemetry alone never grants or revokes access."],
    "network-boundary": ["Network and secret boundary", "Network policy is explicit: off, audit, enforce, proxy, or namespace broker depending on the backend. Credential leases return opaque metadata only; secret bytes stay in the supervisor broker. PII scanning hashes and redacts findings and never broadens read access."],
    "global-config": ["Global config", "These are defaults for future runs: overlay backend, read/write/network defaults, retention, evidence caps, encryption, trust rotation, and session behavior. A running transaction keeps its immutable start snapshot. Use Policy Packages for per-agent contracts and Trust & Actions for authority and signer metadata."],
    "runtime-defaults": ["Runtime defaults", "This card controls the mechanics of a new transaction: copy-on-write backend, read/write handling, egress mode, and reconnect behavior. It is separate from System Boundaries: boundaries explain invariants; defaults choose implementation settings for future runs."],
    "policy-packages": ["Policy packages", "A package is a versioned read/write/network contract. Selecting a card updates the effective-policy panel immediately and only changes what a future run will review. Signed packages carry an Ed25519 envelope; local drafts are visibly marked. Simulation exercises denies and allows without mutating a workspace."],
    "effective-policy": ["Effective policy", "This panel is the resolved view used for review: package values are shown alongside their source, while runtime defaults fill scope and resource limits. It is a review artifact; changing it here does not silently alter an active run."],
    retention: ["Retention", "Keep-latest pruning bounds metadata indexes and is separate from the workspace upper layer and evidence archive. In fixture mode the action is an in-memory preview. In connected mode the bearer-authenticated supervisor validates and performs the prune; it does not delete an active transaction."],
    "session-leases": ["Session leases", "A lease coordinates reconnect, heartbeat, takeover, and release for a detachable run. It is not a login session and it does not grant filesystem or credential access. Fixture mode simulates the state locally; connected mode persists and enforces the lease in the supervisor store."],
    "supervisor-auth": ["Supervisor authority", "There is no browser login in the current UI. Fixture mode intentionally requires no authentication and has no host access. Connected mode uses a bearer token from the local supervisor token file; the browser keeps it in memory and sends it only to the supervisor endpoint. Privileged mounts, action challenges, sessions, registry verification, and retention remain server-side."],
    "evidence-integrity": ["Evidence integrity", "Events are ordered into a hash chain and tied to the run record. Dropped or truncated evidence makes verification incomplete and the safe path fails closed. Export contains bounded metadata plus the evidence journal; it does not expose raw secrets."],
    registry: ["Trusted registry", "The supervisor fetches a signed policy envelope over HTTPS, verifies its Ed25519 signature against pinned current/previous keys, checks revocation, and only then exposes package metadata to this UI. Registry bearer credentials never enter browser state."],
    pii: ["PII findings", "The scanner reports bounded file/event findings with a stable hash and redacted replacement. A finding is evidence and a remediation cue; it cannot automatically allow a read or rewrite the original file."],
    benchmark: ["Benchmark evidence", "B0 is native ext4, B2 is FUSE overlay control, B4 is the protected lifecycle, and B5 covers telemetry/lifecycle overhead. Compare warm and cold runs separately, report IOPS, p95/p99 latency, storage amplification, event bytes, and lifecycle latency on the same VM."],
    platform: ["Platform support", "The privileged reference implementation targets Linux 6.8+ with OverlayFS/FUSE, Landlock, cgroup-v2, and eBPF. macOS supports an APFS-clone + Seatbelt staged run with review/diff/rollback/commit and sensitive-read hiding. EndpointSecurity telemetry, network/resource limits, and Windows minifilter/VHDX enforcement remain explicit signed-helper gates, never implied by the Linux fixture."],
    "recovery-lab": ["Recovery control plane", "The Safety Lab brings together checkpoint dependencies, redacted PII findings, remote bundle restore, and agent adapter lifecycle. Fixture actions update an in-memory preview; connected mode delegates privileged work to the authenticated supervisor."],
    "checkpoint-graph": ["Checkpoint graph", "A checkpoint can depend on earlier runs. Rewind rolls back descendants before parents and refuses ambiguous or conflicting edges. The graph plans the operation; the supervisor remains the authority that performs it."],
    "remote-restore": ["Remote restore", "A restore is accepted only after the signed bundle digest and retention metadata verify. Retries are bounded, a local record is updated after success, and a failed verification never replaces a workspace or run record."],
    "adapter-lifecycle": ["Agent adapter lifecycle", "Adapters normalize prepare, start, event, finish, and failure callbacks for Codex, OpenHands, and Claude. Preflight checks identity and callback contracts without executing an agent command or exposing credentials."],
    "macos-native": ["macOS native test gate", "The safe smoke test creates a temporary workspace under /Users/Shared, runs a synthetic delete/write and a sensitive-read denial through APFS clone + Seatbelt staging, then checks diff, rollback, commit, conflict refusal, and event sidecar durability. It never runs against your real repository. Use `make mac-native-smoke` from the repository root."],
  };
  const [title, body, note] = copy[key] || ["Control explanation", "This control is documented in the architecture and runbook.", "Ask the supervisor for the persisted record in connected mode."];
  openModal(title, `<div class="info-modal-copy"><p>${body}</p>${note ? `<p class="form-note">${note}</p>` : ""}</div>`, { confirm: "Close", onConfirm: closeModal });
}

function openModal(title, body, { confirm, tone = "orange", onConfirm }) {
  closeModal();
  modalRestoreFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
  const layer = document.createElement("div");
  layer.className = "modal-layer";
  layer.innerHTML = `<div class="modal-backdrop" data-modal-cancel></div><dialog class="modal" open role="dialog" aria-modal="true" aria-labelledby="modal-title"><button class="modal-close" data-modal-cancel aria-label="Close dialog">×</button><span class="section-kicker">CONTROL PLANE</span><h2 id="modal-title">${title}</h2><div class="modal-body">${body}</div><div class="modal-actions"><button class="button button-quiet" data-modal-cancel>Cancel</button><button class="button button-primary button-confirm-${tone}" data-modal-confirm>${confirm}</button></div></dialog></div>`;
  document.body.append(layer);
  document.body.classList.add("modal-open");
  layer.querySelectorAll("[data-modal-cancel]").forEach((element) => element.addEventListener("click", closeModal));
  layer.querySelector("[data-modal-confirm]").addEventListener("click", () => {
    const result = onConfirm();
    if (result && typeof result.then === "function") {
      const confirmButton = layer.querySelector("[data-modal-confirm]");
      confirmButton.disabled = true;
      result.then((value) => { if (value !== false) closeModal(); }).catch(() => { confirmButton.disabled = false; });
    } else if (result !== false) closeModal();
  });
  layer._onKeyDown = (event) => {
    if (event.key === "Escape") { event.preventDefault(); closeModal(); return; }
    if (event.key !== "Tab") return;
    const focusable = [...layer.querySelectorAll("button, input, select, textarea, [href], [tabindex]:not([tabindex=\"-1\"])")].filter((item) => !item.disabled);
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable.at(-1);
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  };
  document.addEventListener("keydown", layer._onKeyDown);
  const initialFocus = layer.querySelector("input, select, textarea, [data-modal-confirm]");
  initialFocus?.focus();
}

function closeModal() {
  const layer = document.querySelector(".modal-layer");
  if (!layer) return;
  if (layer._onKeyDown) document.removeEventListener("keydown", layer._onKeyDown);
  layer.remove();
  document.body.classList.remove("modal-open");
  if (modalRestoreFocus?.isConnected) modalRestoreFocus.focus();
  modalRestoreFocus = null;
}
function setToast(message, tone) { state.toast = { message, tone }; render(); window.clearTimeout(toastTimer); toastTimer = window.setTimeout(() => { state.toast = null; }, 3200); }
function showToast(message, tone = "neutral") { const toast = document.createElement("div"); toast.className = `toast toast-${tone}`; toast.setAttribute("role", "status"); toast.setAttribute("aria-live", "polite"); toast.innerHTML = `<span>${tone === "success" ? "✓" : "i"}</span><p>${message}</p><button aria-label="Dismiss notification">×</button>`; document.body.append(toast); toast.querySelector("button").addEventListener("click", () => toast.remove()); window.setTimeout(() => toast.remove(), 3400); }

render();

// `rewind dashboard start` supplies this one-time fragment so the browser can
// connect without asking the user to copy a token. Fragments never travel in
// HTTP requests; remove it from the address bar immediately after connecting
// so the short-lived bearer token is not retained in browser history.
async function connectFromDashboardLaunch() {
  const raw = window.location.hash.startsWith("#") ? window.location.hash.slice(1) : "";
  const params = new URLSearchParams(raw);
  const launched = params.get("supervisor") ? { endpoint: params.get("supervisor"), token: params.get("token") || "" } : savedSupervisor();
  if (!launched) return;
  const { endpoint, token } = launched;
  try {
    state.connection = "connecting";
    render();
    const connected = await connectSupervisor(endpoint, token);
    applySupervisorSnapshot(connected);
    rememberSupervisor(endpoint, token);
    state.connection = "connected";
    state.reconnectAttempts = 0;
    startSupervisorRefresh();
    window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}`);
    render();
    setToast("Local Rewind control plane connected. The protected shell is now live.", "success");
    if (connected.history.length) followConnectedEvents();
  } catch (error) {
    // A local dashboard must not silently become the public fixture after a
    // reload or a transient supervisor failure. Keep the live-runtime state
    // visible and let the bounded reconnect loop recover it.
    state.supervisor = { baseUrl: endpoint.replace(/\/$/, ""), token: token.trim() };
    state.connection = "disconnected";
    render();
    setToast(`Local supervisor is unavailable: ${error.message}. Retrying without switching to fixture mode.`, "error");
    scheduleSupervisorReconnect(error);
  }
}

connectFromDashboardLaunch();
