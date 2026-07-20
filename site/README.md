# RewindBPF project site

This is the dependency-free, jury-facing single-page site for RewindBPF. `index.html` is the publishable entry point; `app.js` composes the page from `sections/*.js`, and `data.js` keeps the displayed facts in one place.

Preview it from the repository root with:

```bash
python3 -m http.server 4173 --directory site
open http://127.0.0.1:4173
```

The page is deliberately a presentation layer. Runtime behavior, roadmap decisions, competitor provenance, and benchmark values stay canonical in the root README, `docs/`, and `benchmarks/` ledgers.

Refresh the measured chart and publish to an explicit local directory or
S3-compatible bucket with:

```bash
make benchmark-verify
REWIND_SITE_DEST=/path/or/s3://bucket make publish-site
```

The publisher refuses an empty destination; external hosting credentials are
intentionally not stored in the repository.
