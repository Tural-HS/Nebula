# ADR-0030: Düstur sahələri (formula fields)

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Maliyyə istifadəçiləri Excel-də yaşayır: "qalıq = məbləğ − ödənilib", "ƏDV = round(məbləğ × 0.18, 2)", "son tarixə
neçə iş günü qalıb". Nebula-da (ADR-0003) fərdi sahələr yalnız əl ilə doldurulur; hesablanan dəyər üçün istifadəçi
cədvəli Excel-ə ixrac edib orada hesablayır, sonra nəticə Nebula ilə uyğunsuz qalır. Hesablanan dəyər cədvəl
sütununda, cəm sətrində (ADR-0014), sıralamada və filtrdə adi rəqəm/tarix sahəsi kimi işləməlidir.

Tələblər: təhlükəsiz dil (eval, dövr, istifadəçi funksiyası yoxdur), pul üçün dəqiq onluq hesab (float64 yox),
dəyişdirilə bilməyən (read-only) dəyərlər, dövrsüz asılılıqlar, workspace saat qurşağında `today()` və iş təqvimi
(ADR-0027) ilə `workdays()`.

## Qərarlar — ifadə dili

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Əl ilə yazılmış rekursiv enmə parseri (`internal/formula`), yalnız standart kitabxana | Qrammatika kiçikdir (~15 qayda), hər addım bizim nəzarətimizdədir; asılılıq yoxdur | `expr-lang/expr`, CEL: güclü, amma dili bizim ehtiyacdan çox genişdir (string, map, makro), hər buraxılışda təhlükəsizlik səthi böyüyür; JS/Lua sandbox: ağır və riskli |
| Dil: onluq ədədlər, sahə açarları, `@due_date` (tapşırığın son tarixi), `+ - * /`, mötərizə, `= != <> < <= > >=`, `and or not`, `true/false` | Excel istifadəçisinə tanışdır; daxili sahələr `@` ilə başlayır, ona görə fərdi sahə açarı ilə toqquşmur | `due_date` adını rezerv etmək: mövcud workspace-lərdə belə açarlı fərdi sahə ola bilər |
| Funksiyalar: `if(c, a, b)`, `round(x, n)`, `min`, `max`, `abs`, `coalesce`, `today()`, `days(a, b)`, `workdays(a, b)`, `date(y, m, d)` | Maliyyə ssenarilərinin hamısını əhatə edir; siyahı qapalıdır | İstifadəçi funksiyaları, dövrlər: qadağan (tələbdə də belədir) |
| Statik tiplər: `number`, `date`, `boolean`; tarix ± tam ədəd = tarix, tarix − tarix = gün sayı | Səhv (məs. `tarix * 2`) saxlama anında, mövqe ilə tutulur, hesablama anında yox | Dinamik tip: xəta yalnız bəzi tapşırıqlarda üzə çıxardı |
| Limitlər: 1000 bayt, 40 səviyyə dərinlik (həm parser rekursiyası, həm ağac), 400 qovşaq, çağırışda 20 arqument, 50 fərqli istinad, 30 rəqəmli literal | İstənilən giriş üçün vaxt və yaddaş məhduddur; fuzz testi (12M+ icra) heç vaxt panic etmədiyini yoxlayır | — |
| Xətalar mövqe ilə: `at 10: unknown field "pad"` (1-dən başlayan simvol, rune); API 422 `field: "options.formula.expression"` | Redaktor xətalı simvolu işarələyir | — |
| Boş dəyər nəticəni boş edir (`amount - paid`, `paid` boşdursa boş); `coalesce(paid, 0)` ilə 0 götürülür; `min/max` boşları atlayır (Excel kimi); `if`-də boş şərt `else` qoluna gedir; `and/or` SQL-in üçqiymətli məntiqi | Maliyyədə "bilinmir" ilə "0" fərqlidir; açıq `coalesce` səhv cəmin qarşısını alır | Excel-in boşu 0 saymağı: ödənilməmiş fakturada qalıq səssizcə məbləğə bərabər olardı |
| `workdays(a, b)`: `a < gün ≤ b` aralığında iş günlərinin sayı (b < a olduqda mənfi); `days` eyni semantika ilə. Hesablama O(bayramlar), gün-gün yox | `workdays(today(), due)` "qalan iş günləri"dir, bu gün 0-dır; hər tapşırıqda ucuzdur | Excel `NETWORKDAYS` (hər iki uc daxil): "bu gün" 1 olardı, `days` ilə uyğunsuz |
| Formula başqa formuladan istifadə edə bilər; dövr və 10 səviyyədən dərin zəncir saxlanarkən rədd edilir (`circular reference: a → b → a`) | "net → vat → total" kimi zəncirlər real ehtiyacdır | Formuladan formula qadağası: istifadəçi ifadəni təkrarlamalı olardı |
| Formula yalnız `number`, `date`, `boolean` və `formula` sahələrindən istifadə edir | Mətn/seçim üzərində hesab mənasızdır, tip yoxlaması sadə qalır | `datetime`: saat qurşağı qaydası tələb edir, sonraya |

