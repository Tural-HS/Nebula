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
| `NEBULA_RATE_LIMIT_IP_RPS` / `_BURST` | `100` / `200` | IP başına, autentifikasiyadan əvvəl; reverse proxy arxasında limiti ingress-də tətbiq edin |
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
| `GET .../projects/{KEY}/items` | Siyahı: `limit`, `cursor`, `q`, `status`, `assignee=me\|none\|{id}`, `field.<açar>=<dəyər>` | guest+ |
| `POST .../projects/{KEY}/items` | Yaratmaq | member+ |
| `GET, PATCH, DELETE .../items/{number}` | Oxumaq / `version` ilə qismən yeniləmək / soft delete | guest+ / member+ |
| `GET .../items/{number}/transitions` | Növbəti mümkün statuslar | guest+ |
| `GET, POST .../items/{number}/comments` | Şərhlər (`cursor`, `limit`) / yazmaq; `<@user-id>` ilə qeydetmə | guest+ / member+ |
| `PATCH, DELETE .../items/{number}/comments/{id}` | Redaktə (yalnız müəllif) / silmə (müəllif və ya admin) | member+ |
| `GET .../items/{number}/activity` | Tapşırığın tarixçəsi (`before`, `limit`) | guest+ |
| `GET .../{ws}/audit-log` | Workspace audit log (`before`, `limit`, `action`, `actor`) | admin+ |
| `POST .../{ws}/webhooks/{id}/ping` | Test çatdırılması | admin+ |
| `GET, POST .../{ws}/webhooks` | Webhook-lar / yaratmaq (secret yalnız bir dəfə qaytarılır) | admin+ |
| `GET, PATCH, DELETE .../{ws}/webhooks/{id}` | Oxumaq / dəyişmək / silmək | admin+ |
| `POST .../{ws}/webhooks/{id}/rotate-secret` | Yeni secret | admin+ |
| `GET .../{ws}/webhooks/{id}/deliveries` | Son çatdırılma cəhdləri | admin+ |
| `GET, POST .../{ws}/automations` | Avtomatlaşdırma qaydaları / yaratmaq | admin+ |
| `GET, PATCH, DELETE .../{ws}/automations/{id}` | Oxumaq / dəyişmək / silmək | admin+ |
| `GET :9090/metrics` | Prometheus metrikləri, route şablonuna görə etiketlənir | daxili |

## Production qeydləri

- Image: distroless Debian 13, root olmayan istifadəçi (UID 65532), ~25 MB. Read-only root FS ilə və bütün capability-lər söndürülmüş halda işlədin.
- `nebula migrate up`-ı API yenilənməsindən əvvəl ayrıca job kimi işlədin (K8s Job / Argo CD PreSync hook).
- Hər miqrasiyanın işləyən `Down` hissəsi olmalı və əvvəlki relizin işləməsinə mane olmamalıdır (expand/contract).
- Rate limit hər replikada ayrıca saxlanılır: N replika ilə faktiki limit N dəfə çoxdur.
- Webhook, avtomatlaşdırma və email işləri PostgreSQL növbəsindədir (transactional outbox). Böyük yükdə `nebula worker`-i ayrıca deployment kimi miqyaslayın.
- Webhook qəbul edənlər `Nebula-Signature`-i yoxlamalı və `Nebula-Event-Id` ilə dublikatları ayırmalıdır (ADR-0005).
