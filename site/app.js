import { Header } from "./sections/header.js";
import { Hero } from "./sections/hero.js";
import { System } from "./sections/system.js";
import { Capabilities } from "./sections/capabilities.js";
import { Roadmap } from "./sections/roadmap.js";
import { Landscape } from "./sections/landscape.js";
import { Benchmarks } from "./sections/benchmarks.js";
import { Footer } from "./sections/footer.js";

const app = document.querySelector("#app");
app.innerHTML = `${Header()}<main id="main">${Hero()}${System()}${Capabilities()}${Roadmap()}${Landscape()}${Benchmarks()}</main>${Footer()}`;

const observer = new IntersectionObserver((entries) => {
  entries.forEach((entry) => {
    if (entry.isIntersecting) {
      entry.target.classList.add("is-visible");
      observer.unobserve(entry.target);
    }
  });
}, { threshold: 0.12 });
document.querySelectorAll(".reveal").forEach((element) => observer.observe(element));

document.querySelectorAll('a[href^="#"]').forEach((link) => {
  link.addEventListener("click", (event) => {
    const target = document.querySelector(link.getAttribute("href"));
    if (!target) return;
    event.preventDefault();
    target.scrollIntoView({ behavior: "smooth", block: "start" });
    history.replaceState(null, "", link.getAttribute("href"));
  });
});