## Qərarlar — dəqiqlik (pul)

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `math/big.Rat` (dəqiq kəsr), aralıq nəticələr 20 onluq rəqəmə yuvarlaqlaşdırılır, \|aralıq\| ≤ 1e30 | Toplama/çıxma/vurma tam dəqiqdir, bölmə 20 rəqəmə qədər; məxrəc sonsuz böyüyə bilmir. Standart kitabxanadır, asılılıq və CVE riski yoxdur | `shopspring/decimal`, `cockroachdb/apd`: əlavə asılılıq, bizə lazım olan hissə (dörd əməl + yuvarlaqlaşdırma) 60 sətirdir; float64: `0.1 + 0.2 ≠ 0.3` |
| Giriş: rəqəm sahəsi JSON-da float64 kimi saxlanılır (ADR-0003, \|v\| ≤ 1e15); ən qısa geri-dönən təsvir (`strconv 'f', -1`) onluğa çevrilir | `0.1` dəqiq 1/10 olur, ikilik yaxınlaşma deyil | — |
| Yuvarlaqlaşdırma: **yarısı sıfırdan uzağa** (`2.5 → 3`, `-2.5 → -3`, `0.125 → 0.13`) — Excel `ROUND` və maliyyədə adət olunan "half-up" | Mühasib Excel ilə eyni nəticəni gözləyir | Bankir yuvarlaqlaşdırması (half-even): statistik cəmlərdə yaxşıdır, amma Excel ilə fərqli rəqəm "səhv" görünür |
| Saxlanılan nəticə sahənin `scale`-inə (0–10, susmaya görə 2) yuvarlaqlaşdırılır, sondakı sıfırlar atılır; \|nəticə\| > 1e15 → `overflow` xətası | Cədvəldə görünən, cəmlənən və digər formulalara ötürülən dəyər **eynidir** ("görünən = istifadə olunan") | Yuvarlaqlaşdırılmamış dəyəri saxlayıb yalnız göstərişdə yuvarlaqlaşdırmaq: cəm sətri ayrı-ayrı sətirlərin cəmindən fərqlənərdi |
| Sıfıra bölmə, daşma, tarix aralığından çıxma, səhv arqument (`due + 0.5`, `date(2026, 2, 30)`) **dəyərdir**: `{"error": "division_by_zero"}` və s. Xəta asılı formulalara ötürülür (Excel `#DIV/0!` kimi) | Bir tapşırığın xətası nə sorğunu, nə də digər tapşırıqları sındırır; UI `#SIFIRA BÖLMƏ` göstərir | Yazmanı rədd etmək: `paid = 0` yazmaq mümkün olmazdı |
| `if` yalnız seçilmiş qolu hesablayır | `if(paid = 0, 0, amount / paid)` xətasızdır | — |

