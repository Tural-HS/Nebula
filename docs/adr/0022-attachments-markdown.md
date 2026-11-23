# ADR-0022: Fayl əlavələri və Markdown

- Status: Qəbul edilib
- Tarix: 2026-11-24
- Miqrasiya: `00017_attachments.sql`

## Kontekst

Komandalar tapşırıqlara ekran görüntüsü, müqavilə, hesabat əlavə edə bilmirdi; təsvir və şərhlər düz mətn idi
(siyahı, kod, cədvəl yox). Hər iki xüsusiyyət ən çox hücum səthi açanlardandır: istifadəçinin yüklədiyi fayl
eyni origin-dən verilərsə saxlanılmış XSS olur, Markdown renderer-i isə klassik XSS mənbəyidir. Ona görə
təhlükəsizlik qərarları funksionallıqdan əvvəl gəlir.

## Qərarlar — fayllar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Baytlar bazada deyil, **blob store**-da: `local` (volume, default) və ya `s3` (istənilən S3-uyğun). Bazada yalnız metadata və **təsadüfi 256-bit açar** (`storage_key`, hex); istifadəçi fayl adı heç vaxt diskə/bucket-ə düşmür | Postgres backup-ları və replikasiya kiçik qalır; path traversal mümkün deyil. Lokal backend bütün girişi `os.Root` ilə edir, fayllar `0600`, qovluqlar `0700` | `bytea`: WAL və backup şişir, 20 MiB sətirlər. Fayl adı ilə saxlama: traversal, toqquşma |
| S3 üçün SDK yox, **~200 sətirlik SigV4 imzası** (yalnız PUT/GET/DELETE). AWS-in rəsmi test vektorları və real S3 server (Garage v2.5.0) ilə yoxlanılıb. `x-amz-content-sha256` faylın real SHA-256-dır — server zədələnmiş ötürməni rədd edir. Redirect-lərə əməl edilmir | `aws-sdk-go-v2` və ya `minio-go` onlarla tranzitiv asılılıq gətirir; bizə 3 əməliyyat lazımdır. MinIO image-ləri artıq açıq yayılmır | SDK: daha çox supply-chain səthi |
| Yükləmə **axınla** gedir: multipart hissəsi staging faylına yazılarkən SHA-256, ölçü və ilk 512 bayt toplanır; limitdən 1 bayt artıq gələn kimi dayanır. Sonra blob bir addımda commit olunur (lokal: `fsync` + `rename`; S3: PUT, uzunluq və heş məlum) | Yaddaşda fayl saxlanılmır; limit və tip yoxlanılmamış heç nə daimi yerə yazılmır | `ParseMultipartForm`: temp fayllara nəzarətsiz yazır |
| **Tip baytlardan təyin olunur** (`http.DetectContentType`), klientin `Content-Type`-ı nəzərə alınmır | `photo.png` adlı HTML səhifəsi HTML kimi qeyd olunur | Klientə inanmaq: tip saxtalaşdırma |
| Endirmə həmişə `Content-Disposition: attachment` + `application/octet-stream`, `X-Content-Type-Options: nosniff`, `Content-Security-Policy: default-src 'none'; sandbox`, `Cross-Origin-Resource-Policy: same-origin`, `Cache-Control: private, no-store`. **İstisna:** yalnız PNG/JPEG/GIF/WebP, yalnız `?inline=1` ilə, öz tipi ilə `inline`. SVG heç vaxt inline deyil | Brauzer heç bir faylı sənəd kimi açmır; açsa belə sandbox CSP skripti bloklayır. Raster şəkil `<img>` kimi təhlükəsizdir | Ayrı "user content" domeni: daha güclüdür, amma tək VPS-də əlavə DNS/TLS; indiki başlıqlarla risk qəbul ediləndir. Gələcəkdə əlavə oluna bilər |
| **Public URL yoxdur.** Endirmə də adi API marşrutudur (Bearer token); SPA faylı `fetch` ilə alır və `blob:` URL-ə çevirir (thumbnail-lar da). Caddy CSP-də `img-src` `blob:` əlavə olundu | Cookie ilə avtomatik autentifikasiya yoxdur (CSRF yoxdur), link sızsa da işləmir | İmzalı müvəqqəti URL-lər: sızma pəncərəsi, əlavə açar idarəsi |
| İcazə tapşırığın görünməsi ilə **eynidir**: marşrut `/projects/{project}/items/{n}/attachments` altındadır, `loadProject` gizli layihəni üzv olmayana 404 qaytarır, tapşırıq canlı olmalıdır, fayl həmin tapşırığa aid olmalıdır (başqa tapşırığın yolu ilə id → 404). Oxuma: qonaq daxil hamı; yükləmə: member+; silmə: yükləyən və ya admin | ADR-0013 ilə eyni model; 404 varlığı da gizlədir | — |
| Şərhə fayl (`comment_id`) yalnız şərhin müəllifi əlavə edə bilər; şərh silinəndə faylları da silinir (audit ilə) | Başqasının sözlərinə fayl əlavə etmək olmaz (şərh redaktəsi qaydası) | — |
| Limitlər: fayl başına **20 MiB** (Caddy-nin 25 MB `/api` limitinə sığır), workspace başına **5 GiB**, tapşırıq başına **100** fayl, istifadəçi başına **30 yükləmə/dəq** (replika başına). Kvota və say **tranzaksiyada, workspace advisory lock** altında yoxlanılır — paralel yükləmələr limiti keçə bilməz; boş və ya dolu workspace bayt oxunmadan rədd edilir | Disk/bucket dolması və DoS | Yalnız handler-də yoxlama: race |
| Fayl köçürmələri API-nin 30 s timeout-undan azaddır və öz müddətini (`NEBULA_ATTACHMENTS_TRANSFER_TIMEOUT`, 5 dəq) bağlantı deadline-ları ilə qoyur | Yavaş şəbəkədə 20 MiB 30 s-yə sığmaya bilər; digər marşrutlar qısa qalır | Qlobal timeout-u artırmaq: slowloris səthi |
| Fayl adı: yalnız əsas ad, idarəetmə və **görünməz formatlama simvolları (bidi override) silinir**, 200 simvol; başlıqda RFC 2231 kodlaması | `cat‮gnp.exe` "catexe.png" kimi görünə bilərdi | — |
| **Yetim blob olmur:** `attachments` sətri necə silinirsə silinsin (API, şərh, kaskad), trigger açarı `attachment_blob_purges`-a yazır; worker (`Tick`) blob-u silir. Yükləmə açarı blob yazılmazdan **əvvəl** `upload` növü ilə 1 saatlıq möhlətlə qeyd olunur və sətir commit olanda silinir — proses blob ilə sətir arasında çöksə də blob təmizlənir. Sətri olan açar heç vaxt silinmir | Transactional outbox modeli (ADR-0005); kaskadlar tətbiq kodundan yan keçir | Periodik "bucket-i siyahıla, bazayla müqayisə et": S3-də bahalı, race-li |
| Silinmiş faylın blob-u `NEBULA_ATTACHMENTS_PURGE_DELAY` (default **7 gün**) saxlanılır, kvotaya sayılmır | Bazanı PITR ilə keçmiş ana qaytaranda fayl backup-ında həmin fayllar olsun (runbook §4.1) | Dərhal silmə: bərpadan sonra itkin fayllar |
| Audit: `attachment.added` / `attachment.removed` (ad, ölçü, tip; məzmun yox), fəaliyyət lentində göstərilir | Kim nəyi yüklədi/sildi izlənilir | — |
| Konfiqurasiya env ilə, sirlər `*_FILE` ilə (`NEBULA_ATTACHMENTS_S3_ACCESS_KEY[_FILE]`, `…_SECRET_KEY[_FILE]`); production-da S3 endpoint `https` olmalıdır; lokal qovluq startda yazıla bilən olmalıdır (fail fast) | ADR-0018 ilə eyni | — |

