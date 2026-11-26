# ADR-0057: Gecikmə pillələri (sarı, narıncı, qırmızı)

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0009](0009-due-dates-and-time-automation.md) (son tarix, gecikmə), [ADR-0023](0023-calendar-bulk-inline.md) (təqvim)

## Kontekst

FinTez müştərinin ödənişi gecikəndə bunun neçə gün olduğunu bir baxışda görmək istəyir. İndiyə qədər gecikən
tapşırıq yalnız bir rənglə (qırmızı) göstərilirdi. Təqvimdə isə tapşırıq yalnız son tarix günündə görünürdü.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Üç pillə: **1–10 gün sarı**, **11–20 narıncı**, **21+ qırmızı** | Müştərinin istədiyi hədlər | Hədləri ayarlanan etmək: hələ ehtiyac yoxdur, sonra əlavə etmək asandır |
| Gecikib-gecikmədiyinə **server** qərar verir (`overdue`); veb yalnız pilləni hesablayır | Bir mənbə: filtrlər, sayğaclar, xülasə eyni qaydanı işlədir | Pilləni serverdə saxlamaq: hər gün yenidən yazmaq lazım olardı |
| Gün sayı brauzerin yerli tarixi ilə hesablanır | Son tarix nişanı hər yerdə işlədilir və workspace-in saat qurşağını bilmir; FinTez üçün eyni qurşaqdır | Saat qurşağını hər komponentə ötürmək: çox dəyişiklik, az fayda |
| Son tarix nişanı rənglənir, gün sayı yazılır ("· 15"); ekran oxuyucusu üçün "15 gün gecikir" | Rəng tək məna daşımır | Yalnız rəng: rəng korları üçün oxunmur |
| Lövhə kartı solda pillə rəngində zolaq alır | Lövhədə uzaqdan seçilir | Bütün kartı rəngləmək: mətn oxunmur |
| Təqvimdə gecikən **hər gün** (son tarixin ertəsindən bu günə qədər) həmin günün pilləsi ilə göstərilir; gecikən tapşırıqlar ayrıca sorğu ilə gəlir (`due=overdue`, ilk səhifə) | Gecikmənin necə uzandığı görünür; keçən ay gecikən də bu ayda görünür | Yalnız son tarix günü: gecikmə müddəti görünmür |
| Lövhə yalnız layihədə tapşırığı olan **tapşırıq növlərinin** sütunlarını göstərir (boş layihədə hamısı) | Başqa layihənin iş axını (məs. Reyestrin "Aktiv/Bağlanıb") MAH lövhəsini qarışdırırdı | Layihəyə növ bağlamaq: model dəyişikliyi tələb edir |

## Nəticələr

- Miqrasiya yoxdur; yalnız veb dəyişir (`web/src/lib/overdue.ts`).
- Son tarix nişanının ekran oxuyucu mətni "(gecikir)" əvəzinə "(N gün gecikir)" olur.
- Təqvimdə gecikən çox tapşırıq olanda gün xanasında 3-ü göstərilir, qalanı "+N gecikən" ilə açılır.
