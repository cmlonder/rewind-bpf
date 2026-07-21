import { siteData } from "../data.js";

export function Header() {
  return `<header class="site-header">
    <a class="brand" href="#top" aria-label="RewindBPF home"><span class="brand-mark" aria-hidden="true">↺</span><span>rewind<span class="brand-bpf">bpf</span></span></a>
    <nav class="top-nav" aria-label="Primary navigation">${siteData.nav.map(([label, href]) => `<a href="${href}">${label}</a>`).join("")}</nav>
    <div class="header-actions"><a class="header-docs" href="https://github.com/cmlonder/rewind-bpf/blob/main/README.md" target="_blank" rel="noreferrer">Docs <span>↗</span></a><a class="header-cta" href="https://github.com/cmlonder/rewind-bpf" target="_blank" rel="noreferrer">View source <span>↗</span></a></div>
  </header>`;
}
