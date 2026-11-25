# ADR-0052: Ana səhifədə şəxsi qeydlər (sticky notes)

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0037](0037-home-page.md) (ana səhifə "Mənim işlərim")

## Kontekst

İstifadəçilər gün ərzində kiçik xatırlatmaları (zəng ediləcək müştəri, "sabah hesabatı yoxla") kağıza və ya
messencerə yazırlar — bunlar tapşırıq deyil, tapşırıq kimi açmaq da artıq yükdür. Ana səhifədə sürətli, şəxsi
qeyd yeri istənildi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Qeydlər **serverdə**, istifadəçiyə bağlı (`user_notes`), workspace-dən asılı deyil | Hər cihazda görünür; workspace-dən çıxanda itmir | `localStorage`: yalnız bir brauzerdə qalır, təmizlənə bilər |
| **Yalnız müəllif** görür: bütün sorğular `user_id` ilə; başqasının qeydi "tapılmadı" (404) | Şəxsi qeyddə müştəri məlumatı ola bilər; mövcudluğu da sızmamalıdır | Komanda lövhəsi: ayrıca icazə modeli tələb edir — lazım olsa növbəti addım |
| Mətn 1–2000 simvol, 5 rəng (sarı, yaşıl, göy, çəhrayı, bənövşəyi), "yuxarıda saxla"; istifadəçi başına ≤ 100 qeyd (yaradılış istifadəçi sətri ilə seriallaşdırılır) | Kiçik və sürətli qalsın; limit yükü məhdudlaşdırır | Zəngin mətn/markdown: XSS riski və mürəkkəblik; mətn `pre-wrap` ilə göstərilir |
| Silmə dərhal, bildirişdə **"Geri qaytar"** (qeydi yenidən yaradır) | Təsdiq dialoqundan sürətli, səhv klik ucuz başa gəlir (ADR-0050) | Təsdiq dialoqu: kiçik qeyd üçün artıq addım |
| Audit hadisəsi yazılmır | Şəxsi qeyd iş məlumatı deyil; workspace audit jurnalına aid deyil | — |

## Nəticələr

- Miqrasiya 00038 (`user_notes`), API `GET/POST /me/notes`, `PATCH/DELETE /me/notes/{id}`.
- UI: ana səhifədə sayğaclardan sonra "Qeydlərim" bölməsi; ilk 8 qeyd, qalanı "Daha N qeyd göstər". Redaktədə
  Ctrl/⌘+Enter saxlayır, Esc ləğv edir. Rəng seçimi radio düymələrdir (klaviatura ilə seçilir).
