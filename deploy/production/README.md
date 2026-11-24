# Nebula production runbook (tək VPS, Docker Compose)

Qərar və trade-off-lar: [ADR-0015](../../docs/adr/0015-production-compose-on-vps.md).
Bütün əmrlər host-da `root` (və ya `sudo`) ilə, `/opt/nebula/deploy/production` qovluğundan işlədilir.

| Fayl | Nədir |
|---|---|
| `compose.yaml` | Bütün servislər: caddy(+web UI), migrate, nebula, keycloak, postgres(+pgBackRest), alloy |
| `.env.example` | Sir olmayan parametrlər (hostnamelər, image digest-ləri, S3 endpoint). `.env` git-ə düşmür |
| `caddy/Caddyfile` | TLS, HTTP→HTTPS, HSTS, CSP, `/api` proxy, `/admin` bloku |
| `web.Dockerfile` | Node 26 ilə UI build + Caddy image |
| `postgres/` | PG 19 + pgBackRest image, `postgresql.conf`, rollar/bazalar (initdb) |
| `keycloak/` | Optimized Keycloak image, realm şablonu (istifadəçisiz) |
| `alloy/` | Alloy konfiqurasiyası |
| `monitoring/` | Alert qaydaları, Alertmanager → Mattermost, Grafana dashboard |
| `scripts/` | `gen-secrets`, `preflight`, `deploy`, `rollback`, `db-roles`, `backup`, `restore`, `restore-test`, `container-info`, `keycloak-bootstrap-admin` |
| `systemd/` | Backup, restore testi və container-info timer-ləri |
| `host/` | `daemon.json`, sysctl, sshd, fail2ban nümunələri |

## 1. Host hazırlığı (Debian 13) — hardening checklist

- [ ] **SSH yalnız açarla**: `host/sshd-hardening.conf` → `/etc/ssh/sshd_config.d/10-hardening.conf`; `sshd -t && systemctl reload ssh`. Root login bağlı, ayrıca sudo istifadəçisi.
- [ ] **ufw**: `ufw default deny incoming; ufw default allow outgoing; ufw allow 22/tcp; ufw allow 80/tcp; ufw allow 443/tcp; ufw allow 443/udp; ufw enable`.
  Diqqət: Docker-in publish etdiyi portlar ufw-ni keçir. Ona görə yalnız Caddy 80/443 publish edir, Keycloak admin portu `127.0.0.1`-ə bağlıdır. Yeni `ports:` əlavə etməyin.
- [ ] **fail2ban**: `apt install fail2ban`; `host/fail2ban-jail.local` → `/etc/fail2ban/jail.d/nebula.local`.
- [ ] **unattended-upgrades**: `apt install unattended-upgrades apt-listchanges; dpkg-reconfigure -plow unattended-upgrades` (security mənbəyi; Docker paketlərini avtomatik yeniləməyin — `Unattended-Upgrade::Package-Blacklist { "docker-ce"; "containerd.io"; }`).
- [ ] **Vaxt sinxronu**: `apt install systemd-timesyncd` (və ya chrony); `timedatectl` → `System clock synchronized: yes`. Token-lər və TLS buna bağlıdır (alert: `NebulaClockSkew`).
- [ ] **Docker Engine** (rəsmi repo, `docker-ce`, `docker-compose-plugin` ≥ 2.35, Engine ≥ 28.1).
  `host/daemon.json` → `/etc/docker/daemon.json` (log limitləri, `live-restore`, `no-new-privileges`, `icc: false`, `userland-proxy: false`); `systemctl restart docker`.
- [ ] **sysctl**: `host/99-nebula-sysctl.conf` → `/etc/sysctl.d/`; `sysctl --system` (HTTP/3 UDP bufferləri, şəbəkə hardening).
- [ ] Alətlər: `apt install jq curl openssl util-linux git` (skriptlər `jq`, `flock` istifadə edir).
- [ ] Disk: Docker data root üçün ən az 40 GB, ayrıca volume tövsiyə olunur. Swap 1–2 GB.
- [ ] Provayder səviyyəsində snapshot (netcup) aylıq — backup əvəzi deyil, əlavə qatdır.

## 2. İlk quraşdırma

### 2.1 DNS və kod

```bash
git clone https://github.com/Tural-HS/Nebula.git /opt/nebula && cd /opt/nebula && git checkout vX.Y.Z
cd deploy/production && cp .env.example .env && chmod 0640 .env
```

DNS: `NEBULA_HOST` (məs. `nebula.example.com`) və `NEBULA_AUTH_HOST` (məs. `auth.nebula.example.com`) üçün A/AAAA qeydləri host-a. CAA qeydi: `0 issue "letsencrypt.org"`.
`.env`-də hostnamelər, `ACME_EMAIL`, S3 parametrləri, `REMOTE_WRITE_URL/USERNAME`, image digest-ləri (bax 2.3).

### 2.2 Sirlər

