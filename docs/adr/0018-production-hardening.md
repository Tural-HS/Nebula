# ADR-0018: Production hardening — fayl sirləri, ayrı DB rolları, healthcheck, növbə metrikləri, image skanı

- Status: Qəbul edilib
- Tarix: 2026-11-23
- Əlaqəli: [ADR-0015](0015-production-compose-on-vps.md) ("App dəyişiklikləri" bölməsinin 1–5-ci bəndlərini həyata keçirir), ADR-0007 (açar rotasiyası), ADR-0010 (etibarlı proxy-lər)

## Kontekst

ADR-0015-in production quruluşu tətbiqdə olmayan imkanları yan yollarla əvəz edirdi:

| Boşluq | Yan yol | Risk |
|---|---|---|
| Distroless image-də healthcheck yoxdur | read-only mount edilmiş busybox | əlavə image, Engine ≥ 28.1 `type: image` asılılığı |
| Sirlər yalnız env-dən | `nebula.env` (env_file) | sirlər `docker inspect`, `/proc/<pid>/environ`, crash dump-larda görünür |
| Növbə metrikləri yoxdur | Postgres-də `SECURITY DEFINER` funksiya + Alloy custom query | tətbiq sxeminə bağlı, migrasiyada unudula bilən gizli kod |
| Access log-da IP yoxdur | — | rate limit və hücum araşdırmasında kor nöqtə |
| Bir DB rolu (sahib) | — | SQL injection və ya RCE → `DROP TABLE`, sxem dəyişikliyi, miqrasiya tarixçəsinin saxtalaşdırılması |

Release workflow-u image-ləri zəiflik skanı olmadan imzalayırdı.

## Qərar

### 1. `nebula healthcheck`

Binary öz `/readyz`-ini loopback üzərindən yoxlayır (`NEBULA_HTTP_ADDR`; `-admin` ilə `NEBULA_ADMIN_ADDR`-də `/healthz` — yalnız worker olan konteynerlər üçün). Proxy yox, redirect yox, default 3 s timeout, exit 0/1 (2 = istifadə xətası). Yalnız dinləmə ünvanını oxuyur, **sirləri və qalan konfiqurasiyanı oxumur** — hər 10 saniyədə sir faylı açılmır, konfiqurasiya xətası healthcheck-i başqa səbəbdən sındırmır. Dockerfile `HEALTHCHECK` elan edir; one-shot `migrate` servisləri onu söndürür.

### 2. `*_FILE` sirləri

`NEBULA_DATABASE_URL`, `NEBULA_DATABASE_PASSWORD`, `NEBULA_MIGRATE_DATABASE_URL`, `NEBULA_MIGRATE_DATABASE_PASSWORD`, `NEBULA_ENCRYPTION_KEY`, `NEBULA_ENCRYPTION_KEYS_OLD`, `NEBULA_SMTP_PASSWORD` — hər biri `X_FILE` kimi də verilə bilər. Qaydalar:

- faylın sonundakı **bir** sətir sonu (`\n` və ya `\r\n`) silinir, qalan hər şey sirdir;
- `X` və `X_FILE` birlikdə → start xətası (hansı dəyərin qüvvədə olduğu heç vaxt qeyri-müəyyən deyil);
- oxuna bilməyən, adi fayl olmayan və ya 64 KiB-dan böyük fayl → aydın start xətası;
- xəta mesajlarında açar adı və yol var, **dəyər heç vaxt yoxdur** (testlə yoxlanılır).

OIDC tərəfində sir yoxdur (public client, yalnız JWKS), webhook sirləri bazada şifrələnib saxlanılır — ona görə `_FILE` siyahısı bununla bitir.

`NEBULA_DATABASE_PASSWORD` parolu sirr olmayan URL-ə əlavə edir: URL compose-da açıq yazılır, parol isə Postgres-in initdb-də istifadə etdiyi **eyni** fayldan gəlir — bir həqiqət mənbəyi. URL-də artıq parol varsa xəta.

