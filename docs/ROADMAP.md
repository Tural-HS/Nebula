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

## Phase 2: Növbəti
- Admin ekranları (sahələr, workflow-lar, webhook-lar, avtomatlaşdırma), Playwright E2E testləri, bildiriş mərkəzi (in-app)

## Phase 2: Şablonlar və görünüşlər
- Proqram təminatı / HR / Satış / Marketinq şablonları; cədvəl, kanban, təqvim, forma görünüşləri

## Phase 3: İnteqrasiyalar
- IMAP ilə email-dən item yaratmaq, xarici baza konnektorları (yalnız oxuma, sandbox-da), Git/CI, Jira/YouTrack/CSV importu

## Phase 4: Korporativ
- Ətraflı RBAC, row-level security, SSO/LDAP, hesabatlar, WASM plaginləri, multi-tenant SaaS, paylanmış (Redis) rate limiting
