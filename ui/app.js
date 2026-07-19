import { fixture, getRun } from "./data/fixture.js";
import { AppShell } from "./components/layout.js";

const app = document.querySelector("#app");
const state = { view: "overview", selectedRun: fixture.runs[0].id, toast: null };

function currentRun() { return getRun(state.selectedRun); }

function render() {
  app.innerHTML = AppShell({ view: state.view, run: currentRun(), fixture });
  bindInteractions();
  if (state.toast) showToast(state.toast.message, state.toast.tone);
}

function bindInteractions() {
  document.querySelectorAll("[data-view]").forEach((element) => element.addEventListener("click", () => { state.view = element.dataset.view; render(); }));
  document.querySelectorAll("[data-run-id]").forEach((element) => element.addEventListener("click", () => { state.selectedRun = element.dataset.runId; state.view = "run-detail"; render(); }));
  document.querySelectorAll("[data-action]").forEach((element) => element.addEventListener("click", () => handleAction(element.dataset.action)));
}

function handleAction(action) {
  if (action === "rollback") {
    const run = currentRun();
    if (run.state === "rolled_back") return setToast("This run is already rolled back.", "neutral");
    run.state = "rolled_back";
    run.elapsed = "00:01:45";
    run.processCount = 0;
    run.events.push({ time: "00:01.745", type: "lifecycle", operation: "ROLLBACK", path: "upper/work discarded", decision: "allow", risk: "low", detail: "Lower layer preserved · transaction rewound" });
    return setToast("Run rolled back. Lower layer remains intact.", "success");
  }
  if (action === "recover") return setToast("Recovery is only available for stale runs.", "neutral");
  if (action === "export") return setToast("Review bundle prepared in fixture mode.", "success");
  if (action === "copy-policy") return setToast("Policy copied to clipboard preview.", "success");
  if (action === "simulate-policy") return setToast("Simulation: 2 denied · 3 allowed · 1 audited.", "success");
  if (action === "new-policy") return setToast("Package editor is the next control-plane slice.", "neutral");
}

function setToast(message, tone) { state.toast = { message, tone }; render(); window.setTimeout(() => { state.toast = null; }, 3200); }

function showToast(message, tone = "neutral") {
  const toast = document.createElement("div");
  toast.className = `toast toast-${tone}`;
  toast.setAttribute("role", "status");
  toast.innerHTML = `<span>${tone === "success" ? "✓" : "i"}</span><p>${message}</p><button aria-label="Dismiss notification">×</button>`;
  document.body.append(toast);
  toast.querySelector("button").addEventListener("click", () => toast.remove());
  window.setTimeout(() => toast.remove(), 3400);
}

render();