```bash
sudo ./scripts/gen-secrets.sh
```

Yaradır (`/etc/nebula/secrets`, qovluq 0750, hər sir bir fayl, `0440 root:nebula-secrets`). Mövcud faylı **heç vaxt** üzərinə yazmır.

| Fayl | Kim istifadə edir | Qeyd |
|---|---|---|
| `postgres_superuser_password` | postgres | yalnız lokal socket/initdb |
| `nebula_db_password` | postgres, `migrate` | `nebula` rolu — sxemin sahibi, **yalnız** `nebula migrate` (DDL) |
| `nebula_app_db_password` | postgres, `nebula` | `nebula_app` rolu — `serve`, yalnız DML (ADR-0018) |
| `keycloak_db_password`, `monitoring_db_password` | postgres, keycloak / alloy | |
| `nebula_encryption_key` | `nebula` | webhook sirlərinin şifrələmə açarı (base64, 32 bayt) |
| `nebula_encryption_keys_old` | `nebula` | boş; yalnız açar rotasiyası zamanı köhnə açarlar (vergüllə) |
| `nebula_smtp_password` | `nebula` | boş = SMTP autentifikasiyası yoxdur |
| `pgbackrest_*`, `remote_write_password` | postgres / alloy | `REPLACE_ME` placeholder |

Sonra əl ilə:

- `pgbackrest_s3_key`, `pgbackrest_s3_key_secret` — yalnız bir bucket-ə (və prefix-ə) yazma/oxuma/silmə hüququ olan açar;
- `remote_write_password` — Prometheus/Mimir remote-write parolu;
- SMTP: `.env`-də `NEBULA_SMTP_HOST/PORT/USERNAME/FROM/TLS` (sir deyil; `TLS` `starttls` və ya `tls`), parol `nebula_smtp_password` faylında.

Nebula sirləri **yalnız fayl** kimi alır (`NEBULA_*_FILE` → `/run/secrets/...`): env-də, `docker inspect`-də və `/proc/<pid>/environ`-da görünmürlər. `NEBULA_DATABASE_URL` sir deyil (parolsuz), parol ayrıca fayldan gəlir.

**Offline nüsxə (vacib):** `pgbackrest_cipher_pass` itərsə BÜTÜN backup-lar oxunmaz olur; `nebula_encryption_key` itərsə webhook sirləri açılmaz. Hər ikisini parol menecerinə / OpenBao-ya yazın.

Niyə 0600 yox, 0440 + qrup: Compose (Swarm olmadan) secret faylını host icazələri ilə bind-mount edir; non-root konteynerlər (postgres 999, keycloak 1000, nebula 65532, alloy) faylı oxumaq üçün `group_add` ilə `nebula-secrets` qrupuna qoşulur. Hər servis yalnız özünə verilən sirləri görür.

**Faylı dəyişəndə** yerində yazın (eyni inode), məs. `sudo sh -c 'umask 337; printf %s "$NEW" > /etc/nebula/secrets/nebula_smtp_password'`. `mv` ilə əvəz edilən fayl artıq işləyən konteynerlərə görünmür (bind mount köhnə inode-u saxlayır) — onda konteyneri yenidən yaradın (`up -d --force-recreate <servis>`). Nebula sirləri yalnız start zamanı oxuyur.

#### Köhnə quraşdırmanın yenilənməsi (`nebula.env` → sir faylları, ADR-0018)

`nebula.env` (env_file) artıq istifadə olunmur. Bir dəfə, ADR-0018-i ehtiva edən ilk release-in deploy-undan əvvəl:

```bash
sudo ./scripts/gen-secrets.sh   # yeni faylları yaradır; NEBULA_ENCRYPTION_KEY(_OLD) və SMTP parolu nebula.env-dən köçürülür
grep -E '^NEBULA_SMTP_(HOST|PORT|USERNAME|FROM|TLS)=' /etc/nebula/secrets/nebula.env   # bunları .env-ə köçürün (FROM-u dırnaqla)
sudo ./scripts/deploy.sh NEBULA_IMAGE=... NEBULA_WEB_IMAGE=... POSTGRES_IMAGE=... KEYCLOAK_IMAGE=...
```

`deploy.sh` əvvəlcə Postgres-i yeni secret mount-u ilə yenidən yaradır (qısa DB restartı), `scripts/db-roles.sh` məntiqi ilə `nebula_app` rolunu yaradır, sonra `migrate` ona cədvəl hüquqlarını verir, `nebula` isə artıq `nebula_app` ilə qoşulur. Köhnə `monitoring.job_queue_stats()` funksiyası silinir. Hər şey işlədikdən sonra `nebula.env`-i silin (`shred -u`). `nebula_encryption_key`-in köhnə açarla eyni olduğunu yoxlayın — yeni açar webhook sirlərini oxunmaz edər.

Bu deploy-da avtomatik rollback söndürülür (köhnə image `*_FILE` oxumur, `deploy.sh` bunu yoxlayır və xəbərdarlıq edir); geri qayıtmaq lazımdırsa §9-a baxın.

