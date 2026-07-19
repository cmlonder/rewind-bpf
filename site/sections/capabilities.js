import { siteData } from "../data.js";

export function Capabilities() {
  return `<section class="capabilities-section section-shell" id="capabilities" aria-labelledby="capabilities-title">
    <div class="section-heading-row reveal"><div><div class="section-kicker">02 / WHAT EXISTS TODAY</div><h2 id="capabilities-title">The safety surface</h2></div><p>Small primitives, one explicit transaction. Every shipped feature has a testable boundary and a failure mode we can explain.</p></div>
    <div class="capability-grid">${siteData.capabilities.map((item, i) => `<article class="capability capability-${item.tone} reveal reveal-delay-${i % 3}"><div class="cap-top"><span class="cap-number">${item.number}</span><span class="status-pill">${item.tag}</span></div><h3>${item.title}</h3><p>${item.body}</p><div class="cap-detail"><span>↳</span>${item.detail}</div></article>`).join("")}</div>
  </section>`;
}
