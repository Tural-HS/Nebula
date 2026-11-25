# ADR-0043: Workspace API tokenləri (inteqrasiyalar üçün servis tokenləri)

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0002](0002-auth-and-api-conventions.md) (OIDC bearer JWT), [ADR-0004](0004-configuration-api-and-rate-limiting.md) (rate limit, təhlükəsizlik
  bazası), [ADR-0018](0018-production-hardening.md) (sirlər), [ADR-0013](0013-project-privacy.md) (gizli layihələr),
  [ADR-0039](0039-archive-and-delete.md) (arxiv), [ADR-0019](0019-mobile-web-ui.md) (telefon), [ADR-0025](0025-ui-languages.md) (4 dil)

## Kontekst

Pilot müştəri (fintech, bek-ofis) Nebula-nı 1C, bank çıxarışı skriptləri, ERP və BI alətləri ilə bağlamaq istəyir: skript hər səhər
çıxarışdan tapşırıq yaradır, BI aləti hesabatları oxuyur, ERP ödəniş statusunu şərh kimi yazır. Bu gün API yalnız Keycloak-ın
insan istifadəçiyə verdiyi OIDC JWT-ni qəbul edir (ADR-0002). Skriptə insanın parolu və ya refresh token-i verilsə: hər şeyə
çatır (bütün workspace-lər, admin ekranları), işdən çıxan əməkdaşla birlikdə sınır və tarixçədə əməliyyatlar insanın adına yazılır.
Lazım olan: **bir workspace-ə bağlı, məhdud, müddətli, ləğv edilə bilən, öz adı ilə görünən** servis tokeni.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Token = ayrıca prinsipal**: hər tokenin öz `users` sətri var (`oidc_issuer = 'nebula:service-token'`, `oidc_subject` = token id, email `<id>@service-token.invalid`, ad "🔑 <ad>"). **Workspace üzvü deyil**: rolu, icazələri və layihə siyahısı `service_tokens`-dədir və yalnız tokenin öz workspace-inə aiddir | Hadisələr, şərhlər, tarixçə, `reporter` mövcud `actor_id → users` mexanizmi ilə tokeni müəllif kimi göstərir — heç bir sorğu dəyişmir. Üzv olmadığı üçün token təyinatçı, izləyici, mention, bildiriş və ya email alıcısı ola bilmir, üzvlər siyahısında görünmür, `AddMember` onu email ilə əlavə etmir. Konfiqurasiya edilmiş OIDC issuer heç vaxt `nebula:service-token` olmur, ona görə JWT bu sətrə düşə bilməz | İnsanın kimliyini istifadə etmək: audit yalan olardı. `workspace_members`-də sətir: token üzvlər ekranında, təyinatçı siyahısında görünərdi və admin rolunu "owner" edə bilərdi. Ayrı `principals` cədvəli: `events.actor_id`, `comments.author_id` və s. üçün hər yerdə iki FK |
| **Format** `nbl_<id 32 hex>_<secret 64 hex>`: id = token UUID, secret = 32 təsadüfi bayt (`crypto/rand`). Parse yalnız dəqiq formatı (kiçik hex, sabit uzunluq) qəbul edir | Sabit prefiks + regex (`nbl_[0-9a-f]{32}_[0-9a-f]{64}`) secret scanner-lər (GitHub, gitleaks) üçün asandır; id ilə PK axtarışı — həşə görə axtarış yoxdur, vaxt oracle-ı yoxdur | JWT: ləğv üçün yenə DB lazımdır, uzun və açıq payload; base64url secret: `_` ayırıcı ilə qarışır |
| **Yalnız SHA-256(secret) saxlanılır**, müqayisə `crypto/subtle.ConstantTimeCompare`; naməlum id-də də dummy həş müqayisəsi | Secret 256 bit entropiyadır: həş sızsa belə brute-force mümkün deyil, ona görə yavaş KDF (bcrypt/argon2) heç nə qorumur, əksinə hər səhv tokenlə serverə CPU yükü verdirərdi (DoS) | bcrypt/argon2: aşağı entropiyalı parollar üçündür; açıq mətn: DB dump-ı = bütün inteqrasiyalara giriş |
| **Secret bir dəfə göstərilir** (yaradılma cavabı); siyahıda, hadisələrdə, loglarda heç vaxt. Hadisə payload-unda ad, rol, icazələr, layihələr, müddət | Itirilən token yenisi ilə əvəz olunur — bərpa yolu sızma yolu olardı | — |
| **Marşrut cədvəli ilə default-deny** (`tokenRoutes`): token yalnız siyahıdakı metod+yol cütlərinə çatır, hər biri bir icazə tələb edir — `GET /me`, workspace, sahələr/tiplər/workflow-lar, layihələr, tapşırıqlar (siyahı, say, oxuma, keçidlər, şərhlər, tarixçə) → `items:read`; `POST items`, `PATCH items/{n}` → `items:write`; `POST …/comments` → `comments:write`; `GET …/items/export` → `export:read`; `GET /reports[/workload.csv]` → `reports:read`. Qalan hər şey `403 token_route_forbidden` | Yeni marşrut əlavə olunanda tokenlər üçün avtomatik bağlıdır; üzvlər, dəvətlər, ayarlar, SMTP, webhook-lar, avtomatlaşdırma, arxiv/silmə, layihə yaratma, tapşırıq silmə və **token idarəsi** (token özünə token yarada bilməz) rola baxmadan bağlıdır | Hər handler-də `requireScope`: unudulan bir yer = sızma; yalnız rol: "admin" token admin ekranlarına çatardı |
| **Kiçik icazə dəsti**: `items:read`, `items:write`, `comments:write`, `reports:read`, `export:read`; qonaq rolunda yazma icazəsi qadağandır (422). İcazə rolu genişləndirmir: marşrut həm icazə, həm mövcud `requireRole` yoxlamasından keçir | İnteqrasiyaların real ehtiyacı; az icazə = admin üçün aydın seçim | İncə icazələr (sahə, status üzrə): indi ehtiyac yoxdur |
| **Rol**: `guest`, `member`, `admin` (DB `CHECK`, sahib heç vaxt); yaradan öz rolundan yüksək rol verə bilməz. Workspace-də görünən rol tokenin roludur | Sahib əməliyyatları (arxiv, silmə, sahibliyin verilməsi) heç vaxt inteqrasiyaya verilmir | — |
| **Layihə siyahısı** (`service_token_projects`, ≤ 50): boşdursa token rolun gördüyü layihələrə çatır (üzv tokeni gizli layihələri görmür); doludursa **yalnız** siyahıdakılara, **gizli layihələr də daxil** — siyahını admin seçir və admin hər layihəni görür (ADR-0013). Siyahıdan kənar layihə 404 (mövcudluq sızmır), layihə siyahısı filtrlənir; workspace üzrə aqreqat (hesabatlar) siyahılı tokenə bağlıdır (`403 token_project_restricted`) | "1C yalnız FIN layihəsinə yazır" — ən çox istənən hal; hesabatlar bütün layihələri cəmlədiyi üçün siyahını keçərdi | Gizli layihə üçün `project_members` sətri: FK `workspace_members` tələb edir (token üzv deyil); siyahının yalnız daraltması: gizli maliyyə layihəsi üçün admin rolu vermək lazım olardı |
| **Workspace sərhədi iki qatda**: siyasət yolun `{ws}` hissəsini tokenin slug-ı ilə müqayisə edir, `loadWorkspace` isə tokenlər üçün `WorkspaceForServiceToken` (slug **və** id) istifadə edir; başqa workspace həmişə 404 | Üzvlük cədvəlindən asılı deyil: token heç bir halda ikinci workspace-ə çatmır | — |
| **Müddət məcburidir**: 1–365 gün (UI: 30/90/365), DB `CHECK (expires_at <= created_at + 366 days)` | Unudulmuş "əbədi" tokenlər əsas sızma mənbəyidir; ildə bir rotasiya | Müddətsiz token: audit tövsiyələrinə ziddir |
| **Hər sorğuda DB yoxlaması** (bir PK sorğusu, workspace ilə join): ləğv, müddət və arxiv dərhal qüvvədədir. Səhv secret / naməlum id → `401 invalid_token` (heç nə demir); **yalnız düzgün secret ilə**: `401 token_expired`, `token_revoked`, `token_workspace_archived` | Ləğv "keşin TTL-i qədər" deyil, dərhal; aydın kod inteqrasiya sahibinə nə edəcəyini deyir, amma təxmin edənə məlumat vermir | Keş: ləğv gecikərdi; JWT: ləğv siyahısı lazımdır |
| **Arxivlənmiş workspace-in tokenləri işləmir** (oxuma da) | Arxiv "dondurulmuş" deməkdir (ADR-0039); unudulmuş inteqrasiya arxivdən məlumat çəkməsin | Yalnız oxuma: insanlar arxivi oxuya bilir, amma inteqrasiyanın buna ehtiyacı yoxdur |
| **Son istifadə**: `last_used_at`, `last_used_ip` (rate limiter-in istifadə etdiyi IP, ADR-0010) **dəqiqədə ən çox bir yazı** (şərtli `UPDATE`) | Admin ölü tokenləri tapır, gözlənilməz IP görür; hər sorğuda yazı yoxdur | Hər sorğuda yazı: yüklü inteqrasiyada lazımsız WAL |
| **Rate limit token başına** (`NEBULA_RATE_LIMIT_TOKEN_RPS/_BURST`, standart 5/20), prinsipalın istifadəçi limiti və IP limiti də qüvvədədir | Səhv dövrəyə düşən skript bütün workspace-i yavaşlatmasın; limit token sayına görə ayrılır | Yalnız istifadəçi limiti (20/s): skript üçün çox genişdir |
| **Audit**: `service_token.created`, `service_token.revoked` (admin actor), `service_token.denied` (rədd edilmiş marşrut, icazə, layihə, başqa workspace) və `service_token.rejected` (müddəti bitmiş/ləğv edilmiş/arxivlənmiş, yalnız düzgün secret ilə) — actor tokenin prinsipalıdır, payload `reason`, metod, yol; **secret heç vaxt**. Rədd hadisələri token+səbəb üzrə dəqiqədə bir dəfə (yaddaşda) | Admin audit log-da "1C tokeni dünən 40 dəfə icazəsiz marşruta getdi" görür; throttle olmadan səhv skript audit log-u və webhook-ları doldurardı. Naməlum tokenlər üçün hadisə yoxdur (workspace bilinmir; IP limiti qoruyur) | Hər rədd üçün hadisə: log/webhook daşqını |
| **İdarə**: `GET, POST /workspaces/{ws}/service-tokens`, `DELETE …/{id}` — yalnız admin+ insanlar; workspace başına ən çox 50 aktiv token; ləğv geri qaytarılmır, ləğv edilmiş tokenlər siyahıda qalır (kimin girişi var idi) | Sadə və audit oluna bilən; "yenidən aktiv et" sızmış tokeni geri qaytarardı | Redaktə (icazə dəyişmək): yeni token yaratmaq daha aydındır |
| **UI**: Ayarlar → "API tokenləri" — kartlar (ad, vəziyyət, rol, icazələr, layihələr, yaradan, bitmə, son istifadə + IP), yaratma dialoqu (ad, rol, icazələr, layihələr, 30/90/365), secret bir dəfə: xəbərdarlıq, kopyala düyməsi, `curl` nümunəsi (`$NEBULA_TOKEN` yer tutucusu ilə); ləğv `Dialog`-da adla təsdiqlənir. 4 dil, 320 px | Kart düzümü telefonda cədvəldən yaxşı oxunur | — |

