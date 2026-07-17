# Benchmarks

Benchmark tasarımı ve B0–B5 karşılaştırma grupları [docs/PROJECT_PLAN.md](../docs/PROJECT_PLAN.md) içinde tanımlıdır.

Planlanan yardımcı araçlar:

- `hyperfine`: makro süre
- `fio`: büyük I/O
- `fs_mark`: küçük dosya/metadata
- `perf stat`: CPU ve kernel sayaçları
- Go workload runner: deterministic agent senaryoları

Ölçüm çıktıları JSON/CSV olarak `benchmarks/results/` altında tutulur; bu dizin `.gitignore` kapsamındadır.
