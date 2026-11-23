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
- Production (tək VPS, Docker Compose): [ADR-0015](docs/adr/0015-production-compose-on-vps.md), runbook [deploy/production/README.md](deploy/production/README.md)
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
nebula worker           yalnız fon worker-ini işə salır (webhook, avtomatlaşdırma, email)
nebula rotate-secrets   saxlanılan secret-ləri cari açarla yenidən şifrələyir (ADR-0007)
nebula migrate up       gözləyən miqrasiyaları tətbiq edir (advisory lock ilə, bir neçə replika üçün təhlükəsiz)
nebula migrate down     son miqrasiyanı geri qaytarır
nebula migrate status   miqrasiyaların vəziyyətini göstərir
nebula version          build versiyasını göstərir
```

## Konfiqurasiya

Bütün konfiqurasiya mühit dəyişənlərindən oxunur; bax: [.env.example](.env.example).
Yanlış konfiqurasiya zamanı proqram başlamır və bütün problemləri bir dəfədə siyahı şəklində göstərir.

| Dəyişən | Default | Qeyd |
|---|---|---|
| `NEBULA_DATABASE_URL` | məcburi | `postgres://...` |
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
| `NEBULA_RATE_LIMIT_IP_RPS` / `_BURST` | `100` / `200` | IP başına, autentifikasiyadan əvvəl; reverse proxy arxasında `NEBULA_TRUSTED_PROXIES`-i təyin edin |
| `NEBULA_TRUSTED_PROXIES` | boş | Reverse proxy-lərin CIDR-ləri və ya IP-ləri (vergüllə, məs. `10.0.0.0/8,fd00::/8`). Yalnız bu ünvanlardan gələn sorğularda `X-Forwarded-For` nəzərə alınır (ADR-0010). Səhv CIDR → işə düşmür |
| `NEBULA_PUBLIC_URL` | boş (keçid yoxdur) | Veb interfeysin ünvanı (`https://nebula.example.com`); Mattermost/Slack mesajlarında tapşırıq keçidləri üçün. `http` yalnız development-də |
| `NEBULA_WORKER_CONCURRENCY` | `4` | Prosesdəki paralel fon işləri; `0` → `serve` daxilində worker yoxdur |
| `NEBULA_SMTP_HOST` | boş (email söndürülüb) | SMTP serveri |
| `NEBULA_SMTP_PORT` / `_TLS` | `587` / `starttls` | `starttls`, `tls` və ya `none` (`none` productionda qadağandır) |
| `NEBULA_SMTP_USERNAME` / `_PASSWORD` / `_FROM` | — | `FROM` email aktiv olduqda məcburidir |
| `NEBULA_WEBHOOK_ALLOW_PRIVATE` | `false` | Daxili ünvanlara webhook; **yalnız development**, productionda qadağandır (SSRF) |

## Endpoint-lər

Bütün `/api/v1` endpoint-ləri (`/version` istisna olmaqla) `Authorization: Bearer <token>` tələb edir.
`{ws}` workspace slug-ıdır, `{KEY}` layihə açarıdır (məs. `OPS`).