#### Opsional: OpenBao Agent axını

Sirləri faylda əl ilə saxlamaq əvəzinə OpenBao Agent (systemd servisi, AppRole və ya TLS cert auth) eyni faylları render edir:

```hcl
template {
  contents    = "{{ with secret \"kv/data/nebula/prod\" }}{{ .Data.data.pgbackrest_s3_key }}{{ end }}"
  destination = "/etc/nebula/secrets/pgbackrest_s3_key"
  perms       = "0440"
  group       = "nebula-secrets"
  # Postgres sirləri yalnız restart ilə oxunur; S3 açarı hər pgbackrest çağırışında oxunur.
}
template {
  contents    = "{{ with secret \"kv/data/nebula/prod\" }}{{ .Data.data.smtp_password }}{{ end }}"
  destination = "/etc/nebula/secrets/nebula_smtp_password"
  perms       = "0440"
  group       = "nebula-secrets"
  # Nebula sirləri yalnız start zamanı oxuyur. Agent faylı rename ilə yazır (yeni inode): recreate.
  command     = "cd /opt/nebula/deploy/production && docker compose up -d --force-recreate nebula"
}
```

DB parollarını OpenBao-da dəyişmək bazadakı parolu dəyişmir — rotasiya bölməsinə baxın. `pgbackrest_cipher_pass` dəyişdirilmir (repo onunla şifrələnib).

### 2.3 Image-lər

`vX.Y.Z` tag-i push olunanda `.github/workflows/release.yml` dörd image yaradır, GHCR-a göndərir, cosign ilə imzalayır və provenance əlavə edir; Summary-də hazır `KEY=image:tag@sha256:...` sətirləri var. Onları `.env`-ə yazın.
Registry olmadan (yalnız müvəqqəti): host-da build edin və yerli registry-yə push edin ki, digest alınsın — digest-siz referans `preflight.sh` tərəfindən rədd edilir.

### 2.4 Bazanı və backup-u başlatmaq

```bash
sudo ./scripts/preflight.sh
docker compose up -d --wait postgres
sudo ./scripts/backup.sh init      # S3-də stanza, WAL arxivinin yoxlanışı
sudo ./scripts/backup.sh full      # ilk tam backup
```

### 2.5 İlk deploy

```bash
set -a; source .env; set +a
sudo ./scripts/deploy.sh NEBULA_IMAGE="$NEBULA_IMAGE" NEBULA_WEB_IMAGE="$NEBULA_WEB_IMAGE" \
  KEYCLOAK_IMAGE="$KEYCLOAK_IMAGE" POSTGRES_IMAGE="$POSTGRES_IMAGE"
```

Caddy ilk sorğuda Let's Encrypt sertifikatı alır (80 və 443 açıq olmalıdır). Test üçün əvvəlcə `.env`-də `ACME_CA`-nı staging-ə qoyun (rate limit). `caddy-data` volume-unu silməyin.

### 2.6 systemd timer-ləri

```bash
sudo cp systemd/* /etc/systemd/system/ && sudo systemctl daemon-reload
sudo systemctl enable --now nebula-backup-full.timer nebula-backup-diff.timer \
  nebula-restore-test.timer nebula-container-info.timer
systemctl list-timers 'nebula-*'
```

Fayllar `/opt/nebula/deploy/production` yolunu nəzərdə tutur; başqa yoldursa `WorkingDirectory`/`ExecStart`-ı dəyişin.

## 3. İlk admin və ilk istifadəçi (Keycloak)

Realm şablonunda (`keycloak/realm/nebula-realm.json`) **heç bir istifadəçi yoxdur**; `nebula-web` client-in redirect URI-ləri `https://${NEBULA_HOST}`-dan yaranır. Realm yalnız mövcud deyilsə import olunur — sonrakı dəyişikliklər admin konsolundan edilir (və ya realm yenidən yaradılır).

> **Giriş səhifəsinin dili (ADR-0025):** realm 2026-11-24-dən əvvəl yaradılıbsa, interfeys dilləri üçün bir dəfə:
> Admin konsol → Realm settings → Localization → *Internationalization* açın, dillər `az, en, tr, ru`, default `az`.

1. Müvəqqəti admin (parolu interaktiv soruşur, heç yerdə saxlanmır):
   ```bash
   sudo ./scripts/keycloak-bootstrap-admin.sh temp-admin
   ```
2. Konsol yalnız SSH tunel ilə: öz kompüterinizdə
   ```bash
   ssh -L 8081:127.0.0.1:8081 deploy@nebula.example.com
   ```
   brauzerdə `http://localhost:8081/admin/`. İctimai `https://auth.../admin` və `/realms/master` 404 qaytarır.
