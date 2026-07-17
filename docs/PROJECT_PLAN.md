# RewindBPF — Proje Kararları, Mimari ve MVP Planı

## 1. Proje özeti

RewindBPF, işletim sistemi üzerinde kontrolsüz çalışan AI ajanlarının dosya bütünlüğünü bozmasını, hassas dosyaları okumasını ve izin verilmeyen kaynaklara erişmesini sınırlayan bir **AI Agent Safety Runtime**’dır.

Ana ürün iddiası:

> Ajanı her çalıştırmada izole bir filesystem transaction içinde başlatırız. eBPF davranışı ölçer, OverlayFS değişiklikleri hapseder, politika katmanı hassas erişimleri sınırlar; hata durumunda transaction tek hamlede geri alınır.

Mühendislik mottosu:

> Sıcak yolu ucuz tut, pahalı işi tembel (copy-on-write) yap.

## 2. Problem ve çözüm

AI ajanlarına terminal yetkisi verildiğinde önceden bilinmeyen yıkıcı işlemler yapabilirler:

- Dosya ve dizin silebilir veya üzerine yazabilir.
- Hassas dosyaları (`.env`, SSH anahtarları, PII dizinleri) okuyabilir.
- Yetki yükseltmeye, mount/ptrace yapmaya veya host’tan kaçmaya çalışabilir.
- Yetkisiz ağ bağlantısıyla veri dışarı çıkarabilir.
- Sonsuz process, CPU, RAM veya disk kullanımıyla sistemi kullanılmaz hale getirebilir.

Klasik işlem öncesi `cp` yedekleri her işlemde I/O ve latency üretir. RewindBPF’nin çözümü, ajan başlamadan önce OverlayFS katmanı kurmak ve değişiklikleri fiziksel kopyalama yerine copy-on-write ile üst katmana yönlendirmektir.

## 3. En kritik mimari düzeltme

eBPF olayı gördükten sonra snapshot almıyoruz. `unlink` veya `write` başladıktan sonra userspace daemon’ın OverlayFS kurması güvenli değildir.

Doğru akış:

```text
Agent başlamadan önce:
  lowerdir = orijinal, tercihen salt-okunur katman
  upperdir = geçici değişiklik katmanı
  workdir  = OverlayFS çalışma alanı
  merged   = ajanın gördüğü çalışma alanı

Agent çalışırken:
  okumalar lower/upper birleşiminden gelir
  yazmalar ve silmeler upperdir’a gider
  eBPF olayları gözlemler ve politika olaylarını üretir

Rollback:
  agent durdurulur
  merged unmount edilir
  upperdir atılır
  lowerdir tekrar görünür hale gelir
```

OverlayFS, alt katmanı gerçekten değiştirmeden silmeleri whiteout kayıtlarıyla ve yazmaları copy-up ile temsil eder. Üst katmanın atılması rollback’in temel mekanizmasıdır. OverlayFS bir yedekleme sistemi değildir; önceden kurulmuş geçici bir çalışma katmanıdır.

## 4. Ürün tanımı ve çalışma modları

Çekirdek ürün, Linux üzerinde çalışan bir runtime uygulamasıdır; bir AI agent değildir.

```text
rewind          CLI
rewindd         userspace daemon
ebpf/*.bpf.c    kernel sensör/policy programları
OverlayFS       filesystem transaction
Landlock/LSM    erişim kontrolü
namespace/VM    izolasyon
```

Örnek kullanım hedefi:

```bash
rewind run --workspace ./project --policy policy.yaml -- agent-command
rewind status
rewind events run_42
rewind rollback run_42
rewind commit run_42
```

Çalışma alanı kapsamları:

- `workspace`: yalnızca proje/workspace korunur; ilk geliştirme modu.
- `system`: disposable Linux VM içinde normal filesystem’in tamamı transaction sınırına alınır.

“Tüm host filesystem’i koruma” MVP’den çıkarılmıyor; ancak güvenli ve tekrarlanabilir demosu disposable VM içinde yapılacak. Canlı host üzerindeki `/proc`, `/sys`, device state, kernel state, açık file descriptor’lar ve ağ durumu OverlayFS rollback kapsamı değildir.

## 5. Güvenlik katmanları