## Təhlükə modeli

| Təhlükə | Nəzarət |
|---|---|
| Token koda/loga/çata sızır | Prefiks secret scanner-lərə tanış; müddət ≤ 1 il; dərhal ləğv; son istifadə IP-si; icazə və layihə ilə məhdud zərər. Access log yalnız route şablonunu yazır, başlıqları və sorğu sətrini heç vaxt |
| DB dump-ı sızır | Yalnız SHA-256 həşi; 256 bit secret həşdən bərpa olunmur |
| Token brute-force / təxmin | 2²⁵⁶ secret; IP limiti autentifikasiyadan əvvəl; naməlum id və səhv secret eyni cavab (`invalid_token`) və təxminən eyni iş (dummy həş) |
| Vaxt hücumu | PK ilə axtarış + `ConstantTimeCompare` |
| Token öz səlahiyyətini genişləndirir | Token idarəsi, üzvlər, dəvətlər, rol dəyişmə marşrutları cədvəldə yoxdur; rol `CHECK` ilə sahib ola bilməz; token üzv olmadığı üçün `UpdateMemberRole` ona təsir etmir |
| Başqa workspace-ə çıxış | Yol slug-ı müqayisəsi + `WorkspaceForServiceToken` (id ilə); 404 |
| Gizli layihə sızması | Siyahısız: rolun görünürlüyü; siyahılı: yalnız siyahı, hesabatlar bağlı; 404 mövcudluğu gizlədir |
| Ləğv edilmiş / müddəti bitmiş / arxiv tokeni | Hər sorğuda DB yoxlaması, keş yoxdur |
| Səhv skript serveri yükləyir | Token başına rate limit; rədd hadisələri throttle olunur; `last_used` dəqiqədə bir yazı |
| `nbl_` dəyəri OIDC verifier-ə gedir (log, IdP sorğusu) | Prefiksli dəyər heç vaxt OIDC middleware-ə ötürülmür |