3. `master` realm-də şəxsi admin yaradın, ona `admin` rolunu verin, **OTP-ni məcburi edin** (Required action: Configure OTP), çıxın, şəxsi admin ilə girin və `temp-admin`-i **silin**.
4. `nebula` realm → Realm settings → Email: SMTP (parol sıfırlama üçün); sonra istəsəniz "Forgot password"-u yandırın.
5. İlk istifadəçi: `nebula` realm → Users → Add user (email, ad, `Email verified`), Credentials → müvəqqəti parol (`Temporary: on`) və ya "Send email: Update password". İstifadəçi ilk dəfə `https://nebula.example.com`-a daxil olur, workspace yaradır və onun sahibi olur. Digər istifadəçiləri workspace sahibi Nebula-da üzv kimi əlavə edir.

Brute-force qoruması açıqdır (5 səhv → artan gözləmə, maks. 15 dəq), access token 5 dəq, SSO idle 30 dəq, maks. 10 saat, refresh token rotasiyası.

## 4. Backup-lar

| Nə | Necə | Nə vaxt |
|---|---|---|
| WAL (davamlı) | `archive_command` → pgBackRest async → S3 | hər seqment, ən gec 60 s |
| Full | `nebula-backup@full` | bazar 02:15 UTC |
| Differential | `nebula-backup@diff` | b.e.–şənbə 02:15 UTC |
| Incremental | `deploy.sh` / `rollback.sh` | hər deploy-dan əvvəl |
| Retention | 4 full (~4 həftə PITR), 7 diff | pgBackRest `expire` |

Hər iki baza (`nebula`, `keycloak`) bir klasterdədir — bir backup hər ikisini əhatə edir. Repo client-side AES-256 ilə şifrələnib. Bucket-də **versioning və ya Object Lock** yandırın (DB konteynerində S3 açarı var; kompromis halında backup-ların silinməsinə qarşı).

```bash
sudo ./scripts/backup.sh info     # backup siyahısı
sudo ./scripts/backup.sh check    # WAL arxivi uçdan-uca
journalctl -u 'nebula-backup@*' --since today
```

RPO ≤ 2–5 dəq, RTO (DB) — restore testinin ölçdüyü müddət (dashboard: "Restore duration"). Host tam itsə RTO 2–4 saatdır (yeni VPS + bu runbook).

### 4.1 Fayllar (attachments, ADR-0022)

pgBackRest **yalnız Postgres-i** saxlayır. Tapşırıqlara əlavə edilən faylların baytları bazada deyil:
default olaraq `attachments` volume-undadır (`/var/lib/nebula/attachments`), S3 backend-də isə bucket-dədir.
Bazada yalnız metadata və təsadüfi açar (`storage_key`) var. Ona görə fayllar **ayrıca** backup olunmalıdır.

**Uyğunluq qaydası.** Fayl əvvəlcə yazılır, sətir sonra commit olunur; fayllar heç vaxt dəyişdirilmir.
Silinmiş faylın blob-u `NEBULA_ATTACHMENTS_PURGE_DELAY` (default `168h` = 7 gün) qədər saxlanılır.
Nəticə: bazanı T anına bərpa edirsinizsə, **T-dən sonrakı ilk fayl snapshot-unu** (T-dən ən çox 7 gün sonra) bərpa edin —
T anında bazada olan bütün faylları ehtiva edir. Fayl backup-ı gündəlikdirsə, gecikmə ≥ 1 gün olmalıdır; 7 gün ehtiyat payıdır.
Artıq fayllar (bazada sətri olmayan) zərərsizdir: worker onları növbəyə salmır, istəsəniz əl ilə silinə bilər.

#### Lokal volume → restic (S3-ə, şifrəli)

pgBackRest-dən **ayrı** bucket/prefix və ayrı açar istifadə edin. Image-i digest ilə pinləyin və yeniləmədən əvvəl
digest-i yoxlayın: `docker buildx imagetools inspect restic/restic:0.20.0`.

```bash
# /etc/nebula/restic.env (0400 root): RESTIC_REPOSITORY=s3:https://<s3-endpoint>/<bucket>/nebula-files
#   AWS_ACCESS_KEY_ID=…  AWS_SECRET_ACCESS_KEY=…  RESTIC_PASSWORD_FILE=/run/secrets/restic_password
# /etc/nebula/secrets/restic_password: openssl rand -base64 32 (offline nüsxəsi olmadan repo bərpa olunmur!)
RESTIC='docker run --rm --env-file /etc/nebula/restic.env
  -v /etc/nebula/secrets/restic_password:/run/secrets/restic_password:ro
  restic/restic:0.20.0@sha256:93999df80d4ac6cb148e82d2ef1fe6a81996f17ff535e772723538211b31555a'
VOL=$(docker volume ls -q --filter label=com.docker.compose.volume=attachments)

$RESTIC init                                                          # bir dəfə
$RESTIC -v "$VOL":/data:ro backup /data --host nebula --tag attachments   # hər gecə, DB backup-dan sonra (~03:00 UTC)
$RESTIC forget --host nebula --tag attachments --keep-daily 14 --keep-weekly 8 --prune   # ≥ 4 həftəlik PITR pəncərəsi
$RESTIC check --read-data-subset=5%                                   # həftəlik
```

