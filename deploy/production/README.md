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
| `keycloak/` | Optimized Keycloak image, realm şablonu (istifadəçisiz), giriş mövzusu `themes/nebula` (ADR-0036) |
| `alloy/` | Alloy konfiqurasiyası |
| `monitoring/` | Alert qaydaları, Alertmanager → Mattermost, Grafana dashboard |
| `scripts/` | `gen-secrets`, `preflight`, `deploy`, `rollback`, `db-roles`, `backup`, `restore`, `restore-test`, `container-info`, `keycloak-bootstrap-admin` |
| `systemd/` | Backup, restore testi və container-info timer-ləri |
| `host/` | `daemon.json`, sysctl, sshd, fail2ban nümunələri |
| `compose.shared-host.yaml`, `shared-host/` | Yalnız pilot: başqa stack-in Caddy-si arxasında işləmək (bölmə 13) |
| `compose.local-s3.yaml`, `scripts/local-s3-init.sh` | Yalnız pilot: eyni hostda pulsuz S3 backup hədəfi (bölmə 13.5) |
| `compose.repo2-offsite.yaml` | Müvəqqəti: yerli backup-dan off-site S3-ə kəsintisiz köçmə (bölmə 13.5) |

## 1. Host hazırlığı (Debian 13) — hardening checklist

- [ ] **SSH yalnız açarla**: `host/sshd-hardening.conf` → `/etc/ssh/sshd_config.d/10-hardening.conf`; `sshd -t && systemctl reload ssh`. Root login bağlı, ayrıca sudo istifadəçisi.
- [ ] **ufw**: `ufw default deny incoming; ufw default allow outgoing; ufw allow 22/tcp; ufw allow 80/tcp; ufw allow 443/tcp; ufw allow 443/udp; ufw enable`.
  Diqqət: Docker-in publish etdiyi portlar ufw-ni keçir. Ona görə yalnız Caddy 80/443 publish edir, Keycloak admin portu `127.0.0.1`-ə bağlıdır. Yeni `ports:` əlavə etməyin.
- [ ] **fail2ban**: `apt install fail2ban`; `host/fail2ban-jail.conf` → `/etc/fail2ban/jail.d/nebula.local`.
- [ ] **unattended-upgrades**: `apt install unattended-upgrades apt-listchanges; dpkg-reconfigure -plow unattended-upgrades` (security mənbəyi; Docker paketlərini avtomatik yeniləməyin — `Unattended-Upgrade::Package-Blacklist { "docker-ce"; "containerd.io"; }`).
- [ ] **Vaxt sinxronu**: `apt install systemd-timesyncd` (və ya chrony); `timedatectl` → `System clock synchronized: yes`. Token-lər və TLS buna bağlıdır (alert: `NebulaClockSkew`).
- [ ] **Arxitektura: `amd64` (x86_64)**. Release image-ləri yalnız `linux/amd64` üçündür; arm64 hostda `pull` `no matching manifest for linux/arm64` ilə dayanır (emulyasiya production üçün deyil).
- [ ] **Docker Engine** (rəsmi repo, `docker-ce`, `docker-compose-plugin` ≥ 2.35, Engine ≥ 28.1).
  `host/daemon.json` → `/etc/docker/daemon.json` (log limitləri, `live-restore`, `no-new-privileges`, `icc: false`, `userland-proxy: false`); `systemctl restart docker`.
- [ ] **sysctl**: `host/99-nebula-sysctl.conf` → `/etc/sysctl.d/`; `sysctl --system` (HTTP/3 UDP bufferləri, şəbəkə hardening).
- [ ] Alətlər: `apt install jq curl openssl util-linux git` (skriptlər `jq`, `flock` istifadə edir).
- [ ] **cosign** (`deploy.sh --verify` üçün): GitHub release-dən binar + `cosign_checksums.txt` ilə `sha256sum -c`, `/usr/local/bin/cosign`.
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
| `nebula_evidence_signing_key` | `nebula` | audit sübut paketlərinin Ed25519 imza açarı (base64, 32 bayt seed, ADR-0029); barmaq izi: Parametrlər → Audit log və ya `docker compose exec nebula /nebula evidence public-key` |
| `pgbackrest_*`, `remote_write_password` | postgres / alloy | `REPLACE_ME` placeholder |

Sonra əl ilə:

- `pgbackrest_s3_key`, `pgbackrest_s3_key_secret` — yalnız bir bucket-ə (və prefix-ə) yazma/oxuma/silmə hüququ olan açar;
- `remote_write_password` — Prometheus/Mimir remote-write parolu;
- SMTP: `.env`-də `NEBULA_SMTP_HOST/PORT/USERNAME/FROM/TLS` (sir deyil; `TLS` `starttls` və ya `tls`), parol `nebula_smtp_password` faylında.

Nebula sirləri **yalnız fayl** kimi alır (`NEBULA_*_FILE` → `/run/secrets/...`): env-də, `docker inspect`-də və `/proc/<pid>/environ`-da görünmürlər. `NEBULA_DATABASE_URL` sir deyil (parolsuz), parol ayrıca fayldan gəlir.

**Offline nüsxə (vacib):** `pgbackrest_cipher_pass` itərsə BÜTÜN backup-lar oxunmaz olur; `nebula_encryption_key` itərsə webhook sirləri açılmaz; `nebula_evidence_signing_key` itərsə köhnə paketlər yoxlanmağa davam edir, amma yeni paketlər başqa barmaq izi ilə imzalanır və auditorlara yeni barmaq izi ayrıca kanalla bildirilməlidir. Hamısını parol menecerinə / OpenBao-ya yazın.

**Mövcud quraşdırmanın yenilənməsi (ADR-0029):** `sudo ./scripts/gen-secrets.sh` yalnız çatışmayan `nebula_evidence_signing_key` faylını yaradır (qalanlara toxunmur); onsuz `preflight.sh` deploy-u dayandırır. Açarın dəyişdirilməsi (rotasiya): faylı yerində yeni `openssl rand -base64 32` ilə yazın və `nebula`-nı yenidən yaradın; köhnə barmaq izini qeyd edin — köhnə paketlər `nebula evidence verify --key <köhnə barmaq izi>` ilə yoxlanır.

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
  KEYCLOAK_IMAGE="$KEYCLOAK_IMAGE" POSTGRES_IMAGE="$POSTGRES_IMAGE" --verify
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
Skriptlərə əlavə mühit lazımdırsa (`NEBULA_COMPOSE_OVERRIDE`, `RESTORE_TEST_NETWORK`) unit-lərə drop-in ilə verin (`systemctl edit nebula-backup@.service` → `[Service]` / `Environment=...`): timer-lər interaktiv shell-in mühitini görmür.

## 3. İlk admin və ilk istifadəçi (Keycloak)

Realm şablonunda (`keycloak/realm/nebula-realm.json`) **heç bir istifadəçi yoxdur**; `nebula-web` client-in redirect URI-ləri `https://${NEBULA_HOST}`-dan yaranır. Realm yalnız mövcud deyilsə import olunur — sonrakı dəyişikliklər admin konsolundan edilir (və ya realm yenidən yaradılır).

