# ADR-0015: Production — tək VPS-də Docker Compose

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

Nebula ilk production mühitinə çıxır: bir komanda, yüzlərlə istifadəçi, gündə on minlərlə sorğu.
Tələblər: təhlükəsizlik birinci, digest ilə sabitlənmiş image-lər, sınanmış bərpa ilə ehtiyat nüsxələr,
mövcud Grafana-ya monitorinq və Mattermost-a alert, sənədləşdirilmiş upgrade/rollback.
Kubernetes klasteri yoxdur və indiki yük üçün onun idarəetmə xərci (control plane, ingress, CSI,
operatorlar, upgrade-lər) faydasından çoxdur.

## Qərar

Tək ayrılmış Linux VPS (Debian 13) + Docker Engine + Compose v2. Bütün fayllar `deploy/production/`-dadır,
runbook `deploy/production/README.md`-dir.

```
              İnternet (yalnız 80/tcp, 443/tcp, 443/udp)
                              │
                     ┌────────▼────────┐   ACME (Let's Encrypt), HSTS, CSP
                     │ caddy + web UI  │   statik SPA, /config.json
                     └──┬──────────┬───┘
            edge (sabit │ subnet)  │
              ┌─────────▼──┐   ┌───▼──────────┐   admin konsolu: yalnız 127.0.0.1:8081 (SSH tunel)
              │ nebula     │   │ keycloak     │
              │ API+worker │   │ start --opt. │
              └─────┬──────┘   └──────┬───────┘
          db (internal: true, çıxışı yoxdur)
                    └──────┬──────────┘
                    ┌──────▼──────┐  egress (yalnız çıxış) ──► S3 (pgBackRest, şifrəli)
                    │ postgres 19 │
                    └─────────────┘
   alloy (metrics + db + egress) ──► mövcud Prometheus/Mimir ──► Alertmanager ──► Mattermost
```

### Komponentlər və trade-off-lar

| Sahə | Seçim | Səbəb / alternativ |
|---|---|---|
| Orkestrasiya | Compose, tək host | Ən sadə işlək variant; `restart: unless-stopped`, healthcheck, `live-restore` ilə daemon yenilənməsi konteynerləri dayandırmır. HA yoxdur (aşağıda) |
| Edge | Caddy 2.11 | Avtomatik TLS və yenilənmə, HTTP/3, sadə konfiqurasiya. Traefik Docker socket istəyir (root ekvivalenti) — rədd edildi |
| Web UI | Caddy image-inə bişirilmiş statik build (`web.Dockerfile`) | Ayrı static server və əlavə hop yoxdur; UI və edge bir digest ilə versiyalanır, rollback atomikdir. `/config.json` Caddy-də mühitdən yaranır: bir build hər mühitdə işləyir |
| Non-root Caddy | UID 65532, `net.ipv4.ip_unprivileged_port_start=0` (yalnız konteynerin namespace-i), binarın file capability-si silinib | `cap_drop: ALL` + `no-new-privileges` ilə `cap_net_bind_service+ep` olan binar ümumiyyətlə işə düşmür |
| IdP | Keycloak 26.8.2, `start --optimized`, ayrıca subdomen, öz DB-si | Ayrıca origin: IdP cookie-ləri tətbiq origin-i ilə bölüşülmür. `/admin` və `/realms/master` Caddy-də 404, konsol yalnız loopback portda (SSH tunel) |
| DB | PostgreSQL 19, bir klaster, iki baza (`nebula`, `keycloak`), hər birinin öz rolu | Bir backup axını hər ikisini əhatə edir. Monitorinq rolu yalnız `pg_monitor` + aqreqat qaytaran `SECURITY DEFINER` funksiya |
| Backup | **pgBackRest** 2.60 → S3, client-side AES-256, async WAL arxivi | WAL-G ilə müqayisə: pgBackRest-də blok-inkremental, bundling, `verify`/`check`, retention və PITR hazırdır, PGDG paketi var, PG 19 dəstəyi təsdiqlənib. WAL-G-nin üstünlüyü (tək binar) bizə həlledici deyil |
| Monitorinq | Grafana Alloy (tək agent) | node_exporter + cAdvisor + postgres_exporter + blackbox + Prometheus agent əvəzinə bir proses. Alert qaydaları **mərkəzdə** qiymətləndirilir: host ölsə `absent()` yenə işləyir |
| Konteyner metrikləri | cAdvisor xam cgroup-dan, socket olmadan; id → servis adı host-dakı textfile skripti ilə | Docker və ya containerd socket-i konteynerə vermək host-da root deməkdir (socket-proxy ilə də cAdvisor containerd socket istəyir) |
| Sirlər | Fayllar `/etc/nebula/secrets` (0440 root:nebula-secrets, `nebula.env` 0600), Compose secrets; OpenBao Agent opsional | Git-də heç bir sir yoxdur. Nebula yalnız env oxuyur (`*_FILE` yoxdur), ona görə onun sirləri `env_file` ilə gəlir — aşağıda "App dəyişiklikləri" |
| Image-lər | Hamısı `name@sha256:digest`; app image-ləri `release.yml` ilə GHCR-a, cosign keyless imza + provenance | `deploy.sh` digest olmayan referansı rədd edir, `--verify` imzanı yoxlayır |
| Miqrasiya | `migrate` one-shot servis, `service_completed_successfully` | Goose advisory lock; köhnə image yeni sxemdə `migrate up` → no-op (goose naməlum yüksək versiyalarda xəta vermir, yoxlanılıb) |