| Yol | Təyinat | Rol |
|---|---|---|
| `GET /healthz` | Liveness: proses işləyir (asılılıqları yoxlamır) | — |
| `GET /readyz` | Readiness: baza əlçatandır | — |
| `GET /api/v1/version` | Build versiyası | ictimai |
| `GET /api/v1/me` | Cari istifadəçi | — |
| `GET, POST /api/v1/workspaces` | Mənim workspace-lərim / yaratmaq (yaradan owner olur) | — |
| `GET /api/v1/workspaces/{ws}` | Workspace və mənim rolum | guest+ |
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
| `GET .../projects/{KEY}/items` | Siyahı: `limit`, `cursor`, `q`, `status`, `assignee=me\|none\|{id}`, `field.<açar>=<dəyər>`, `due`, `sort=number\|due` (kursor sıralamaya bağlıdır) | guest+ |
| `GET .../projects/{KEY}/items/sums?fields=a,b` | Rəqəm sahələrinin bütün filtrlənmiş nəticə üzrə cəmi və sayı (siyahının eyni filtrləri, ən çox 10 sahə) | guest+ |
| `GET, POST .../projects/{KEY}/views` | Saxlanılan görünüşlər (öz şəxsiləri + ümumilər) / yaratmaq (`shared: true` admin+) | guest+ |
| `GET, PATCH, DELETE .../projects/{KEY}/views/{id}` | Oxumaq / dəyişmək / silmək (şəxsi: sahibi; ümumi: sahibi və ya admin+) | guest+ |
| `POST .../projects/{KEY}/items` | Yaratmaq | member+ |
| `POST .../projects/{KEY}/imports/preview` | CSV/xlsx önbaxışı (multipart `file`, `sheet`): vərəqlər, başlıqlar, ilk 20 sətir, təklif olunan uyğunlaşdırma | admin+ |
| `POST .../projects/{KEY}/imports[?dry_run=true]` | İdxal (multipart `file`, `sheet`, `mapping`): hamısı və ya heç nə; `dry_run` yalnız yoxlayır | admin+ |
| `GET, PATCH, DELETE .../items/{number}` | Oxumaq / `version` ilə qismən yeniləmək / soft delete | guest+ / member+ |
| `GET .../items/{number}/transitions` | Növbəti mümkün statuslar | guest+ |
| `GET, POST .../items/{number}/comments` | Şərhlər (`cursor`, `limit`) / yazmaq; `<@user-id>` ilə qeydetmə | guest+ / member+ |
| `PATCH, DELETE .../items/{number}/comments/{id}` | Redaktə (yalnız müəllif) / silmə (müəllif və ya admin) | member+ |
| `GET .../items/{number}/activity` | Tapşırığın tarixçəsi (`before`, `limit`) | guest+ |
| `GET .../{ws}/audit-log` | Workspace audit log (`before`, `limit`, `action`, `actor`) | admin+ |
| `POST .../{ws}/webhooks/{id}/ping` | Test çatdırılması | admin+ |
| `GET, POST .../{ws}/webhooks` | Webhook-lar / yaratmaq (`format`: `nebula` və ya `slack`; secret yalnız bir dəfə qaytarılır, `slack` üçün `null`) | admin+ |
| `GET, PATCH, DELETE .../{ws}/webhooks/{id}` | Oxumaq / dəyişmək / silmək | admin+ |
| `POST .../{ws}/webhooks/{id}/rotate-secret` | Yeni secret | admin+ |
| `GET .../{ws}/webhooks/{id}/deliveries` | Son çatdırılma cəhdləri | admin+ |
| `GET, POST .../{ws}/automations` | Avtomatlaşdırma qaydaları / yaratmaq | admin+ |
| `GET, PATCH, DELETE .../{ws}/automations/{id}` | Oxumaq / dəyişmək / silmək | admin+ |
| `GET :9090/metrics` | Prometheus metrikləri, route şablonuna görə etiketlənir | daxili |

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

- Image: distroless Debian 13, root olmayan istifadəçi (UID 65532), ~25 MB. Read-only root FS ilə və bütün capability-lər söndürülmüş halda işlədin.
- `nebula migrate up`-ı API yenilənməsindən əvvəl ayrıca job kimi işlədin (Compose: `migrate` servisi, `deploy/production`; K8s Job / Argo CD PreSync hook).
- Hər miqrasiyanın işləyən `Down` hissəsi olmalı və əvvəlki relizin işləməsinə mane olmamalıdır (expand/contract).
- Rate limit hər replikada ayrıca saxlanılır: N replika ilə faktiki limit N dəfə çoxdur.
- Webhook, avtomatlaşdırma və email işləri PostgreSQL növbəsindədir (transactional outbox). Böyük yükdə `nebula worker`-i ayrıca deployment kimi miqyaslayın.
- Webhook qəbul edənlər `Nebula-Signature`-i yoxlamalı və `Nebula-Event-Id` ilə dublikatları ayırmalıdır (ADR-0005).
- Mattermost/Slack webhook-u (`format: "slack"`) imzalanmır: URL özü açardır, şifrələnmiş saxlanılır və API-də maskalanır. URL sızıbsa, Mattermost-da yenisini yaradıb webhook-un URL-ini dəyişin (ADR-0010).
- Reverse proxy (Caddy, ingress) arxasında `NEBULA_TRUSTED_PROXIES`-ə **yalnız** proxy-nin ünvanlarını yazın; API-yə proxy-dən yan keçən birbaşa giriş olmamalıdır (NetworkPolicy).