> **Giriş səhifəsinin dili (ADR-0025):** realm 2026-11-24-dən əvvəl yaradılıbsa, interfeys dilləri üçün bir dəfə:
> Admin konsol → Realm settings → Localization → *Internationalization* açın, dillər `az, en, tr, ru`, default `az`.

#### Giriş səhifəsinin mövzusu (ADR-0036)

Image `nebula` giriş mövzusunu (`keycloak/themes/nebula`) daşıyır; yeni realm-lar onu şablondan alır. Realm
2026-11-24-dən əvvəl yaradılıbsa, yeni image deploy edildikdən sonra **bir dəfə** (parolu interaktiv soruşur, token
fayla yazılmır):

```bash
docker compose exec -it keycloak /opt/keycloak/bin/kcadm.sh update realms/nebula -s loginTheme=nebula \
  --no-config --server http://localhost:8080 --realm master --user <admin>
```

`realms/nebula` — `KEYCLOAK_REALM` dəyəri. kcadm parol axını ilə daxil olur; OTP məcburi olan admin üçün bu alınmaya
bilər — onda admin konsol (SSH tunel): Realm settings → Themes → Login theme: `nebula` → Save. Dərhal qüvvəyə minir,
restart lazım deyil; geri qaytarmaq: `loginTheme=keycloak.v2`. `nebula.css` dəyişəndə `theme.properties`-də `?v=`
artırılmalıdır (brauzer keşi 30 gün).

1. Müvəqqəti admin (parolu interaktiv soruşur, heç yerdə saxlanmır):
   ```bash
   sudo ./scripts/keycloak-bootstrap-admin.sh temp-admin
   ```
2. Konsol yalnız SSH tunel ilə: öz kompüterinizdə
   ```bash
   ssh -L 8081:127.0.0.1:8081 deploy@nebula.example.com
   ```
   brauzerdə `http://localhost:8081/admin/`. İctimai `https://auth.../admin` və `/realms/master` 404 qaytarır.
   **Bir dəfə, konsola girməzdən əvvəl:** `master` realm-ın ünvanını tunelə yönəldin. Əks halda konsol
   "Timeout when waiting for 3rd party check iframe message" xətası verir: yoxlama iframe-i ictimai
   `https://NEBULA_AUTH_HOST/realms/master/...`-dən yüklənir, Caddy isə onu bağlayır (404). Parolu interaktiv soruşur:
   ```bash
   docker compose exec -it keycloak /opt/keycloak/bin/kcadm.sh update realms/master \
     -s attributes.frontendUrl=http://localhost:8081 --no-config --server http://localhost:8080 --realm master --user temp-admin
   ```
   Bu yalnız `master` realm-a aiddir; `nebula` realm və istifadəçi girişi dəyişmir, `/realms/master` ictimai olaraq 404 qalır.
   Tunel yerli `8081` portunu tələb edir (`KC_HOSTNAME_ADMIN`); kompüterinizdə bu port məşğuldursa (məs. yerli dev Keycloak),
   əvvəlcə onu dayandırın. Bağlantı qırılanda tunel asılı qalmasın: `ssh -N -o ServerAliveInterval=15 -o ServerAliveCountMax=3 -L …`.
3. `master` realm-də şəxsi admin yaradın, ona `admin` rolunu verin, **OTP-ni məcburi edin** (Required user actions-da **yalnız** Configure OTP: Verify Email SMTP olmadan girişi bloklayır, WebAuthn təhlükəsizlik açarı istəyir), çıxın, şəxsi admin ilə girin və `temp-admin`-i **silin**.
4. `nebula` realm → Realm settings → Email: SMTP (parol sıfırlama üçün); sonra istəsəniz "Forgot password"-u yandırın.
5. İlk istifadəçi: `nebula` realm → Users → Add user (email, ad, `Email verified`), Credentials → müvəqqəti parol (`Temporary: on`) və ya "Send email: Update password". İstifadəçi ilk dəfə `https://nebula.example.com`-a daxil olur, workspace yaradır və onun sahibi olur. Digər istifadəçiləri workspace sahibi Nebula-da üzv kimi əlavə edir.

Brute-force qoruması açıqdır (5 səhv → artan gözləmə, maks. 15 dəq), access token 5 dəq, SSO idle 30 dəq, maks. 10 saat, refresh token rotasiyası.

> **Özünüqeydiyyat bağlı qalmalıdır** (`registrationAllowed: false`, şablonda belədir). Açılsa, auditoriyası "hesabı olan hər kəs" olan qəbul formaları (ADR-0032) praktiki olaraq ictimai olur: istənilən şəxs hesab yaradıb tələb göndərə bilər.

## 4. Backup-lar

| Nə | Necə | Nə vaxt |
|---|---|---|
| WAL (davamlı) | `archive_command` → pgBackRest async → S3 | hər seqment, ən gec 60 s |
| Full | `nebula-backup@full` | bazar 02:15 UTC |
| Differential | `nebula-backup@diff` | b.e.–şənbə 02:15 UTC |
| Incremental | `deploy.sh` / `rollback.sh --migrate-down-to` | hər deploy-dan və Down miqrasiyasından əvvəl |
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
# Docker seçimləri (-v) image adından ƏVVƏL gəlməlidir, ona görə komanda və image ayrıdır.
RESTIC='docker run --rm --env-file /etc/nebula/restic.env
  -v /etc/nebula/secrets/restic_password:/run/secrets/restic_password:ro'
IMG=restic/restic:0.20.0@sha256:93999df80d4ac6cb148e82d2ef1fe6a81996f17ff535e772723538211b31555a
VOL=$(docker volume ls -q --filter label=com.docker.compose.volume=attachments)