### 3. Access log-da `client_ip`

Rate limiter-in istifadə etdiyi eyni resolver (ADR-0010): `X-Forwarded-For` yalnız `NEBULA_TRUSTED_PROXIES`-dən gələndə, sağdan sola. Log route pattern-i yazır (xam path və query yox), header yazmır — token, cookie, `?access_token=` log-a düşmür (testlə yoxlanılır).

### 4. Növbə metrikləri tətbiqdən

| Metrik | Məna |
|---|---|
| `nebula_job_queue_pending_jobs{state="due"}` | indi icra oluna bilən, heç bir worker-də olmayan |
| `nebula_job_queue_pending_jobs{state="running"}` | lease-i aktiv olan |
| `nebula_job_queue_pending_jobs{state="scheduled"}` | `run_at` gələcəkdə (retry backoff) |
| `nebula_job_queue_recently_failed_jobs` | son 1 saatda birdəfəlik uğursuz |
| `nebula_job_queue_oldest_due_age_seconds` | ən köhnə `due` işin gözləmə müddəti |
| `nebula_job_queue_stats_last_success_timestamp_seconds`, `..._errors_total` | kollektorun öz sağlamlığı |

Hər proses `NEBULA_JOB_METRICS_INTERVAL` (default 30 s, 5 s–10 dəq) ilə **bir** sorğu edir (partial indekslər üzərindən, timeout ≤ 5 s); scrape-lər keşlənmiş nəticədən cavablanır, ona görə scraper sayı və ya "scrape storm" bazaya çatmır. İlk uğurlu sorğuya qədər gauge-lər ixrac edilmir (yanıldıcı sıfır yoxdur). Sorğu uğursuzdursa son nəticə qalır və `NebulaJobQueueStatsStale` alerti yaşı izləyir. Bütün replikalar eyni (baza miqyaslı) dəyəri verir — alertlər `max by (instance)` istifadə edir.

`SECURITY DEFINER` funksiyası, `monitoring` sxemi və Alloy custom query-si silinir; `nebula_monitor` yalnız `pg_monitor` hüququ ilə qalır.

### 5. Ayrı DB rolları

```
nebula      (sxem sahibi)  ── yalnız `nebula migrate`  ── DDL
nebula_app  (runtime)      ── `serve`, `worker`         ── SELECT/INSERT/UPDATE/DELETE, sequence USAGE/SELECT/UPDATE
                                                          goose_db_version: yalnız SELECT
```

- `NEBULA_MIGRATE_DATABASE_URL` yalnız `migrate` üçündür, boşdursa `NEBULA_DATABASE_URL`-ə düşür (dev stack dəyişmir: bir rol).
- `NEBULA_DB_RUNTIME_ROLE` verilibsə `migrate up` sonda (bir transaction-da, idempotent) runtime roluna mövcud cədvəl/sequence-lərə hüquq verir **və** `ALTER DEFAULT PRIVILEGES` qoyur — gələcək miqrasiyaların yaratdığı obyektlər avtomatik əhatə olunur. Hər `migrate up` bunu təkrarlayır, ona görə drift (başqa rolun yaratdığı obyekt, əl ilə REVOKE) da düzəlir. Superuser və ya miqrasiya rolunun özü runtime rolu kimi rədd edilir.
- Rolun özünü (LOGIN, `NOINHERIT`, CONNECT) superuser yaradır: `deploy/production/postgres/initdb/20-nebula-runtime-role.sql` — initdb-də və hər deploy-da (idempotent) işləyir, parolu psql özü fayldan oxuyur (command line-da yox), parol olan ifadə üçün server log-u söndürülür.
- Production-da runtime rolu sxemi dəyişə bilirsə `serve`/`worker` start zamanı xəbərdarlıq yazır.
- **Testlər:** `testdb.New` artıq bütün integration testləri təsadüfi adlı `NOINHERIT` runtime rolu ilə işlədir — store-a DML-dən artıq hüquq tələb edən sorğu əlavə olunsa CI-da sınır, production-da yox. Ayrıca test DDL, `TRUNCATE`, trigger, sxem yaratmaq, yenidən grant və miqrasiya tarixçəsinə yazmağın rədd edildiyini və gələcək cədvəllərin əlavə grant olmadan işlədiyini sübut edir.

