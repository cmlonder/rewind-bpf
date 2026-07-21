import { siteData } from "../data.js";

export function Hero() {
  const run = siteData.heroRun;
  return `<section class="hero section-shell" id="top" aria-labelledby="hero-title">
    <div class="hero-copy reveal">
      <div class="eyebrow"><span class="pulse-dot"></span>${siteData.status.label}<span class="eyebrow-rule"></span>${siteData.status.sublabel}</div>
      <h1 id="hero-title">Let agents work.<br /><em>Keep project and secrets yours.</em></h1>
      <p class="hero-lede">RewindBPF gives an autonomous agent a disposable write layer and a policy-hidden view of sensitive paths, then asks for explicit acceptance before anything becomes permanent.</p>
      <div class="hero-actions"><a class="button button-primary" href="#system">See the system <span>↓</span></a><a class="button button-secondary" href="#benchmarks">Read the evidence <span>↗</span></a></div>
      <div class="hero-proof"><span class="proof-line"></span><span>Hot path: observe.</span><span>Slow path: copy-on-write.</span></div>
    </div>
    <div class="hero-instrument reveal reveal-delay-1" aria-label="Live transaction status">
      <div class="instrument-top"><span class="instrument-label">RUN / DEMO-042</span><span class="instrument-status"><i></i> protected</span></div>
      <div class="instrument-screen">
        <div class="screen-grid"></div>
        <div class="screen-core"><span class="core-ring ring-a"></span><span class="core-ring ring-b"></span><span class="core-icon">↺</span></div>
        <div class="screen-callout callout-one"><b>LOWER</b><span>original / intact</span></div>
        <div class="screen-callout callout-two"><b>UPPER</b><span>agent changes / 0 B</span></div>
        <div class="screen-callout callout-three"><b>POLICY</b><span>enforce / ready</span></div>
        <div class="screen-axis axis-x"></div><div class="screen-axis axis-y"></div>
      </div>
      <div class="instrument-foot"><span>overlayfs.transaction</span><span>evidence ✓</span></div>
    </div>
    <div class="hero-evidence reveal reveal-delay-2">
      <article class="proof-panel proof-timeline">
        <div class="proof-panel-head"><span>${run.id}</span><span class="proof-status"><i></i>${run.status}</span></div>
        <div class="proof-panel-title"><strong>Event timeline</strong><span>ordered evidence</span></div>
        <ol class="proof-timeline-list">${run.timeline.map(([operation, decision, time], i) => `<li class="${decision.startsWith("deny") ? "is-denied" : ""}"><span class="timeline-node"></span><span class="timeline-operation">${operation}</span><span class="timeline-decision">${decision}</span><time>${time}</time></li>`).join("")}</ol>
      </article>
      <article class="proof-panel proof-diff">
        <div class="proof-panel-head"><span>STAGED DIFF</span><span>3 findings</span></div>
        <div class="proof-panel-title"><strong>Candidate changes</strong><span>merged view</span></div>
        <div class="proof-diff-list">${run.diff.map(([kind, path, note]) => `<div class="proof-diff-row diff-${kind}"><span class="diff-mark">${kind === "deleted" ? "−" : kind === "created" ? "+" : "!"}</span><code>${path}</code><span>${note}</span></div>`).join("")}</div>
        <div class="proof-panel-foot"><span>lower layer</span><strong>unchanged</strong></div>
      </article>
      <article class="proof-panel proof-decision">
        <div class="proof-panel-head"><span>REVIEW DECISION</span><span class="decision-lock">● protected</span></div>
        <div class="decision-state"><span class="decision-glyph">↺</span><div><strong>Candidate held</strong><span>Nothing is permanent yet.</span></div></div>
        <div class="decision-actions"><span>ROLLBACK</span><span>COMMIT <small>confirm</small></span></div>
        <div class="proof-panel-foot"><span>destination</span><strong>conflict check pending</strong></div>
      </article>
    </div>
  </section>`;
}