### Kaskad qaydaları (miqrasiya 00017)

- `(workspace_id, item_id) → items` `ON DELETE CASCADE` — sətir həmişə öz workspace-indəki öz tapşırığına bağlıdır.
- `(workspace_id, item_id, comment_id) → comments` `ON DELETE CASCADE` — şərh eyni tapşırığın şərhi olmalıdır
  (`comments`-ə `UNIQUE (workspace_id, item_id, id)` əlavə olundu). API şərhləri yumşaq silir və faylları özü silir.
- `uploader_id → users` `ON DELETE SET NULL` — hesab silinsə fayl qalır.
- `workspace_id → workspaces` `ON DELETE CASCADE`. Hər silinən sətir trigger ilə blob silinməsini növbəyə qoyur.
- Tapşırıq yumşaq silinəndə (`deleted_at`) fayllar API-dən itir, amma saxlanılır (bərpa üçün) və kvotaya sayılır.

## Qərarlar — Markdown

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **react-markdown 10.1.0 + remark-gfm 4.0.1** (npm-də canlı yoxlanılıb, unified kollektivi saxlayır). Nəticə React elementləridir — **`innerHTML` heç yerdə yoxdur** | Struktur olaraq XSS-ə qapalıdır: mətn həmişə mətn node-udur | `markdown-it`/`marked` + DOMPurify: HTML sətri + sanitizer-ə güvənmək (sanitizer bypass-ları tarixən olub). Qiymət: ~100 kiçik tranzitiv paket (lock faylda) |
| Xam HTML **atılır** (`skipHtml`), yalnız ağ siyahıdakı elementlər (`p`, başlıqlar, siyahılar, `code`/`pre`, `blockquote`, cədvəl, `a`, `del`, …) render olunur | `<img onerror>`, `<iframe>`, `<style>`, `<form>` mümkün deyil | Sanitize edilmiş HTML-ə icazə: lazım deyil |
| Linklər: yalnız `http`, `https`, `mailto`, `/yol` və `#anchor`; qalan hər şey (`javascript:`, `data:`, `vbscript:`, `//host`, `file:`, entity ilə gizlədilmiş sxemlər) mətnə çevrilir. Xarici linklər `target="_blank" rel="noopener noreferrer nofollow"` | Tabnabbing və referrer sızması yoxdur | react-markdown-un default `urlTransform`-u: nisbi yollara da icazə verir — bizə lazım deyil |
| **Şəkillər yüklənmir**: `![alt](url)` link kimi göstərilir ("🖼 alt") | Xarici şəkil = izləmə pikseli, oxuyanın IP-si üçüncü tərəfə; CSP də `img-src 'self'`-dir. Əlavə edilmiş şəkillər fayl bölməsində thumbnail kimi görünür | Proxy ilə xarici şəkil: SSRF səthi |
| Task list-lər yalnız oxunur (checkbox `disabled`) | Klik etmək mətni dəyişməlidir — ayrıca xüsusiyyətdir | — |
| `<@uuid>` mention-ları remark plugin-i ilə **yalnız adi mətndə** çipə çevrilir; `kod` içində hərfi qalır | Mövcud mention modeli (ADR-0007) dəyişmir, server tərəfi eynidir | — |
| Redaktor: adi `textarea` + "Yaz / Önizləmə" tabları (təsvir və şərhlər); təsvir fokusdan çıxanda yadda saxlanır, "Ləğv et" qaralamanı atır | Sadə, mobil klaviatura ilə işləyir (ADR-0019) | WYSIWYG: böyük asılılıq, XSS səthi |
| Server Markdown render etmir; email-lərdə mətn olduğu kimi gedir | Render yalnız bir yerdə (brauzer), sanitizasiya bir yerdə test olunur | — |