Hələ timer yoxdur: cron/systemd-ə əl ilə əlavə edin və nəticəni izləyin (follow-up: `nebula-files-backup.timer` + metrik).

**Bərpa:**

```bash
docker compose stop nebula
$RESTIC snapshots --tag attachments                                   # T-dən sonrakı ilk snapshot-u seçin
$RESTIC -v "$VOL":/data restore <snapshot-id> --target / --include /data
docker compose up -d --wait nebula
# Yoxlama: bazada olub faylı olmayan açar qalmamalıdır (çıxış boş olmalıdır).
docker compose exec -T postgres psql -U postgres -d nebula -Atc 'SELECT storage_key FROM attachments' | sort >/tmp/db-keys
docker run --rm -v "$VOL":/data:ro --entrypoint find restic/restic:0.20.0@sha256:93999df80d4ac6cb148e82d2ef1fe6a81996f17ff535e772723538211b31555a \
  /data -type f -not -path '*/.staging/*' -exec basename {} \; | sort >/tmp/blob-keys
comm -23 /tmp/db-keys /tmp/blob-keys
```

restic UID/GID-ni saxlayır (fayllar `65532`, rejim `0600`/`0700`). Yeni volume-a bərpa edirsinizsə, əvvəlcə Nebula-nı bir dəfə
başladın ki, volume image-dən düzgün sahiblik alsın.

#### S3 backend (`NEBULA_ATTACHMENTS_BACKEND=s3`)

- Bucket-də **versioning** yandırın və lifecycle qaydası ilə köhnə (noncurrent) versiyaları **≥ 35 gün** saxlayın; mümkünsə başqa regiona replikasiya.
- Nebula açarları heç vaxt təkrar istifadə etmir və obyektləri üzərinə yazmır; silinmə yalnız worker-in `DELETE`-idir (gecikmədən sonra).
  Bərpa: bazanı T-yə qaytarın, sonra `comm` yoxlamasındakı itkin açarlar üçün delete marker-ləri silin (`aws s3api delete-object --version-id <marker>`).
- Açarları sir faylları kimi verin (`*_FILE`), heç vaxt environment-də yox — `compose.override.yaml` nümunəsi:

```yaml
services:
  nebula:
    environment:
      NEBULA_ATTACHMENTS_BACKEND: s3
      NEBULA_ATTACHMENTS_S3_ENDPOINT: https://<s3-endpoint>
      NEBULA_ATTACHMENTS_S3_BUCKET: <bucket>
      NEBULA_ATTACHMENTS_S3_PREFIX: attachments/
      NEBULA_ATTACHMENTS_S3_ACCESS_KEY_FILE: /run/secrets/nebula_attachments_s3_access_key
      NEBULA_ATTACHMENTS_S3_SECRET_KEY_FILE: /run/secrets/nebula_attachments_s3_secret_key
      NEBULA_ATTACHMENTS_TMP_DIR: /tmp
    tmpfs:
      - /tmp:size=256m,mode=1777   # yükləmə yoxlanılarkən staging (maks. fayl × paralel yükləmə)
    secrets: [nebula_attachments_s3_access_key, nebula_attachments_s3_secret_key]
secrets:
  nebula_attachments_s3_access_key: { file: /etc/nebula/secrets/nebula_attachments_s3_access_key }
  nebula_attachments_s3_secret_key: { file: /etc/nebula/secrets/nebula_attachments_s3_secret_key }
```

  S3 açarına yalnız bu bucket/prefix üzrə `GetObject`, `PutObject`, `DeleteObject` icazəsi verin.

`restore-test.sh` faylları yoxlamır; rübdə bir dəfə yuxarıdakı bərpanı test volume-una edin.

## 5. Restore testi (aylıq, avtomatik)

`nebula-restore-test.timer` hər ayın ilk çərşənbə axşamı `scripts/restore-test.sh`-ı işlədir:
ən son backup + WAL-ı S3-dən **müvəqqəti** konteyner və volume-a bərpa edir (`archive_mode=off` — repo-ya heç nə yazmır, production şəbəkəsinə qoşulmur), recovery-nin bitməsini gözləyir, sonra yoxlayır:
sxem versiyası = production, əsas cədvəllərin sətir sayı (±2%), Keycloak realm-in mövcudluğu, sağlamlıq sorğusu.
Nəticə metrikdir (`nebula_restore_test_*`); 35 gündə uğurlu test yoxdursa `NebulaRestoreTestStale` alerti. Konteyner və volume hər halda silinir.

Əl ilə: `sudo ./scripts/restore-test.sh`.

## 6. Disaster recovery (restore)

```bash
sudo ./scripts/restore.sh                                     # son vəziyyət
sudo ./scripts/restore.sh --target '2026-12-01 09:59:00+00'   # insidentdən əvvəlki an (PITR)
sudo ./scripts/restore.sh --set 20261201-021501F              # konkret backup
```

