# ADR-0044: Pul sahələri (məbləğ + valyuta)

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0003](0003-custom-fields.md) (fərdi sahələr), [ADR-0011](0011-spreadsheet-import.md) (idxal), [ADR-0014](0014-saved-views-and-table-totals.md)
  (cəmlər, sıralama), [ADR-0023](0023-calendar-bulk-inline.md) (cədvəldə redaktə), [ADR-0025](0025-ui-languages.md) (4 dil, rəqəm üslubu),
  [ADR-0030](0030-formula-fields.md) (düsturlar), [ADR-0040](0040-item-table-export.md) (ixrac)

## Kontekst

Pilot müştəri (fintech, bek-ofis) məbləğləri bu gün `number` sahəsində saxlayır: valyuta ya sahənin adındadır ("Məbləğ, AZN"),
ya ayrıca seçim sahəsindədir. Nəticədə cədvəlin cəm sətri AZN ilə USD-ni toplayır, sıralama və ixrac valyutanı itirir, idxalda
"$300" valyutasız 300 olur. `number` sahəsi həm də float64-dür (ADR-0003, |v| ≤ 1e15): qəpik dəqiqliyi zəmanətli deyil.
Lazım olan: **dəqiq onluq məbləğ + ISO 4217 valyuta**, valyutaları heç vaxt qarışdırmayan cəmlər və ixrac.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Yeni sahə tipi `money`**; dəyər `items.fields`-də obyektdir: `{"amount": "1250.50", "currency": "AZN"}`. Məbləğ **onluq mətndir**, həmişə sahənin `scale`-i qədər onluq rəqəmlə (kanonik forma), valyuta sahənin icazəli siyahısından ISO 4217 kodudur | Hər dəyərin tək kanonik forması var, ona görə mövcud `fields @> …` filtri (GIN `jsonb_path_ops`) dəyişmədən işləyir; mətn heç vaxt float-dan keçmir. Ayrıca cədvəl və ya sütun lazım deyil | `number` + ayrıca valyuta sahəsi: iki sahə sinxron qalmır, cəm yenə qarışır. Məbləği qəpiklə tam ədəd (`125050`) saxlamaq: hər oxuyan (API, ixrac, webhook) scale-i bilməlidir. JSON number: float64 |
| **Miqrasiya 00034 yalnız `field_type` CHECK-ini genişləndirir**; seçimlər `field_definitions.options.money`-dədir | Dəyərlər mövcud JSONB-dədir; yeni cədvəl/sütun yoxdur. CHECK olmadan `money` tipi yaradıla bilməz (00001/00023 siyahısı) | — |
| **Seçimlər**: `currencies` (standart AZN, USD, EUR; ən çox 30, yalnız aktiv ISO 4217 kodları — fond, metal, test kodları yox), `default_currency` (standart AZN, siyahıda olmalıdır), `scale` 0–4 (standart 2). Kod siyahısı serverdə əl ilə saxlanılır, heç nə internetdən yüklənmir | Siyahı on ildə bir neçə dəfə dəyişir; şəbəkədən asılılıq və SSRF riski yoxdur. Admin JPY üçün 0, kriptovalyutasız mühitdə 4 seçə bilər | Sərbəst kod: "AZM", "MANAT" kimi səhvlər cəmləri bölərdi |
| **`scale` yaradıldıqdan sonra dəyişmir** (422, "yeni sahə yaradın"); valyutalar əlavə/silinə bilər — silinmiş valyutada olan dəyər oxunur, cəmlənir, yalnız yeni yazı icazəli valyuta tələb edir | Kanonik forma scale-dən asılıdır: scale dəyişsə köhnə dəyərlər bərabərlik filtrinə düşməzdi və ya bütün `items` yenidən yazılmalı olardı | Fon miqrasiyası ilə yenidən yazmaq: böyük workspace-də uzun, riskli əməliyyat; bu raund üçün ehtiyac yoxdur |
| **Dəqiqlik**: məbləğ ən çox **15 rəqəm** (tam + onluq; scale 2-də 9 999 999 999 999,99). **Yuvarlaqlaşdırma yoxdur**: scale-dən çox onluq — 422 "ən çox N onluq rəqəm, yuvarlaqlaşdırın" | 15 rəqəm float64-də və Excel-də dəqiqdir: ixrac heç vaxt qəpik dəyişmir. Səssiz yuvarlaqlaşdırma maliyyədə səhvi gizlədir | Sərhədsiz `numeric`: Excel 15 rəqəmdən sonrasını kəsir |
| **API girişi**: `{"amount": "1250.5" \| 1250.5, "currency": "usd"}` — kod böyük hərfə çevrilir, valyuta verilməsə standart valyuta; `amount` boş və ya `null` dəyəri silir. API-də məbləğ yalnız nöqtəli, qruplaşdırmasız onluqdur | Bir aydın format; lokal formatlar (`1 250,50`) UI və idxalın işidir | API-də lokal format: "1,250" 1.25 və ya 1250 ola bilərdi |
| **Göstərmə**: `1 250,50 AZN` (az/ru), `1,250.50 AZN` (en), `1.250,50 AZN` (tr) — mövcud `formatDecimal` rəqəm üslubu (ADR-0025) + qırılmayan boşluq + **ISO kodu**, simvol yox. Sağa düzlənmiş, `tabular-nums` | Rəqəm sahələri ilə eyni üslub; kod birmənalıdır ("$" 20-dən çox valyutadır) və ekran oxuyucu onu düzgün oxuyur — əlçatan ad valyutanı ehtiva edir | `₼`/`$` simvolları: birmənalı deyil, ekran oxuyucularda fərqli oxunur |
| **Redaktə**: cədvəldə (inline) və panel/yaratma dialoqunda eyni `MoneyEditor` — məbləğ (`inputMode="decimal"`, "Qiymət: məbləğ") + valyuta seçimi ("Qiymət: valyuta"), `role="group"`. Fokus hər ikisindən çıxanda və ya Enter ilə saxlanılır, Escape ləğv edir. Lokal yazılış qəbul olunur (`1 250,50`, `1250.5`, en-də `1,250`); xəta sahənin adını və düzəlişi deyir | Bir komponent — iki yerdə eyni davranış; xəta mesajları "nə etməli" deyir | İki ayrı xana (məbləğ/valyuta sütunları): sinxronlaşma və cəm problemi geri qayıdır |
| **Cəmlər valyuta üzrə**: `GET …/items/sums?fields=price` money sahəsi üçün `sum` qaytarmır, `currencies: [{currency, sum, count}]` (kod sırası ilə) qaytarır; cəm sətri `1 200,00 AZN · 300,00 USD`. Hesablama SQL `numeric`-də, `GROUP BY` valyuta; saxlanma formasına uyğun olmayan məlumat (regex qoruması) nəzərə alınmır, sorğu heç vaxt səhv cast ilə yıxılmır | AZN ilə USD heç vaxt toplanmır; cəmlər siyahı filtrlərinə tabedir (valyuta filtri daxil) | Bir cəm + "qarışıq" nişanı: istifadəçiyə faydasız |
| **Valyuta konvertasiyası (FX) yoxdur** | Kurs mənbəyi (CBAR), tarix (əməliyyat günü?), saxlanma və audit ayrıca qərar tələb edir; internetdən kurs çəkmək bu raundda qadağandır | Sabit kurs cədvəli: köhnəlir və yanlış hesabat verir |
| **Düsturlar money sahəsinə istinad edə bilməz** (422: "formulas cannot use money fields yet") | Düsturun valyutası yoxdur; `price_usd + price_azn` səssizcə valyutaları qarışdırardı. Ən təhlükəsiz başlanğıc | Yalnız məbləği istifadə etmək: məhz qarışdırma riskidir. Eyni valyuta yoxlaması: düsturun nəticəsi üçün valyuta tipi lazımdır (növbəti addım) |
| **Sıralama məbləğə görədir** (`sort=field.price`, keyset kursoru mövcud rəqəm sıralaması kimi), valyutaya baxmadan; boşlar sonda | Sadə və kursorla uyğundur. Mənalı sıra üçün valyuta filtri ilə birləşdirilir (`field.price=USD&sort=-field.price`) — UI filtri təklif edir | Əvvəl valyuta, sonra məbləğ: kompozit kursor, valyuta sırasının özü (əlifba) heç nə demir |
| **Filtr valyutaya görədir**: `?field.price=USD` → `fields @> {"price": {"currency": "USD"}}` (indeksli). Filtr menyusunda sahənin valyutaları | Bek-ofisin tipik sualı "bütün USD ödənişləri" | Məbləğ aralığı filtri: ayrıca iş (rəqəm sahələri üçün də yoxdur) |
| **İdxal**: `1250.50 AZN`, `AZN 1250`, `1 250,50 ₼`, `$300`, `€5,5`, `(100) manat`, valyutasız (standart valyuta). Rəqəm qaydaları `ParseNumber`-dəndir (birmənalı olmayan `1.234` rədd edilir), amma məbləğ **mətn kimi** çıxarılır. Simvol yalnız sahənin icazəli valyutalarından **tam birinə** uyğunsa qəbul olunur (`$` USD-dir, sahə CAD-ı da icazə verirsə — xəta "kodu yazın"); iki valyuta, naməlum kod, icazəsiz valyuta, artıq onluq — sətir xətası | Səssiz səhv idxal (1.234 əvəzinə 1234 manat, CAD əvəzinə USD) soruşmaqdan pisdir | Simvolu həmişə USD saymaq: Kanada/Avstraliya dolları ilə səhv |
| **İxrac**: money sütunu **iki sütundur** — məbləğ (xlsx-də ədədi xana, `#,##0.00` formatı sahənin scale-i ilə; CSV-də qruplaşdırmasız, ayırıcıya görə `,`/`.`) və "<Ad> (valyuta)" mətn sütunu | Ədədi xanalar ədədi qalır (Excel-də SUM, pivot), valyuta pivotda qrup ola bilir. Valyutaya görə format sətri (`#,##0.00 "AZN"`): xana başına stil, pivot valyutaya görə bölə bilmir | Mətn "1 250,50 AZN": Excel üçün ədəd deyil |
| **Hesabatlar və qəbul formaları** money sahəsini hələ dəstəkləmir (formalar sahə siyahısında göstərmir) | Bu raundun əhatəsi; hər biri valyuta üzrə qruplaşdırma tələb edir | — |