### 5.1 Dosya bütünlüğü

OverlayFS ile aşağıdaki değişiklikler geri alınır:

- `write`, `pwrite`, `truncate`, `ftruncate`, `fallocate`
- `unlink`, `rmdir`, `rename`
- Yeni dosya/dizin, symlink veya link oluşturma
- Metadata değişiklikleri (MVP kapsamına göre)

eBPF hedeflenen process/cgroup için `execve`, `openat/openat2`, `write`, `unlinkat`, `renameat2`, `truncate/ftruncate` gibi olayları gözlemler. Tek bir `sys_enter_write` hook’u tüm yazma yollarını kapsamaz; bu nedenle gözlem kapsamı açıkça belgelenir.

### 5.2 Okuma gizliliği

`.env` yalnızca örnek pattern’dir. Kullanıcı pattern tabanlı politika tanımlar:

```yaml
read:
  mode: enforce # off | audit | enforce
  deny:
    - "**/.env"
    - "**/*.pem"
    - "**/*.key"
    - "/home/*/.ssh/**"
    - "/data/pii/**"
  allow:
    - "/workspace/.env.example"
```

Modlar:

- `off`: okuma koruması kapalı.
- `audit`: erişim loglanır, işlem engellenmez.
- `enforce`: erişim reddedilir ve event üretilir.

Kullanıcı pattern’i glob olarak yazar; policy compiler bunu filesystem hiyerarşisi ve erişim kurallarına dönüştürür. Path string’ini her syscall’da regex ile eşleştirmek yerine Landlock ve/veya BPF LSM enforcement kullanılmalıdır. eBPF tracepoint’i audit ve telemetry için uygundur; tek başına güvenilir deny mekanizması olarak sunulmaz.

İlk MVP path tabanlı erişim kontrolü yapar. Dosya içeriğinden otomatik PII sınıflandırması ve redaction sonraki aşamadır.

### 5.3 Ayrıcalık, ağ ve kaynak politikaları

Gelecek veya sınırlı MVP politikaları:

| Risk | Uygun katman |
|---|---|
| `mount`, `ptrace`, setuid, BPF erişimi | BPF LSM + seccomp |
| Yetkisiz network bağlantısı | Network namespace + cgroup eBPF |
| Fork bombası, CPU/RAM/PID tüketimi | cgroups |
| Host path veya kernel arayüzü erişimi | namespaces + Landlock |
| Süreç zinciri ve komut soy ağacı | eBPF `execve` telemetry |

eBPF bütün güvenlik işlerini tek başına yapmaz; kernel hook’ları, namespace, Landlock, seccomp ve cgroups tamamlayıcı katmanlardır.

## 6. Teknik stack

- **Linux VM:** Ubuntu/Debian, OverlayFS ve BPF/BTF destekli kernel.
- **Filesystem:** ext4 ile tekrarlanabilir ilk deney ortamı.
- **eBPF:** C + libbpf/CO-RE; kernel sensörlerinin ve mümkünse BPF LSM hook’larının taşınabilir yüklenmesi.
- **Daemon:** Go; process, mount namespace, policy, ring buffer, JSON ve CLI orkestrasyonu.
- **Policy:** YAML/JSON giriş, glob pattern, `off/audit/enforce` modları.
- **CLI:** İlk MVP’de web UI yok; terminal timeline ve komutlar yeterli.
- **Benchmark:** `hyperfine`, `fio`, `fs_mark`, `perf stat`, custom Go workload runner.
- **Doğrulama:** Hash/metadata manifest, JSON event log ve CSV/JSON benchmark çıktısı.

Rust + Aya uygulanabilir bir alternatiftir; 7 günlük MVP’de Go + C/libbpf daha düşük entegrasyon riski taşır.

## 7. Benchmark stratejisi

### 7.1 Karşılaştırma grupları

| Grup | Filesystem | eBPF | Daemon | Amaç |
|---|---|---:|---:|---|
| B0 | Native ext4 | Yok | Yok | Saf baseline |
| B1 | Native ext4 | Var | Yok | Sadece eBPF maliyeti |
| B2 | OverlayFS | Yok | Yok | OverlayFS maliyeti |
| B3 | OverlayFS | Var | Yok | eBPF + OverlayFS |
| B4 | OverlayFS | Var | Var | Gerçek ürün yolu |
| B5 | OverlayFS | Var | Var + policy | Pause/kill enforcement maliyeti |

