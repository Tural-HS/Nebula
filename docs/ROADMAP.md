# Yol xəritəsi

Hər mərhələ yalnız testlər, miqrasiyalar (up və down) və CI yaşıl olduqda tamamlanmış sayılır.

## Phase 0: Təməl (tamamlandı)
- [x] Arxitektura qərarları (ADR-0001)
- [x] Əsas sxem: workspace-lər, istifadəçilər, layihələr, item tipləri, fərdi sahələr, workflow-lar, item-lər, şərhlər, hadisələr
- [x] Go servis skeleti: konfiqurasiya, strukturlu loglama, graceful shutdown, health/readiness, metriklər
- [x] Docker image, docker-compose, Makefile, CI
- [x] OIDC autentifikasiya (istənilən OIDC IdP; dev mühitində Keycloak), ADR-0002
- [x] Workspace-lər, üzvlər, rollar, layihələr
- [x] Items API: yaratmaq/oxumaq/yeniləmək/silmək/siyahı; keyset pagination, filtrlər, tam mətnli axtarış və optimistic locking
- [x] Hər dəyişikliyin eyni tranzaksiyada hadisə loguna yazılması

## Phase 0.5: Fərdiləşdirmə nüvəsi (tamamlandı)
- [x] Fərdi sahələr: workspace səviyyəsində təriflər, item tipinə bağlama (məcburilik bağlamada), tipli validasiya, sahəyə görə filtr (ADR-0003)
- [x] Workflow, status və item tipi idarəetmə API-si (ADR-0004)
- [x] Rate limiting: IP başına (autentifikasiyadan əvvəl) və istifadəçi başına (ADR-0004)

## Phase 1: Workflow və avtomatlaşdırma (tamamlandı, ADR-0005)
- [x] Status keçidlərinin məcburi tətbiqi və növbəti mümkün statuslar endpoint-i
- [x] Transactional outbox + PostgreSQL job növbəsi (SKIP LOCKED, retry/backoff, `nebula worker`)
- [x] Çıxan webhook-lar: HMAC imza, şifrələnmiş secret, SSRF qoruması, çatdırılma logu, avtomatik söndürmə
- [x] Avtomatlaşdırma qaydaları (trigger, şərt, əməliyyat), dövrə qoruması, hadisə anındakı snapshot
- [x] SMTP bildirişləri (məcburi TLS, header injection qoruması), dev mühitində Mailpit

## Veb interfeys v1 (tamamlandı, ADR-0006)
- [x] OIDC + PKCE ilə giriş (token yalnız yaddaşda), workspace və layihə siyahıları, yaratma formaları
- [x] Virtual cədvəl (sonsuz sürüşdürmə, axtarış, filtrlər), kanban lövhəsi (sürüklə-burax, workflow qaydalarına hörmət)
- [x] Tapşırıq paneli: başlıq, təsvir, status, icraçı, fərdi sahələr; optimistic locking konfliktlərinin idarəsi

## Phase 1.5: Əməkdaşlıq (tamamlandı, ADR-0007)
- [x] Şərhlər (tombstone silmə, yalnız müəllif redaktəsi) və @mention email bildirişləri
- [x] Tapşırıq aktivlik lenti və workspace audit log
- [x] Webhook "test ping", fasiləsiz şifrələmə açarı rotasiyası (`nebula rotate-secrets`)
- [x] LISTEN/NOTIFY: fon işləri ~23 ms-də emal olunur
- [x] UI: tapşırıq panelində "Şərhlər" və "Aktivlik" bölmələri, @ ilə avtomatik tamamlama

## Phase 2a: Admin ekranları (tamamlandı)
- [x] Workspace ayarları: üzvlər (rol dəyişmə, çıxarma, "son sahib" qoruması), sahələr və seçimlər, tapşırıq tipləri və sahə bağlamaları
- [x] Workflow redaktoru: statuslar, sıralama, başlanğıc status, keçidlər, keçid məcburiliyi
- [x] Webhook-lar (secret bir dəfə göstərilir, test, çatdırılma logu, secret rotasiyası), vizual avtomatlaşdırma qayda qurucusu, audit log