Axın: Caddy/Nebula/Keycloak dayanır → Postgres dayanır → `pgbackrest restore --delta` → recovery və promote → yeni timeline-ın tam backup-ı → stack başlayır → smoke test.
Yanlış hədəf seçilibsə yenidən işlədin: köhnə timeline repo-da qalır. Bərpadan sonra məlumatı komanda ilə yoxlayın.

**Host tam itibsə:** yeni VPS → bölmə 1 → `git clone` → `.env` və `/etc/nebula/secrets`-i offline nüsxədən/OpenBao-dan bərpa edin → `docker compose up -d --wait postgres` (boş klaster yaranır) → `restore.sh` → `deploy.sh` eyni digest-lərlə → DNS-i yeni IP-yə yönəldin.

## 7. Monitorinq və alertlər

- **Alloy** (`alloy/config.alloy`) Nebula `:9090`, Caddy `:2020`, Keycloak `:9000`, Postgres (`nebula_monitor`, yalnız `pg_monitor`, cədvəl hüququ yoxdur), host, konteynerlər (cgroup, socket olmadan) və HTTPS probları yığır və `REMOTE_WRITE_URL`-ə göndərir. Bütün seriyalarda `instance=$NEBULA_INSTANCE`.
- **Alert qaydaları**: `monitoring/alerts/nebula-rules.yaml` → mərkəzi Prometheus `rule_files` / `mimirtool rules load` (`promtool check rules` ilə yoxlanılıb). Əhatə: API down, ictimai endpoint down, 5xx nisbəti (API və edge), p95 gecikmə, job növbəsi gecikməsi/həcmi, uğursuz job-lar, disk >85% / >95%, yaddaş təzyiqi, OOM, saat sinxronu, backup >26 saat, backup uğursuz, WAL arxivi xətası, restore testi köhnə, sertifikat <14 gün / <3 gün.
- **Job növbəsi** metrikləri Nebula-nın özündəndir (ADR-0018): `nebula_job_queue_pending_jobs{state="due|running|scheduled"}`, `nebula_job_queue_recently_failed_jobs` (son 1 saat), `nebula_job_queue_oldest_due_age_seconds`, `nebula_job_queue_stats_last_success_timestamp_seconds`. Hər proses `NEBULA_JOB_METRICS_INTERVAL` (default 30s) ilə bir sorğu edir, scrape-lər keşdən cavablanır. Bütün replikalar eyni dəyəri verir — `max()` ilə yığın. Sorğu alınmırsa `NebulaJobQueueStatsStale` alerti.
- **Access log**: hər sorğuda `client_ip` (rate limiter-in istifadə etdiyi ünvan: `X-Forwarded-For` yalnız Caddy-dən), route pattern (xam path və query yox), status, müddət, `request_id`. Token, cookie və query parametrləri log-a düşmür.
- **Mattermost**: `monitoring/alertmanager/mattermost.yaml`-ı mərkəzi Alertmanager-ə birləşdirin. Webhook URL sirdir: `/etc/alertmanager/secrets/mattermost_webhook_url` faylında (OpenBao/ESO), konfiqurasiyada yox (`api_url_file`).
- **Dashboard**: `monitoring/grafana/nebula-dashboard.json` → Grafana → Import (data source seçilir).
- Self-probe DNS və upstream şəbəkə problemlərini görmür: mərkəzdən də blackbox probu qurun.

## 8. Upgrade

```bash
sudo ./scripts/deploy.sh NEBULA_IMAGE=ghcr.io/tural-hs/nebula:v1.4.0@sha256:... \
                         NEBULA_WEB_IMAGE=ghcr.io/tural-hs/nebula-web:v1.4.0@sha256:... --verify
```

Addımlar: digest yoxlaması → (`--verify`: cosign imzası) → `.env` yenilənir (köhnəsi `/var/lib/nebula/releases/env.*.bak`) → preflight → pull → image yoxlaması (`*_FILE` + `nebula healthcheck` dəstəyi, `image_supports_file_secrets`) → **deploy-dan əvvəl incremental backup** → DB rolları (`20-nebula-runtime-role.sql`, idempotent) → `nebula migrate up` (one-shot, `nebula` rolu ilə; sonra `nebula_app`-a DML hüquqları) → request path servisləri (`postgres keycloak nebula caddy`) `--wait` ilə → smoke test (TLS, `/`, `/config.json`, `/api/v1/me` = 401, OIDC discovery, `/admin` = 404) → monitorinq servisləri → release qeydi (`/var/lib/nebula/releases`).
Smoke test uğursuzdursa əvvəlki image-lər **avtomatik** qaytarılır (sxem yeni qalır — expand/contract).
Monitorinq agentinin xətası yalnız xəbərdarlıqdır, release-i geri qaytarmır.