Ana karşılaştırma B0 ↔ B4’tür; B1 ve B2 maliyetin kaynağını ayrıştırır.

### 7.2 Deney kontrolü

- Önce B0 baseline alınır.
- 3 warm-up, en az 15 ölçümlü tekrar.
- Cold-cache ve warm-cache ayrı raporlanır.
- Aynı VM, kernel, CPU governor, disk, mount seçenekleri ve dataset kullanılır.
- Background servisler azaltılır; dataset ve workload seed’i sabitlenir.
- Her sonuçta commit hash, kernel config, dataset manifest ve komut saklanır.

### 7.3 Workload’lar

- Read-heavy: `rg`, `find`, `git status`, küçük/büyük dosya okuma, derleme/test.
- Write-heavy: 10.000 küçük dosya, append, büyük dosya overwrite, truncate, rename.
- Metadata-heavy: create/unlink, recursive rename, chmod/chown, symlink/hardlink.
- Mixed agent: create → modify → rename → delete → `rm -rf src/` → new files.
- Concurrency: 1, 2 ve 4 paralel agent.
- Policy: deny hit, audit hit, allow hit, event flood.

### 7.4 Metrikler

- Toplam süre ve throughput.
- p50, p95, p99 latency.
- CPU cycles, CPU yüzdesi, context switch, page fault.
- Read/write I/O bytes, peak RSS.
- eBPF event latency ve dropped event sayısı.
- Copy-up süresi ve `upperdir` boyutu.
- Görünür kurtarma süresi ve tam cleanup süresi.

Formüller:

```text
overhead (%) = ((variant_time - baseline_time) / baseline_time) × 100
space amplification = upperdir_bytes / logical_changed_bytes
```

İlk hedefler hipotezdir, garanti değildir: read-heavy işlerde düşük tek haneli overhead, mixed işlerde kabul edilebilir overhead, demo workspace’inde 1 saniyeye yakın görünür rollback ve normal workload’ta sıfır event loss.

## 8. Doğruluk ve güvenlik testleri

Her testten önce lower layer manifest’i oluşturulur. Rollback sonrasında içerik, dosya yapısı, mode, UID/GID, symlink hedefi, xattr ve boyut/timestamp karşılaştırılır.

Temel senaryolar:

1. Dosya değiştirme → eski içerik geri gelir.
2. Dosya silme → dosya geri gelir.
3. Recursive `rm -rf src/` → tüm ağaç geri gelir.
4. Yeni dosya oluşturma → rollback sonrası yok olur.
5. Dizin rename → eski isim geri gelir.
6. Büyük dosya overwrite → eski içerik korunur.
7. Agent `kill -9` → rollback çalışır.
8. Daemon kapanması → OverlayFS sınırı korumayı sürdürür; yeni run başlatma fail-closed olur.
9. Event flood → queue taşması ve event loss görünür.
10. Kullanıcı pattern’i → `off/audit/enforce` davranışı doğrulanır.
11. Yasaklı `.env`, `.pem`, SSH veya PII path’i → enforce modunda okunamaz.
12. Symlink/path traversal → workspace dışına erişim politikaya göre engellenir.
13. Yetkisiz network → bağlantı policy’ye göre deny/audit olur.
14. Açık file descriptor, dış writer ve host mount davranışı belgelenir.
15. Başarılı run → commit/export sonrası beklenen diff korunur.

Ana invariant:

> Rollback sonrası lower layer değişmemiş olmalı ve koruma alanı dışındaki sentinel dosyalar aynı kalmalıdır.

## 9. Büyük demo akışı

1. Ajan izole VM/workspace transaction’ında başlatılır.
2. Normal dosya değişiklikleri eBPF timeline’ında görünür.
3. Ajan `rm -rf src/` çalıştırır.
4. OverlayFS alt katmanı sağlam tutar; üst katmanda deletion kayıtları oluşur.
5. `rewind rollback <run_id>` çalıştırılır.
6. Proje geri gelir ve hash manifest’i kurtarmayı kanıtlar.
7. İkinci sahnede ajan `.env` veya kullanıcı tanımlı hassas pattern’i okumaya çalışır.
8. Erişim `audit` veya `enforce` moduna göre loglanır/reddedilir.