Qalıq risklər: limitlər və rədd throttle-u replika başınadır (ADR-0004 kimi); prosesin yaddaşı oxunarsa (core dump) sorğudakı token
görünə bilər; token sahibi skriptin işlədiyi maşın qədər təhlükəsizdir.

## Nəticələr

- Miqrasiya **00033**: `service_tokens`, `service_token_projects`. Kilid qeydləri faylın başlığında (yeni boş cədvəllər); Down
  cədvəlləri silir və token prinsipallarını deaktiv edir; deploy README §9-da sətir.
- Kod: `internal/apitoken` (generate/parse/hash/verify), `internal/store/servicetokens.go`, `internal/server/servicetokens_api.go`
  (`authenticate`, `tokenPolicy`, `tokenRoutes`, idarə marşrutları), `api.go`-da `loadWorkspace`/`projectLoader`/`listProjects`
  tokenlər üçün; konfiqurasiya `NEBULA_RATE_LIMIT_TOKEN_RPS/_BURST`. Web: `lib/serviceTokens.ts`, `api/serviceTokens.ts`,
  `pages/settings/TokensPage.tsx`, lüğət `tokens` (4 dil).
- Testlər: `internal/apitoken` unit (format, parse rədləri, verify), `servicetokens_api_test.go` (marşrut cədvəli, default-deny),
  `servicetokens_integration_test.go` (idarə icazələri, validasiya, secret sızmır, aktor atributu, son istifadə, ləğv, hər marşrut
  qrupu üzrə icazələr, admin tokeninin bağlı marşrutları, layihə siyahısı və gizli layihələr, workspace-lərarası 404, müddət, arxiv,
  rate limit, rədd hadisələri və throttle), vitest `lib/serviceTokens.test.ts`, Playwright `web/e2e/api-tokens.spec.ts`.
- README-də "API tokenləri (inteqrasiyalar)" bölməsi və `curl` nümunəsi.

## Açıq suallar

- Workspace silinəndə token prinsipalları `users`-də (aktiv olmayan tokenlərlə) qalır; purge onları da deaktiv/silə bilər.
- Tokenin icazələrini redaktə etmək və "rotasiya" (köhnəni N gün saxlayıb yenisini vermək).
- Müddət bitməzdən əvvəl adminlərə xatırlatma (email/bildiriş).
- IP allow-list (yalnız 1C serverinin ünvanından).
- Paylanmış (Redis) rate limit və rədd throttle-u (ADR-0004 açıq sualı ilə birlikdə).
- Hadisə `source` sahəsində `api_token` dəyəri (indi actor-dan bilinir).