Infra image-ləri (Postgres, Keycloak, Caddy+UI) eyni skriptlə yenilənir. Postgres-in **major** versiyası (19 → 20) bu axınla dəyişmir: `pg_upgrade` və ya dump/restore planı ayrıca hazırlanır.
Üçüncü tərəf image-ləri (Alloy) `.env`-də override ilə yenilənir; həmişə `name:tag@sha256`.
Release image-ləri imzalanmadan əvvəl Trivy ilə skan olunur: düzəlişi olan HIGH/CRITICAL zəiflik release-i dayandırır (`release.yml`, ADR-0018).

## 9. Rollback

```bash
sudo ./scripts/rollback.sh --list
sudo ./scripts/rollback.sh                       # əvvəlki release, yalnız image-lər
sudo ./scripts/rollback.sh --to 20261201T101500Z
sudo ./scripts/rollback.sh --migrate-down-to 9   # əvvəlcə 9-dan yeni miqrasiyaların Down-u
```

Qayda (README "Production qeydləri"): hər miqrasiya expand/contract-dır — əvvəlki release yeni sxemlə işləyir. Ona görə default rollback **yalnız image-ləri** qaytarır; köhnə image-in `migrate up`-ı yeni sxemdə heç nə etmir.
Sxemi də geri qaytarmaq lazımdırsa `--migrate-down-to N` Down hissələrini **cari** (yeni) image ilə işlədir (köhnə image-də o fayllar yoxdur), əvvəlcə backup alır.

| Miqrasiya | Down | Data təsiri |
|---|---|---|
| 00001 core schema | var | **hər şeyi silir** — heç vaxt production-da |
| 00002 item type fields | var | sahə–tip bağlamaları itir |
| 00003 automation | var | webhook-lar, qaydalar, job-lar, çatdırılma logu itir |
| 00004 collaboration | var | mention-lar itir |
| 00005 board index | var | təhlükəsiz (yalnız indeks) |
| 00006 due dates | var | `item.overdue` qaydaları və gecikmə qeydləri silinir |
| 00007 webhook format | var | `slack` webhook-ları söndürülür (URL bərpa olunmur) |
| 00008 import source | var | `import` mənbəyi `user`-ə çevrilir (metadata itir) |
| 00009 recurrences | var | təkrarlanan qaydalar silinir |
| 00010 project privacy | var | ⚠️ **təhlükəsizlik**: `visibility` və layihə üzvləri silinir, **gizli layihələr bütün workspace üzvlərinə görünən olur**. Gizli layihə varsa Down əvəzinə PITR istifadə edin |
| 00011 saved views | var | saxlanılan görünüşlər silinir; indeks `CONCURRENTLY` silinir (yazmaları bloklamır) |
| 00012 webhook projects | var | ⚠️ **təhlükəsizlik**: webhook-ların layihə filtri (`project_ids`) silinir; köhnə image-də **gizli layihələrin hadisələri bütün webhook-lara gedir** (ADR-0013-ün bağlanmış boşluğu). Gizli layihə varsa: rollback-dən əvvəl webhook-ları söndürün, yalnız image-i qaytarın (sxem qalsın) və ya Down əvəzinə PITR |
| 00013 invitations | var | gözləyən dəvətlər silinir (linklər işləməz); qəbul edilmiş üzvlüklər qalır |
| 00014 notifications | var | bütün tətbiqdaxili bildirişlər silinir |
| 00015 calendar views | var | data silmir; `calendar` növlü saxlanılan görünüş varsa Down xəta ilə dayanır — əvvəlcə onları `table`/`board`-a dəyişin və ya silin |
| 00016 subtasks | var | bütün alt tapşırıq əlaqələri silinir (tapşırıqlar qalır, hamısı yuxarı səviyyəli olur); indekslər `CONCURRENTLY` qurulur |
| 00017 attachments | var | ⚠️ **data itkisi:** bütün əlavələrin metadata-sı (`attachments`) silinir, fayllar isə blob store-da (volume/S3) yetim qalır — əvvəlcə metadata-nı ixrac edin, geri qayıtdıqdan sonra blob store-u ayrıca boşaldın (ADR-0022, §4.1). Mümkünsə yalnız image-i geri qaytarın, migrasiyanı yox |
| 00018 search | var | axtarış indeksi (`search_docs`) və trigger-lər silinir, heç bir əsas məlumat itmir; köhnə image-də workspace axtarışı yoxdur. **Kilidlər:** Up-da trigger yaradılması `items`/`comments`-ə millisaniyəlik `SHARE ROW EXCLUSIVE` (`lock_timeout` 5 s), backfill 2000 sətirlik partiyalarla commit olunur (yazmalar davam edir; 100k tapşırıq + 200k şərh ≈ 22–26 s); Down-da trigger silinməsi eyni qısa kilid, `DROP TABLE search_docs` yalnız həmin cədvəli kilidləyir. Təkrar Up təhlükəsizdir (backfill davam edir). `pg_trgm` extension-u qalır |