Alternativlər: (a) yalnız initdb-də default privileges — mövcud quraşdırmaları və başqa rolla yaradılan obyektləri əhatə etmir; (b) runtime roluna grant verən goose miqrasiyası — rol adı klaster səviyyəsindədir və miqrasiyada sabit yazıla bilməz. Seçilən: tətbiqin özündə idempotent grant + default privileges.

### 6. Release-də Trivy

Hər image: **digest ilə push (tag-sız)** → Trivy (`aquasecurity/trivy-action` v0.36.0, commit SHA ilə; Trivy v0.76.2) həmin digest-i skan edir → cosign imzası → provenance → yalnız sonda tag əlavə olunur və tag-in eyni digest-ə göstərdiyi yoxlanılır. Düzəlişi olan HIGH/CRITICAL (OS paketləri və kitabxanalar) release-i dayandırır; düzəlişi olmayanlar dayandırmır. Skan uğursuzdursa registry-də yalnız tag-sız, imzasız manifest qalır — deploy olunacaq heç nə yoxdur, və skan edilən bayt-bayt imzalanan və deploy olunan image-dir.

SARIF yüklənmir: release job-una `security-events: write` lazım olardı, halbuki bu job onsuz da ən çox hüquqlu job-dur (packages, id-token, attestations) və gate tapıntı saxlamır (cədvəl log-dadır). Qəbul edilmiş risk `.trivyignore`-a səbəb və bitmə tarixi ilə yazılır.

Trivy action-ın 2026-03 supply-chain insidenti (tag-ların ələ keçirilməsi) səbəbilə yalnız `v`-prefiksli, insidentdən sonra yaradılmış release-in commit SHA-sı istifadə olunur və o commit-in `master` tarixçəsində olduğu yoxlanılıb.

### 7. Postgres image-ində təhlükəsizlik yeniləmələri

Production Postgres image-i (`deploy/production/postgres/Dockerfile`) build zamanı Debian təhlükəsizlik yeniləmələrini quraşdırır və `gosu`-nu silir: konteyner həmişə `999:999` kimi işləyir, `gosu` heç vaxt istifadə olunmur, amma köhnə Go ilə yığıldığı üçün skanda düzəlişi olan CRITICAL/HIGH kimi görünürdü. Image digest ilə deploy olunduğu üçün deploy-lar təkrarlana biləndir; build isə hər release-də ən son yamaqları götürür.

### 8. İlk qəbul edilmiş risklər (`.trivyignore.yaml`)

Lokal sınaq skanı (2026-11-23, release ilə eyni Trivy versiyası və parametrlər): `nebula` təmiz; Postgres §7-dən sonra təmiz; bir istisna — ən dar əhatə (path/purl), səbəb və bitmə tarixi ilə. Bitmiş istisna gate-i yenidən sındırır və yeni qərar tələb edir.

| CVE | Harada | Niyə qəbul | Bitmə |
|---|---|---|---|
| CVE-2026-78661 (`golang.org/x/net/html` DoS) | `usr/bin/caddy` | Caddy 2.11.9 (ən son) düzəlişsiz `x/net`-lə gəlir; Caddyfile-da `templates`/markdown yoxdur — Caddy etibarsız HTML parse etmir | 2026-12-31 |

### 9. Keycloak-da guava yaması (istisna əvəzinə, 2026-11-24)

