// The browser adapter is intentionally unprivileged. It talks to a local
// authenticated supervisor HTTP endpoint; privileged actions and config writes
// remain supervisor decisions and never run in browser code.
export async function connectSupervisor(baseUrl, token = "") {
  const root = baseUrl.replace(/\/$/, "");
  const headers = { Accept: "application/json" };
  if (token.trim()) headers.Authorization = `Bearer ${token.trim()}`;
  const [health, capabilities, history, audit, policies, workspaces] = await Promise.all([
    fetch(`${root}/health`, { headers }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/capabilities`, { headers }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/history`, { headers }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/audit?limit=100`, { headers }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/policies`, { headers }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/workspaces`, { headers }).then(assertResponse).then((response) => response.json()),
  ]);
  return {
    health,
    capabilities,
    history: Array.isArray(history) ? history : [],
    audit: Array.isArray(audit) ? audit : [],
    policies: Array.isArray(policies) ? policies : [],
    workspaces: Array.isArray(workspaces) ? workspaces : [],
    token: token.trim(),
    baseUrl: root,
    action: (request) => executeAction(root, token.trim(), request),
    createPolicy: (value) => createResource(root, token.trim(), "policies", value),
    assignWorkspace: (value) => createResource(root, token.trim(), "workspaces", value),
  };
}

async function createResource(baseUrl, token, resource, value) {
  const response = await fetch(`${baseUrl.replace(/\/$/, "")}/v1/${resource}`, {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json", Authorization: `Bearer ${token.trim()}` },
    body: JSON.stringify(value),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.message || `supervisor returned HTTP ${response.status}`);
  return payload;
}

export async function executeAction(baseUrl, token, request) {
  const root = baseUrl.replace(/\/$/, "");
  const response = await fetch(`${root}/v1/actions`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      Authorization: `Bearer ${token.trim()}`,
    },
    body: JSON.stringify(request),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(payload.message || `supervisor returned HTTP ${response.status}`);
  return payload;
}

// Follow the authenticated SSE stream without EventSource because browser
// EventSource cannot attach the supervisor bearer header. The caller owns the
// AbortSignal and can reconnect after a supervisor restart.
export async function followEvents(baseUrl, token, runId, onEvent, signal) {
  const root = baseUrl.replace(/\/$/, "");
  const response = await fetch(`${root}/v1/events?run_id=${encodeURIComponent(runId)}&follow=true`, {
    headers: { Accept: "text/event-stream", Authorization: `Bearer ${token.trim()}` },
    signal,
  });
  if (!response.ok) throw new Error(`supervisor returned HTTP ${response.status}`);
  if (!response.body) throw new Error("supervisor event stream is unavailable");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done });
    const blocks = buffer.split("\n\n");
    buffer = blocks.pop() || "";
    for (const block of blocks) {
      const line = block.split("\n").find((item) => item.startsWith("data: "));
      if (!line) continue;
      try { onEvent(JSON.parse(line.slice(6))); } catch (_) { /* ignore malformed server frames */ }
    }
    if (done) break;
  }
}

async function assertResponse(response) {
  if (!response.ok) throw new Error(`supervisor returned HTTP ${response.status}`);
  return response;
}
