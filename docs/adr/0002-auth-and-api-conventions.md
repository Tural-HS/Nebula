# ADR-0002: Autentifikasiya, avtorizasiya və API qaydaları

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Autentifikasiya

- API yalnız **OIDC JWT access token** qəbul edir (`Authorization: Bearer`). Nebula şifrə saxlamır.
- Yoxlanılanlar:
  - imza (yalnız asimmetrik alqoritmlər; `HS*` və `none` rədd edilir);
  - `iss` tam uyğunluğu;
  - `aud` daxilində `NEBULA_OIDC_AUDIENCE`;
  - `exp`;
  - `sub` və `email` olması.

  Keycloak ID token-ləri (`typ: ID`) rədd edilir.
- Açarlar discovery ilə və ya ayrıca `NEBULA_OIDC_JWKS_URL` (daxili ünvan) ilə alınır. Productionda hər ikisi üçün https məcburidir.
- IdP ilə əlaqə tənbəl (lazy) qurulur: IdP işləməsə belə Nebula başlayır və health check-lərə cavab verir. Bu müddətdə auth tələb edən sorğular 503 alır.
- İstifadəçilər `(issuer, subject)` cütü ilə müəyyən edilir. İlk girişdə yaradılırlar, email və ad token-dən sinxronlaşdırılır.
  Başqa identikliyə bağlı email rədd edilir: bu, ikinci IdP hesabı ilə hesabın ələ keçirilməsinin qarşısını alır.
- **Principal cache yoxdur**: istifadəçi sətri hər sorğuda oxunur, ona görə də istifadəçinin deaktiv edilməsi dərhal qüvvəyə minir. Qiyməti bir indeksli sorğudur.

### Runbook: IdP dəyişdikdə identikliyin yenidən bağlanması

İstifadəçinin `sub` dəyəri dəyişərsə, bütün sorğuları `403 identity_conflict` alır. Bu, məsələn, IdP-nin
yenidən qurulması, realm-in təkrar import edilməsi və ya başqa IdP-yə köçmə zamanı baş verir.
Bu, təhlükəsizlik qaydasının düzgün işləməsidir, xəta deyil.

1. Dəyişikliyin həqiqətən eyni şəxsə aid olduğunu IdP-də təsdiqləyin (email-ə etibar etməyin).
2. Yeni `sub` dəyərini IdP-dən götürün (məs. Keycloak: Users → istifadəçi → ID).
3. Bir tranzaksiyada köhnə sətri yeniləyin. Item-lər, üzvlüklər və hadisələr eyni `users.id`-yə bağlı qalır:

   ```sql
   BEGIN;
   UPDATE users SET oidc_issuer = '<yeni issuer>', oidc_subject = '<yeni sub>'
    WHERE email = '<email>' AND oidc_subject = '<köhnə sub>';  -- dəqiq 1 sətir olmalıdır
   COMMIT;
   ```

Toplu köçmədə eyni əməliyyat `(köhnə sub → yeni sub)` xəritəsi ilə aparılır. Dev mühitində
istifadəçilərin ID-si realm faylında sabitlənib, Keycloak məlumatları isə `kcdata` volume-unda saxlanılır.
Ona görə də konteyner yenidən yaradıldıqda `sub` dəyişmir. Gələcəkdə bu əməliyyat üçün audit olunan admin API əlavə ediləcək.

## Avtorizasiya

| Rol | Oxumaq | Item yaratmaq/dəyişmək/silmək | Layihə, üzv, sahə, workflow, item tipi idarəsi | Owner təyin etmək |
|---|---|---|---|---|
| guest | bəli | xeyr | xeyr | xeyr |
| member | bəli | bəli | xeyr | xeyr |
| admin | bəli | bəli | bəli | xeyr |
| owner | bəli | bəli | bəli | bəli |

- Üzv olmayan istifadəçi hər workspace URL-i üçün **404** alır, heç vaxt 403 almır. Beləliklə workspace-in mövcudluğu belə sızmır.
- Tenant izolyasiyası iki dəfə təmin edilir: API-də üzvlük yoxlaması və bazada composite foreign key-lər.

## API qaydaları

- Baza yol `/api/v1`-dir. URL-lər insan üçün oxunaqlıdır: `/workspaces/{slug}/projects/{KEY}/items/{number}`.
- Yalnız JSON qəbul edilir. Naməlum sahələr, əlavə məlumat və 1 MiB-dan böyük gövdələr rədd edilir (400/413).
- Bir xəta formatı var: `{"error": {"code", "message", "field?"}}`. Kodlar: 422 validasiya, 409 konflikt, 404 tapılmadı/gizli, 429 limit aşıldı.
- `PATCH` üç halı ayırır: sahə göndərilməyib / `null` / dəyər. Hər `PATCH` oxuduğu `version`-u göndərməlidir.
  Köhnə versiya 409 `version_conflict` alır, beləliklə heç bir dəyişiklik itmir.
- Siyahılar şəffaf olmayan `next_cursor` ilə **keyset pagination** istifadə edir. İstənilən dərinlikdə sabit və sürətlidir; OFFSET istifadə edilmir.
- Səhv formatlı query string (məs. `%%%`) səssizcə nəzərə alınmamaq əvəzinə 400 qaytarır.
- Hər dəyişiklik öz audit hadisəsini eyni tranzaksiyada yazır (`events` yalnız əlavə edilə bilən cədvəldir).
