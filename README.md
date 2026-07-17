# RewindBPF

RewindBPF, yapay zekâ ajanlarını Linux üzerinde geri alınabilir ve politika kontrollü filesystem transaction’ları içinde çalıştırmak için tasarlanan bir **AI Agent Safety Runtime** prototipidir.

Temel fikir:

```text
Agent başlatılır
    ↓
Mount namespace + OverlayFS hazırlanır
    ↓
eBPF filesystem/process olaylarını gözlemler
    ↓
Landlock/BPF LSM okuma politikalarını uygular
    ↓
Başarılıysa commit, hatalıysa rollback
```

Bu proje bir AI agent, Codex skill’i veya IDE extension’ı değildir. Çekirdek ürün; bir Linux daemon’ı, CLI, eBPF programı ve OverlayFS tabanlı sandbox’tan oluşan runtime’dır. İleride MCP, plugin veya IDE adaptörleri eklenebilir.

## Durum

Bu commit başlangıç boilerplate’idir. Çalışan ürün davranışı henüz eklenmemiştir; mimari kararlar ve 7 günlük MVP planı için [docs/PROJECT_PLAN.md](docs/PROJECT_PLAN.md) dosyasına bakın.

## Hedeflenen bileşenler

- `rewind`: kullanıcı CLI’ı
- `rewindd`: sandbox, process, policy ve rollback yöneticisi
- `ebpf/`: C + libbpf/CO-RE kernel programları
- OverlayFS: değişiklikleri üst katmanda tutan filesystem transaction katmanı
- Landlock/BPF LSM: kullanıcı tanımlı filesystem erişim politikaları
- VM/namespace: ajanı host’tan ayıran çalışma ortamı
- `benchmarks/`: baseline, overhead ve rollback ölçümleri

## Geliştirme

Gereksinimler: Go, Linux VM, OverlayFS ve eBPF/BTF destekli kernel.

```bash
make build
make test
./bin/rewind --help
```

Gerçek kernel entegrasyonu için macOS host yerine izole Ubuntu VM kullanılmalıdır. Ayrıntılı kapsam, güvenlik sınırları, benchmark matrisi ve test senaryoları proje planında tanımlıdır.
