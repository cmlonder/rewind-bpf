import { fixture, getRun } from "./data/fixture.js";
import { AppShell } from "./components/layout.js";

const app = document.querySelector("#app");
const state = { view: "overview", selectedRun: fixture.runs[0].id, runFilter: "all", toast: null };
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
  if (action === "hold-review") return setToast("Review hold selected. The real CLI equivalent is --on-success review.", "neutral");
  if (action === "rollback") return openConfirm({ title: "Rollback this run?", kicker: "DESTRUCTIVE TO UPPER LAYER", body: "This discards the temporary upper/work layer while preserving the original lower layer and evidence record.", confirm: "Rollback run", tone: "orange", onConfirm: rollback });
  if (action === "recover") return openConfirm({ title: "Recover stale run?", kicker: "PROCESS DRAIN", body: "The supervisor will drain descendants, remove the temporary mount, and preserve the lower workspace.", confirm: "Recover run", tone: "orange", onConfirm: () => setToast("Recovery completed in fixture mode.", "success") });
  if (action === "export") return setToast("Review bundle prepared in fixture mode.", "success");
  if (action === "copy-policy") return copyPolicy();
  if (action === "simulate-policy") return openSimulation();
  if (action === "new-policy") return openPolicyEditor();
  if (action === "new-workspace") return openWorkspaceEditor();
  if (action === "edit-workspace") return openWorkspaceEditor(element.closest(".workspace-card")?.querySelector("h2")?.textContent);
  if (action === "simulate-workspace") return setToast("Boundary test: 2 denied · 3 allowed · 1 audited.", "success");
  if (action === "view-revisions") return setToast("Revision history is already visible in this fixture.", "neutral");
  if (action === "inspect-audit") return setToast("Audit event details are available after supervisor integration.", "neutral");
  if (action === "config-change") return openConfigEditor(element.dataset.configKey);
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
    fixture.policies.unshift({ name: data.get("name"), version: data.get("version"), state: "available", description: data.get("description"), reads: "audit", writes: "rollback", network: "off", assigned: 0, updated: "just now" });
    closeModal(); render(); setToast("Policy package created in fixture mode.", "success");
  } });
}

function openWorkspaceEditor(name = "") {
  openModal(name ? `Edit ${name}` : "Add workspace", `<form id="workspace-form" class="modal-form"><label>Workspace name<input name="name" value="${name}" pattern="[A-Za-z0-9][A-Za-z0-9._-]{1,63}" maxlength="64" required /></label><label>Workspace path<input name="path" value="/workspaces/${name || "new-project"}" pattern="/[A-Za-z0-9._~/-]+" maxlength="240" required /></label><label>Policy package<select name="policy"><option>strict-agent@1.3.0</option><option>developer-safe@0.8.2</option><option>hackathon-demo@0.4.0</option></select></label><div class="form-note">Assignment is stored as a revision and used by new runs only.</div></form>`, { confirm: name ? "Save assignment" : "Add workspace", onConfirm: () => {
    const form = document.querySelector("#workspace-form");
    if (!form?.reportValidity()) return false;
    const data = new FormData(form);
    const existing = fixture.workspaces.find((workspace) => workspace.name === name);
    if (existing) { existing.path = data.get("path"); existing.policy = data.get("policy"); } else fixture.workspaces.push({ name: data.get("name"), path: data.get("path"), policy: data.get("policy"), status: "protected", lastRun: "—", agent: "not configured", network: "off" });
    closeModal(); render(); setToast("Workspace assignment saved for future runs.", "success");
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
  layer.querySelector("[data-modal-confirm]").addEventListener("click", () => onConfirm() !== false && closeModal());
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
