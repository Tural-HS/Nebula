<p align="center"><img src="docs/brand/logo-mark.png" alt="Nebula" width="120"></p>

# Nebula

Ümumi təyinatlı iş idarəetmə platforması (proqram təminatı, HR, satış, marketinq):
böyük həcmli məlumatlarda sürətli, tam fərdiləşdirilə bilən, öz serverində qurula bilən.

- Arxitektura: [ADR-0001](docs/adr/0001-core-architecture.md)
- Autentifikasiya və API qaydaları: [ADR-0002](docs/adr/0002-auth-and-api-conventions.md)
- Fərdi sahələr: [ADR-0003](docs/adr/0003-custom-fields.md)
- Konfiqurasiya API və rate limiting: [ADR-0004](docs/adr/0004-configuration-api-and-rate-limiting.md)
- Status keçidləri, avtomatlaşdırma, webhook-lar və email: [ADR-0005](docs/adr/0005-automation-webhooks-email.md)
- Veb interfeys: [ADR-0006](docs/adr/0006-web-ui.md)
- Əməkdaşlıq (şərhlər, @mention, aktivlik, audit, açar rotasiyası): [ADR-0007](docs/adr/0007-collaboration.md)
- Mattermost/Slack bildirişləri və etibarlı proxy-lər: [ADR-0010](docs/adr/0010-chat-notifications-and-trusted-proxies.md)
- Saxlanılan görünüşlər, cədvəl sütunları və cəm sətri: [ADR-0014](docs/adr/0014-saved-views-and-table-totals.md)
- Tətbiqdaxili bildirişlər: [ADR-0017](docs/adr/0017-in-app-notifications.md)
- Fayl əlavələri və Markdown: [ADR-0022](docs/adr/0022-attachments-markdown.md)
- Workspace üzrə axtarış: [ADR-0024](docs/adr/0024-search.md)
- İnterfeys dilləri (AZ, EN, TR, RU): [ADR-0025](docs/adr/0025-ui-languages.md)
- Production (tək VPS, Docker Compose): [ADR-0015](docs/adr/0015-production-compose-on-vps.md), runbook [deploy/production/README.md](deploy/production/README.md)
- Production hardening (fayl sirləri, ayrı DB rolları, healthcheck, növbə metrikləri, Trivy): [ADR-0018](docs/adr/0018-production-hardening.md)
- Mattermost Boards-dan keçid (azərbaycanca şablon, layihə şablonları, `.boardarchive` importu): [ADR-0020](docs/adr/0020-fintez-migration-kit.md), runbook: [docs/runbooks/mattermost-boards-migration.md](docs/runbooks/mattermost-boards-migration.md)
- Alt tapşırıqlar (bir səviyyə, DB-də qorunur): [ADR-0021](docs/adr/0021-subtasks.md)
- Təqvim, toplu və sətirdaxili redaktə: [ADR-0023](docs/adr/0023-calendar-bulk-inline.md)
- Status keçidlərində "dörd göz" təsdiqi (hazırlayan öz işini təsdiqləyə bilməz, N təsdiq): [ADR-0026](docs/adr/0026-four-eyes-approvals.md)
- İş təqvimi (həftə sonu, bayramlar, köçürmələr) və iş gününə bağlı təkrarlanma: [ADR-0027](docs/adr/0027-working-day-calendar.md)
- Tapşırıq əlaqələri və izləyicilər: [ADR-0028](docs/adr/0028-item-links-and-watchers.md)
- Təsdiqlər Mattermost/Slack kanalında (kart və "Nebula-da aç", addım 2 dizaynı): [ADR-0031](docs/adr/0031-chat-approval-cards.md)
- Audit sübut paketinin ixracı (imzalı ZIP, `nebula evidence verify`): [ADR-0029](docs/adr/0029-audit-evidence-export.md), auditor üçün runbook: [docs/runbooks/audit-evidence.md](docs/runbooks/audit-evidence.md)
- Düstur sahələri (dəqiq onluq hesab, `today()`, `workdays()`, cəm və sıralama): [ADR-0030](docs/adr/0030-formula-fields.md)
- Qəbul formaları və "Tələblərim" (yalnız daxil olmuş istifadəçilər, anonim formalar təxirə salınıb): [ADR-0032](docs/adr/0032-intake-forms.md)
- Komanda növünə görə workspace şablonları (Maliyyə, İT, HR): [ADR-0033](docs/adr/0033-workspace-kinds.md)
- Təsdiq qutusu, dəyişiklikdə yenidən təsdiq və iş günü ilə təsdiq müddətləri: [ADR-0034](docs/adr/0034-approval-inbox-reapproval-deadlines.md)
- Rəng rejimi (açıq/tünd/sistem), workspace vurğu rəngi, tapşırıq siyahısı və panelinin "Ledger" dizaynı: [ADR-0035](docs/adr/0035-design-and-theme.md)
- Nebula brendli Keycloak giriş səhifəsi (`keycloak.v2` üzərində yalnız CSS, açıq/tünd): [ADR-0036](docs/adr/0036-keycloak-login-theme.md)
- Ana səhifə "Mənim işlərim" (bütün workspace-lər üzrə sayğaclar, diqqət siyahısı, workspace kartları): [ADR-0037](docs/adr/0037-home-page.md)
- Workspace üzrə SMTP ayarları (şifrələnmiş parol, server konfiqurasiyasına ehtiyat, SSRF qoruması, sınaq məktubu): [ADR-0038](docs/adr/0038-workspace-smtp-settings.md)
- Yol xəritəsi: [docs/ROADMAP.md](docs/ROADMAP.md)

