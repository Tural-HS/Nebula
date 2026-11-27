# ADR-0065: Layihə üzrə sayğaclar və status kateqoriyası filtri

- Status: Qəbul edilib
- Tarix: 2026-11-27
- Əsas: [ADR-0035](0035-design-and-theme.md) (sürətli filtrlər)

## Kontekst

FinTez-in aylıq hesabatında "Mənə təyin", "Təsdiq gözləyir", "Bloklanmış" sayğacları həmişə 0-dır; əvəzində
"Tamamlanmış" və "Bu ay bitir" (yalnız hələ görülməmişlər) lazımdır.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Siyahı filtri `category` (todo, in_progress, done, canceled): URL, görünüş və sayğaclarda | Status adları iş axınından asılıdır, kateqoriya hər yerdə eynidir | Status ID-ləri ilə: başqa layihədə işləmir |
| Yeni sayğaclar: `todo`, `done`, `month` (son tarix ay sonuna qədər **və** kateqoriya todo) | "Bu ay bitir" ödənilmişləri və gecikənləri saymır | `month`-u hamısı üçün: ödənilmiş kartlar da sayılardı |
| Layihə qaydalarında `queues`: göstərilən sayğaclar (boş = standart dəst) | Hər layihə öz ehtiyacına görə | Hamı üçün dəyişmək: digər layihələrə "bu həftə" lazımdır |

## Nəticələr

- Miqrasiya 00047: `project_rules.queues text[]`.
- FinTez MAH: Hamısı, Gecikən, Tamamlanmış, Bu ay bitir.