## Nəticələr

- `deploy/production`: `attachments` volume-u (image-də `65532` sahibli boş qovluq — volume ilk dəfə doğru sahiblik alır),
  env dəyişənləri, Caddy CSP `img-src blob:`. **pgBackRest faylları saxlamır** — runbook §4.1: lokal volume restic ilə S3-ə,
  S3 backend-də versioning + lifecycle; bərpa qaydası (DB-ni T-yə, faylları T-dən sonrakı ilk snapshot-a) və `comm` yoxlaması.
  Fayl backup-ı üçün systemd timer və metrik hələ yoxdur (follow-up).
- Bir replikadan çox işlədilərsə `local` backend paylaşılan volume tələb edir; çox replika üçün `s3` seçin.
  Yükləmə rate limit-i replika başınadır (ADR-0004 ilə eyni).
- UI: tapşırıq panelində "Fayllar" bölməsi (düymə və sürüklə-burax, thumbnail, endirmə, silmə); şərhə fayl əlavə etmək
  hələ yalnız API-dədir (`comment_id`).
- Testlər: Go inteqrasiya testləri DML-only rol ilə (həddən böyük fayl, saxta tip, gizli layihə/başqa tapşırıq → 404, kvota,
  say limiti, rate limit, yetim blob təmizliyi, purge gecikməsi), blobstore (AWS SigV4 vektorları, lokal, saxta və real S3),
  vitest-də 30+ Markdown XSS vektoru, E2E `web/e2e/attachments.spec.ts`.