## Nəticələr

- Yeni: `internal/store/money.go` (tip, seçimlər, ISO 4217 siyahısı, `CanonicalAmount`, `ResolveCurrency`), cəm/sıralama/filtr
  `itemsort.go`/`fields.go`-da, idxal `importer.ParseMoney`, ixrac `export_api.go`; veb: `lib/money.ts`, `MoneyEditor`, `MoneySettings`.
- Audit: sahənin yaradılması (`field.created`, `money` seçimləri ilə) və seçimlərin dəyişməsi (`field.updated`, `options.money` fərqi) —
  mövcud hadisələr; dəyər dəyişiklikləri mövcud `item.updated` tarixçəsindədir.
- Rollback: 00034 Down money sahəsi varsa dayanır (köhnə image tipi tanımır); `deploy/production/README.md` §9.
- Webhook və API istehlakçıları yeni dəyər formasını (obyekt) görür; yalnız yeni tipdə — mövcud sahələr dəyişmir.

## Açıq suallar

- FX: CBAR kurslarını server tərəfində (SSRF qaydaları ilə, `internal/safehttp`) gündəlik saxlamaq və cəmi "AZN ekvivalenti" kimi göstərmək?
- Düsturlarda eyni valyutalı arifmetika (nəticə tipi "money", valyuta istinad edilən sahədən) və `amount(price)` funksiyası.
- Hesabatlarda (ADR-0041) valyuta üzrə məbləğ cəmləri; qəbul formalarında money sahəsi.
- `number` sahələrinin money-ə çevrilməsi (valyuta seçərək) üçün admin aləti.
- Məbləğ aralığı filtri (`>=`, `<=`) rəqəm və money sahələri üçün.
