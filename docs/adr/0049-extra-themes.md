# ADR-0049: Əlavə rəng temaları — Kağız, Gecə göyü, Yüksək kontrast

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0035](0035-design-and-theme.md) (tema seçimi, aksent), [ADR-0047](0047-ledger-records.md) (logoya çalan neytral şkala)

## Kontekst

Komanda gün boyu cədvəllə işləyir; işıqlı rejim bəzilərinə yorucu gəlir, otaq və ekran şəraiti fərqlidir. Açıq və Tünd
kifayət etmir: isti, az parlayan fon, brendə uyğun tünd variant və zəif görmə/günəşli otaq üçün maksimum kontrast lazımdır.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Cəmi 6 seçim: **Sistem, Açıq, Tünd, Kağız, Gecə göyü, Yüksək kontrast**. Seçim brauzerdə (`nebula.theme`), əvvəlki kimi dərhal tətbiq olunur; `theme-init.js` ilk boyamadan əvvəl tətbiq edir (ağ parıltı yoxdur) | Az, fərqli məqsədli seçimlər; mövcud mexanizm (ADR-0035) dəyişmir | İstifadəçi rəng redaktoru: dəstək və kontrast yoxlaması çətinləşir |
| Palitra = **neytral şkalanın** (`--color-zinc-*`) əvəzlənməsi `:root[data-theme=…]` ilə; Kağızda `--color-white` da krem olur. Gecə göyü tünd bazadadır (`.dark` sinfi saxlanılır, bütün `dark:` stillər işləyir), Kağız və Yüksək kontrast işıqlı bazada | Bütün komponentlər `zinc-*` və `dark:` istifadə edir — heç bir komponent dəyişmir; aksent (brand) ayrıca qalır | Hər tema üçün ayrıca utility sinifləri: yüzlərlə faylda dəyişiklik |
| Kontrast WCAG AA ilə yoxlanılıb (oklch → sRGB parlaqlığı): Kağız boz mətn fonda 5.54:1; Gecə göyü 400 fonda 7.99:1; Yüksək kontrast boz mətn 8.57:1, xətlər 3.67:1 | Yeni temalar oxunaqlılığı pisləşdirməməlidir | — |

## Nəticələr

- Miqrasiya və server dəyişikliyi yoxdur. Köhnə versiyaya qayıdanda saxlanmış yeni dəyər "Sistem" kimi oxunur.
- Testlər: vitest (`data-theme`, `dark` sinfi, parse), E2E (seçim, reload-dan sonra qalması, Gecə göyünün tünd bazası).

## Əlavə (2026-11-26): Yaşıl (FinTez)

Dördüncü palitra `green`: açıq əsas, neytral şkala yaşıla çalır (hue 165–184), **brend şkalası da yaşıldır**
(FinTez rəngləri #11b67a və #00524d əsasında). Kontrast: brend 600 üzərində ağ mətn 5.21:1, 700 — 7.44:1; 500
fokus halqası ağ fonda 3.42:1; boz 500 səhifədə 5.12:1. İş yerinin öz aksent rəngi (ADR-0035) elementin üzərində
təyin olunduğu üçün yenə üstündür.

## Açıq suallar

- Workspace səviyyəsində standart tema (admin seçir, istifadəçi dəyişə bilər).
- Yüksək kontrastın tünd variantı.
