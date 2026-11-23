# ADR-0011: CSV və Excel (.xlsx) cədvəllərindən tapşırıq idxalı

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

Şirkətdə müştəri və ödəniş izləmə cədvəlləri Excel-də saxlanılır (məs. ~60 müştərilik vərəq: filial,
müqavilə nömrəsi, başlama tarixi, aylıq məbləğ ₼ ilə, qalıq, doğum günü). Bu cədvəlləri Nebula layihəsinə
köçürmək üçün indiyə qədər əl ilə yazılmış DB skriptləri lazım idi: workflow, məcburi sahələr və audit
log bu yolda yan keçilirdi. Lazım olan: admin faylı yükləyir, sütunları sahələrə uyğunlaşdırır, xətaları
əvvəlcədən görür və hər şey adi tapşırıq yaratma qaydaları ilə idxal olunur.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| Fayl **serverdə, Go-da** oxunur: CSV üçün `encoding/csv`, .xlsx üçün `github.com/xuri/excelize/v2` **v2.11.1** (2026-11-02, govulncheck: zəiflik yoxdur) | Bütün yoxlamalar və limitlər bir yerdədir; brauzerə əlavə cədvəl parser-i (və onun hücum səthi) gəlmir. excelize aktiv saxlanılan, ən geniş yayılmış Go kitabxanasıdır |
| **Stateless**: fayl heç yerdə saxlanılmır, 00008 miqrasiyası lazım olmadı. Önbaxış və idxal eyni faylı yenidən göndərir | Diskdə/bazada müştəri məlumatı olan fayl qalmır; təmizləmə işi, TTL, icazə problemi yoxdur. 10 MB-ı iki-üç dəfə göndərmək ucuzdur |
| Multipart axın kimi oxunur (`MultipartReader`), `ParseMultipartForm` istifadə edilmir; excelize-də `UnzipXMLSizeLimit = UnzipSizeLimit` | Hər ikisi böyük hissələri müvəqqəti fayla yazır. Belə heç bir bayt diskə düşmür |
| Format məzmuna görə təyin olunur (zip imzası = xlsx); köhnə `.xls` və şifrəli fayllar (OLE imzası), `.ods` aydın xəta ilə rədd edilir | Uzantıya etibar edilmir; dəstəklənməyən formatda "səssiz zibil" yoxdur |
| CSV: ayırıcı birinci boş olmayan sətirdən təyin olunur (`,` `;` tab, dırnaq içi nəzərə alınmır); UTF-8 (BOM ilə və ya BOM-suz). UTF-16 və digər kodlaşdırmalar **rədd edilir** ("CSV UTF-8 kimi saxlayın") | Windows-1251/1254-ü təxmin etmək Azərbaycan hərflərini səssizcə korlayır; aydın xəta daha təhlükəsizdir |
| xlsx: yalnız bir vərəq (default — birinci görünən, ya da seçilən); xanalar **saxlanan dəyər** kimi oxunur (`RawCellValue`), düsturlar **hesablanmır** (keşlənmiş nəticə götürülür) | Formatlama lokaldan asılıdır ("3/15/26"); xam dəyər deterministikdir. Düstur hesablamaq həm risk, həm də yavaşlıqdır |
| Birinci boş olmayan sətir başlıqdır; tam boş sətirlər atlanır, amma xəta hesabatında **fayldakı sətir nömrəsi** göstərilir | İstifadəçi Excel-də həmin sətri dərhal tapır |
| Uyğunlaşdırma serverdə başlıq adına görə **təklif olunur** (böyük/kiçik hərf, İ/ı/i, boşluq və simvollar nəzərə alınmır); əvvəl sahə açarı/adı və əsas adlar, sonra sinonimlər ("Ad", "Müştəri" → başlıq, "Qeyd" → təsvir, "Məsul" → icraçı) | Tipik vərəqlərdə uyğunlaşdırma demək olar ki, hazır gəlir; fərdi sahə adı sinonimdən üstündür |
| Hədəflər: başlıq (məcburi), təsvir, tip (açar və ya ad), status (**tipin workflow-unda ad ilə**), icraçı (**workspace üzvünün e-poçtu**), fərdi sahələr açarla. Uyğunlaşdırılmayan sütunlar nəzərə alınmır | ADR-0003 sahə tipləri ilə birbaşa uyğunluq |
| Rəqəm: boşluq/nöqtə/vergül/apostrof min ayırıcıları, `₼`, `AZN`, `manat`, `$`, `€` və s. silinir; ondalıq ayırıcı `,` və ya `.`. **Tək ayırıcı + düz 3 rəqəm** (`1.234`, `12,500`) qeyri-müəyyən sayılır və **xəta** verir. xlsx dəyərləri maşın formatındadır, birbaşa oxunur | Pul məbləğində 1.234 ilə 1234-ü qarışdırmaq səssiz və bahalı səhvdir; soruşmaq daha yaxşıdır |
| Tarix: `YYYY-MM-DD`, `DD.MM.YYYY`, `DD/MM/YYYY`, `DD-MM-YYYY` (gün əvvəl), RFC 3339; xlsx-də Excel seriya nömrələri (1900 və 1904 sistemi). Saat zonası olmayan tarix-saat UTC sayılır | Azərbaycanda gün-ay sırası istifadə olunur; seriya nömrəsi yalnız xlsx-də qəbul edilir (CSV-də "2024" tarix deyil) |
| Seçim: etiketə görə (böyük/kiçik hərf və İ/ı fərqi nəzərə alınmır) və ya seçim id-si ilə; arxivlənmiş seçimlər qəbul edilmir; iki etiketə uyğun gələn dəyər xətadır. Çoxlu seçim `,`/`;` ilə ayrılır. Boolean: bəli/xeyr, hə/yox, true/false, 1/0, ✓ | |
| Yazma yalnız **admin+**; önbaxış da admin+ | Bir sorğu ilə minlərlə tapşırıq yaratmaq admin səlahiyyətidir |
| Hər sətir `CreateItem` ilə **eyni** yoxlamadan keçir (`prepareItem`: başlıq/təsvir uzunluğu, tip, sahə tipləri, məcburi sahələr, workflow statusu, üzvlük). `CreateItem` da indi eyni funksiyanı çağırır | Qaydalar iki yerdə ayrılıb "uzaqlaşa" bilməz |
| **Hamısı və ya heç nə — bir tranzaksiya.** Əvvəl bütün sətirlər yoxlanılır (bütün xətalar toplanır), xəta varsa heç nə yazılmır. Daxiletmələr 500 sətirlik pgx batch-ləri ilə göndərilir | Yarımçıq idxal (məs. 3200-cü sətirdə kəsilmiş) əl ilə təmizlənməli olardı. Ölçülüb: 20 000 sətir ≈ 3 s, dry run ≈ 0,1 s (lokal Postgres) — bir tranzaksiya kifayətdir, partiyalara bölmək lazım deyil |
| Layihə sətri idxal boyu `FOR UPDATE` ilə kilidlənir; nömrə aralığı (məs. ACME-1…ACME-60) birdən ayrılır | Paralel yaradılan tapşırıqlarla nömrə toqquşması/boşluğu olmur |
| Hər tapşırıq `item.created` hadisəsini **tarixçə üçün** yazır (`source = 'import'`, dispatch işi **yoxdur**); idxalın özü üçün bir `project.imported` hadisəsi: fayl adı, format, vərəq, sətir sayı, uyğunlaşdırma, yaradılan say, ilk/son açar, icraçı. Yalnız bu hadisə webhook-lara gedir (migrasiya 00008) | Aktivlik və audit tam qalır, amma böyük idxal Mattermost kanalına minlərlə mesaj, icraçılara minlərlə email göndərmir. Webhook-lar "N tapşırıq idxal olundu" xülasəsini alır |
| Sütun "Son tarix" (`due_date`) hədəfinə uyğunlaşdırıla bilər ("Ödəniş tarixi", "Deadline" kimi başlıqlar avtomatik təklif olunur); tarix sahələri kimi oxunur | Keçmiş tarixli sətirlər idxaldan dərhal sonra "gecikir" olur və ADR-0009-dakı gecikmə qaydaları/xülasəsi onlara tətbiq olunur |
| `dry_run=true` eyni yolu tranzaksiya daxilində işlədir və geri qaytarır; real idxalda xəta olarsa **422** + eyni hesabat | Dry run ilə real idxalın nəticəsi arasında fərq yoxdur |
| Bütün idxal rate limit üçün **bir sorğu** sayılır (bir HTTP sorğusu) | |