## Qərarlar — saxlama və yenidən hesablama

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Yazma anında hesablama**: nəticə `items.fields`-də adi dəyər kimi saxlanılır (JSON number / `"YYYY-MM-DD"` / boolean / xəta obyekti) | Cəm (`numeric` ilə, ADR-0014), sıralama və `@>` filtri SQL-də dəyişmədən işləyir, oxuma ucuz qalır | Oxuma anında hesablama: SQL-də cəm/sıralama mümkün olmazdı, hər səhifədə bütün sətirlər Go-da hesablanmalı idi |
| Hər yazma yolu eyni koddan keçir: yaratma, yeniləmə, idxal (ADR-0011), toplu redaktə (ADR-0023), təkrarlanan tapşırıqlar, təsdiqin tətbiqi (ADR-0026) | Bir qayda, istisna yoxdur | — |
| Formula workspace səviyyəsindədir və workspace-in **bütün** tapşırıqları üçün hesablanır; bağlama (item type) yalnız paneldə görünüşü idarə edir | Sadə və sabit: tip bağlamasını dəyişmək dəyərləri silmir/yaratmır; girişi olmayan tapşırıqda nəticə onsuz da boşdur | Yalnız bağlı tiplər üçün: hər bağlama dəyişikliyi fon keçidi tələb edərdi |
| Fon keçidi (`formula_recompute`, workspace başına bir sətir): formula yaradılanda/dəyişəndə/bərpa olunanda, iş təqvimi dəyişəndə (`workdays()` olan formulalar), saat qurşağı dəyişəndə (`today()` olanlar). Worker hər dəqiqə 500 tapşırıqlıq tranzaksiyalarla, `id` sırası ilə gedir (tick başına ≤ 20 partiya) | Böyük workspace-də uzun kilid yoxdur; yeni sorğu keçidi əvvəldən başladır | Tərif dəyişikliyində bütün tapşırıqları eyni tranzaksiyada yeniləmək: 100k sətirdə uzun kilid və timeout |
| Keçid kilidlənmiş sətirləri **gözləmir** (`FOR UPDATE SKIP LOCKED`), kursoru ilk ötürülən sətirdən əvvəl saxlayır və növbəti partiyada təkrarlayır | Toplu redaktə ilə deadlock mümkün deyil; heç bir sətir buraxılmır | Adi `FOR UPDATE`: fərqli kilid sırası ilə toplu redaktə ilə deadlock |
| Sıralama zəmanəti: tapşırıq yazmaları workspace sətrini `FOR SHARE` ilə, tərif/təqvim/saat qurşağı dəyişiklikləri `FOR NO KEY UPDATE` ilə kilidləyir | Dəyişiklik commit olunandan sonra hər yazma yeni tərifi görür, əvvəlki yazmaları isə fon keçidi düzəldir: köhnə tərif ilə yazılmış "itən" tapşırıq qalmır | Kilidsiz: yaradılan tapşırıq keçidin artıq keçdiyi `id` aralığına düşsə, köhnə dəyərlə qalardı |
| `today()` istifadə edən ("volatile") formulalar da saxlanılır və workspace-in **yerli** günü dəyişəndə (`ScheduleVolatileFormulas`, hər tick, idempotent) yenidən hesablanır | Cəm/sıralama onlar üçün də işləyir; gecə yarısından ən çox bir neçə dəqiqə sonra dəyərlər təzədir | Volatile-ları cəm/sıralamadan çıxarmaq: "son tarixə qalan günlər" ən çox istənən sütundur |
| Fon keçidi `version`-u artırmır, hadisə yazmır və `updated_at`-a toxunmur (tranzaksiya daxilində `nebula.computed_only`, yeni `items_set_updated_at()` trigger funksiyası) | `updated_at` "insan və ya qayda dəyişdi" mənasını saxlayır; gündəlik keçid bütün tapşırıqları "yenilənmiş" göstərməzdi; dəyər dəyişmirsə `UPDATE` heç göndərilmir | Adi `UPDATE`: hər gecə 100k tapşırıq "indicə yenilənib" görünərdi |
| Tapşırıq yazılanda formula dəyərləri köhnəlibsə (məs. keçid kilidli sətri ötürüb), istifadəçi heç nə dəyişməsə də yalnız hesablanan dəyər səssizcə düzəldilir | Köhnə dəyər uzun qalmır | — |

