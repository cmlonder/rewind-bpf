import { siteData } from "../data.js";

export function Header() {
  return `<header class="site-header">
    <a class="brand" href="#top" aria-label="RewindBPF home"><span class="brand-mark" aria-hidden="true">↺</span><span>rewind<span class="brand-bpf">bpf</span></span></a>
    <nav class="top-nav" aria-label="Primary navigation">${siteData.nav.map(([label, href]) => `<a href="${href}">${label}</a>`).join("")}</nav>
    <a class="header-cta" href="https://github.com/rewindbpf/rewind" target="_blank" rel="noreferrer">View source <span>↗</span></a>
  </header>`;
}