## 10. Kapsam ve kapsam dışı

### MVP kapsamı

- Linux VM ve tekrarlanabilir kurulum.
- Workspace OverlayFS transaction.
- eBPF process/filesystem telemetry.
- Rollback.
- Kullanıcı tanımlı read pattern politikası.
- `.env` gibi örneklerin yanında genel glob desteği.
- Disposable VM içinde system scope deneyi.
- B0–B5 benchmark matrisi.

### Kapsam dışı veya sonraki aşama

- Tüm içeriklerde otomatik PII sınıflandırma/redaction.
- Genel host üzerinde mutlak kernel/cihaz/ağ rollback’i.
- Karmaşık çatışma çözebilen production-grade merge.
- Çoklu filesystem ve network filesystem garantisi.
- Web dashboard ve IDE entegrasyonu.

## 11. 7 günlük build planı

### Gün 1 — Ortam ve baseline

- Ubuntu VM, kernel ve araçlar.
- OverlayFS/eBPF/Landlock capability check.
- Deterministic dataset generator.
- B0 native filesystem baseline.

### Gün 2 — OverlayFS sandbox

- lower/upper/work/merged yaşam döngüsü.
- Workspace mode.
- Agent process başlatma ve namespace izolasyonu.
- Basit rollback.

### Gün 3 — eBPF telemetry

- `execve`, `openat`, `unlinkat`, `renameat2`, write/truncate event’leri.
- Ring buffer → Go daemon.
- PID/cgroup filtreleme ve event JSON logu.

### Gün 4 — Read policy

- YAML policy parser.
- Glob → filesystem rule compiler.
- `off/audit/enforce` modları.
- Landlock/BPF LSM veya desteklenen kernel enforcement.

### Gün 5 — CLI, lifecycle ve fail-safe

- `run`, `status`, `events`, `rollback`, `commit` komutları.
- Crash/daemon failure davranışı.
- Workspace dışında erişim ve sentinel testleri.

### Gün 6 — Benchmark ve doğruluk

- B1–B5 ölçümleri.
- Correctness test matrisi.
- Hash manifest, event loss ve rollback ölçümü.
- Grafik ve CSV/JSON artifact’leri.

### Gün 7 — Demo ve sunum

- Deterministic destructive agent demo.
- Secret-read policy demo.
- Recovery proof ve latency göstergesi.
- Teknik iddia, sınırlamalar ve benchmark sonuçlarının son kontrolü.

## 12. Açık riskler

- OverlayFS üst ve work dizinleri için filesystem/xattr/d_type gereksinimleri.
- Root filesystem’i canlı host üzerinde overlay’leme karmaşıktır; system mode VM ile sınırlandırılır.
- eBPF syscall telemetry tam filesystem semantiği değildir; mmap ve bazı indirect write yolları ayrıca ele alınmalıdır.
- Event sonrası userspace kararı geçmiş işlemi geri alamaz; koruma önceden mount/policy ile sağlanır.
- Açık file descriptor’lar ve agent’ın namespace/capability kaçışı açık tehdit modelidir.
- “Sıfır overhead” yerine ölçülmüş “düşük hot-path overhead” iddiası kullanılmalıdır.

## 13. Karar özeti

- Çekirdek ürün: Linux userspace runtime + eBPF + OverlayFS.
- eBPF snapshot başlatmaz; telemetry ve gerektiğinde enforcement sağlar.
- Snapshot her agent run’ından önce hazırdır.
- Rollback MVP’nin birincil işlevidir; commit basit/ kontrollü diff olarak kalır.
- Okuma politikası `.env` hardcode değildir; kullanıcı glob pattern’i ve `off/audit/enforce` modları vardır.
- Tüm host filesystem kapsamı VM içinde MVP’ye dahil edilir; canlı host kernel/device state için mutlak rollback iddiası yoktur.
- Go + C/libbpf/CO-RE, 7 günlük sprint için seçilen stack’tir.
- Benchmark önce B0 baseline ile başlar; B1–B5 katmanları sonradan karşılaştırılır.
