# ADR-0040: Tapşırıq cədvəlinin Excel (.xlsx) və CSV-yə ixracı

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0013](0013-project-privacy.md) (gizli layihələr), [ADR-0014](0014-saved-views-and-table-totals.md) (görünüşlər, sütunlar),
  [ADR-0025](0025-ui-languages.md) (dillər, onluq işarəsi), [ADR-0029](0029-audit-evidence-export.md) (sübut ixracı: limit və audit nümunəsi),
  [ADR-0030](0030-formula-fields.md) (düsturlar), [ADR-0035](0035-design-and-theme.md) (növbələr, alət paneli), [ADR-0019](0019-mobile-web-ui.md) (telefon)

## Kontekst

Maliyyə komandası (pilot) cədvəli hər həftə Excel-ə köçürür: ödəniş reyestri, bağlanış siyahısı, auditora göndəriş. İxrac yox idi —
istifadəçilər ekrandan kopyalayırdı: yalnız yüklənmiş səhifə, mətn kimi rəqəmlər, itən tarixlər. Lazım olan: cədvəlin **göründüyü
kimi** (filtrlər, axtarış, növbə, sıralama, görünən sütunlar, bütün səhifələr) Excel üçün düzgün tiplərlə və eyni məxfilik qaydaları ilə.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **`GET …/projects/{key}/items/export`** siyahının bütün sorğu parametrləri + `format=xlsx\|csv`, `cols`, `lang`, `delimiter=comma\|semicolon` | Siyahı ilə eyni URL dili: UI `itemsQueryString`-i təkrar istifadə edir, API istifadəçisi eyni filtrləri bilir; GET — yalnız oxuyur, arxivlənmiş workspace-də də işləyir (ADR-0039) | POST + JSON: filtrlərin ikinci təsviri; asinxron job + link: faylın serverdə saxlanması (məxfi məlumatın surəti) |
| Sətirlər **siyahının öz kod yolundan**: `parseItemFilters` + `store.ListItems` keyset səhifələri (500-lük) ilə, `loadProject` məxfilik yoxlaması | Guest/üzv dəqiq siyahıda gördüyünü alır; gizli layihə üzv olmayana 404 (inteqrasiya testi). Yeni filtr əlavə olunsa ixrac avtomatik onu tanıyır | Ayrı SQL: filtr və məxfilik qaydalarının iki nüsxəsi — gec-tez ayrılardı |
| Rol: **siyahını görən hər kəs** (guest daxil) | İxrac siyahıda olanı başqa formatda verir, yeni məlumat açmır | Yalnız admin: guest/auditor üçün ən lazımlı imkanı bağlayardı; qorunma limit və auditdədir |
| **Limit 50 000 sətir**: əvvəl `count(*) … LIMIT 50 001` (filtrlər tətbiq olunub), çoxdursa **422 `export_too_large` (`field: rows`)** — "filtrləri daraldın" | Böyük layihədə də sayma limitə qədər dayanır; istifadəçi yarımçıq fayl almır | Kəsib göndərmək: səssiz natamam hesabat — maliyyədə ən pis nəticə |
| **Axın (streaming)**: CSV hər sətirdə yazılır (4 KiB bufer); xlsx — excelize `StreamWriter` (16 MiB yaddaş, artığı `/tmp`-yə), **sheet XML-i ~24 MiB büdcə** ilə, eyni anda **2 ixrac** | Yaddaş sətir sayından asılı deyil. xlsx zip-i sonda yazılır, ona görə büdcə aşılırsa hələ başlıq göndərilməyib — düzgün 422 (`field: bytes`, "CSV seçin və ya sütunları azaldın") qaytarılır. Prod-da `/tmp` 64 MiB tmpfs-dir (importla paylaşılır): 2 × 24 MiB sığır | Hamısını yaddaşda qurmaq: 50k × 30 sütun yüzlərlə MB; limitsiz xlsx: tmpfs dolardı |
| CSV axın ortasında xəta verərsə əlaqə **qırılır** (`http.ErrAbortHandler`) | Başlıqlar artıq gedib; tam görünən, amma natamam fayl yaranmasın | 200 ilə yarımçıq fayl |
| **Kitabxana: `github.com/xuri/excelize/v2` v2.11.1** — artıq `go.mod`-dadır (import xlsx oxuyur), `go list -m -versions` ilə canlı yoxlandı: ən son buraxılış v2.11.1 | **Yeni asılılıq yoxdur** (təchizat zənciri dəyişmir); geniş istifadə olunan, aktiv saxlanılan, BSD-3; `StreamWriter` inline string yazır (paylaşılan sətir cədvəli yaddaşda böyümür) | Öz SpreadsheetML yazıcımız: tam sabit yaddaş, amma format səhvləri riski və saxlanma yükü; ikinci kitabxana: təchizat zənciri genişlənərdi |
| **Tiplər**: rəqəm sahəsi və rəqəm düsturu → rəqəm xanası (düsturun `scale`-i varsa `#,##0.00` formatı, yoxsa General); tarix → tarix xanası (format 14), `created_at`/`updated_at`/datetime → tarix+saat (format 22) **workspace saat qurşağında**; düsturlar **dəyər** kimi; düstur xətası `#div0` kimi mətn; seçim → etiket, çoxlu seçim → "a, b", istifadəçi → ad; bəli/xeyr → dilin sözü | Excel rəqəmi cəmləyə, tarixi süzə bilir; format 14/22 oxucunun regional ayarına uyğun göstərilir — onluq işarəsi və qruplaşma oxucunun Excel-ində "workspace konvensiyası" kimi görünür. Bəli/xeyr cədvəldəki kimi (TRUE/FALSE Excel dilində görünərdi) | Hamısı mətn: maliyyə üçün yararsız |
| **CSV**: UTF-8 + BOM, CRLF (RFC 4180), standart **vergül** ayırıcı və `.` onluq; seçimlə **nöqtəli vergül** və `,` onluq (az/ru/tr regional ayarlı Excel), rəqəmlər qruplaşmasız, tarix `YYYY-MM-DD`, tarix+saat `YYYY-MM-DD HH:MM:SS` | BOM ilə Excel azərbaycan hərflərini düzgün açır; ayırıcı və onluq işarəsi Excel-də birlikdə dəyişir — ayrı seçim mənasızdır; ISO tarixi hər lokal tanıyır | Dilə görə avtomatik `;`: eyni dil, fərqli Excel ayarı ola bilər — seçim istifadəçidədir |
| **Düstur inyeksiyasına qarşı (OWASP CSV Injection)**: mətn xanası `=` `+` `-` `@` (və tam enli formaları), tab və ya CR ilə başlayırsa (baş boşluq/yeni sətirdən sonra da) önünə `'` qoyulur — **yalnız CSV-də**, başlıqlar daxil. Sırf rəqəm mətni (`-12.50`) toxunulmur; real rəqəm sahələri rəqəm xanasıdır — mənfi məbləğ rəqəm qalır. **xlsx-də mətn olduğu kimi** inline string xanası kimi yazılır (`t="inlineStr"`, `<f>` yoxdur) — heç vaxt hesablanmır (2026-11-25 təhlükəsizlik baxışından sonra dəyişdi) | CSV-ni elektron cədvəl hər açılışda yenidən təhlil edir, xlsx-in tipli xanasını isə yox: apostrof xlsx-də yalnız məlumatı dəyişirdi (`-ödənilməyib` → `'-ödənilməyib`) | Hər iki formatda apostrof (əvvəlki qərar): xlsx məlumatı təhrif olunurdu; xlsx-də `quotePrefix` stili: excelize `Style` API-si onu açmır, inline string kifayətdir. Qalıq risk: istifadəçi xananı redaktə edib (F2/Enter) yenidən daxil etsə, Excel `=…` mətnini düstur kimi qəbul edə bilər — bu istifadəçinin öz hərəkətidir |
| Sütunlar: `cols` (cədvəlin regex-i, ≤30), naməlum/arxivlənmiş sahə atılır (cədvəl kimi), **açar və başlıq həmişə** (başlıq cədvəldəki yerdə); başlıqlar cədvəlin sözləri (`lang` = UI dili), sahə üçün sahənin adı | "Ekranda gördüyüm" ilə eyni | — |
| **Fayl adı**: `Content-Disposition: attachment; filename="<KEY>-<tarix>.xlsx"; filename*=UTF-8''<layihə adı>-<tarix>.xlsx` (RFC 5987; idarə simvolları, `/\:*?"<>\|` → `_`), tarix workspace saat qurşağında; `Cache-Control: no-store` | Köhnə klientlər ASCII adı, qalanları "Keyfiyyət-2026-11-25.xlsx"; başlıq inyeksiyası mümkün deyil (unit test) | — |
| **Limit**: istifadəçi başına saatda 30 (ardıcıl 10), eyni anda 2 ixrac (`429 rate_limited` / `export_busy` + `Retry-After`) | Hər ixrac 100 sorğuya qədər DB işidir; kütləvi çıxarışı yavaşladır və auditdə görünür | Limitsiz: məlumat çıxarışı və DB yükü |
| **Audit `items.exported`** layihə hadisəsi kimi, ilk bayt getməzdən əvvəl: format, sətir sayı, sütun id-ləri, dil, ayırıcı və filtr **xülasəsi** (axtarış olub-olmaması, status sayı, sahə açarları, icraçı növü `me/none/user`, son tarix, təsdiq, bloklanma, sıralama). **Xana dəyəri və axtarış mətni yoxdur** | Kim nə qədər çıxardı — izlənir; axtarış mətni özü məxfi ola bilər. Layihə hadisəsi olduğu üçün webhook layihə filtrinə tabedir (ADR-0013) | Hadisəsiz: maliyyə məlumatının izsiz çıxarışı |
| UI: cədvəl rejimində alət panelində **"İxrac"** menyusu (Excel / CSV, `;` seçimi), telefonda filtr pəncərəsində eyni bölmə; yüklənmə vəziyyəti (`aria-busy`, `aria-live`, spinner `motion-reduce`), xəta mətni nə edəcəyini deyir + "Yenidən cəhd et"; uğurda bildiriş; fayl Bearer token ilə `fetch` + blob (birbaşa link yoxdur — cookie ilə autentifikasiya yoxdur). 4 dil, 320 px | ADR-0019, ADR-0022 (fayllar heç vaxt ictimai link deyil) | `<a href>` birbaşa: token URL-də olmalı idi |