CVE-2026-102554 (Guava deserialization DoS) üçün əvvəlcə müvəqqəti istisna qoyulmuşdu: Keycloak 26.8.2 (ən son) guava 33.6.0 ilə gəlir, düzəliş isə guava ≥ 33.7.2-dədir. İstisna əvəzinə **öz Keycloak image-imizdə guava-nı 33.8.0-a yüksəldirik**:

| Qərar | Səbəb |
|---|---|
| `kc.sh build`-dən əvvəl `lib/lib/main/com.google.guava.guava-*.jar` 33.8.0-jre ilə əvəz olunur | Zəif kod bu gündən işləmir; release gate istisnasız keçir; `failureaccess` versiyası dəyişmir (1.0.3) |
| Jar Maven Central-dan `ADD --checksum=sha256:…` ilə yüklənir; pin qoyularkən SHA-1 Central ilə və PGP imzası Guava-nın release açarı ilə (`BDB5FA4F…F6D4A1D411E9D1AE`) yoxlanılıb | Təchizat zənciri: build yalnız təsdiqlənmiş fayl ilə keçir |
| Fayl adı dəyişmir | Quarkus classpath elementlərini yol ilə saxlayır; Trivy versiyanı jar-ın `pom.properties`-indən oxuyur |
| **Özü bitən yamaq**: Keycloak düzəlmiş guava (≥ 33.7.2) ilə gələndə build bilərəkdən **sınır** və yamağı silməyi tələb edir | Yamaq unudulub əbədi qalmır |
| Dependabot production Dockerfile-larını izləyir (Keycloak, Postgres, Caddy/web) | Yeni Keycloak versiyası avtomatik PR kimi gəlir |

Risk: Keycloak-ın paylanmasını dəyişmiş oluruq (uyğunluğu yoxlamaq bizim işimizdir). Yoxlama (2026-11-24, izolə edilmiş konteynerlər, dev stack-ə toxunmadan):

- Trivy 0.76.2, release parametrləri ilə, **istisnasız**: guava `33.8.0-jre` kimi tanınır, düzəlişi olan HIGH/CRITICAL zəiflik **0**.
- Yamaqlı image `start --optimized` ilə 2,4 s-də açılır; realm importu, OIDC discovery, parol ilə token (`email_verified` daxil), refresh token, userinfo, admin API (istifadəçi siyahısı) işləyir; səhv parol rədd edilir; loglarda heç bir exception yoxdur.

## Nəticələr

- Sirlər Nebula konteynerinin env-ində yoxdur; `nebula.env` ləğv edilib, sirr olmayan SMTP parametrləri `.env`-dədir.
- SQL injection və ya RCE olsa belə tətbiq sxemi dəyişə, cədvəl silə və ya miqrasiya tarixçəsini saxtalaşdıra bilməz (DML hələ də mümkündür — bu, rolların yox, tətbiqin məsuliyyətidir).
- Növbə monitorinqi tətbiqlə birlikdə versiyalanır; Postgres-də gizli kod yoxdur.
- **Uyğunsuzluq:** yeni `compose.yaml` köhnə image ilə işləmir (köhnə binary `*_FILE` oxumur). `deploy.sh`/`rollback.sh` image-i yoxlayır (`nebula healthcheck -timeout 1ms` şəbəkəsiz: yeni binary 1, köhnə 2 qaytarır), uyğunsuz image-i rədd edir və belə image-ə avtomatik rollback-i söndürür. Köhnə release-ə qayıtmaq üçün `deploy/production`-un köhnə versiyası lazımdır (runbook §9).
- İlk yeniləmə deploy-u Postgres konteynerini yenidən yaradır (yeni secret mount) — qısa DB restartı.
- Release Trivy gate-i yuxarı axın (upstream) image-lərindəki düzəlişi olan zəifliklər üzündən dayana bilər; bu, gözlənilən davranışdır: base image digest-ini yeniləyin və ya `.trivyignore`-da əsaslandırın.
