# ADR-0063: Ada görə sıralama (Azərbaycan əlifbası)

- Status: Qəbul edilib
- Tarix: 2026-11-27
- Əsas: [ADR-0014](0014-saved-views-and-table-totals.md) (görünüşlər, sıralama)

## Kontekst

FinTez müştəri siyahısını adla, əlifba sırası ilə baxmaq istəyir. Sıralama yalnız nömrə, son tarix və sahələrə
görə idi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `sort=title` (A–Z) və `sort=-title` (Z–A); nömrə bərabər adları ayırır | Tam sıra: keyset səhifələmə nə sətir buraxır, nə təkrarlayır | OFFSET: böyük siyahıda yavaş və qeyri-dəqiq |
| Sıralama `az-x-icu` collation ilə (deterministik): C, Ç, …, E, Ə, …, S, Ş | Azərbaycan əlifbası; ICU PostgreSQL image-də var | `C` collation: Ə, Ç, Ş sonda düşür |
| Kursor başlığı base64 ilə saxlayır (`t1:`), kursor limiti 2000 simvol | Başlıqda ":" və istənilən mətn ola bilər | — |

## Nəticələr

- Miqrasiya yoxdur (indeks yoxdur: layihə siyahıları kiçikdir; lazım olsa `(project_id, title COLLATE "az-x-icu", number)` indeksi əlavə olunar).
- Veb: sıralama menyusunda "Ada görə (A–Z)" və "(Z–A)".
