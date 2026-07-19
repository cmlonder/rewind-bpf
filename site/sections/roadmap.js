import { siteData } from "../data.js";

export function Roadmap() {
  return `<section class="roadmap-section section-shell" id="roadmap" aria-labelledby="roadmap-title">
    <div class="section-heading-row reveal"><div><div class="section-kicker">03 / WHAT COMES NEXT</div><h2 id="roadmap-title">From demo to durable runtime</h2></div><p>The roadmap favors correctness over surface area: finish the reversible transaction, then make policy, evidence, and operations composable.</p></div>
    <div class="roadmap-list">${siteData.roadmap.map((item, i) => `<article class="roadmap-item reveal reveal-delay-${i}"><div class="roadmap-phase">${item.phase}</div><div class="roadmap-copy"><div class="roadmap-title"><h3>${item.title}</h3><span class="roadmap-status ${i === 0 ? "is-done" : ""}">${item.status}</span></div><p>${item.body}</p></div><div class="roadmap-index">0${i + 1}</div></article>`).join("")}</div>
    <div class="delivered-block reveal"><div class="delivered-heading"><span class="section-kicker">SHIPPED SINCE MVP</span><p>A live ledger of the hardening work already implemented and validated in the disposable VM.</p></div><div class="delivered-grid">${siteData.delivered.map(([number, title, body], i) => `<article class="delivered-card reveal reveal-delay-${i % 3}"><span>${number}</span><h3>${title}</h3><p>${body}</p></article>`).join("")}</div></div>
    <div class="roadmap-foot reveal"><span>Next proof point</span><strong>Policy learn → human review → safe profile</strong><span class="roadmap-line"></span><a href="https://github.com/rewindbpf/rewind/blob/main/docs/PHASE2_PLAN.md" target="_blank" rel="noreferrer">Read Phase 2 plan ↗</a></div>
  </section>`;
}