## Limitlər

| Limit | Dəyər | Aşılanda |
|---|---|---|
| Fayl ölçüsü | 10 MiB (multipart gövdəsi 10 MiB + 256 KiB) | 413 `file_too_large` |
| Açılmış xlsx (bütün zip hissələri) | 64 MiB; zip hissələri ≤ 2000 | 413 `file_too_large` |
| Məlumat sətri | 20 000 | 413 `too_many_rows` |
| Sütun | 200 | 413 `too_many_columns` |
| Xana uzunluğu | 32 767 simvol (Excel-in öz limiti) | 413 `cell_too_long` |
| Oxuma vaxtı | 15 s (sətirlər arasında yoxlanılır); bütün sorğu `NEBULA_REQUEST_TIMEOUT` (30 s) | 400 `timeout` |
| Eyni anda idxal (proses başına) | 2; boş yer 5 s gözlənilir | 503 `import_busy` + `Retry-After` |
| Hesabatda xəta | ilk 500 (`error_count` hamısını sayır) | `truncated: true` |
| Mapping JSON | 64 KiB, naməlum açarlar rədd edilir | 400 / 413 |

Digər xətalar: `invalid_encoding`, `unsupported_format`, `malformed_file`, `empty_file`, `sheet_not_found` → 400;
uyğunlaşdırma xətaları (başlıq yoxdur, eyni hədəf iki dəfə, naməlum sahə/tip, mövcud olmayan sütun) → 422.

