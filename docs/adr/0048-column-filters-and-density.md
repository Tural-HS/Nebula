# ADR-0048: Sütun filtrləri (mətndə axtarış, aralıq) və cədvəl sıxlığı

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0003](0003-custom-fields.md) (sahələr, `?field.<key>=` filtrləri), [ADR-0014](0014-saved-views-and-table-totals.md)
  (görünüşlər, cəm sətri), [ADR-0047](0047-ledger-records.md) (reyestr cədvəli)

## Kontekst

İdxal olunmuş reyestrlərdə (yığım, müştəri aylıq hesabatı) komanda Excel/Boards kimi işləyir: "telefonu 5541 ilə
olanlar", "qalığı 900-dən çox olanlar", "krediti noyabrda verilənlər". Sahə filtrləri indiyə qədər yalnız **dəqiq
dəyər** (JSON `@>`, GIN indeksi) idi və UI-da yalnız seçim, bəli/xeyr və valyuta üçün göstərilirdi; tam mətn axtarışı
yalnız başlıq və təsviri əhatə edir. Bundan əlavə, eyni cədvəl müxtəlif ekran və gözlər üçün fərqli sıxlıq tələb edir.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Operatorlar dəyərin içində**, eyni `?field.<key>=` parametrində: `~mətn` — mətn tipli sahələrdə (`text`, `long_text`, `url`, `email`) "içində", böyük-kiçik hərf və Azərbaycan hərf variantları nəzərə alınmadan (`lower(search_fold(...))`), 1–100 simvol, `%`/`_` hərfi mənada; `min..max` — rəqəm və tarix sahələrində (formula nəticəsi daxil) daxil olan aralıq, bir tərəf boş ola bilər. Operatorsuz dəyər əvvəlki kimi dəqiq uyğunluqdur | Link, saxlanmış görünüş (`config.filters.fields`), ixrac, cəm və sayğaclar eyni xəritəni artıq daşıyır — yeni parametr və model lazım deyil; köhnə linklər dəyişmir | Ayrıca `?filter=` DSL: hər yerdə yeni parse və validasiya. Mətn üçün həmişə "içində": mövcud dəqiq filtrlərin mənası dəyişərdi |
| **SQL**: "içində" — `LIKE` (indekssiz), aralıq — `jsonb_typeof` yoxlaması ilə `::numeric` və ya ISO tarix mətni müqayisəsi; hamısı parametrlərlə, açar regex ilə yoxlanmış | Siyahı həmişə layihə və digər filtrlərlə məhdudlaşır; layihə başına minlərlə tapşırıqda skan millisaniyələrdir. Tipi yanlış saxlanmış dəyər cast-ı qırmır | `pg_trgm` GIN indeksi: miqrasiya, yazıda əlavə xərc; həcm buna hələ ehtiyac göstərmir — yavaşlıq görünsə növbəti addım |
| **UI: sütun başlığında menyu** (▾): sıralanan sahələrdə Artan/Azalan, filtr redaktoru sahə tipinə görə ("İçində", "Başlanğıc/Son", seçim siyahısı). Panel `<body>`-yə portal ilə, `fixed`; Esc və kənar klik bağlayır, fokus düyməyə qayıdır (cədvəl yenidən yüklənib remount olsa da). Aktiv filtr və ya sıralama düymədə rəng və ox ilə, adında "filtr var". Filtr çipləri operatoru sözlə göstərir (`Qalıq: ≥ 900`, `Telefon: içində "5541"`) | Excel/Boards vərdişi; filtr sütunun yanında, siyahının yuxarısında çip | Yalnız "Filtrlər" panelində: 15 sahəli reyestrdə uzun siyahı. Başlığın içində popover: başlıq sətri `overflow-clip`-dir və sağa sürüşür |
| **Sıxlıq: "Normal / Sıx"** (toolbar-da "Sıx" düyməsi, `aria-pressed`): Normal 28 px sətir, 13 px şrift; Sıx 24 px, 12 px. Seçim **brauzerdə** (`localStorage`), görünüşdə yox; saxlanmasa da (məxfi rejim) səhifədə işləyir. Toxunma ekranında sətirlər 44 px qalır (ADR-0019) | Sıxlıq oxucunun ekranından və gözündən asılıdır, siyahıdan yox; bir nəfər üçün sıx, başqası üçün yorucudur | Hamı üçün 12 px: uzun oxunuşda yorucu (kontrast və ölçü şikayətinin özü). Görünüşdə saxlamaq: paylaşılan görünüşdə başqasının seçimi hamıya keçərdi |

## Nəticələr

- Miqrasiya yoxdur. Köhnə image operatorlu dəyəri dəqiq uyğunluq kimi oxuyur (nəticə boş olur, xəta deyil — rəqəm və
  tarixdə 422), ona görə rollback-dən sonra operatorlu saxlanmış görünüşlər boş siyahı göstərə bilər.
- Testlər: inteqrasiya (içində, hərf qatlanması, `%`/`_`, aralıqlar, birləşmə, sayğaclar, validasiya, qonaq, görünüş
  validasiyası), vitest (dəyər qurma/parse, çip mətni), E2E (menyu ilə aralıq və mətn filtri, çiplər, fokus, təmizləmə,
  Esc, sıralama, sıxlığın yadda qalması).

## Əlavə (2026-11-26): sütun eni

Masaüstündə başlığın sağ kənarındakı tutacaqla sütunu dartıb genişləndirmək/daraltmaq olur (3–40 rem); klaviatura
ilə ←/→ (yarım rem), Home və ya iki klik standart enə qaytarır. Tutacaq `role="separator"` dəyərlə (ekran oxuyucu
eni deyir). En brauzerdə, layihə üzrə yadda qalır (sıxlıq kimi — ekrana bağlıdır, görünüşə yox); telefonda
cədvəl öz uyğun enlərini saxlayır.

## Açıq suallar

- "Boşdur / boş deyil" operatoru.
- Böyük layihələrdə "içində" üçün `pg_trgm` indeksi.
- Seçim sahələrində çoxlu seçim (bir sütunda "A və ya B").
