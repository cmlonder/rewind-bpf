import { siteData } from "../data.js";

const mark = (value) => value === "Yes" ? `<span class="matrix-mark yes">✓</span>` : value === "No" ? `<span class="matrix-mark no">—</span>` : `<span class="matrix-partial">${value}</span>`;

export function Landscape() {
  return `<section class="landscape-section section-shell" id="landscape" aria-labelledby="landscape-title">
    <div class="section-heading-row reveal"><div><div class="section-kicker">04 / THE LANDSCAPE</div><h2 id="landscape-title">Different tools.<br /><span>Different promises.</span></h2></div><p>We compare capabilities, not marketing slogans. RewindBPF’s wedge is the combination of a pre-created COW write boundary, kernel evidence, and a reviewable run lifecycle.</p></div>
    <div class="matrix-wrap reveal"><table class="comparison-table"><caption>Feature comparison · public documentation reviewed July 2026</caption><thead><tr><th>System</th><th>Safety model</th><th>COW writes</th><th>Kernel policy</th><th>Rollback</th><th>Sensitive reads</th><th>Agent agnostic</th></tr></thead><tbody>${siteData.competitors.map((c) => `<tr class="${c.highlight ? "is-current" : ""}"><th scope="row"><span class="competitor-name">${c.name}</span><small>${c.relation}</small></th><td>${c.model}</td><td>${mark(c.cow)}</td><td>${mark(c.kernel)}</td><td>${mark(c.rollback)}</td><td>${mark(c.read)}</td><td>${mark(c.agent)}</td></tr>`).join("")}</tbody></table></div>
    <div class="landscape-note reveal"><span class="note-icon">!</span><p><strong>Benchmark provenance:</strong> nono is the closest product comparison. Tetragon and KubeArmor are kernel-policy neighbors; AgentFS is a filesystem/history alternative; DeltaBox is a research reference. None is silently installed in the VM, so external numbers stay <em>not-comparable</em> until their exact environment can be reproduced.</p><a href="https://github.com/cmlonder/rewind-bpf/blob/main/benchmarks/COMPETITOR_MATRIX.md" target="_blank" rel="noreferrer">See provenance ledger ↗</a></div>
  </section>`;
}
