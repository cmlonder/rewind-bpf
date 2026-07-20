export function releaseProof(fixture) {
  const metrics = fixture.metrics || {};
  return `<div class="release-proof panel"><div><span class="panel-kicker">RELEASE PROOF</span><h2>VM gate and evidence bundle are verified.</h2><p>The public benchmark ledger and this control plane use the same ARM64 Ubuntu acceptance record.</p></div><div class="release-proof-grid"><div><strong>${metrics.vmAcceptance || "—"}</strong><span>VM acceptance</span></div><div><strong>${metrics.releaseBundle || "—"}</strong><span>bundle integrity</span></div><div><strong>${metrics.storageAmplification || "—"}</strong><span>storage amplification</span></div></div></div>`;
}

export function benchmarkProof(fixture) {
  const metrics = fixture.metrics || {};
  return `<div class="benchmark-proof panel"><div><span class="panel-kicker">EVIDENCE LEDGER</span><h2>Performance and footprint, together.</h2><p>Throughput is only one dimension of safety. The release record also tracks event density, lifecycle time, and copy-up amplification.</p></div><div class="benchmark-proof-grid"><div><strong>${metrics.storageAmplification || "—"}</strong><span>upper-layer amplification</span></div><div><strong>${metrics.eventBytes || "—"}</strong><span>journal density</span></div><div><strong>${metrics.lifecycle || "—"}</strong><span>B4 wrapper lifecycle</span></div></div></div>`;
}
