import { fixture, getRun } from "./data/fixture.js";
import { connectSupervisor, followEvents } from "./data/supervisor-adapter.js";
import { AppShell } from "./components/layout.js";

const app = document.querySelector("#app");
const state = { view: "overview", selectedRun: fixture.runs[0].id, runFilter: "all", toast: null, supervisor: null, eventAbort: null };
let modalRestoreFocus = null;
let toastTimer = null;

function currentRun() { return getRun(state.selectedRun); }

function render() {
  app.innerHTML = AppShell({ view: state.view, run: currentRun(), fixture });
  bindInteractions();
  if (state.toast) showToast(state.toast.message, state.toast.tone);
}

function bindInteractions() {
  document.querySelectorAll("[data-view]").forEach((element) => element.addEventListener("click", () => { state.view = element.dataset.view; render(); }));
  document.querySelectorAll("[data-run-id]").forEach((element) => element.addEventListener("click", () => { state.selectedRun = element.dataset.runId; state.view = "run-detail"; render(); }));
  document.querySelectorAll("[data-action]").forEach((element) => element.addEventListener("click", () => handleAction(element.dataset.action, element)));
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
  document.querySelectorAll("[data-policy]").forEach((card) => card.classList.toggle("is-selected", card.dataset.policy === name));
  const policy = fixture.policies.find((item) => item.name === name);
  if (policy) setToast(`${name}@${policy.version} selected for review.`, "neutral");
}

function handleAction(action, element) {
  if (action === "notifications") return openModal("Notifications", `<div class="notification-list"><div><span class="notification-dot"></span><div><strong>Evidence stream healthy</strong><p>Run <code>${currentRun().shortId}</code> has no dropped or truncated events.</p><small>just now · system</small></div></div><div><span class="notification-dot notification-dot-muted"></span><div><strong>Fixture adapter active</strong><p>Actions are simulated in memory until the supervisor API is connected.</p><small>today · control plane</small></div></div></div>`, { confirm: "Done", onConfirm: closeModal });
  if (action === "connect-supervisor") return openSupervisorConnector();
  if (action === "hold-review") { currentRun().state = "succeeded"; return setToast("Run held for review. Conflict-checked acceptance is now available.", "neutral"); }
  if (action === "simulate-credentials") return setToast("Boundary check passed: broker unavailable, raw secret exposure refused.", "neutral");
  if (action === "retention") return setToast("Retention is fixture-backed in this demo; the P4 index prunes by keep-latest policy.", "neutral");
  if (action === "rollback") return openConfirm({ title: "Rollback this run?", kicker: "DESTRUCTIVE TO UPPER LAYER", body: "This discards the temporary upper/work layer while preserving the original lower layer and evidence record.", confirm: "Rollback run", tone: "orange", onConfirm: () => runSupervisorAction("rollback", rollback) });
  if (action === "commit") return openConfirm({ title: "Accept reviewed changes?", kicker: "CONFLICT-CHECKED APPLY", body: "Rewind will compare the immutable base with the current destination first. Same-path drift refuses the apply; only the reviewed candidate is written.", confirm: "Accept changes", tone: "sage", onConfirm: () => runSupervisorAction("commit", commitRun, "COMMIT") });
  if (action === "recover") return openConfirm({ title: "Recover stale run?", kicker: "PROCESS DRAIN", body: "The supervisor will drain descendants, remove the temporary mount, and preserve the lower workspace.", confirm: "Recover run", tone: "orange", onConfirm: () => runSupervisorAction("recover", () => setToast("Recovery completed in fixture mode.", "success")) });
  if (action === "export") return setToast("Review bundle prepared in fixture mode.", "success");
  if (action === "copy-policy") return copyPolicy();
  if (action === "simulate-policy") return openSimulation();
  if (action === "new-policy") return openPolicyEditor();
  if (action === "import-policy") return openSignedPolicyImport();
  if (action === "new-workspace") return openWorkspaceEditor();
  if (action === "edit-workspace") return openWorkspaceEditor(element.closest(".workspace-card")?.querySelector("h2")?.textContent);
  if (action === "simulate-workspace") return setToast("Boundary test: 2 denied · 3 allowed · 1 audited.", "success");
  if (action === "view-revisions") return setToast("Revision history is already visible in this fixture.", "neutral");
  if (action === "inspect-audit") return setToast("Audit event details are available after supervisor integration.", "neutral");
  if (action === "config-change") return openConfigEditor(element.dataset.configKey);
}