## Qərarlar — bütövlük

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Formula dəyəri **yalnız oxunur**: API (yaratma/PATCH, `null` da daxil), toplu redaktə (sorğu başlamazdan əvvəl 422), idxal (formula hədəf kimi təklif edilmir, xəritədə 422) rədd edir; təkrarlanan tapşırıq şablonu da qəbul etmir | Dəyər həmişə düsturun nəticəsidir | — |
| Formula sahəsi **məcburi** edilə bilməz | Dəyəri istifadəçi verə bilmir | — |
| İstinad edilən sahə (fərdi və ya formula) aktiv formula tərəfindən istifadə olunarkən **arxivlənə bilməz**: 409 `field_in_use` istifadəçiləri sadalayır. Arxivlənmiş formulanın bərpası cari sahələrə qarşı yenidən yoxlanılır | Formula heç vaxt yox olmuş sahəyə istinad etmir; sahə silmək yoxdur, yalnız arxiv (ADR-0003) | Səssizcə boş nəticə: hesabat xəbərsiz səhv olardı |
| Sahə tipi dəyişməzdir (ADR-0003), ona görə tip uyğunsuzluğu yaranmır. Formulanın **nəticə tipi** də dəyişməzdir: `number` formulanı `date`-ə çevirən ifadə rədd edilir | Görünüşlərin cəm/sıralama konfiqurasiyası etibarlı qalır | — |
| Arxivlənmiş formulanın dəyərləri donur (oxunur, yenilənmir) | ADR-0003-ün arxiv semantikası | — |
| Törəmə məlumat (`result_type`, `refs`, `volatile`, `calendar`) saxlanarkən `options.formula`-ya yazılır, API onu qəbul etmir (strict decoder) | Asılıları SQL ilə (`options->'formula'->'refs' ? key`) parse etmədən tapmaq | — |

## Hadisələr və audit

| Nə | Necə |
|---|---|
| Tərif yaradılması | `field.created` payload-da `formula: {expression, result_type, scale}` |
| İfadə/scale dəyişikliyi, arxiv | `field.updated` `changes["options.formula"] = {from, to}` (köhnə və yeni ifadə) |
| Hesablanan dəyərin dəyişməsi | **Audit olunmur**: `item.updated` `changes`-də yalnız istifadəçinin dəyişdirdiyi giriş sahələri var; fon keçidi hadisə, bildiriş, webhook yaratmır. Dəyər deterministikdir: girişlər, ifadə, tarix və təqvim — hamısı öz hadisələrində audit olunur |
| Avtomatlaşdırma | `item.updated` snapshot-u formula dəyərlərini də ehtiva edir (`fields.<key>`), şərtlər onları oxuya bilər |

## API (ADR-0002)

| Metod və yol | Rol | Təsvir |
|---|---|---|
| `POST /workspaces/{ws}/fields` | admin+ | `{"type": "formula", "options": {"formula": {"expression": "amount - paid", "scale": 2}}}` |
| `PATCH /workspaces/{ws}/fields/{key}` | admin+ | `options.formula` = tam yeni `{expression, scale}` |
| `POST /workspaces/{ws}/fields/formula-preview` | admin+ | `{expression, scale?, key?, item?: "FIN-12", values?: {...}}` → `{valid, error?: {position, message}, result_type, refs, volatile, calendar, value, item}`; heç nə yazmır. Kompilyasiya xətası 200 + `valid: false`. Nümunə tapşırıq gizli layihədədirsə və baxan onu görmürsə "item not found" (ADR-0013) |
| `GET …/items?sort=field.<key>` / `sort=-field.<key>` | guest+ | `number`, `date` və nəticəsi rəqəm/tarix olan formula sahələri; boşlar hər iki istiqamətdə sonda; bərabərlikdə nömrə; kursor `f1:<sort>:<dəqiq dəyər>:<n>` (dəyər SQL-dən mətn kimi götürülür, float64 yuvarlaqlaşdırması kursoru pozmur). Saxlanılan görünüşlərin `config.sort`-u da bunu qəbul edir |
| `GET …/items/sums?fields=<formula>` | guest+ | Nəticəsi rəqəm olan formula cəmlənir; tarix formulası 422 |
| `GET …/items?field.<formula>=<v>` | guest+ | Bərabərlik filtri nəticə tipinə görə (rəqəm, tarix, boolean) |

## UI