## Tələblər

- Go 1.27.4+ (`go.mod`-da sabitlənib)
- Docker və Compose v2
- Node.js 26 LTS (`web/.nvmrc`); [fnm](https://github.com/Schniz/fnm) quraşdırılıbsa Makefile onu avtomatik istifadə edir

## Sürətli başlanğıc

```bash
make up                                   # Postgres 19 + Keycloak + Mailpit + miqrasiyalar + API Docker-də
TOKEN=$(make -s token USER=alice)         # Keycloak-dan dev access token (alice və ya bob)
curl -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/me
curl -H "Authorization: Bearer $TOKEN" -d '{"slug":"acme","name":"Acme"}' localhost:8080/api/v1/workspaces
make down
```

Dev istifadəçiləri (`deploy/keycloak/nebula-realm.json`, yalnız lokal mühit üçün): `alice` / `bob`.
Keycloak admin paneli: http://localhost:8081. Göndərilən bütün email-lər: http://localhost:8025 (Mailpit).
Ardıcıl 5 səhv şifrə dev istifadəçini 60 saniyəlik bloklayır (brute-force qoruması productiondakı kimi aktivdir).

Veb interfeys (API işləyərkən):

```bash
make web-install              # asılılıqları lock faylına uyğun quraşdırır
make web-dev                  # http://localhost:5173 → Keycloak ilə giriş (alice / bob)
```

API konteyneri olmadan lokal inkişaf:

```bash
make run                      # Postgres + Keycloak + Mailpit-i başladır, miqrasiyaları tətbiq edir, API-ni go run ilə işə salır
```

## Keyfiyyət yoxlamaları

| Komanda | Nə edir |
|---|---|
| `make test` | Race detector ilə unit testlər |
| `make test-integration` | Unit + inteqrasiya testləri; hər test öz müvəqqəti Postgres bazasını alır |
| `make lint` | `go vet` + golangci-lint |
| `make vuln` | govulncheck (standart kitabxana + asılılıqlar) |
| `make web-check` | Veb: npm audit, ESLint, TypeScript, vitest, build |
| `make check` | Yuxarıdakıların hamısı: CI-nin işlətdiyi yoxlamalar |
| `make e2e` | Playwright E2E: real stack (`make up`) üzərində Chromium ilə giriş, tapşırıqlar, lövhə, şərhlər, rollar, admin ekranları |

E2E testləri hər spec üçün API ilə öz workspace-ini yaradır, ona görə mövcud dev məlumatlarına toxunmur;
test bitəndə həmin çalışmanın workspace-ləri silinir (`E2E_KEEP_DATA=1` saxlayır, `make e2e-clean` qalıqları təmizləyir).
`make e2e` işləyən dev serverdən istifadə edir; `make e2e-ci` CI kimi production build + `vite preview` ilə işləyir.
Uğursuz testlər üçün trace, screenshot və video `web/test-results`-də qalır (`npm run e2e:report`).

CI əlavə olaraq Docker image-i Trivy ilə skan edir: düzəldilə bilən HIGH/CRITICAL zəiflik build-i dayandırır.
Bütün GitHub Action-lar commit SHA-ya, base image-lər digest-ə sabitlənib; Dependabot onları həftəlik yeniləyir.

## Binar fayl

```
nebula serve            API-ni və fon worker-ini işə salır (default)
nebula worker           yalnız fon worker-ini işə salır (webhook, avtomatlaşdırma, email, bildirişlər)
nebula rotate-secrets   saxlanılan secret-ləri cari açarla yenidən şifrələyir (ADR-0007)
nebula migrate up       gözləyən miqrasiyaları tətbiq edir (advisory lock ilə, bir neçə replika üçün təhlükəsiz)
nebula migrate down     son miqrasiyanı geri qaytarır
nebula migrate status   miqrasiyaların vəziyyətini göstərir
nebula evidence verify  audit sübut paketinin imzasını və fayllarını yoxlayır; konfiqurasiya və DB tələb etmir (ADR-0029)
nebula evidence public-key  konfiqurasiya edilmiş imza açarının barmaq izini göstərir
nebula version          build versiyasını göstərir
```

## Konfiqurasiya

Bütün konfiqurasiya mühit dəyişənlərindən oxunur; bax: [.env.example](.env.example).
Yanlış konfiqurasiya zamanı proqram başlamır və bütün problemləri bir dəfədə siyahı şəklində göstərir.

**Sirlər fayldan (`*_FILE`, ADR-0018):** `NEBULA_DATABASE_URL`, `NEBULA_DATABASE_PASSWORD`, `NEBULA_MIGRATE_DATABASE_URL`, `NEBULA_MIGRATE_DATABASE_PASSWORD`, `NEBULA_ENCRYPTION_KEY`, `NEBULA_ENCRYPTION_KEYS_OLD`, `NEBULA_SMTP_PASSWORD`, `NEBULA_ATTACHMENTS_S3_ACCESS_KEY`, `NEBULA_ATTACHMENTS_S3_SECRET_KEY` dəyişənlərinin hər biri `..._FILE` kimi də verilə bilər (məs. `NEBULA_ENCRYPTION_KEY_FILE=/run/secrets/nebula_encryption_key`). Faylın sonundakı bir sətir sonu silinir. İkisi birlikdə, oxuna bilməyən və ya 64 KiB-dan böyük fayl → proqram başlamır. Xəta mesajlarında dəyər heç vaxt göstərilmir.

| Dəyişən | Default | Qeyd |
|---|---|---|
| `NEBULA_DATABASE_URL` | `serve`/`worker` üçün məcburi | `postgres://...`; productionda DML-only runtime rolu (ADR-0018) |
| `NEBULA_DATABASE_PASSWORD` | boş | Parolu ayrıca verir (URL-də parol olmamalıdır); adətən `_FILE` ilə |
| `NEBULA_MIGRATE_DATABASE_URL` / `_PASSWORD` | `NEBULA_DATABASE_URL` | Yalnız `nebula migrate`: sxemin sahibi rolu (DDL) |
| `NEBULA_DB_RUNTIME_ROLE` | boş | Verilibsə `migrate up` bu rola bütün cədvəl/sequence-lərdə DML (DDL yox) və gələcək obyektlər üçün default privileges verir |
| `NEBULA_JOB_METRICS_INTERVAL` | `30s` | Növbə metriklərinin (`nebula_job_queue_*`) bazadan yenilənmə intervalı, 5s–10m |
| `NEBULA_ENCRYPTION_KEY` | `serve`/`worker` üçün məcburi | 32 bayt base64 (`openssl rand -base64 32`); secret store-da saxlayın |
| `NEBULA_ENCRYPTION_KEYS_OLD` | boş | Rotasiya zamanı köhnə açarlar (vergüllə), yalnız oxumaq üçün |
| `NEBULA_OIDC_ISSUER` | `serve` üçün məcburi | Token-in `iss` dəyəri ilə tam eyni olmalıdır; productionda https |
| `NEBULA_OIDC_AUDIENCE` | `serve` üçün məcburi | Token-in `aud` dəyərində olmalıdır |
| `NEBULA_OIDC_JWKS_URL` | discovery | Açarlar üçün daxili ünvan (istəyə bağlı); productionda https |
| `NEBULA_ENV` | `production` | `development` və ya `production` |
| `NEBULA_HTTP_ADDR` | `:8080` | İctimai API |
| `NEBULA_ADMIN_ADDR` | `:9090` | Metriklər; heç vaxt ictimaiyyətə açmayın |
| `NEBULA_DB_MAX_CONNS` | `20` | 1-dən 500-ə qədər |
| `NEBULA_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `NEBULA_LOG_FORMAT` | `json` | `json` və ya `text` |
| `NEBULA_SHUTDOWN_TIMEOUT` | `25s` | Orkestratorun grace period-undan az olmalıdır (K8s default 30s) |
| `NEBULA_REQUEST_TIMEOUT` | `30s` | Hər sorğu üçün son müddət |
| `NEBULA_RATE_LIMIT_USER_RPS` / `_BURST` | `20` / `40` | İstifadəçi başına, hər replika üçün; `0` söndürür |
| `NEBULA_RATE_LIMIT_TOKEN_RPS` / `_BURST` | `5` / `20` | API tokeni başına (ADR-0043), istifadəçi limitinə əlavə olaraq; `0` söndürür |
| `NEBULA_RATE_LIMIT_IP_RPS` / `_BURST` | `100` / `200` | IP başına, autentifikasiyadan əvvəl; reverse proxy arxasında `NEBULA_TRUSTED_PROXIES`-i təyin edin |
| `NEBULA_TRUSTED_PROXIES` | boş | Reverse proxy-lərin CIDR-ləri və ya IP-ləri (vergüllə, məs. `10.0.0.0/8,fd00::/8`). Yalnız bu ünvanlardan gələn sorğularda `X-Forwarded-For` nəzərə alınır (ADR-0010). Səhv CIDR → işə düşmür |
| `NEBULA_PUBLIC_URL` | boş (keçid yoxdur) | Veb interfeysin ünvanı (`https://nebula.example.com`); Mattermost/Slack mesajlarında tapşırıq keçidləri üçün. `http` yalnız development-də |
| `NEBULA_WORKER_CONCURRENCY` | `4` | Prosesdəki paralel fon işləri; `0` → `serve` daxilində worker yoxdur |
| `NEBULA_SMTP_HOST` | boş (email söndürülüb) | SMTP serveri |
| `NEBULA_SMTP_PORT` / `_TLS` | `587` / `starttls` | `starttls`, `tls` və ya `none` (`none` productionda qadağandır) |
| `NEBULA_SMTP_USERNAME` / `_PASSWORD` / `_FROM` | — | `FROM` email aktiv olduqda məcburidir |
| `NEBULA_WEBHOOK_ALLOW_PRIVATE` | `false` | Daxili ünvanlara webhook; **yalnız development**, productionda qadağandır (SSRF) |
| `NEBULA_SMTP_WORKSPACE_ALLOW_PRIVATE` | `false` | Workspace SMTP serveri daxili/loopback ünvanda ola bilər; **yalnız development** (SSRF, ADR-0038) |
| `NEBULA_SMTP_WORKSPACE_ALLOW_PLAINTEXT` | `false` | Workspace SMTP üçün `tls_mode=none`; **yalnız development** (ADR-0038) |
| `NEBULA_SMTP_WORKSPACE_PORTS` | `25,465,587,2525` | Workspace SMTP serverinə icazəli portlar (ADR-0038) |
| `NEBULA_ATTACHMENTS_BACKEND` | `local` | `local`, `s3` və ya `disabled` (ADR-0022) |
| `NEBULA_ATTACHMENTS_DIR` | `/var/lib/nebula/attachments` | `local`: davamlı volume; startda yazıla bilən olmalıdır. **pgBackRest bunu saxlamır** (runbook §4.1) |
| `NEBULA_ATTACHMENTS_S3_ENDPOINT` / `_BUCKET` / `_REGION` / `_PREFIX` / `_PATH_STYLE` | — / — / `us-east-1` / boş / `true` | `s3`: istənilən S3-uyğun server; productionda https |
| `NEBULA_ATTACHMENTS_S3_ACCESS_KEY` / `_SECRET_KEY` | — | `s3`: adətən `_FILE` ilə |
| `NEBULA_ATTACHMENTS_TMP_DIR` | sistem temp | `s3`: yükləmə yoxlanılarkən staging fayllar |
| `NEBULA_ATTACHMENTS_MAX_FILE_MB` | `20` | Fayl başına limit (reverse proxy body limitindən kiçik olmalıdır) |
| `NEBULA_ATTACHMENTS_WORKSPACE_QUOTA_MB` | `5120` | Workspace başına cəmi; `0` limitsiz |
| `NEBULA_ATTACHMENTS_MAX_PER_ITEM` | `100` | Tapşırıq başına fayl sayı; `0` limitsiz |
| `NEBULA_ATTACHMENTS_UPLOADS_PER_MINUTE` | `30` | İstifadəçi başına, replika başına; `0` söndürür |
| `NEBULA_ATTACHMENTS_TRANSFER_TIMEOUT` | `5m` | Bir yükləmə/endirmə üçün son müddət (API timeout-u əvəzinə) |
| `NEBULA_ATTACHMENTS_PURGE_DELAY` | `168h` | Silinmiş faylın blob-u bu qədər saxlanılır (PITR + fayl backup-ı uyğunluğu) |

## Endpoint-lər

Bütün `/api/v1` endpoint-ləri (`/version` istisna olmaqla) `Authorization: Bearer <token>` tələb edir.
`{ws}` workspace slug-ıdır, `{KEY}` layihə açarıdır (məs. `OPS`).

| Yol | Təyinat | Rol |
|---|---|---|
| `GET /healthz` | Liveness: proses işləyir (asılılıqları yoxlamır) | — |
| `GET /readyz` | Readiness: baza əlçatandır | — |
| `GET /api/v1/version` | Build versiyası | ictimai |
| `GET /api/v1/me` | Cari istifadəçi | — |
| `GET /api/v1/me/home?show=attention\|overdue\|approval\|week\|assigned&limit=1..200` | Ana səhifə: sayğaclar, siyahı (`items`, `total`), workspace kartları, izlədiyim işlər, son layihələr, admin seçdiyi keçidlər (`pinned`, ADR-0053) — yalnız görünən layihələr (ADR-0037) | — |
| `GET, POST /api/v1/me/notes`, `PATCH, DELETE /api/v1/me/notes/{id}` | Şəxsi qeydlər (ana səhifə, ADR-0052): yalnız müəllif görür; `body` 1–2000, `color`, `pinned`; ən çox 100 | — |
| `GET, POST /api/v1/workspaces` | Mənim workspace-lərim / yaratmaq (yaradan owner olur) | — |
| `GET /api/v1/workspaces/{ws}` | Workspace və mənim rolum | guest+ |
| `PATCH /api/v1/workspaces/{ws}` | Ad, saat qurşağı, xülasə saatı, vurğu rəngi, ana səhifə keçidləri (`home_projects`, ≤ 8 aktiv layihə id-si, ADR-0053) | admin+ |
| `GET, POST /api/v1/workspaces/{ws}/members` | Üzvlər / əlavə etmək | guest+ / admin+ |
| `PATCH /api/v1/workspaces/{ws}/members/{userID}` | Rolu dəyişmək (sahib rolu yalnız sahib tərəfindən; son sahib qorunur) | admin+ |
| `DELETE /api/v1/workspaces/{ws}/members/{userID}` | Üzvü çıxarmaq / özü tərk etmək | admin+ / özü |
| `GET, POST .../{ws}/projects` | Siyahı / yaratmaq | guest+ / admin+ |
| `GET .../{ws}/item-types` | Item tipləri, statusları və bağlı sahələri ilə | guest+ |
| `POST .../{ws}/item-types` | Item tipi yaratmaq | admin+ |
| `PATCH, DELETE .../{ws}/item-types/{type}` | Ad/ikon dəyişmək / arxivləmək | admin+ |
| `PUT, DELETE .../{ws}/item-types/{type}/fields/{field}` | Sahəni tipə bağlamaq / ayırmaq | admin+ |
| `GET, POST .../{ws}/fields` | Fərdi sahə tərifləri | guest+ / admin+ |
| `GET, PATCH .../{ws}/fields/{field}` | Sahəni oxumaq / dəyişmək, arxivləmək | guest+ / admin+ |
| `GET, POST .../{ws}/workflows` | Workflow-lar / yaratmaq | guest+ / admin+ |
| `PATCH .../{ws}/workflows/{id}` | Ad, `enforce_transitions` | admin+ |
| `POST, DELETE .../{ws}/workflows/{id}/transitions[/{tid}]` | Status keçidi əlavə etmək / silmək | admin+ |
| `POST .../{ws}/workflows/{id}/statuses` | Status əlavə etmək | admin+ |
| `PATCH, DELETE .../{ws}/workflows/{id}/statuses/{statusID}` | Status dəyişmək / silmək | admin+ |
| `GET .../projects/{KEY}/items` | Siyahı: `limit`, `cursor`, `q`, `status`, `assignee=me\|none\|{id}`, `field.<açar>=<dəyər>`, `due`, `sort=number\|due` (kursor sıralamaya bağlıdır), `parent=<nömrə\|açar>`, `top_level=true` (ADR-0021), `approval=pending\|mine` (ADR-0026) | guest+ |
| `GET .../projects/{KEY}/items/sums?fields=a,b` | Rəqəm sahələrinin bütün filtrlənmiş nəticə üzrə cəmi və sayı, üstəlik `count` — uyğun tapşırıqların sayı (`fields` boş ola bilər; siyahının eyni filtrləri, ən çox 10 sahə) | guest+ |
| `GET, POST .../projects/{KEY}/views` | Saxlanılan görünüşlər (öz şəxsiləri + ümumilər) / yaratmaq (`shared: true` admin+) | guest+ |
| `GET, PATCH, DELETE .../projects/{KEY}/views/{id}` | Oxumaq / dəyişmək / silmək (şəxsi: sahibi; ümumi: sahibi və ya admin+) | guest+ |
| `GET, PUT .../projects/{KEY}/cycle` | Aylıq dövr (ADR-0051): `enabled`, `day_of_month` 1–28, `clear_fields`, `refill` (`target` ← `source`), `include_closed`, `reset_status_id` | member+ / admin+ |
| `GET, PUT .../projects/{KEY}/date-reminder` | Ad günü xatırlatması (ADR-0054): `field` (tarix sahəsinin açarı və ya `null`); cavabda `last_sent`, `hour` | member+ / admin+ |
| `POST .../projects/{KEY}/cycle/preview`, `POST .../cycle/close` | Ayı bağlamağın önizləməsi / əl ilə bağlamaq (`period`: `YYYY-MM`, boş = keçən ay) | admin+ |
| `GET .../projects/{KEY}/cycle/closes[/{YYYY-MM}[/export?format=xlsx\|csv]]` | Bağlanmış aylar / bir ayın snapshot-ı / Excel və ya CSV kimi | member+ |
| `POST .../projects/{KEY}/items` | Yaratmaq; `parent` (nömrə və ya açar) ilə alt tapşırıq. `PATCH`-da `parent: null` ayırır | member+ |
| `POST .../projects/{KEY}/imports/preview` | CSV/xlsx önbaxışı (multipart `file`, `sheet`): vərəqlər, başlıqlar, ilk 20 sətir, təklif olunan uyğunlaşdırma | admin+ |
| `POST .../projects/{KEY}/imports[?dry_run=true]` | İdxal (multipart `file`, `sheet`, `mapping`): hamısı və ya heç nə; `dry_run` yalnız yoxlayır | admin+ |
| `GET, PATCH, DELETE .../items/{number}` | Oxumaq / `version` ilə qismən yeniləmək / soft delete | guest+ / member+ |
| `GET .../items/{number}/transitions` | Növbəti mümkün statuslar (`requires_approval`: təsdiq tələb edir, ADR-0026) | guest+ |
| `GET, POST .../items/{number}/approvals` | Təsdiq sorğuları / qorunan statusa keçid üçün sorğu (`to_status_id`, `version`, `comment`) | guest+ / member+ |
| `POST .../items/{number}/approvals/{id}/approve\|reject\|cancel` | Təsdiqlə (`comment`), rədd et (`reason` məcburi), ləğv et (sorğunu göndərən və ya admin); `on_behalf_of`: məzuniyyətdə olan təsdiqçinin adına (ADR-0042) | member+ |
| `PUT, DELETE .../{ws}/workflows/{id}/statuses/{sid}/approval-policy` | Statusa daxil olmaq üçün təsdiq siyasəti (`min_approvals`, `approver_roles`, `approver_user_ids`; ADR-0034: `reapprove_on_change`, `reapprove_fields`, `due_in_working_days`) | admin+ |
| `GET .../{ws}/approvals?state=pending&mine=true\|false` | Workspace üzrə gözləyən təsdiq sorğuları (yalnız görünən layihələr; `mine`: qərar verə biləcəkləriniz), `limit`, `cursor` (ADR-0034) | guest+ |
| `GET .../{ws}/approvals/count?mine=true` | Nişan: `count` (ən çox 100), `overdue` (ADR-0034) | guest+ |
| `GET, POST .../{ws}/absences` | Cari və gələcək məzuniyyətlər (`user_id` filtri; qeyd yalnız özünə, əvəzləyənə və adminə) / məzuniyyət qeyd etmək (`starts_on`, `ends_on` workspace tarixi, `delegate_id`, `note`; başqası üçün `user_id` yalnız admin) (ADR-0042) | member+ |
| `PATCH .../{ws}/absences/{id}`, `POST .../{id}/end` | Əvəzləyəni, tarixləri, qeydi dəyişmək / vaxtından əvvəl bitirmək və ya ləğv etmək (özü və ya admin) | member+ |
| `GET, POST .../items/{number}/comments` | Şərhlər (`cursor`, `limit`) / yazmaq; `<@user-id>` ilə qeydetmə | guest+ / member+ |
| `PATCH, DELETE .../items/{number}/comments/{id}` | Redaktə (yalnız müəllif) / silmə (müəllif və ya admin) | member+ |
| `GET, POST .../items/{number}/attachments` | Fayllar və limitlər / yükləmə (`multipart/form-data`: `file`, istəyə bağlı `comment_id`) | guest+ / member+ |
| `GET .../items/{number}/attachments/{id}/content` | Endirmə (həmişə `attachment`; `?inline=1` yalnız PNG/JPEG/GIF/WebP) | guest+ |
| `DELETE .../items/{number}/attachments/{id}` | Silmə (yükləyən və ya admin) | member+ |
| `GET .../items/{number}/activity` | Tapşırığın tarixçəsi (`before`, `limit`) | guest+ |
| `GET .../{ws}/audit-log` | Workspace audit log (`before`, `limit`, `action`, `actor`) | admin+ |
| `POST .../{ws}/webhooks/{id}/ping` | Test çatdırılması | admin+ |
| `GET, PUT, DELETE .../{ws}/email-settings` | Workspace SMTP ayarları: parol heç vaxt qaytarılmır (`password_set`); `PUT`-da `password` yoxdur = saxla, `null` = sil; `DELETE` server konfiqurasiyasına qaytarır (ADR-0038) | admin+ |
| `POST .../{ws}/email-settings/test` | Sınaq məktubu (`{to?}`: standart öz təsdiqlənmiş emailiniz, başqa ünvan yalnız üzvün emaili; 5 / 10 dəq) | admin+ |
| `GET, POST .../{ws}/service-tokens` | API tokenləri (inteqrasiyalar) / yaratmaq (`name`, `role`: `guest`/`member`/`admin`, `scopes`, `project_ids`, `expires_in_days` 1–365); token yalnız yaradılanda bir dəfə qaytarılır (ADR-0043) | admin+ |
| `DELETE .../{ws}/service-tokens/{id}` | Tokeni dərhal ləğv etmək | admin+ |
| `GET, POST .../{ws}/webhooks` | Webhook-lar / yaratmaq (`format`: `nebula` və ya `slack`; secret yalnız bir dəfə qaytarılır, `slack` üçün `null`) | admin+ |
| `GET, PATCH, DELETE .../{ws}/webhooks/{id}` | Oxumaq / dəyişmək / silmək | admin+ |
| `POST .../{ws}/webhooks/{id}/rotate-secret` | Yeni secret | admin+ |
| `GET .../{ws}/webhooks/{id}/deliveries` | Son çatdırılma cəhdləri | admin+ |
| `GET, POST .../{ws}/automations` | Avtomatlaşdırma qaydaları / yaratmaq | admin+ |
| `GET, PATCH, DELETE .../{ws}/automations/{id}` | Oxumaq / dəyişmək / silmək | admin+ |
| `GET :9090/metrics` | Prometheus metrikləri, route şablonuna görə etiketlənir | daxili |

## API tokenləri (inteqrasiyalar)

1C, bank çıxarışı skriptləri, ERP və BI alətləri üçün workspace adminləri **Ayarlar → API tokenləri**-ndə token yaradır
(ətraflı: [ADR-0043](docs/adr/0043-workspace-api-tokens.md)). Token bir workspace-ə bağlıdır, öz rolu (`guest`, `member`,
`admin` — sahib heç vaxt), icazələri (`items:read`, `items:write`, `comments:write`, `reports:read`, `export:read`), istəyə
bağlı layihə siyahısı və məcburi müddəti (30/90/365 gün, ən çox 1 il) var. Tarixçədə öz adı ilə görünür ("🔑 1C inteqrasiyası").

- Token formatı `nbl_<32 hex>_<64 hex>`; **yalnız yaradılanda bir dəfə göstərilir**, serverdə yalnız SHA-256 həşi saxlanılır.
  Onu secret store-da (OpenBao, Key Vault, CI secret) saxlayın, koda və ya loga yazmayın.
- Token üzvləri, dəvətləri, ayarları, SMTP-ni, webhook-ları, arxiv/silməni və token idarəsini **heç vaxt** görmür; yalnız
  tapşırıqlar, şərhlər, hesabatlar və ixrac marşrutları açıqdır. Ləğv dərhal işləyir; müddəti bitmiş token `401 token_expired`,
  ləğv edilmiş `401 token_revoked`, arxivlənmiş workspace-in tokeni `401 token_workspace_archived` qaytarır.
- Limit token başına (`NEBULA_RATE_LIMIT_TOKEN_*`, standart 5 sorğu/s, 20 ardıcıl); `429`-da `Retry-After`-ə əməl edin.

```sh
export NEBULA_TOKEN='nbl_<id>_<secret>'   # secret store-dan oxuyun

# Tapşırıqları oxumaq (items:read)
curl -sS -H "Authorization: Bearer $NEBULA_TOKEN" \
  https://nebula.example.com/api/v1/workspaces/maliyye/projects/FIN/items?limit=50

# Tapşırıq yaratmaq (items:write)
curl -sS -X POST -H "Authorization: Bearer $NEBULA_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title": "Bank çıxarışı: 2026-11-24", "due_date": "2026-11-28"}' \
  https://nebula.example.com/api/v1/workspaces/maliyye/projects/FIN/items
```

## Cədvəldən idxal (CSV / Excel)

Layihə səhifəsində **İdxal et** (yalnız admin): fayl → vərəq və sütun uyğunlaşdırması → yoxlama (dry run) →
idxal. Qısa qaydalar (ətraflı: [ADR-0011](docs/adr/0011-spreadsheet-import.md)):

- Formatlar: `.xlsx` və CSV (UTF-8, ayırıcı `,` `;` və ya tab). Köhnə `.xls` üçün Excel-də "Save As → .xlsx"
  və ya "CSV UTF-8" seçin. Limitlər: 10 MB, 20 000 sətir, 200 sütun.
- Birinci boş olmayan sətir başlıqdır. Başlıq üçün bir sütun mütləqdir; status workflow-dakı adı ilə,
  icraçı workspace üzvünün e-poçtu ilə, seçim sahələri etiketlə yazılır.
- Məbləğlər: `1 250,50 ₼`, `1.250,50`, `1250.5` qəbul edilir; `1.250` kimi qeyri-müəyyən yazılış xəta verir.
  Tarixlər: `15.03.2026`, `2026-03-15` və Excel tarix xanaları.
- Hər hansı sətirdə xəta varsa heç nə idxal olunmur; yoxlama hesabatı sətir nömrəsi və sütunla xətaları göstərir.
- Fayl serverdə yalnız yaddaşda oxunur, heç yerdə saxlanılmır; idxal audit log-a `project.imported` kimi düşür.

API nümunəsi:

```bash
curl -fsS -H "Authorization: Bearer $(make -s token)" \
  -F file=@musteriler.csv \
  -F 'mapping={"columns":[{"column":0,"target":"title"},{"column":4,"target":"field.monthly"}],"default_type":"task"}' \
  'http://localhost:8080/api/v1/workspaces/acme/projects/ACME/imports?dry_run=true'
```

## Production qeydləri

- Buraxılış qeydləri: [docs/releases/](docs/releases/) (son: [v0.2.0](docs/releases/v0.2.0.md), ilk: [v0.1.0](docs/releases/v0.1.0.md)).
- Image: distroless Debian 13, root olmayan istifadəçi (UID 65532), ~25 MB. Read-only root FS ilə və bütün capability-lər söndürülmüş halda işlədin.
- `nebula migrate up`-ı API yenilənməsindən əvvəl ayrıca job kimi işlədin (Compose: `migrate` servisi, `deploy/production`; K8s Job / Argo CD PreSync hook).
- Hər miqrasiyanın işləyən `Down` hissəsi olmalı və əvvəlki relizin işləməsinə mane olmamalıdır (expand/contract).
- Rate limit hər replikada ayrıca saxlanılır: N replika ilə faktiki limit N dəfə çoxdur.
- Webhook, avtomatlaşdırma və email işləri PostgreSQL növbəsindədir (transactional outbox). Böyük yükdə `nebula worker`-i ayrıca deployment kimi miqyaslayın.
- Webhook qəbul edənlər `Nebula-Signature`-i yoxlamalı və `Nebula-Event-Id` ilə dublikatları ayırmalıdır (ADR-0005).
- Mattermost/Slack webhook-u (`format: "slack"`) imzalanmır: URL özü açardır, şifrələnmiş saxlanılır və API-də maskalanır. URL sızıbsa, Mattermost-da yenisini yaradıb webhook-un URL-ini dəyişin (ADR-0010).
- Reverse proxy (Caddy, ingress) arxasında `NEBULA_TRUSTED_PROXIES`-ə **yalnız** proxy-nin ünvanlarını yazın; API-yə proxy-dən yan keçən birbaşa giriş olmamalıdır (NetworkPolicy).
