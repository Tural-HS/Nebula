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

## Phase 1: Workflow və avtomatlaşdırma
- Status keçidlərinin məcburi tətbiqi, avtomatlaşdırma qaydaları (trigger, şərt, əməliyyat), SMTP bildirişləri, çıxan webhook-lar

## Phase 2: Şablonlar və görünüşlər
- Proqram təminatı / HR / Satış / Marketinq şablonları; cədvəl, kanban, təqvim, forma görünüşləri

## Phase 3: İnteqrasiyalar
- IMAP ilə email-dən item yaratmaq, xarici baza konnektorları (yalnız oxuma, sandbox-da), Git/CI, Jira/YouTrack/CSV importu

## Phase 4: Korporativ
- Ətraflı RBAC, row-level security, SSO/LDAP, hesabatlar, WASM plaginləri, multi-tenant SaaS, paylanmış (Redis) rate limiting
