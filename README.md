# Nebula

Ümumi təyinatlı iş idarəetmə platforması (proqram təminatı, HR, satış, marketinq):
böyük həcmli məlumatlarda sürətli, tam fərdiləşdirilə bilən, öz serverində qurula bilən.

- Arxitektura: [ADR-0001](docs/adr/0001-core-architecture.md)
- Autentifikasiya və API qaydaları: [ADR-0002](docs/adr/0002-auth-and-api-conventions.md)
- Fərdi sahələr: [ADR-0003](docs/adr/0003-custom-fields.md)
- Konfiqurasiya API və rate limiting: [ADR-0004](docs/adr/0004-configuration-api-and-rate-limiting.md)
- Yol xəritəsi: [docs/ROADMAP.md](docs/ROADMAP.md)

## Tələblər

- Go 1.27.4+ (`go.mod`-da sabitlənib)
- Docker və Compose v2

## Sürətli başlanğıc

```bash
make up                                   # Postgres 19 + Keycloak + miqrasiyalar + API Docker-də
TOKEN=$(make -s token USER=alice)         # Keycloak-dan dev access token (alice və ya bob)
curl -H "Authorization: Bearer $TOKEN" localhost:8080/api/v1/me
curl -H "Authorization: Bearer $TOKEN" -d '{"slug":"acme","name":"Acme"}' localhost:8080/api/v1/workspaces
make down
```

Dev istifadəçiləri (`deploy/keycloak/nebula-realm.json`, yalnız lokal mühit üçün): `alice` / `bob`.
Keycloak admin paneli: http://localhost:8081.
Ardıcıl 5 səhv şifrə dev istifadəçini 60 saniyəlik bloklayır (brute-force qoruması productiondakı kimi aktivdir).

API konteyneri olmadan lokal inkişaf:

```bash
make run                      # Postgres + Keycloak-u başladır, miqrasiyaları tətbiq edir, API-ni go run ilə işə salır
```

## Keyfiyyət yoxlamaları

| Komanda | Nə edir |
|---|---|
| `make test` | Race detector ilə unit testlər |
| `make test-integration` | Unit + inteqrasiya testləri; hər test öz müvəqqəti Postgres bazasını alır |
| `make lint` | `go vet` + golangci-lint |
| `make vuln` | govulncheck (standart kitabxana + asılılıqlar) |
| `make check` | Yuxarıdakıların hamısı: CI-nin işlətdiyi yoxlamalar |

CI əlavə olaraq Docker image-i Trivy ilə skan edir: düzəldilə bilən HIGH/CRITICAL zəiflik build-i dayandırır.
Bütün GitHub Action-lar commit SHA-ya, base image-lər digest-ə sabitlənib; Dependabot onları həftəlik yeniləyir.

## Binar fayl

```
nebula serve            API-ni işə salır (default)
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
| `POST /api/v1/workspaces/{ws}/members` | Üzv əlavə etmək | admin+ |
| `GET, POST .../{ws}/projects` | Siyahı / yaratmaq | guest+ / admin+ |
| `GET .../{ws}/item-types` | Item tipləri, statusları və bağlı sahələri ilə | guest+ |
| `POST .../{ws}/item-types` | Item tipi yaratmaq | admin+ |
| `PATCH, DELETE .../{ws}/item-types/{type}` | Ad/ikon dəyişmək / arxivləmək | admin+ |
| `PUT, DELETE .../{ws}/item-types/{type}/fields/{field}` | Sahəni tipə bağlamaq / ayırmaq | admin+ |
| `GET, POST .../{ws}/fields` | Fərdi sahə tərifləri | guest+ / admin+ |
| `GET, PATCH .../{ws}/fields/{field}` | Sahəni oxumaq / dəyişmək, arxivləmək | guest+ / admin+ |
| `GET, POST .../{ws}/workflows` | Workflow-lar / yaratmaq | guest+ / admin+ |
| `PATCH .../{ws}/workflows/{id}` | Adını dəyişmək | admin+ |
| `POST .../{ws}/workflows/{id}/statuses` | Status əlavə etmək | admin+ |
| `PATCH, DELETE .../{ws}/workflows/{id}/statuses/{statusID}` | Status dəyişmək / silmək | admin+ |
| `GET .../projects/{KEY}/items` | Siyahı: `limit`, `cursor`, `q`, `status`, `assignee=me\|none\|{id}`, `field.<açar>=<dəyər>` | guest+ |
| `POST .../projects/{KEY}/items` | Yaratmaq | member+ |
| `GET, PATCH, DELETE .../items/{number}` | Oxumaq / `version` ilə qismən yeniləmək / soft delete | guest+ / member+ |
| `GET :9090/metrics` | Prometheus metrikləri, route şablonuna görə etiketlənir | daxili |

## Production qeydləri

- Image: distroless Debian 13, root olmayan istifadəçi (UID 65532), ~25 MB. Read-only root FS ilə və bütün capability-lər söndürülmüş halda işlədin.
- `nebula migrate up`-ı API yenilənməsindən əvvəl ayrıca job kimi işlədin (K8s Job / Argo CD PreSync hook).
- Hər miqrasiyanın işləyən `Down` hissəsi olmalı və əvvəlki relizin işləməsinə mane olmamalıdır (expand/contract).
- Rate limit hər replikada ayrıca saxlanılır: N replika ilə faktiki limit N dəfə çoxdur.