### RPO / RTO (dürüst)

| | Hədəf | Əsas |
|---|---|---|
| RPO | ≤ 2–5 dəq | `archive_timeout=60`, async push; S3 əlçatmazdırsa WAL spool-da yığılır (4 GiB-a qədər) və RPO böyüyür → alert |
| RTO (DB) | < 1 saat (indiki ölçüdə dəqiqələr) | Aylıq restore testi faktiki müddəti `nebula_restore_test_duration_seconds` kimi ölçür; hədəf bu ölçüyə görə yenilənir |
| RTO (host itkisi) | 2–4 saat | Yeni VPS + Docker + runbook + restore. Hazır ikinci host yoxdur — bu qəbul edilmiş riskdir |

## Nəticələr

- Müsbət: az komponent, hər şey bir faylda oxunur, dəqiqələrlə deploy/rollback, sınanmış PITR,
  container səthi minimaldır (read-only FS, `cap_drop: ALL`, non-root, daxili şəbəkələr).
- Mənfi: **single point of failure** (host, disk, provayder). Deploy zamanı Nebula konteyneri bir neçə
  saniyəlik yenidən başlayır; Caddy sorğuları 15 s-ə qədər saxlayır (`lb_try_duration`), uzun sorğular kəsilə bilər.
- Mənfi: Postgres konteynerinin S3-ə çıxışı və S3 açarı var (WAL push). Kompromis olunmuş DB backup-ları
  silə bilər → bucket-də versioning/Object Lock tövsiyə olunur. Gələcək addım: ayrıca pgBackRest repo-host.
- Mənfi: konteynerlər arası trafik (Nebula → Postgres) TLS-siz, yalnız host daxilindəki internal bridge-dədir.

## Kubernetes/Helm-ə keçid trigger-ləri

Aşağıdakılardan biri baş verəndə Helm chart-a keçirik (Compose faylı artıq buna uyğun qurulub: bir servis =
bir Deployment, healthcheck-lər → probe-lar, sirlər → External Secrets/OpenBao, `migrate` → Job/PreSync hook,
Alloy → DaemonSet, Caddy → ingress + cert-manager):

- Uptime SLO 99.9%+ və ya host baxımı zamanı sıfır downtime tələbi (bir hostda mümkün deyil);
- API üçün birdən çox replika lazımdır (CPU > 70% davamlı) və ya worker-i ayrıca miqyaslamaq lazımdır;
- ikinci mühit (staging) və ya ikinci tenant/region; GitOps (Argo CD) tələbi;
- DB HA (Patroni/CloudNativePG) və ya < 15 dəq RTO tələbi.

## App dəyişiklikləri (production üçün tövsiyə olunur, bu ADR-də edilmədi)

> Hamısı [ADR-0018](0018-production-hardening.md)-də həyata keçirilib; aşağıdakı yan yollar artıq yoxdur.

1. `nebula healthcheck` alt-komandası — indi distroless image üçün healthcheck read-only mount edilmiş busybox ilə edilir.
2. Sirlər üçün `*_FILE` dəstəyi (`NEBULA_DATABASE_URL_FILE`, `NEBULA_ENCRYPTION_KEY_FILE`, `NEBULA_SMTP_PASSWORD_FILE`) — sirlər env-də görünməsin.
3. Job növbəsi üçün app metrikləri (`nebula_jobs_pending`, ən köhnə işin yaşı) — indi Postgres-dən `SECURITY DEFINER` funksiya ilə oxunur.
4. Access log-a müştəri IP-si (etibarlı proxy-dən sonra) — `NEBULA_TRUSTED_PROXIES`-in işini log-da görmək üçün.
5. Miqrasiya və runtime üçün ayrı DB rolları (DDL hüququ yalnız `migrate`-də).