function openSupervisorConnector() {
  openModal("Connect local supervisor", `<form id="supervisor-form" class="modal-form"><label>Read-only endpoint<input name="endpoint" value="http://127.0.0.1:8787" inputmode="url" pattern="https?://[^ ]+" required /></label><label>Bearer token<input name="token" type="password" autocomplete="off" placeholder="from supervisor token file" /></label><div class="form-note">The browser requests health, capability, and history only. It never receives root access or raw credentials.</div></form>`, { confirm: "Connect", onConfirm: async () => {
    const form = document.querySelector("#supervisor-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    const endpoint = data.get("endpoint");
    const token = data.get("token");
    try {
      const connected = await connectSupervisor(endpoint, token);
      state.supervisor = connected;
      fixture.environment = `Connected supervisor · ${connected.capabilities.platform || "unknown"}`;
      fixture.history = connected.history.map((item) => ({ id: item.run_id, state: item.state, workspace: item.workspace || "unknown", updated: item.updated_at || "just now", size: `${item.upper_bytes || 0} bytes upper` }));
      if (connected.history.length) {
        fixture.runs = connected.history.map(remoteRun);
        state.selectedRun = fixture.runs[0].id;
      }
      if (connected.policies.length) fixture.policies = connected.policies.map(remotePolicy);
      if (connected.workspaces.length) fixture.workspaces = connected.workspaces.map(remoteWorkspace);
      if (connected.audit.length) fixture.audit = connected.audit.map((item) => [new Date(item.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }), item.action, item.run_id || "supervisor", item.ok ? "supervisor" : "refused"]);
      followConnectedEvents();
      closeModal(); render(); setToast("Supervisor connected: live evidence and authenticated actions enabled.", "success");
    } catch (error) { setToast(`Supervisor connection refused: ${error.message}`, "error"); }
  } });
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
    render();
  }, controller.signal).catch((error) => {
    if (error.name !== "AbortError") setToast(`Event stream disconnected: ${error.message}`, "error");
  });
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

function runSupervisorAction(action, localFallback, confirmation = "") {
  if (!state.supervisor) return localFallback();
  return state.supervisor.action({ action, run_id: currentRun().id, confirmation }).then((response) => {
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
    const value = { name: data.get("name"), version: data.get("version"), description: data.get("description"), policy: { read: { mode: "audit" }, write: { mode: "rollback", scope: "workspace" }, network: { mode: "off" } } };
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

function openWorkspaceEditor(name = "") {
  openModal(name ? `Edit ${name}` : "Add workspace", `<form id="workspace-form" class="modal-form"><label>Workspace name<input name="name" value="${name}" pattern="[A-Za-z0-9][A-Za-z0-9._-]{1,63}" maxlength="64" required /></label><label>Workspace path<input name="path" value="/workspaces/${name || "new-project"}" pattern="/[A-Za-z0-9._~/-]+" maxlength="240" required /></label><label>Policy package<select name="policy"><option>strict-agent@1.3.0</option><option>developer-safe@0.8.2</option><option>hackathon-demo@0.4.0</option></select></label><div class="form-note">Assignment is stored as a revision and used by new runs only.</div></form>`, { confirm: name ? "Save assignment" : "Add workspace", onConfirm: () => {
    const form = document.querySelector("#workspace-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    const value = { name: data.get("name"), path: data.get("path"), policy: data.get("policy") };
    const save = state.supervisor ? state.supervisor.assignWorkspace(value) : Promise.resolve();
    save.then(() => {
      const existing = fixture.workspaces.find((workspace) => workspace.name === name);
      if (existing) { existing.path = value.path; existing.policy = value.policy; } else fixture.workspaces.push({ name: value.name, path: value.path, policy: value.policy, status: "protected", lastRun: "—", agent: "not configured", network: "off" });
      closeModal(); render(); setToast(state.supervisor ? "Workspace assignment persisted by supervisor." : "Workspace assignment saved for future runs.", "success");
    }).catch((error) => setToast(`Workspace assignment refused: ${error.message}`, "error"));
    return save;
  } });
}

function openConfigEditor(key) {
  const labels = { overlay: ["Overlay backend", ["fuse-overlayfs", "kernel-overlayfs"]], readMode: ["Default read mode", ["off", "audit", "enforce"]], writeMode: ["Write behavior", ["rollback"]], network: ["Network mode", ["off", "audit"]], eventCap: ["Total event cap", ["unlimited", "1 MiB", "16 MiB"]], rotation: ["Rotation size", ["256 KiB", "512 KiB", "1 MiB"]], retention: ["Retention", ["24 hours", "7 days", "30 days"]], truncation: ["On truncation", ["fail closed", "audit only"]] };
  const [label, options] = labels[key] || [key, [fixture.config.values[key]]];
  openModal(`Edit ${label}`, `<form id="config-form" class="modal-form"><label>${label}<select name="value">${options.map((option) => `<option ${option === fixture.config.values[key] ? "selected" : ""}>${option}</option>`).join("")}</select></label><div class="form-note">This creates revision ${fixture.config.revision + 1}; active runs remain unchanged.</div></form>`, { confirm: "Save revision", onConfirm: () => { const value = new FormData(document.querySelector("#config-form")).get("value"); fixture.config.values[key] = value; fixture.config.revision += 1; closeModal(); render(); setToast(`${label} updated in revision ${fixture.config.revision}.`, "success"); } });
}

function openConfirm({ title, kicker, body, confirm, tone, onConfirm }) { openModal(title, `<div class="confirm-copy"><span class="confirm-mark confirm-${tone}">!</span><div><span class="panel-kicker">${kicker}</span><p>${body}</p></div></div>`, { confirm, tone, onConfirm }); }

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
  layer.querySelector("[data-modal-confirm]").focus();
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
