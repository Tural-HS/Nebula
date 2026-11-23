# ADR-0006: Veb interfeys

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

Nebula-nın backend-i hazırdır, lakin istifadəçilərin real olaraq sınaya biləcəyi məhsul üçün veb interfeys lazımdır.
UI eyni zamanda API-dəki çatışmazlıqları erkən aşkar etməyə kömək edir.

## Qərarlar

| Sahə | Qərar | Səbəb |
|---|---|---|
| Framework | React 19 + TypeScript (strict) + Vite | Ekosistem, tip təhlükəsizliyi, sürətli inkişaf |
| TypeScript versiyası | 6.0.x (TS 7 deyil) | `typescript-eslint` hələ TS 7-ni dəstəkləmir (`<6.1`); dəstək gələndə keçid ediləcək |
| Node.js | 26 LTS (`web/.nvmrc`) | Node 25 LTS deyil və `vitest`/`jsdom` onu dəstəkləmir |
| Məlumat | TanStack Query | Cache, təkrar cəhd, invalidation |
| Böyük cədvəl | `@tanstack/react-virtual` (TanStack Table olmadan) | Yalnız ekrandakı sətirlər DOM-dadır; bir asılılıq azdır |
| Kanban | `@dnd-kit/core` | Klaviatura ilə də əlçatandır |
| Stil | Tailwind CSS 4 | Sürətli, ardıcıl dizayn; işıqlı/tünd rejim |
| Routing | React Router 8 | Standart |
| UI dili | Azərbaycan dili (`src/i18n.ts`) | Digər dillər eyni strukturda əlavə ediləcək |

## Təhlükəsizlik

| Təhdid | Tədbir |
|---|---|
| Şifrənin UI-dan keçməsi | OIDC Authorization Code + **PKCE (S256, məcburi)**; şifrə yalnız Keycloak səhifəsinə daxil edilir |
| XSS ilə token oğurluğu | Token-lər **yalnız yaddaşda** saxlanılır (`InMemoryWebStorage`), localStorage/sessionStorage-da deyil. Səhifə yeniləndikdə IdP-nin SSO sessiyası ilə səssiz giriş olur |
| XSS | React mətni avtomatik escape edir; `dangerouslySetInnerHTML` ESLint qaydası ilə **qadağandır**. Təsvirlər düz mətn kimi göstərilir |
| CSRF | API yalnız `Authorization: Bearer` qəbul edir, cookie göndərilmir (`credentials: "omit"`) |
| CORS | Lazım deyil: UI və API eyni origin-dədir (dev-də Vite proxy, production-da ingress) |
| Açıq yönləndirmə (open redirect) | Girişdən sonra yalnız eyni origin-in nisbi yollarına qayıdılır (`safeReturnPath`) |
| URL injection | Bütün API yol seqmentləri `encodeURIComponent` ilə kodlanır (`path` template funksiyası) |
| Yanlış redirect URI | Keycloak client yalnız `http://localhost:5173/auth/callback`-i qəbul edir (production-da öz domeniniz) |
| Supply chain | Dəqiq versiyalar (`save-exact`), `package-lock.json`, CI-da `npm ci --ignore-scripts` və `npm audit --audit-level=high`, Dependabot |

## Konfiqurasiya

`/config.json` iş vaxtı yüklənir (build zamanı deyil): bir build bütün mühitlərdə işləyir.

```json
{ "oidcAuthority": "https://sso.example.com/realms/nebula", "oidcClientId": "nebula-web" }
```

Production-da bu fayl hər mühit üçün mount edilir (ConfigMap).

## Production yerləşdirmə

- `npm run build` → `web/dist/` statik fayllar. İstənilən statik serverdən (nginx, ingress) verilə bilər.
- **Eyni origin** məcburidir: `/api/*` → Nebula API, qalan hər şey → `index.html` (SPA fallback).
- Server başlıqları: `Content-Security-Policy: default-src 'self'; connect-src 'self' https://<sso-host>; frame-ancestors 'none'; base-uri 'self'; form-action 'self' https://<sso-host>`,
  `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`.
- `assets/` fayllarının adında hash var: uzun müddət cache etmək olar. `index.html` və `config.json` cache edilməməlidir.

## Bilinən məhdudiyyətlər

- Lövhə (kanban) ilk 500 tapşırığı göstərir; daha çoxu üçün filtrlərdən istifadə edilməlidir.
- Komponent testləri hələ yoxdur: məntiq vitest ilə, UI isə real brauzerdə yoxlanılır. Playwright E2E testləri növbəti addımdır.
- Admin ekranları (`/w/{ws}/settings/...`) yalnız admin+ üçün açıqdır; üzv siyahısı hamıya görünür. UI yalnız rahatlıq üçün gizlədir, icazəni həmişə API yoxlayır.
