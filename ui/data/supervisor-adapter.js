// The browser adapter is intentionally unprivileged. It talks to a local
// read-only supervisor HTTP endpoint; privileged actions remain supervisor
// decisions and never run in browser code.
export async function connectSupervisor(baseUrl) {
  const root = baseUrl.replace(/\/$/, "");
  const [health, capabilities, history] = await Promise.all([
    fetch(`${root}/health`, { headers: { Accept: "application/json" } }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/capabilities`, { headers: { Accept: "application/json" } }).then(assertResponse).then((response) => response.json()),
    fetch(`${root}/v1/history`, { headers: { Accept: "application/json" } }).then(assertResponse).then((response) => response.json()),
  ]);
  return { health, capabilities, history: Array.isArray(history) ? history : [] };
}

async function assertResponse(response) {
  if (!response.ok) throw new Error(`supervisor returned HTTP ${response.status}`);
  return response;
}