## Phase 2b: Növbəti
- [x] Playwright E2E testləri (real Keycloak girişi, PKCE, rollar, admin ekranları, @mention email-i); CI-də hər PR-da işləyir
- [x] Lövhədə status sütunları (fərqli workflow-ların eyni statusları birləşir), sütun-sütun sonsuz yükləmə, həqiqi sütun sayları, icazəsiz keçidlərin solğunlaşması (ADR-0008)
- [x] 429-un düzgün idarəsi: `Retry-After` ilə təkrar; yazmalarda yalnız 429 (dəyişiklik itkisi aradan qaldırıldı)
- [x] CSV və Excel (.xlsx) importu: önbaxış, sütun uyğunlaşdırması (başlıq adına görə təklif), dry run ilə sətir xətaları, hamısı-və-ya-heç-nə idxal, audit qeydi (ADR-0011)
- [x] Tətbiqdaxili bildiriş mərkəzi: @mention, təyinat, gecikmə və şərh bildirişləri, başlıqda zəng düyməsi (oxunmamış sayı, 30 s polling), gizli layihələrdə oxuma anında giriş yoxlaması, 90/180 günlük saxlama (ADR-0017, miqrasiya 00014)
- [x] Son tarix (`due_date`), workspace saat qurşağı, avtomatik "gecikir" nişanı və filtr; `item.overdue` qayda trigger-i, gündəlik gecikmə xülasəsi (`workspace.overdue_digest`), "Ümumi" ayarlar bölməsi (ADR-0009)
- [x] Mattermost/Slack bildiriş hədəfi: webhook `format: slack`, azərbaycanca qısa mesajlar, tapşırıq keçidi, URL şifrələnmiş və maskalanmış (ADR-0010)
- [x] Etibarlı proxy-lər üçün rate limit: `NEBULA_TRUSTED_PROXIES`, `X-Forwarded-For` yalnız etibarlı CIDR-lərdən, sağdan sola (ADR-0010)
- [x] Təkrarlanan tapşırıqlar: gündəlik/həftəlik/aylıq (ayın son günü daxil)/illik, yalnız növbəti tapşırıq `lead_days` əvvəl, buraxılmış tarixlərin bərpası, xətalı qaydanın söndürülməsi (ADR-0012)
- [x] Layihə səviyyəsində məxfilik: gizli layihələr (yalnız üzvlər + workspace adminləri), layihə üzvləri, icraçı və @mention yalnız layihəni görə bilənlərə (ADR-0013)
- [x] Saxlanılan görünüşlər (şəxsi/ümumi, filtrlər, sütunlar, sıralama, qruplaşdırma), fərdi sahə sütunları, bütün filtrlənmiş nəticə üzrə cəm sətri, son tarixə görə sıralama, kod bölünməsi (ADR-0014)
- [x] Production: tək VPS-də Docker Compose (Caddy + TLS/CSP, Keycloak production rejimi, PG 19 + pgBackRest → S3, PITR və aylıq avtomatik restore testi, Alloy + alertlər → Mattermost, digest ilə deploy/rollback skriptləri) (ADR-0015, `deploy/production/README.md`)
- [x] Webhook-lara layihə filtri: boş = açıq layihələr, gizli layihə yalnız açıq seçiləndə; çatdırılmada təkrar yoxlama; gündəlik xülasə hər webhook üçün süzülür (ADR-0013, migrasiya 00012)
- [x] Email dəvətləri: tək istifadəlik link (yalnız heş saxlanılır), IdP-nin təsdiqlədiyi email uyğun gəlməlidir, 7 gün, ləğv/yenidən göndər, limitlər (ADR-0016)
- [x] Production hardening: `*_FILE` sirləri, ayrı miqrasiya/runtime DB rolları (runtime yalnız DML), `nebula healthcheck`, access log-da `client_ip`, app-dan növbə metrikləri, release-də Trivy gate (ADR-0018)
- [x] Telefon və planşet: lövhədə toxunma ilə sürükləmə (basılı saxla, scroll ilə vuruşmur), sütunlarda snap, telefonda bir sətirlik alətlər paneli + "Filtrlər" vərəqi, sabit başlıqlı cədvəl, tam ekran tapşırıq paneli (klaviatura və safe-area), 44 px toxunma hədəfləri, heç bir səhifədə yana sürüşmə yoxdur; Playwright cihaz emulyasiyası ilə E2E (ADR-0019)
- [x] Mattermost Boards-dan keçid: azərbaycanca workspace şablonu (Görüləcək → İcrada → Yoxlamada → Tamamlandı, Asia/Baku), mövcud layihədən layihə şablonu (üzvlər, təkrarlanan tapşırıqlar, ümumi görünüşlər), `.boardarchive` importu (kart mətni + köhnə şərhlər təsvirdə, Mattermost istifadəçilərinin üzvlərə uyğunlaşdırılması), keçid runbook-u (ADR-0020, `docs/runbooks/mattermost-boards-migration.md`)
- [x] Təqvim görünüşü (bazar ertəsindən ay şəbəkəsi, workspace saat qurşağı, "+N daha", sürükləyərək son tarix, son tarixsiz siyahı, telefonda gündəm), `due_from`/`due_to` aralıq filtri; toplu redaktə (500-ə qədər, hamısı-və-ya-heç-nə, tapşırıq-tapşırıq xəta siyahısı, admin üçün toplu silmə) və cədvəldə sətirdaxili redaktə (ADR-0023)
- [x] Alt tapşırıqlar: bir səviyyə (DB trigger-i ilə qorunur, eyni layihə), paneldə siyahı + inline əlavə + irəliləyiş "2/5", əsas tapşırıq keçidi, cədvəl/lövhədə nişan, `?parent=`/`?top_level=true` filtrləri, əsas tapşırıq silinəndə alt tapşırıqlar ayrılır (ADR-0021, miqrasiya 00016)
- [x] Fayl əlavələri (lokal volume və ya S3, baytlardan tip təyini, həmişə `attachment` + sandbox CSP, yalnız raster şəkil önizləməsi, gizli layihə icazələri, kvota və rate limit, yetim blob təmizliyi, audit) və təsvir/şərhlərdə təhlükəsiz Markdown (GFM, xam HTML yox, təhlükəsiz linklər, önizləmə tabı) (ADR-0022, miqrasiya 00017)
- [x] Workspace üzrə axtarış: tapşırıqlar (açar, başlıq, təsvir) və şərhlər bir yerdə, Azərbaycan hərflərinin qatlanması ("odenis" ↔ "ödəniş"), prefiks və yazı xətası (pg_trgm) axtarışı, `QA-12` tam açarı birinci, gizli layihələr heç bir formada sızmır, Cmd/Ctrl+K dialoqu klaviatura ilə; 100k tapşırıq + 200k şərhdə seçici sorğular p95 < 5 ms, ən pis halda (sənədlərin 90%-i uyğun) ~0.25 s (ADR-0024, miqrasiya 00018)
- [x] İnterfeys dilləri: azərbaycanca, ingiliscə, türkcə, rusca; başlıqda dil seçici, brauzer dilinin təyini, yalnız seçilmiş dilin yüklənməsi, tsc ilə yoxlanılan lüğətlər, dilə görə tarix və cəm formaları, Keycloak giriş səhifəsi eyni dildə, türkcə və rusca workspace şablonları (ADR-0025)

## Phase 2: Şablonlar və görünüşlər
- Proqram təminatı / HR / Satış / Marketinq şablonları; cədvəl, kanban, təqvim, forma görünüşləri

## Phase 3: İnteqrasiyalar
- IMAP ilə email-dən item yaratmaq, xarici baza konnektorları (yalnız oxuma, sandbox-da), Git/CI, Jira/YouTrack/CSV importu

## Phase 4: Korporativ
- Ətraflı RBAC, row-level security, SSO/LDAP, hesabatlar, WASM plaginləri, multi-tenant SaaS, paylanmış (Redis) rate limiting