**ADR-0018-dən əvvəlki release-ə rollback:** köhnə image `*_FILE` sirləri oxumur və `nebula healthcheck` bilmir, yeni `compose.yaml` ilə işləmir; `rollback.sh` bunu yoxlayır və `.env`-i qaytarıb dayanır. Belə halda: `git -C /opt/nebula checkout <köhnə tag> -- deploy/production`, `nebula.env`-i bərpa edin (ondan `NEBULA_DATABASE_URL` `nebula` rolu ilə), sonra `rollback.sh`. `nebula_app` rolu və onun hüquqları qala bilər (zərərsizdir).

Down-u olan amma data silən miqrasiyadan sonra, və ya Down-u olmayan (gələcək) miqrasiyadan sonra, **məlumatı qaytarmağın yeganə yolu PITR-dir**: `restore.sh --target '<deploy-dan əvvəlki an>'` (deploy-dan əvvəlki incremental backup bu nöqtəni təmin edir) — həmin andan sonrakı yazılar itir.

## 10. Rotasiya

- **`nebula_app` parolu** (runtime): yeni dəyəri `nebula_app_db_password`-a yerində yazın → `sudo ./scripts/db-roles.sh` (parolu fayldan götürür, log-a yazmır) → `docker compose up -d --force-recreate nebula`.
- **`nebula` parolu** (sxem sahibi, yalnız migrate): faylı yerində yazın → `docker compose exec postgres psql -U postgres`, orada `\password nebula` (interaktiv, eyni dəyər; parol log-a düşmür) — növbəti `migrate` faylı oxuyur. Keycloak/monitorinq eyni qayda ilə (Keycloak, Alloy restart).
- **Şifrələmə açarı**: cari açarı `nebula_encryption_keys_old`-a, yenisini (`openssl rand -base64 32`) `nebula_encryption_key`-ə yerində yazın → `docker compose up -d --force-recreate nebula` → `docker compose run --rm --no-deps nebula rotate-secrets` → `nebula_encryption_keys_old`-u boşaldın → yenidən `--force-recreate nebula` (ADR-0007).
- **S3 açarı**: yeni açarı fayla yazın — növbəti pgbackrest çağırışı onu oxuyur, restart lazım deyil. `backup.sh check`.

## 11. İnsident checklist

1. **Təsiri təsdiqlə**: alert, `https://nebula.example.com`, `docker compose ps`, `systemctl status docker`.
2. **Son dəyişiklik?** `rollback.sh --list`, `git -C /opt/nebula log -1`. Deploy-dan sonra başlayıbsa → `rollback.sh`.
3. **Loglar**: `docker compose logs --since 30m nebula caddy keycloak postgres` (JSON; `request_id` ilə izləyin).
4. **Resurslar**: `df -h`, `docker system df`, `free -m`, `docker stats --no-stream`. Disk doludursa: köhnə image-lər (`docker image prune`), `pgbackrest-spool` (S3 əlçatmazdır?), Postgres logu.
5. **DB**: `docker compose exec postgres pg_isready`; `backup.sh check`; bloklar: `SELECT * FROM pg_stat_activity WHERE wait_event IS NOT NULL`.
6. **TLS/ACME**: `docker compose logs caddy | grep -i acme`; 80/443 açıqdır? DNS düzdür?
7. **Giriş problemi**: Keycloak `health/ready`, saat sinxronu, `NEBULA_AUTH_HOST` DNS, realm client redirect URI-ləri.
8. **Məlumat korlanıb/silinib**: yazmanı dayandırın (`docker compose stop caddy`), anı müəyyən edin, `restore.sh --target`.
9. **Kompromis şübhəsi**: hostu şəbəkədən təcrid edin (provayder firewall), disk snapshot-u alın, bütün sirləri rotasiya edin, Keycloak sessiyalarını ləğv edin (Realm → Sessions → Sign out all), yeni hostda backup-dan qurun.
10. **Sonra**: blameless postmortem, runbook/alert boşluqlarını düzəldin.

## 12. Yerli yoxlama (production olmadan)

```bash
docker compose -f deploy/production/compose.yaml --env-file deploy/production/.env.example config --quiet
docker run --rm -v "$PWD/deploy/production:/mnt:ro" -w /mnt koalaman/shellcheck:v0.11.0 -x scripts/*.sh postgres/nebula-pgbackrest postgres/initdb/*.sh keycloak/*.sh
go test ./internal/database -run RuntimeRole   # NEBULA_TEST_DATABASE_URL ilə: runtime rolu DDL edə bilmir
```

Smoke test lazımdırsa: fərqli project adı (`COMPOSE_PROJECT_NAME=nebula-prodtest`), `NEBULA_BIND_ADDR=127.0.0.1`, yüksək portlar, ayrıca edge subnet; dev stack-ə (`nebula` project) toxunmayın və bitəndə `docker compose -p nebula-prodtest down -v` edin.