## API

- `GET /workspaces/{ws}/projects/{key}/items/export?format=xlsx|csv[&cols=key,title,field.amount][&lang=az|en|tr|ru][&delimiter=comma|semicolon]`
  + siyahının parametrləri (`q`, `status`, `assignee`, `due`, `due_from`, `due_to`, `field.<key>`, `sort`, `approval`, `blocked`, `parent`, `top_level`).
  `limit` və `cursor` nəzərə alınmır (bütün səhifələr).
- 200: `text/csv; charset=utf-8` və ya `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`;
  400 (parametr), 404 (layihəni görmür), 410 (arxivlənmiş layihə), 422 `export_too_large` (`field: rows|bytes`, `limit`), 429.

## Nəticələr

- Miqrasiya yoxdur. Yeni paket `internal/tableexport` (CSV/xlsx yazıcıları, `Neutralize`), `internal/server/export_api.go`,
  `store.CountItemsUpTo`, `store.RecordItemsExport`.
- 50 000 sətir × 30 sütunluq xlsx büdcəni aşa bilər — onda CSV təklif olunur (CSV-nin bayt limiti yoxdur, yalnız sətir limiti).
- İxrac adi sorğu müddətinə (`NEBULA_REQUEST_TIMEOUT`, standart 30 s) tabedir; 50k sətir lokal testdə bir neçə saniyədir.
- Say ilə axın arasında yaranan tapşırıqlar limitə qədər daxil olur; sətir sayı auditdə sayma anındakıdır.
- `'` prefiksi Excel-də görünür (`'=1+1`) — OWASP-ın tövsiyə etdiyi güzəşt.

## Açıq suallar

- Lövhə/təqvim rejimindən ixrac (hazırda yalnız cədvəl).
- Cəm sətri (cədvəldəki kimi) xlsx-də `SUBTOTAL` düsturu ilə — düstur yazmaq inyeksiya qaydasına istisna tələb edir.
- Çox layihəli (workspace üzrə) ixrac və planlı göndəriş (həftəlik reyestr email ilə).
- Böyük ixraclar üçün `/tmp` əvəzinə diskdə ayrıca qovluq (`excelize.Options.TmpDir`) — pilotun həcminə görə.