- Ayarlar → Sahələr: "Düstur" tipi; monospace düstur sahəsi, hər fasilədə (350 ms) server önizləməsi, xəta mövqeyi
  işarələnmiş simvolla, nəticə tipi, nümunə tapşırıq (`FIN-12`) üçün dəyər, `today()`/`workdays()` qeydləri; rəqəm
  nəticəsi üçün onluq rəqəm sayı. Parser yalnız serverdədir, ona görə redaktorun qəbul etdiyi = saxlamanın qəbul etdiyi.
- Cədvəl: formula sütununun başlığında `ƒ`, xana yalnız oxunur (sətirdaxili redaktor yoxdur), `title`-da düstur;
  nəticəsi rəqəm olan formula cəm sətrində; sıralama seçimində cədvəldəki rəqəm/tarix/formula sütunları ↑/↓.
- Tapşırıq paneli: dəyər `ƒ` ilə, yalnız oxunur, düstur tooltip-də; xəta qırmızı (`#SIFIRA BÖLMƏ`).
- Rəqəmlər interfeys dilinə görə formatlanır (ADR-0025): az/ru `1 234,5`, tr `1.234,5`, en `1,234.5`. Bu, bütün
  rəqəm sahələrinə və cəm sətrinə aiddir.
- Bütün mətnlər `locales/<dil>/formulas.ts`-dədir (az mənbədir; en/ru-da `plural()`), fasad `i18n-formulas.ts`.

## Miqrasiya 00023

- `field_definitions.field_type` CHECK-ə `formula` əlavə olunur (cədvəl kiçikdir, qısa kilid).
- `formula_recompute` cədvəli (workspace başına bir sətir, `pending`, `after_id`, `volatile_day`).
- `items_updated_at` trigger-i `items_set_updated_at()` funksiyasına keçir: `nebula.computed_only = on` olan tranzaksiyada
  `updated_at` dəyişmir; trigger əvəzlənməsi `items`-də millisaniyəlik kilid (`lock_timeout` 5 s), cədvəl yenidən yazılmır.
- Down: formula sahəsi varsa xəta ilə dayanır (köhnə buraxılış bu tipi tanımır); yoxdursa trigger, cədvəl və CHECK
  əvvəlki halına qayıdır. Əl ilə təmizləmə: `deploy/production/README.md`, rollback cədvəli, 00023.

## Nəticələr

- Müsbət: Excel-dəki hesablamalar Nebula-da, dəqiq onluqlarla, cəm/sıralama/filtr ilə; xətalar tapşırıq səviyyəsində
  qalır; `workdays()` iş təqvimi ilə (ADR-0027) — rəqiblərdə nadir rast gəlinən imkan.
- Mənfi: sahə sıralaması üçün indeks yoxdur (layihənin filtrlənmiş sətirləri skan olunur); 100k tapşırıqlı layihədə
  ölçüləndən sonra isti sahələr üçün expression indeks əlavə oluna bilər.
- Mənfi: API cavabında rəqəmlər hələ də JSON number-dir (float64-ə decode olunur): 15–17 rəqəmdən uzun nəticələr
  göstərişdə son rəqəmini itirə bilər; saxlanılan dəyər və SQL cəmi dəqiqdir.
- Mənfi: `today()` formulası olan böyük workspace-də hər gecə dəyəri dəyişən bütün tapşırıqlar yenidən yazılır
  (100k sətir ≈ 200 partiya, bir neçə dəqiqə); `updated_at` və hadisələr toxunulmur.
- Mənfi: hər tapşırıq yazması workspace sətrini `FOR SHARE` kilidləyir və aktiv tərifləri oxuyur (bir sorğu); tərif,
  təqvim və saat qurşağı dəyişiklikləri gedən yazmaları (uzun idxal daxil) gözləyir.
- Riskli nöqtə: xəta kodları (`division_by_zero` və s.) API müqaviləsinin hissəsidir; yeni kod UI-də `#kod` kimi
  göstərilir.
- Açıq suallar: mətn funksiyaları (`concat`, mətn müqayisəsi), `datetime` girişi, `edate`/`eomonth`, alt tapşırıqlar
  üzrə toplama (rollup) — ayrıca ADR ilə.