## Təhlükəsizlik

- **Zip bomb**: excelize açmazdan əvvəl zip mərkəzi kataloqundakı elan olunmuş ölçülər toplanır və limitlə
  müqayisə edilir. `archive/zip` elan olunandan çox açılan hissəni rədd edir, ona görə yalançı başlıq da
  limiti keçə bilməz; excelize-in öz `UnzipSizeLimit`-i ikinci qatdır.
- **Yaddaş**: bir idxal ən çox ~10 MiB fayl + 64 MiB açılmış məzmun saxlayır; paralel idxallar proses
  başına 2 ilə məhdudlaşır.
- **Disk**: heç nə yazılmır (yuxarıya bax). Fayl məzmunu loglanmır; logda yalnız say və format var.
- **Fayl adı**: yalnız əsas ad, idarəetmə simvolları silinir, 200 simvol; yalnız JSON-da və audit-də istifadə olunur.
- **Düsturlar** hesablanmır; xarici linklər, makrolar (`.xlsm`-də) icra edilmir — yalnız xana dəyərləri oxunur.
- **CSV injection** (`=HYPERLINK(...)`) Nebula-da təhlükə deyil (dəyərlər mətn kimi saxlanılır və UI-da
  mətn kimi göstərilir). Gələcəkdə Excel-ə **ixrac** əlavə olunanda `= + - @` ilə başlayan xanalar qaçırılmalıdır.
- Tenant izolyasiyası: marşrut workspace üzvlüyünü yoxlayır (üzv olmayan üçün 404), üzvlər və seçimlər
  yalnız həmin workspace-dən götürülür, tərkib FK-lar dəyişməyib.

## Nəticələr

- İdxal olunan sətirlər üçün `item.created` qaydaları **işləmir** (məs. "yaradılanda icraçıya bildiriş").
  Bu qəsdəndir: idxal toplu köçürmədir, hər sətir yeni iş deyil. Gecikmə qaydaları (`item.overdue`) isə
  idxal olunmuş tapşırıqlara da normal tətbiq olunur, çünki onlar sonradan, zamanlayıcıdan gəlir.
- Sətir xəta mesajları API-nin ümumi dili kimi ingiliscədir; UI fayl səviyyəli xəta kodlarını Azərbaycan dilinə çevirir.
- Yalnız yaratma: mövcud tapşırıqları yeniləmək (upsert, məs. müqavilə nömrəsinə görə) bu mərhələyə daxil deyil.
- Başlıq sətri həmişə birinci boş olmayan sətirdir; üstündə başlıq/loqo olan vərəqlər əvvəlcə təmizlənməlidir.
- Fayl iki-üç dəfə yüklənir (önbaxış, dry run, idxal). Böyük fayllarda bu trafikdir, amma vəziyyətsiz
  dizaynın sadəliyi və təhlükəsizliyi buna dəyər.
