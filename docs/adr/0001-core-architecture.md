# ADR-0001: Əsas arxitektura

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

Nebula ümumi təyinatlı iş idarəetmə platformasıdır (proqram təminatı, HR, satış, marketinq). Tələblər:

- Böyük həcmli məlumatlarda sürət (hədəf: hər ekran < 100 ms, 1 milyon item-də axtarış < 300 ms).
- Tam fərdiləşdirmə: fərdi sahələr, item tipləri, workflow-lar, görünüşlər, avtomatlaşdırma.
- İnteqrasiyalar: SMTP/IMAP, xarici bazalar, webhook-lar, Git/CI.
- Öz serverində qurula bilmə və kiçik komanda tərəfindən idarə oluna bilmə; kövrək daxili (embedded) saxlama yoxdur.

Mövcud alətlərdən çıxarılan dərslər: YouTrack-in daxili bazası və ayrıca mətn indeksi qəfil
söndürülmələrdə korlanır. Jira-nın konfiqurasiyası yalnız UI-da yaşayır, versiyalana bilmir
və ayrıca admin tələb edir.

## Qərarlar

| Sahə | Qərar | Səbəb |
|---|---|---|
| Backend dili | Go | Tək statik binar, az yaddaş, güclü paralellik, mütəxəssis tapmaq asandır |
| HTTP | `net/http` + `chi` | Standart kitabxana ilə uyğun, framework asılılığı yoxdur |
| Baza | PostgreSQL 19, yeganə həqiqət mənbəyi | JSONB, FTS, partitioning, RLS, yetkin backup/HA |
| Bazaya giriş | `pgx/v5`, əl ilə yazılmış SQL | ORM yoxdur: sorğular və performans proqnozlaşdırıla biləndir |
| Miqrasiyalar | `goose`, SQL faylları binarın içinə daxil edilib | Versiyalanır, review olunur, CI/CD-də `nebula migrate up` |
| Axtarış | Postgres FTS (`simple` konfiqurasiya, çoxdilli: AZ/TR/RU/EN) | Korlana biləcək ikinci baza yoxdur; ehtiyac ölçüldükdə OpenSearch/Meilisearch |
| ID-lər | Tətbiqin yaratdığı UUIDv7 + layihə daxilində insan oxuya bilən nömrə (`NEB-123`) | Paylanmış yaratma üçün təhlükəsiz, B-tree üçün səmərəli; insan üçün rahat açarlar |
| Multi-tenancy | Hər tenant cədvəlində `workspace_id` + composite foreign key-lər | Workspace-lər arası istinadı baza səviyyəsində qeyri-mümkün edir; RLS sonrakı ADR-də |
| Fərdi sahələr | `field_definitions` + `items.fields JSONB` (GIN indeks) | EAV join partlayışı olmadan çeviklik (ətraflı: ADR-0003) |
| Workflow | Məlumatla idarə olunan state machine (`workflows`, `statuses`, `workflow_transitions`) | Bütün şöbələr üçün bir mühərrik |
| Audit/hadisələr | Yalnız əlavə edilə bilən `events` cədvəli | Audit log, aktivlik lenti, hesabatlar və webhook-lar bir axından |
| Paralel redaktə | `items.version` ilə optimistic locking | Paralel düzəlişlərdə dəyişiklik itmir |
| Autentifikasiya | Xarici OIDC (Keycloak/Authentik); Nebula şifrə saxlamır | Auth-u özümüz yazmırıq; SSO/LDAP hazır gəlir |
| Müşahidə | `log/slog` JSON loglar, ayrıca admin portunda Prometheus metrikləri, `/healthz` + `/readyz` | Metriklər heç vaxt ictimaiyyətə açılmır |
| Paketləmə | Distroless, root olmayan image, read-only root FS, digest ilə sabitlənmiş base image | Minimal hücum səthi |
| Frontend | React + TypeScript + TanStack Table/Virtual (ayrıca ADR) | Böyük cədvəllər üçün virtual render |

## Prinsip: bir mühərrik, çoxlu şablon

Şöbələr ayrıca modullar deyil. Bir mühərrik var: item-lər, fərdi sahələr, workflow, görünüşlər
və avtomatlaşdırma. "Proqram təminatı", "HR", "Satış" isə bu mühərrikin üzərində quraşdırılan
şablonlardır (item tipləri, sahələr, workflow-lar). Yüzlərlə funksiyanı idarə oluna bilən saxlamağın
yeganə yolu budur.

## Nəticələr

- Müsbət: backup və idarəetmə üçün bir baza; sxem Git-də versiyalanır; tenant izolyasiyasını baza təmin edir.
- Mənfi: JSONB sahələrinə ağır filtrlər üçün aydın indeks strategiyası lazımdır; `events` sürətlə böyüyəcək
  və ~50M sətirdən sonra partisiyalanmalı və ya ClickHouse-a köçürülməlidir (gələcək ADR).