$RESTIC $IMG init                                                          # bir dəfə
$RESTIC -v "$VOL":/data:ro $IMG backup /data --host nebula --tag attachments   # hər gecə, DB backup-dan sonra (~03:00 UTC)
$RESTIC $IMG forget --host nebula --tag attachments --keep-daily 14 --keep-weekly 8 --prune   # ≥ 4 həftəlik PITR pəncərəsi
$RESTIC $IMG check --read-data-subset=5%                                   # həftəlik
```

Hələ timer yoxdur: cron/systemd-ə əl ilə əlavə edin və nəticəni izləyin (follow-up: `nebula-files-backup.timer` + metrik).

**Bərpa:**

```bash
docker compose stop nebula
$RESTIC $IMG snapshots --tag attachments                              # T-dən sonrakı ilk snapshot-u seçin
$RESTIC -v "$VOL":/data $IMG restore <snapshot-id> --target / --include /data
docker compose up -d --wait nebula
# Yoxlama: bazada olub faylı olmayan açar qalmamalıdır (çıxış boş olmalıdır).
docker compose exec -T postgres psql -U postgres -d nebula -Atc 'SELECT storage_key FROM attachments' | sort >/tmp/db-keys
docker run --rm -v "$VOL":/data:ro --entrypoint find $IMG \
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
| 00019 approvals | var | ⚠️ bütün təsdiq siyasətləri, sorğuları və qərarları (`approval_*`) və təsdiq bildirişləri silinir; təsdiq izi yalnız `events`-də (audit log) qalır. Gözləyən sorğusu olan tapşırıqlar kilidsiz qalır. **Kilidlər:** `notifications`-də qısa `ACCESS EXCLUSIVE` (kind CHECK yenidən yaradılır) |
| 00020 working calendar | var | iş təqvimi (həftə sonu günləri, bayramlar, köçürmələr) silinir; iş günü seçimi (`work_day`) və ya köçürmə (`shift`) istifadə edən təkrarlanma varsa Down xəta ilə dayanır — əvvəlcə onları adi tarixə dəyişin və ya silin. **Kilidlər:** Up/Down qısa `ACCESS EXCLUSIVE` (`workspaces`, `recurrences`, `recurrence_runs`; `lock_timeout` 5 s), cədvəl yenidən yazılmır |
| 00021 links and watchers | var | bütün tapşırıq əlaqələri (`item_links`), izləyicilər (`item_watchers`) və izləyici bildirişləri silinir; əlaqə hadisələri `events`-də qalır. **Kilidlər:** `notifications`-də qısa `ACCESS EXCLUSIVE` (kind CHECK yenidən yaradılır) |
| 00023 formula fields | var | formula sahəsi varsa Down xəta ilə dayanır (köhnə image `formula` tipini tanımır). Əvvəlcə təriflərini və dəyərlərini silin (ifadələr `events`-də `field.created`/`field.updated` kimi qalır, dəyərlər girişlərdən yenidən hesablanır): `DELETE FROM item_type_fields WHERE field_id IN (SELECT id FROM field_definitions WHERE field_type = 'formula'); UPDATE items i SET fields = i.fields - ARRAY(SELECT key FROM field_definitions f WHERE f.workspace_id = i.workspace_id AND f.field_type = 'formula') WHERE i.fields ?\| ARRAY(SELECT key FROM field_definitions f WHERE f.workspace_id = i.workspace_id AND f.field_type = 'formula'); DELETE FROM field_definitions WHERE field_type = 'formula';` Saxlanılan görünüşlərdəki `field.<key>` / `-field.<key>` sıralamasını köhnə image tanımır (UI onu nömrə sırası kimi açır, görünüşü yadda saxlamaq isə 422 verir) — `number`/`due`-ya dəyişin. **Kilidlər:** Up/Down `field_definitions`-də qısa `ACCESS EXCLUSIVE` (CHECK), `items`-də millisaniyəlik trigger əvəzlənməsi (`lock_timeout` 5 s), cədvəl yenidən yazılmır |
| 00025 intake forms | var | ⚠️ bütün qəbul formaları və göndərişlər (`forms`, `form_submissions`: tələb edənlərin "Tələblərim" tarixçəsi və komandanın mesajları) silinir; formadan yaradılmış tapşırıqlar, onların faylları və hadisələr qalır. `events.source` CHECK-ində `form` qalır (audit yenidən yazılmır). **Kilidlər:** `items`-ə millisaniyəlik `SHARE ROW EXCLUSIVE` (trigger), `events`-ə qısa `ACCESS EXCLUSIVE` (CHECK dəyişikliyi), `VALIDATE` yazmaları bloklamır; `lock_timeout` 5 s |
| 00027 approval re-approval and deadlines | var | təsdiq siyasətlərinin "dəyişiklikdə yenidən təsdiq" və müddət seçimləri, sorğulardakı `due_on`, `invalidated_at`, `reopened_from` silinir (sorğular və qərarlar qalır; etibarsız olma izi `events`-də `approval.invalidated` kimi qalır), `approval_overdue` / `approval_invalidated` bildirişləri silinir. Köhnə image yenidən təsdiq tələb etmir və xatırlatma göndərmir. **Kilidlər:** `approval_policies`, `approval_requests` və `notifications`-də qısa `ACCESS EXCLUSIVE` (sütun/CHECK; sabit default — cədvəl yenidən yazılmır), `VALIDATE` kiçik cədvəldə |
| 00028 workspace accent | var | yalnız workspace-lərin vurğu rəngi seçimi (`workspaces.accent`) silinir, hamısı standart firuzəyiyə qayıdır; başqa məlumat itmir, seçim `events`-də (`workspace.updated`) qalır. Köhnə image sütunu oxumur, hər iki sxemlə işləyir — çox vaxt yalnız image-i qaytarmaq kifayətdir. **Kilidlər:** Up/Down `workspaces`-ə millisaniyəlik `ACCESS EXCLUSIVE` (default-suz NULL sütun yalnız metadata dəyişikliyidir, cədvəl yenidən yazılmır və skan edilmir); `lock_timeout` 5 s |
| 00029 project default view | var | yalnız layihələrin standart görünüş seçimi (`projects.default_view_id`) silinir; layihələr yenə "Hamısı" ilə açılır, görünüşlər qalır. Köhnə image sütunu oxumur. **Kilidlər:** `projects`/`saved_views`-də millisaniyəlik kilid (sütun NULL, FK yoxlamasında axtarış yoxdur) |
| 00030 workspace email settings | var | workspace SMTP ayarları (şifrələnmiş parollarla) silinir; bütün workspace-lər yenə server konfiqurasiyası (`NEBULA_SMTP_*`) ilə göndərir. Köhnə image cədvəli oxumur; növbədəki `workspace_id`-li email job-larını köhnə image instance ilə göndərir. **Kilidlər:** yeni cədvəl, `workspaces`/`users`-də millisaniyəlik FK kilidi |
| 00031 archive and delete | var | arxiv və silmə markerləri (`workspaces.archived_at`, `workspaces.deletion_requested_at`, `projects.deletion_requested_at`) silinir: **arxivlənmiş workspace-lər köhnə image-də yenidən aktiv və yazıla bilən olur** (arxivlənmiş layihələr gizli qalır — `projects.archived_at` 00001-dəndir). Silmə davam edərkən Down xəta ilə dayanır — əvvəlcə worker-in silməni bitirməsini gözləyin. `workspace_deletions` (silinmiş workspace-lərin yeganə qeydi) qəsdən saxlanılır. **Kilidlər:** `workspaces`/`projects`-də millisaniyəlik `ACCESS EXCLUSIVE` (default-suz NULL sütunlar); `CHECK`-lər iki kiçik cədvəli bir dəfə skan edir; yeni cədvəlin `users`-ə FK kilidi |
| 00032 approval delegation | var | **Gözləyən təsdiq sorğusunda əvəzləyənin qərarı varsa Down dayanır** (köhnə image onu əvəzləyənin öz səsi sayar və kimin hələ təsdiqləməli olduğu dəyişərdi): əvvəlcə həmin sorğuların bağlanmasını gözləyin və ya ləğv edin. `workspace_absences` (məzuniyyətlər və əvəzləyənlər) silinir; `approval_decisions.on_behalf_of` və `approval_decisions_principal_idx` silinir — kimin adına verildiyi `approval.approved`/`approval.rejected` hadisələrinin `on_behalf_of` sahəsində (audit log, sübut paketi) qalır, qərar sətri real icraçını saxlayır. Down-suz yalnız image rollback-də köhnə image sütunu və cədvəli oxumur; yalnız bir kənar halda (əvəzləyən artıq qərar verib, sonra məzuniyyətdəki təsdiqçi özü təsdiqləməyə çalışır) köhnə image unikal indeksdən 500 qaytarır — dörd göz pozulmur. **Kilidlər:** yeni cədvəl (`workspaces`/`users`-də millisaniyəlik FK kilidi); `approval_decisions`-də default-suz sütun (millisaniyəlik `ACCESS EXCLUSIVE`), `CHECK` bir skan, unikal indeks qurularkən cədvələ yazma bloklanır (kiçik cədvəl, < 1 s) |
| 00033 service tokens | var | `service_tokens` və `service_token_projects` silinir: **bütün API tokenləri dərhal işləməz olur** (köhnə image `nbl_` formatını tanımır, inteqrasiyalar 401 alır); rollback-dən sonra yenidən yaradılmalıdır. Token prinsipalları `users`-də qalır, Down onları deaktiv edir — tarixçədə "🔑 …" adı görünməyə davam edir. Down-suz yalnız image rollback-də cədvəllər qalır, köhnə image onları oxumur və tokenləri qəbul etmir. **Kilidlər:** iki yeni boş cədvəl; `workspaces`/`users`/`projects`-də millisaniyəlik FK kilidi, mövcud sətirlər skan edilmir |
| 00034 money fields | var | `field_type` CHECK-i əvvəlki siyahıya qaytarılır. **Money sahəsi varsa Down dayanır** (köhnə image tipi tanımır): əvvəl sahələri silin və ya dəyərləri ixrac edin. Down-suz yalnız image rollback-də money sahələri qalır, köhnə image onların dəyərlərini validasiya edə bilmir (yazılar 500/422), oxuma işləyir. **Kilidlər:** `field_definitions`-də millisaniyəlik `ACCESS EXCLUSIVE` (kiçik cədvəl, CHECK yenidən yoxlanır), `items`-ə toxunulmur |
| 00035 SLA targets | var | `project_sla_policies` (layihələrin SLA hədəfləri), `item_sla` (hesablanmış SLA müddətləri, cavab/həll vaxtları), iki trigger və `sla_at_risk`/`sla_breached` bildirişləri silinir; `item.sla_*` və `project.sla_updated` hadisələri `events`-də qalır. Hədəfləri yenidən daxil etmək üçün əvvəlcədən qeyd edin (`GET …/projects/{key}/sla`). Down-suz yalnız image rollback-də cədvəllər qalır, köhnə image onları oxumur; triggerlər işləməyə davam edir (zərərsiz). **Kilidlər:** yeni cədvəllər; `items` və `comments`-də millisaniyəlik `SHARE ROW EXCLUSIVE` (CREATE TRIGGER, skan yoxdur); `notifications`-də CHECK dəyişikliyi üçün `ACCESS EXCLUSIVE` və sətirlərin yoxlanması (retensiya ilə məhdud, böyük quraşdırmada saniyələr) |
| 00036 service token restriction | var | `service_tokens.project_restricted` silinir. Down əvvəlcə **siyahısı boşalmış məhdud tokenləri ləğv edir** (prinsipallar deaktiv) — köhnə image boş siyahını "bütün layihələr" kimi oxuyur və onları workspace-ə genişləndirərdi; siyahısı olan tokenlər işləməyə davam edir. Down-suz yalnız image rollback-də sütun qalır, köhnə image onu oxumur: əvvəlcə boş siyahılı məhdud tokenləri yoxlayın (`SELECT id FROM service_tokens t WHERE project_restricted AND revoked_at IS NULL AND NOT EXISTS (SELECT 1 FROM service_token_projects sp WHERE sp.token_id = t.id)`) və ləğv edin. **Kilidlər:** `service_tokens`-də millisaniyəlik `ACCESS EXCLUSIVE` (sabit default ilə sütun, yenidən yazılma yoxdur); backfill yalnız məhdud tokenləri yeniləyir |
| 00037 period cycles | var | `period_cycles` (aylıq dövr ayarları), `period_closes` (bağlanmış aylar) və `period_snapshots` (ayların snapshot-ı) silinir — **bağlanmış ayların tarixçəsi itir**; əvvəlcə lazım olan ayları cədvəl kimi ixrac edin. `cycle.updated`, `period.closed` hadisələri `events`-də qalır. Down-suz yalnız image rollback-də cədvəllər qalır, köhnə image onları oxumur və scheduler aylıq bağlanışı etmir. **Kilidlər:** yalnız yeni cədvəllər |
| 00038 user notes | var | `user_notes` (şəxsi qeydlər) silinir — **qeydlər itir**. Down-suz image rollback-də cədvəl qalır, köhnə image onu oxumur. **Kilidlər:** yalnız yeni cədvəl |
| 00039 workspace home projects | var | `workspaces.home_projects` silinir — yalnız ana səhifə keçidlərinin seçimi itir. Köhnə image sütunu oxumur, Down-suz rollback təhlükəsizdir. **Kilidlər:** `workspaces`-da millisaniyəlik `ACCESS EXCLUSIVE` (sabit default, yenidən yazılma yoxdur) |
| 00040 date reminders | var | `date_reminders` (ad günü xatırlatması seçimi) və `anniversary` bildirişləri silinir, bildiriş növləri CHECK-i əvvəlki siyahıya qayıdır. Köhnə image cədvəli oxumur və bu növü göndərmir; Down-suz rollback-də yeni CHECK köhnə növlərin hamısını qəbul edir. **Kilidlər:** yeni cədvəl; `notifications`-da CHECK yoxlaması müddətində `ACCESS EXCLUSIVE` (ən çox 180 günlük bildiriş) |

