# ADR-0058: Əlavə tünd temalar

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0049](0049-colour-themes.md) (rəng palitraları)

## Kontekst

Komanda daha çox tünd tema istədi. Tünd rejimdə yalnız "Tünd" və "Gecə göyü" var idi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Dörd yeni tünd palitra: **Tünd yaşıl** (`evergreen`), **Qara (OLED)** (`oled`), **Bənövşəyi gecə** (`dusk`), **Qəhvə** (`mocha`) | Fərqli zövqlər: FinTez yaşılı, tam qara ekran, soyuq və isti çalarlar | İstifadəçinin öz rəngini seçməsi: kontrastı yoxlamaq olmur |
| Eyni üsul: neytral şkala (`zinc`) əvəz olunur, açıqlıq pillələri "Gecə göyü" ilə eynidir, yalnız çalar dəyişir | Kontrast artıq yoxlanılıb (400 / 950 təxminən 8:1); bütün komponentlər dəyişmədən işləyir | Hər tema üçün ayrıca komponent rəngləri: çox iş, səhv riski |
| Tünd yaşıl brend şkalasını da dəyişir; 600/700 "Yaşıl" palitradakı kimi (ağ mətn 5.21:1) | Düymələr FinTez yaşılı olur, AA saxlanır | Daha açıq yaşıl: ağ mətn 4.5:1-dən aşağı düşür |
| OLED: səhifə tam qara, səthlər 0.16 açıqlıqda | OLED ekranlarda enerji və göz üçün | Səthləri də qara etmək: kartlar fondan ayrılmır |
| Seçim əvvəlki kimi brauzerdə saxlanır, `theme-init.js` ilk çəkilişdən əvvəl tətbiq edir | Ağ parıltı olmur | — |

## Nəticələr

- Miqrasiya və API dəyişikliyi yoxdur; yalnız veb (`index.css`, `lib/theme.ts`, `public/theme-init.js`).
- İş sahəsinin vurğu rəngi (ADR-0035) bu temalarda da üstündür.
