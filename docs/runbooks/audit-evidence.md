# Runbook: audit sübut paketi (ixrac və yoxlama)

Kimin üçün: workspace admini (ixrac edir) və auditor / tənzimləyici (yoxlayır). Qərarlar: [ADR-0029](../adr/0029-audit-evidence-export.md).

## 1. Admin: paketi ixrac etmək

- **Bir tapşırıq:** tapşırığı açın → başlıqda **Sübut ixracı**. Paketə tapşırığın bütün tarixçəsi, şərhləri, təsdiq
  sorğuları və qərarları, əlaqələri və faylların SHA-256 siyahısı düşür.
- **Dövr üzrə:** **Parametrlər → Audit log → Audit sübut paketi**. Tarix aralığı məcburidir (ən çox 366 gün, workspace
  saat qurşağında, daxil); layihə seçilməyibsə — gördüyünüz bütün layihələr; "Yalnız təsdiqdən keçən tapşırıqlar" —
  aralıqda təsdiq sorğusu açılmış və ya bağlanmış tapşırıqlar.
- Limit aşılırsa (5000 tapşırıq, 50 000 hadisə və s.) paket **verilmir** — daha az layihə və ya daha qısa aralıq seçin.
  Saatda 20 ixrac (istifadəçi başına).
- Hər ixrac audit log-a `evidence.exported` kimi yazılır (manifestin SHA-256-sı ilə); "İxrac tarixçəsi" cədvəlində görünür.

## 2. Admin: barmaq izini auditora çatdırmaq

Parametrlər → Audit log səhifəsində **Açarın barmaq izi** (`SHA256:…`) və hazır əmr var. Barmaq izini paketlə **eyni
kanalla göndərməyin** (paketi dəyişdirən onu da dəyişdirə bilər): telefonla oxuyun, imzalı məktuba yazın və ya
auditorun portalına ayrıca yükləyin. Serverdə:

```bash
docker compose exec nebula /nebula evidence public-key
```

## 3. Auditor: paketi yoxlamaq

`nebula` binary-si (və ya image) lazımdır; konfiqurasiya, şəbəkə və verilənlər bazası **lazım deyil**:

```bash
nebula evidence verify --key 'SHA256:<barmaq izi>' paket.zip
```

```bash
docker run --rm -v "$PWD":/w:ro --entrypoint /nebula <nebula-image> evidence verify --key 'SHA256:<barmaq izi>' /w/paket.zip
```

Nəticə `result: OK` (çıxış kodu 0) — hər fayl manifestdəki SHA-256-ya uyğundur və manifest həmin açarla imzalanıb.
`INVALID` (kod 1) — səbəblər `FAIL:` sətirlərindədir, məsələn:

| Sətir | Mənası |
|---|---|
| `report.html was changed` | fayl imzadan sonra dəyişdirilib |
| `the signature does not match manifest.json` | manifest dəyişdirilib (məs. hash-lər yenidən hesablanıb) |
| `signed by SHA256:…, not by the trusted key` | paket başqa açarla (yenidən) imzalanıb |
| `… is not listed in the manifest` / `… is missing` | fayl əlavə edilib / silinib |

`--key` verilməsə əmr imzalayan açarın barmaq izini çap edir — onu admindən aldığınızla **əl ilə** müqayisə edin.

## 4. Auditor: iki ixrac arasında jurnalın dəyişmədiyini yoxlamaq

Əvvəlki auditdən qalan paket lövbərdir. Yeni paketi (eyni və ya daha geniş layihələr və tarix aralığı) onunla müqayisə edin:

```bash
nebula evidence verify --key 'SHA256:<barmaq izi>' --against 2026-III-rub.zip 2026-illik.zip
```

- `event N was changed after …` — hadisə köhnə ixracdan sonra verilənlər bazasında redaktə edilib;
- `event N is missing: deleted after …` — hadisə silinib (yalnız yeni paketin əhatəsi köhnəni tam əhatə edəndə yoxlanılır;
  çıxışda `edits and deletions checked` yazılır, əks halda yalnız redaktələr yoxlanılır).

İstifadəçi adının və ya layihə açarının dəyişməsi fərq sayılmır.

## 5. Paketin içi

| Fayl | Nə var |
|---|---|
| `report.html` | oxunaqlı hesabat: brauzerdə açın, **Çap → PDF kimi saxla** |
| `data/events.json`, `data/events.csv` | hadisələr (kim, nə vaxt, nə dəyişdi), CSV Excel üçün (UTF-8) |
| `data/items.json` | tapşırıqlar ixrac anındakı vəziyyətdə |
| `data/approvals.json` | təsdiq sorğuları, siyasət snapshot-u, hər qərar (kim, nə vaxt, şərh / rədd səbəbi) |
| `data/comments.json`, `links.json`, `attachments.json` | şərhlər, əlaqələr, faylların adı, ölçüsü və SHA-256-sı (faylın özü yox) |
| `data/people.json`, `projects.json`, `statuses.json` | id → ad, email, rol; layihələr; statuslar |
| `manifest.json`, `manifest.sig` | hər faylın SHA-256-sı, parametrlər, ixrac edən, vaxt, server versiyası; Ed25519 imzası |

Faylın özünü yoxlamaq üçün: faylı Nebula-dan yükləyin və `sha256sum fayl.pdf` nəticəsini `attachments.json`-dakı ilə müqayisə edin.