**Audit sübut ixracı (ADR-0029) miqrasiyasızdır** (00022 nömrəsi istifadə edilmədi): rollback yalnız image-dir. Köhnə image `NEBULA_EVIDENCE_SIGNING_KEY_FILE`-ı nəzərə almır, `evidence.exported` hadisələri audit log-da qalır; artıq verilmiş paketlər bu və sonrakı release-lərin `nebula evidence verify` əmri ilə yoxlanmağa davam edir.

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
# glob-lar host-un shell-ində açılır, ona görə deploy/production qovluğundan:
(cd deploy/production && docker run --rm -v "$PWD:/mnt:ro" -w /mnt koalaman/shellcheck:v0.11.0 -x scripts/*.sh postgres/nebula-pgbackrest postgres/initdb/*.sh keycloak/*.sh)
go test ./internal/database -run RuntimeRole   # NEBULA_TEST_DATABASE_URL ilə: runtime rolu DDL edə bilmir
```

Smoke test lazımdırsa: fərqli project adı (`COMPOSE_PROJECT_NAME=nebula-prodtest`), `NEBULA_BIND_ADDR=127.0.0.1`, yüksək portlar, ayrıca edge subnet; dev stack-ə (`nebula` project) toxunmayın və bitəndə `docker compose -p nebula-prodtest down -v` edin.

## 13. Paylaşılan hostda pilot

Host-da artıq başqa Docker Compose stack-i işləyir və onun Caddy-si 80/443-ü tutur. Nebula həmin Caddy-nin arxasında işləyir:
xarici Caddy `NEBULA_HOST` və `NEBULA_AUTH_HOST` üçün TLS-i bitirir və sorğuları ortaq Docker şəbəkəsi üzərindən HTTP ilə Nebula-nın öz Caddy-sinə ötürür.
Nebula Caddy-si bütün qaydalarını saxlayır (CSP, HSTS və digər header-lər, `/api` proxy, `/admin` bloku, body limitləri), ona görə tətbiqin davranışı bölmə 1–11 ilə eynidir.

```
brauzer ──TLS──▶ mövcud Caddy (80/443, şəbəkə stack_web) ──HTTP──▶ nebula-edge:80 (Nebula Caddy) ──▶ nebula / keycloak
nebula (OIDC discovery, JWKS) ──host-gateway:443──▶ mövcud Caddy ──▶ … (TLS ictimai sertifikatla doğrulanır)
```

**Nə vaxt:** yalnız pilot və qısa müddətli sınaq üçün, ayrıca VPS hələ yoxdursa. Production ayrıca VPS-dədir (bölmə 1–11); köçmə aşağıda.

| | Ayrıca VPS | Paylaşılan host |
|---|---|---|
| TLS, ACME, HTTP→HTTPS | Nebula Caddy | Mövcud Caddy |
| Publish olunan portlar | Nebula Caddy 80/443 | Heç biri (yalnız Keycloak admin `127.0.0.1:8081`) |
| Header-lər, CSP, `/api`, `/admin` bloku, body limitləri | Nebula Caddy | Nebula Caddy (dəyişməz) |
| Müştəri IP-si | TCP peer | `X-Forwarded-For` yalnız `NEBULA_SHARED_PROXY_SUBNET`-dən |
| Nebula → IdP (`https://NEBULA_AUTH_HOST`) | edge alias ilə Nebula Caddy | `host-gateway` → mövcud Caddy (ictimai sertifikat) |
| `host/daemon.json`, sysctl, ufw | tətbiq olunur | **tətbiq olunmur** (mövcud stack-ə təsir edir) |

Nebula tərəfi: `compose.shared-host.yaml` (override) Nebula Caddy-ni xarici şəbəkəyə (`NEBULA_SHARED_PROXY_NETWORK`) `NEBULA_SHARED_PROXY_ALIAS` (default `nebula-edge`) adı ilə qoşur, port publish etmir, `NEBULA_SITE_SCHEME=http` ilə Caddyfile-da ACME və redirect-i söndürür, `NEBULA_TRUSTED_PROXY_CIDRS` ilə xarici proxy-nin subnet-inə etibar edir. Nebula və Keycloak yenə yalnız Nebula Caddy-nin sabit ünvanına etibar edir; Caddy onlara həll olunmuş müştəri IP-sini bir hop kimi ötürür. Bütün ictimai URL-lər (`NEBULA_PUBLIC_URL`, OIDC issuer, `KC_HOSTNAME`) `https://` qalır.
Caddyfile web image-ə bişirilmir (`./caddy/Caddyfile` mount olunur), ona görə bu rejim mövcud image-lərlə işləyir.

### 13.1 Risklər

- **Blast radius:** ictimai tətbiqlə eyni kernel, Docker daemon, disk və yaddaş. Birinin OOM-u, disk dolması, Docker restartı və ya upgrade-i hər ikisini dayandırır; bir konteynerdən host-a qaçış digərini də ələ keçirir. Hər iki stack-in sahibi və yeniləmə pəncərələri razılaşdırılmalıdır.
- **Yaddaş:** rehearsal-da Nebula stack-i sabit ~1.5 GiB, pik ~1.9 GiB (Keycloak ~0.75–0.9 GiB, Alloy ~0.37, Postgres ~0.2–0.4, Nebula ~0.13–0.19, Caddy ~0.06), xarici Caddy ~20–30 MiB. Konteyner limitlərinin cəmi ~5.3 GiB-dir (Postgres 2G, Keycloak 1.5G, Nebula 1G, Alloy 512M, Caddy 256M): 8 GB-lıq hostda hamısı eyni anda limitə çatarsa yaddaş çatmaz. `free -m`-i və `NebulaMemoryPressure`/OOM alertlərini izləyin; swap 1–2 GB olsun.
- **Mövcud Caddy ortaq nöqtədir:** onun konfiqurasiya xətası və ya restartı Nebula-nı da dayandırır, Nebula bloku isə səhv olarsa digər saytları da sındıra bilər. Ona görə həmişə əvvəlcə `caddy validate`, sonra `caddy reload` (restart yox).
- **Ortaq şəbəkə:** `stack_web`-dəki hər konteyner Nebula Caddy-yə birbaşa HTTP ilə çata bilər və `X-Forwarded-For`-u saxtalaşdıra bilər (rate limit açarı, access log-dakı `client_ip`). Bütün subnet etibarlıdır, çünki xarici Caddy-nin IP-si sabit deyil. Autentifikasiya buna bağlı deyil (token tələb olunur). Pilot üçün qəbul edilən risk; o stack-in konteynerləri sizin nəzarətinizdədir.
- **Host hardening yoxdur:** `host/daemon.json` (`icc: false`, `userland-proxy: false`, `live-restore`), sysctl və ufw dəyişiklikləri mövcud stack-ə təsir edir və `systemctl restart docker` hər iki stack-i dayandırır, ona görə tətbiq olunmur. Nebula servislərinin öz hardening-i (read-only FS, `cap_drop: ALL`, `no-new-privileges`, limitlər) qalır.
- **Pinlənməmiş image:** mövcud stack-in `caddy:2-alpine` tag-i digest ilə pinlənməyib. `docker compose pull` gözlənilmədən yeni Caddy versiyası gətirə bilər, bu da Nebula-ya da təsir edər. Tövsiyə: onu digest ilə pinləyin (`caddy:2.11.4-alpine@sha256:…`, digest `docker buildx imagetools inspect caddy:2.11.4-alpine` ilə). Bu runbook həmin stack-in compose faylını dəyişmir.
- **Host-un öz sorğuları:** host-dan `127.0.0.1:443`-ə gələn sorğular (məs. smoke test) Nebula log-unda xarici Caddy-nin IP-si ilə görünür (userland-proxy). Xaricdən gələn sorğularda real IP görünür.
- **Monitorinq:** Alloy Nebula-nı yığır, mövcud Caddy-ni yox. İctimai prob (`blackbox`) xarici Caddy-dən keçir, yəni əsl istifadəçi yolunu ölçür.

### 13.2 Quraşdırma

Mövcud stack-in compose faylı dəyişdirilmir. Ona yalnız Caddyfile-a sayt bloku əlavə olunur və `caddy reload` edilir.

1. **Yoxlama.** Mövcud Caddy-nin şəbəkəsi və subnet-i, boş portlar, yaddaş və disk:
   ```bash
   docker network inspect <stack_web> -f '{{range .IPAM.Config}}{{.Subnet}} {{end}}'   # məs. 172.19.0.0/16
   docker network ls -q | xargs docker network inspect -f '{{.Name}} {{range .IPAM.Config}}{{.Subnet}} {{end}}'   # 172.30.10.0/24 boşdur?
   ss -ltn | grep -E ':(8081)\b' || echo "8081 boşdur"
   free -m; df -h /var/lib/docker
   ```
   Bölmə 1-dən yalnız bunlar: amd64, alətlər (`jq curl openssl util-linux git`), cosign, vaxt sinxronu. `daemon.json`, sysctl və ufw-ya toxunmayın.
2. **DNS:** `NEBULA_HOST` və `NEBULA_AUTH_HOST` üçün A/AAAA qeydləri bu host-a (CAA: `0 issue "letsencrypt.org"`). Sertifikatı mövcud Caddy alır, ona görə DNS 4-cü addımdan əvvəl hazır olmalıdır.
3. **Kod, `.env`, sirlər:** bölmə 2.1 və 2.2 eynidir. `.env`-də əlavə olaraq (nümunə `.env.example`-dadır):
   ```bash
   COMPOSE_FILE=compose.yaml:compose.shared-host.yaml   # skriptlər VƏ əl ilə docker compose eyni faylları görür
   NEBULA_SHARED_PROXY_NETWORK=<stack_web>
   NEBULA_SHARED_PROXY_SUBNET=<172.19.0.0/16>            # 1-ci addımdakı subnet
   NEBULA_SHARED_PROXY_ALIAS=nebula-edge
   ```
   `NEBULA_HTTPS_PORT=443` qalsın (smoke test xarici Caddy-dən keçir). `ACME_EMAIL`/`ACME_CA` doldurulmalıdır, amma bu rejimdə istifadə olunmur. `NEBULA_BIND_ADDR`/`NEBULA_HTTP_PORT` nəzərə alınmır.
   `COMPOSE_FILE` `.env`-də olmalıdır, təkcə `NEBULA_COMPOSE_OVERRIDE`-də yox: əks halda əl ilə `docker compose up` override olmadan Caddy-ni 80/443-də açmağa çalışar, Nebula isə IdP-yə yolunu itirər. `preflight.sh` bunu, şəbəkənin mövcudluğunu, subnet-i və alias-ın başqa konteynerdə olmadığını yoxlayır.
   ```bash
   sudo ./scripts/preflight.sh
   ```
4. **Mövcud Caddy-yə sayt bloku:** `shared-host/outer-caddy.snippet`-dəki nümunə hostnamelərini `NEBULA_HOST`/`NEBULA_AUTH_HOST` ilə əvəz edib onun Caddyfile-ının sonuna əlavə edin. Caddyfile çox vaxt tək fayl kimi mount olunur: faylı **yerində** dəyişin (`>>` ilə əlavə və ya `cat new > Caddyfile`). `sed -i` və bəzi redaktorlar yeni inode yaradır, onda konteyner köhnə faylı görür.
   ```bash
   cd /opt/stack
   cp -p Caddyfile Caddyfile.bak.$(date +%F)
   { echo; sed -e 's/auth\.nebula\.example\.com/<NEBULA_AUTH_HOST>/g' -e 's/nebula\.example\.com/<NEBULA_HOST>/g' \
       /opt/nebula/deploy/production/shared-host/outer-caddy.snippet; } >> Caddyfile
   docker compose exec <caddy> tail -n 5 /etc/caddy/Caddyfile                      # konteyner yeni məzmunu görür?
   docker compose exec <caddy> caddy validate --config /etc/caddy/Caddyfile        # "Valid configuration"
   docker compose exec <caddy> caddy reload --config /etc/caddy/Caddyfile          # restart yox
   docker compose logs --since 2m <caddy> | grep -E 'certificate obtained|error'   # iki sertifikat
   curl -sS -o /dev/null -w '%{http_code}\n' https://<mövcud sayt>/               # digər saytlar işləyir
   ```
   Bu anda Nebula hostnamelərində 502 normaldır (Nebula hələ yoxdur).
5. **Baza, backup, deploy:** bölmə 2.4 və 2.5 eynidir (`deploy.sh … --verify`). Smoke test `127.0.0.1:443` üzərindən xarici Caddy-dən keçir (real TLS).
6. **Timer-lər və admin:** bölmə 2.6 və 3 eynidir. `COMPOSE_FILE` `.env`-də olduğu üçün systemd drop-in lazım deyil.
7. **Yoxlama:**
   ```bash
   curl -sS -o /dev/null -D - https://<NEBULA_HOST>/ | grep -iE 'strict-transport|content-security|x-frame'   # hər biri bir dəfə
   docker compose logs --since 5m nebula | grep '"route":"/api/v1/me"' | tail -3   # client_ip = xarici müştəri IP-si, 172.19.x deyil
   ```

### 13.3 Rollback və pilotun dayandırılması

- **Nebula release-ini geri qaytarmaq:** bölmə 9 eynidir; mövcud stack-ə toxunmur.
- **Pilotu söndürmək** (məlumat qalır): `docker compose down` (volume-lar qalır; `stack_web` external-dır, silinmir). Sonra mövcud Caddyfile-dan Nebula blokunu silin (yerində, məs. `cp -p Caddyfile.bak.<tarix> /tmp/c && cat /tmp/c > Caddyfile`), `caddy validate`, `caddy reload`. Söndürülmüş Nebula-nın hostnamelərinə mövcud Caddy 502 qaytarır (HSTS ilə), digər saytlar işləməyə davam edir.
- **Tam silmək:** yalnız son backup-ın (`backup.sh info`) və fayl backup-ının (bölmə 4.1) yoxlanmasından sonra: `docker compose down -v`, `/etc/nebula`, `/var/lib/nebula`.

### 13.4 Ayrıca VPS-ə köçmə

Bölmə 6-dakı "host tam itibsə" axını ilə (rehearsal-da yoxlanılıb), eyni image digest-ləri ilə:

1. Yeni VPS: bölmə 1 tam (hardening daxil). Yerli backup (13.5) istifadə olunursa, əvvəlcə off-site S3-ə köçün: köhnə hostun diski yeni hosta gəlmir. `/opt/nebula`, `.env` (shared-host sətirlərini silin: `COMPOSE_FILE`, `NEBULA_SHARED_PROXY_*`), `/etc/nebula/secrets` (offline nüsxədən və ya OpenBao-dan, eyni dəyərlərlə).
2. DNS TTL-ini bir gün əvvəl 300 s-ə endirin.
3. Köhnə hostda yazmaları dayandırın və son vəziyyəti arxivləşdirin: `docker compose stop caddy nebula keycloak`, `sudo ./scripts/backup.sh incr`, `sudo ./scripts/backup.sh check`, faylları restic ilə (bölmə 4.1). Sonra `docker compose down`: **iki Postgres eyni pgBackRest stanza-sına eyni anda arxiv yazmamalıdır.**
4. Yeni hostda: `docker compose up -d --wait postgres` → `sudo ./scripts/restore.sh` → attachments restic-dən → `sudo ./scripts/deploy.sh <eyni digest-lər> --verify`.
5. DNS-i yeni IP-yə yönəldin; yeni Nebula Caddy sertifikatı özü alır (80/443 açıq olmalıdır). Yoxlama: login, `/api/v1/me`, `backup.sh check`, `restore-test.sh`.
6. Köhnə hostda Nebula blokunu mövcud Caddyfile-dan silin (13.3), volume-ları bir müddət saxlayıb sonra silin.

### 13.5 Pulsuz yerel backup (pilot)

Pilot dövründə xarici object storage üçün büdcə yoxdursa, pgBackRest **eyni hostdakı** S3 serverinə yazır:
`backup-s3` servisi (versitygw, posix backend), məlumat host-da `${NEBULA_STATE_DIR}/backup-s3` qovluğunda adi fayllar kimi saxlanılır.
Bu **müvəqqəti** həlldir; production off-site S3 ilə işləyir (bölmə 4).

```
postgres (pgBackRest, TLS ca.crt ilə doğrulanır) ──HTTPS, şəbəkə <project>-backup (internal)──▶ backup-s3:7070 ──▶ /var/lib/nebula/backup-s3
restore-test konteyneri ──eyni şəbəkə──▶ backup-s3
```

**Nədən qoruyur:** səhv silinmə və ya yenilənmə, məntiqi korlanma, uğursuz miqrasiya — PITR ilə istənilən ana qayıtmaq (retention pəncərəsi daxilində), aylıq restore testi.
**Nədən QORUMUR:** host-un, diskin və ya VPS hesabının itməsi, ransomware/root kompromisi (Postgres konteynerində `backup-s3`-ün root açarı var; host-da root olan hər kəs faylları silə bilər). Backup verilənlər bazası ilə **eyni diskdədir**.

- **netcup server snapshot-larını mütləq yandırın** (provayder panelində, məs. gündəlik və ya həftəlik, bir neçə nüsxə): host itkisinə qarşı yeganə əlavə qat budur. Snapshot ardıcıl deyilsə belə, pgBackRest repo-su (şifrəli, `backup-s3` qovluğu) ondan bərpa oluna bilər.
- Mümkünsə əlavə olaraq qovluğu başqa maşına köçürün (repo artıq client-side şifrəlidir): `rsync -aX --delete /var/lib/nebula/backup-s3/ <başqa-host>:/backup/nebula-s3/` (`-X`: versitygw obyekt metadata-sını xattr-da saxlayır).
- **Qayda:** pilot bitməzdən əvvəl, ən gec **Mattermost Boards söndürülməzdən və Nebula məlumatın yeganə nüsxəsinə çevrilməzdən əvvəl** off-site S3-ə keçin (aşağıda "Off-site S3-ə köçmə"). O vaxta qədər Boards ikinci nüsxədir.

#### Quraşdırma (13.2-nin 3-cü addımına əlavə)

`.env`-də (nümunə `.env.example`-dadır; yuxarıdakı `PGBACKREST_S3_*` dəyərlərini əvəz edir):

```bash
COMPOSE_FILE=compose.yaml:compose.shared-host.yaml:compose.local-s3.yaml
NEBULA_LOCAL_S3=1
PGBACKREST_S3_ENDPOINT=backup-s3:7070        # port endpoint-də host:port kimi
PGBACKREST_S3_REGION=us-east-1               # istənilən; backup-s3 eyni regionla başlayır
PGBACKREST_S3_BUCKET=nebula-backups
PGBACKREST_S3_URI_STYLE=path
PGBACKREST_S3_VERIFY_TLS=y                   # TLS doğrulanır (aşağıda)
PGBACKREST_S3_CA_FILE=/var/lib/nebula/backup-s3-tls/ca.crt
RESTORE_TEST_NETWORK=nebula-prod-backup      # <COMPOSE_PROJECT_NAME>-backup
```

```bash
sudo ./scripts/local-s3-init.sh                    # CA + sertifikat (SAN backup-s3), bucket qovluğu; idempotent
sudo NEBULA_LOCAL_S3=1 ./scripts/gen-secrets.sh    # pgbackrest_s3_key / _secret təsadüfi yaradılır (REPLACE_ME varsa əvəz olunur)
sudo ./scripts/preflight.sh                        # TLS faylları, sahiblik, bucket qovluğu, endpoint, RESTORE_TEST_NETWORK
docker compose up -d --wait postgres               # backup-s3 də başlayır (depends_on: service_started)
sudo ./scripts/backup.sh init && sudo ./scripts/backup.sh full
```

Sonra 2.5 (deploy) və 2.6 (timer-lər) dəyişməz. `RESTORE_TEST_NETWORK` `.env`-dədir, drop-in lazım deyil.

**Dizayn qərarları:**

- **Bir həqiqət mənbəyi:** `backup-s3`-ün root açarları pgBackRest-in oxuduğu eyni sir fayllarıdır (`pgbackrest_s3_key`, `pgbackrest_s3_key_secret`, compose secrets). versitygw açarları yalnız flag və ya env-dən oxuyur, ona görə image-in öz `sh`-i faylları oxuyub `exec`-dən dərhal əvvəl prosesin env-inə yükləyir: açarlar `.env`-də, compose faylında, komanda sətrində və `docker inspect`-də (`Config.Env`) görünmür. Konteyner daxilində `/proc/1/environ`-u yalnız həmin uid (10002) və root oxuya bilər.
- **TLS doğrulanır** (`PGBACKREST_S3_VERIFY_TLS=n` lazım deyil): pgBackRest `repo1-storage-ca-file` seçimini environment-dən oxuyur (`PGBACKREST_REPO1_STORAGE_CA_FILE`), ona görə buraxılmış image yenidən qurulmur. `local-s3-init.sh` öz CA-sını yaradır, sertifikatı imzalayır və **CA açarını silir** (başqa heç nə imzalana bilməz). Rehearsal-da səhv CA ilə `[095] unable to verify certificate` alındı.
- **Şəbəkə:** `backup-s3` yalnız `<project>-backup` şəbəkəsindədir (`internal: true`, yalnız postgres və restore-test konteyneri). Host-da port publish olunmur; edge, db, metrics, egress və `stack_web`-dən nə DNS adı, nə IP əlçatandır (rehearsal-da yoxlanılıb). Host-un özü (root namespace) konteyner IP-sinə çata bilər — hər konteyner kimi; açarsız heç nə edə bilməz.
- **Davamlılıq:** `VGW_FSYNC=true` — hər obyekt diskə yazılmadan PUT təsdiqlənmir (təsdiqlənmiş WAL seqmenti host çöksə də itməməlidir).
- **Hardening:** read-only root FS, `cap_drop: ALL`, `no-new-privileges`, uid 10002, 256M limit, healthcheck (`/health`), `init: true`, log rotasiyası. Bind mount-lar `create_host_path: false` ilə: qovluq yoxdursa konteyner başlamır (Docker onu root sahibliyi ilə yaratmır).
- **`backup-s3` restartı:** async arxiv WAL-ı spool-da saxlayır; rehearsal-da yük altında restart və 20 s dayanma zamanı Postgres crash-restart etmədi (`init: true`), arxiv özü bərpa olundu.

**Sertifikat:** 10 il etibarlıdır (yalnız daxili ad), `preflight.sh` bitməyə 30 gün qalmış FAIL verir. Yeniləmə:
`sudo ./scripts/local-s3-init.sh --renew && docker compose up -d --force-recreate backup-s3 postgres` (postgres `ca.crt`-ni fayl kimi mount edir, köhnə inode-u görür).

#### Disk istifadəsi və retention

Retention `postgres/pgbackrest.conf`-dadır və dəyişmir: 4 full (həftəlik → ~4 həftə PITR), 7 diff; köhnə full silinəndə ondan əvvəlki WAL da silinir (`expire` hər backup-dan sonra). Repo zstd ilə sıxılır və şifrələnir.

| Rehearsal (sintetik məlumat) | Ölçü |
|---|---|
| Klaster (nebula + keycloak, 2000–2500 tapşırıq) | 38–55 MB |
| Bir full backup repo-da | 4.6–6.9 MB (~12%) |
| WAL, 62 seqment (yük testi daxil) | 14 MB (median seqment ~115 KB, maks. 2.8 MB) |
| Boş stack | ~1 seqment / 10 dəq |

Gözlənilən: repo ≈ 5 × full (rotasiya zamanı müvəqqəti 5-ci) + diff-lər + ~5 həftəlik WAL. 1 GB-lıq baza üçün ~0.5–1.5 GB full-lar + gündə 50–200 MB sıxılmış WAL ≈ **cəmi 3–8 GB** — 228 GB-da problem deyil, amma disk **Docker və digər stack ilə ortaqdır**: `df -h /var/lib/nebula`, `du -sh /var/lib/nebula/backup-s3` (həftəlik) və disk alertini izləyin; `preflight.sh` 80%-dən sonra xəbərdarlıq edir. Böyük importdan və ya `VACUUM FULL`-dan sonra WAL müvəqqəti artır.

#### Off-site S3-ə köçmə

**Tövsiyə olunan yol: pgBackRest multi-repo (repo2)** — rehearsal-da yoxlanılıb. WAL köçmə boyu hər iki repo-ya gedir, PITR-də boşluq yaranmır; köhnə yerli repo yeni repo-da restore testi keçənə qədər toxunulmaz qalır. Eyni stanza, eyni şifrə (`pgbackrest_cipher_pass`): off-site repo sonra dəyişmədən repo1 olur. Bir stanza-ya iki **ayrı** Postgres yazmır, ona görə toqquşma yoxdur; iki ayrı konfiqurasiya ilə eyni stanza-nı paralel işlətməyin.

1. Off-site bucket (versioning/Object Lock ilə, bölmə 4) və yalnız bu bucket üçün açar. Sir faylları (`0440 root:nebula-secrets`):
   `/etc/nebula/secrets/pgbackrest_repo2_s3_key`, `/etc/nebula/secrets/pgbackrest_repo2_s3_key_secret`.
2. `.env`:
   ```bash
   COMPOSE_FILE=compose.yaml:compose.shared-host.yaml:compose.local-s3.yaml:compose.repo2-offsite.yaml
   PGBACKREST_REPO2_S3_ENDPOINT=<s3-endpoint>
   PGBACKREST_REPO2_S3_REGION=<region>
   PGBACKREST_REPO2_S3_BUCKET=<bucket>
   PGBACKREST_REPO2_S3_URI_STYLE=host           # path: MinIO/Garage
   PGBACKREST_REPO2_PATH=/nebula-prod
   PGBACKREST_BACKUP_REPO=2                     # iki repo ilə backup --repo tələb edir; timer-lər yeni repo-ya yazır
   ```
   v0.1.0 image-inin wrapper-i yalnız repo1 açarlarını yükləyir; override repo-dakı yeni `postgres/nebula-pgbackrest`-i mount edir (image yenilənəndə həmin sətir silinir).
3. ```bash
   sudo ./scripts/preflight.sh
   docker compose up -d --wait postgres          # repo2 env ilə yenidən yaranır
   sudo ./scripts/backup.sh init                 # stanza-create: repo1 "already exists and is valid", repo2 yaradılır; check hər ikisini
   sudo ./scripts/backup.sh full                 # repo2-də ilk full
   sudo RESTORE_TEST_REPO=2 ./scripts/restore-test.sh   # yeni repo-dan bərpa (RESTORE_TEST_REPO2_NETWORK, default bridge)
   ```
   `stanza-upgrade` yalnız Postgres major versiyası dəyişəndə lazımdır; burada `stanza-create` kifayətdir.
4. Bir neçə gün hər iki repo ilə işləyin (`backup.sh info` hər ikisini göstərir). Sonra **cut-over:** repo2 dəyərləri repo1-ə keçir:
   ```bash
   # sir faylları yerində (inode saxlanılır): off-site açarları pgbackrest_s3_key / _secret-ə
   for k in key key_secret; do f=/etc/nebula/secrets/pgbackrest_s3_$k; chmod 0640 $f
     cat /etc/nebula/secrets/pgbackrest_repo2_s3_$k > $f; chmod 0440 $f; done
   # .env: COMPOSE_FILE=compose.yaml:compose.shared-host.yaml; PGBACKREST_S3_* = off-site dəyərləri,
   #       PGBACKREST_S3_VERIFY_TLS=y, RESTORE_TEST_NETWORK=bridge (və ya silin);
   #       NEBULA_LOCAL_S3, PGBACKREST_S3_CA_FILE, PGBACKREST_BACKUP_REPO, PGBACKREST_REPO2_* sətirlərini silin
   sudo ./scripts/preflight.sh
   docker compose up -d --wait --remove-orphans  # postgres yeni repo1 ilə; backup-s3 konteyneri silinir, qovluq qalır
   sudo ./scripts/backup.sh check && sudo ./scripts/backup.sh incr && sudo ./scripts/restore-test.sh
   ```
5. Yerli repo qovluğunu (`/var/lib/nebula/backup-s3`, `backup-s3-tls`) ən azı bir retention dövrü (~4 həftə) saxlayın, sonra silin. Repo2 sir fayllarını silin.

**Alternativ (multi-repo olmadan):** repo1-in endpoint və açarlarını birbaşa off-site-a dəyişin → `docker compose up -d --wait --remove-orphans` → `backup.sh init` (yeni boş repo-da `stanza-create`) → dərhal `backup.sh full` → `restore-test.sh`. Daha sadədir, amma kəsilmə ilə ilk full arasında PITR yalnız köhnə repo-dan (kəsilmə anına qədər) mümkündür; yeni repo restore testini keçməyibsə köhnə konfiqurasiyaya qayıdıb yenidən `backup.sh full` edin.
