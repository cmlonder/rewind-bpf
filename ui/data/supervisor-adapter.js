// The browser adapter is intentionally unprivileged. It talks to a local
// read-only supervisor HTTP endpoint; privileged actions remain supervisor
// decisions and never run in browser code.
export async function connectSupervisor(baseUrl, token = "") {
  const root = baseUrl.replace(/\/$/, "");
  const headers = { Accept: "application/json" };
  if (token.trim()) headers.Authorization = `Bearer ${token.trim()}`;
  const [health, capabilities, history, audit] = await Promise.all([
    fetch(`${root}/health`, { headers }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/capabilities`, { headers }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/history`, { headers }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/audit?limit=100`, { headers }).then(assertResponse).then((response) => response.json()),
  ]);
  return {
    health,
    capabilities,
    history: Array.isArray(history) ? history : [],
    audit: Array.isArray(audit) ? audit : [],
    token: token.trim(),
    action: (request) => executeAction(root, token.trim(), request),
  };
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

async function assertResponse(response) {
  if (!response.ok) throw new Error(`supervisor returned HTTP ${response.status}`);
  return response;
}
