# ADR-0064: Cədvəlin üstündə xülasə zolağı

- Status: Qəbul edilib
- Tarix: 2026-11-27
- Əsas: [ADR-0061](0061-project-rules.md) (layihə qaydaları), [ADR-0014](0014-saved-views-and-table-totals.md) (cəmlər)

## Kontekst

FinTez Mattermost-da aylıq hesabatın başlığını görürdü: "📅 Noyabr 2026 hesabatı · 💰 Aylıq ödəniş cəmi ·
📊 Yığılası qalıq · 💵 Ödənilib". Nebula-da cəmlər yalnız cədvəlin altında, sütun-sütun idi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Layihə qaydalarına `summary`: ≤ 6 sətir `{label, field, minus?, unit?}`; dəyər = Σ field − Σ minus | "Ödənilib = Aylıq ödəniş − Qalıq" bir sətirlə | Düstur sahəsi: boş qalığı (ödənilib) sıfır saymır, hər kartda əlavə sahə yaradır |
| Cəmlər mövcud `GET …/items/sums` ilə, cədvəlin **cari filtrləri** ilə; boş dəyər 0 sayılır | Görünən siyahı ilə uyğun; yeni API yoxdur | Ayrıca hesabat endpoint-i: təkrar məntiq |
| Başlıq: "📅 {ay il} hesabatı" (iş sahəsinin bu günü); canlı yenilənir | Mattermost-dakı kimi, amma həmişə aktual | Gündəlik snapshot: köhnəlmiş rəqəm |
| Sahələr: rəqəm, pul və rəqəm düsturu | Cəmlənə bilən tiplər | — |

## Nəticələr

- Miqrasiya 00046: `project_rules.summary jsonb` (≤ 6, CHECK).
- Veb: Layihə ayarları → "Tamamlanma və gecikmə qaydaları" → "Xülasə zolağı".
- FinTez MAH: "Aylıq ödəniş cəmi" (monthly_payment), "Yığılası qalıq" (balance), "Ödənilib" (monthly_payment − balance), ₼.
