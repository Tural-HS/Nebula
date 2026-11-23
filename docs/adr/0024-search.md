# ADR-0024: Workspace üzrə axtarış

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

İndiyədək axtarış yalnız bir layihənin tapşırıq siyahısında idi (`?q=`, `items.search`, 00001) və şərhləri
əhatə etmirdi. İstifadəçilər "o ödəniş məsələsi harada idi?" sualına bütün layihələr üzrə, şərhlər daxil,
bir yerdən cavab istəyir; `QA-12` yazanda düz həmin tapşırığa getmək istəyirlər. Üç çətinlik var:

1. **Azərbaycan dili üçün Postgres lüğəti yoxdur.** İnsanlar tez-tez ASCII ilə yazır ("odenis"), mətn isə
   "ödəniş"dir (və ya əksinə); `simple` konfiqurasiyası bunları fərqli leksem sayır.
2. **Böyük həcm.** "Böyük məlumatda sürətli" əsas məhsul hədəfidir: yüz minlərlə tapşırıq və şərhdə də sorğu
   onlarla millisaniyə olmalı, miqrasiya isə canlı sistemi kilidləməməlidir.
3. **Məxfilik (ADR-0013).** Gizli layihənin tapşırığı, şərhi, sayı, snippet-i, hətta açarla tapılması belə
   heç bir formada sızmamalıdır.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| Ayrıca `search_docs` cədvəli (miqrasiya 00018): hər canlı tapşırıq (`id = item_id`) və hər canlı şərh (`id = comment_id`) üçün bir sətir — `kind`, `workspace_id`, `project_id`, `item_id`, `comment_id`, `doc tsvector`, `title_fold` | Bir GIN indeksi, bir sorğu, tapşırıq və şərhlər birlikdə sıralanır və keyset ilə səhifələnir |
| Sənədlər **trigger**-lərlə saxlanır: `items` (INSERT; title/description/deleted_at/project_id dəyişəndə, `WHEN … IS DISTINCT FROM`) və `comments` (INSERT; body/deleted_at). Silinmiş tapşırığın sənədləri (şərhləri daxil) silinir, bərpa olunanda qayıdır; layihə dəyişəndə şərhlərin `project_id`-si də yenilənir | Sənəd yalnız canlı məzmun üçün var: sorğu `items`-ə join etmədən layihə görünürlüyü ilə süzülür. Kod yolu yoxdur ki, unudulsun — idxal (ADR-0011), təkrarlanan tapşırıqlar, gələcək toplu əməliyyatlar avtomatik indekslənir |
| **Generated column deyil**, ayrıca cədvəl | `items`-ə `STORED` generated sütun əlavə etmək bütün cədvəli `ACCESS EXCLUSIVE` kilidi altında yenidən yazır (böyük cədvəldə dəqiqələrlə oxuma və yazma dayanır). Trigger ilə saxlanan sütun isə hər sətrin backfill `UPDATE`-ini tələb edir: `items_updated_at` trigger-i `updated_at`-ı dəyişərdi (sıralama və "son dəyişiklik" yalan olardı) və ən isti cədvəl şişərdi |
| Qatlama: `search_fold(text)` — `translate()` ilə ə→e, ı/İ/I→i, ö→o, ü→u, ğ→g, ş→s, ç→c, geniş yayılmış Latın aksentləri (á, é, ñ, …) və ayrıca diakritik işarələr (NFD: o + ̈) silinir; sonra `simple` konfiqurasiyası (kiçik hərf, stemming yox) | `unaccent` əvəzinə: `unaccent` `IMMUTABLE` deyil (indeksdə wrapper lazımdır), ə-ni bilmir və əlavə extension tələb edir. `translate()` daxili, `IMMUTABLE` və sürətlidir. Stemming yoxdur: Azərbaycan dili üçün etibarlı stemmer yoxdur, prefiks axtarışı ("ödən" → "ödənişdən") şəkilçiləri praktikada əhatə edir |
| Funksiyalar SQL-standart gövdəli (`RETURN …`), `IMMUTABLE PARALLEL SAFE`, adlar `pg_catalog.`-la | Gövdə yaradılanda bağlanır: `search_path` ilə dəyişdirilə bilməz; planner inline edir |
| Çəkilər: başlıq **A** (1.0), təsvir **B** (0.4), şərh **C** (0.2); `ts_rank(doc, q, 1)` (uzunluğun loqarifminə bölünür); bərabərlikdə `id` | Başlığında söz olan tapşırıq sözü şərhdə keçəndən yuxarıdadır; uzun mətn təkrarla qalib gəlmir; sıra deterministikdir (keyset üçün şərt) |
| Sorğu Go-da sözlərə bölünür (yalnız hərf/rəqəm qalır), qatlanır, ən çox **8 söz**, tək hərflər atılır (tək rəqəm qalır), hər söz prefiks kimi: `odenis:* & 12:*` | Bütün sözlər lazımdır (AND); tsquery sintaksisi (`!`, `|`, `:*`, mötərizə) istifadəçidən heç vaxt keçmir — escape lazım deyil, inyeksiya mümkün deyil. Tək hərfli prefiks indeksin böyük hissəsini gəzərdi |
| Tam açar (`^[A-Za-z][A-Za-z0-9]{1,9}-\d+$`, böyük-kiçik hərf fərqi yoxdur) ilk səhifədə **birinci** gəlir (`match: "key"`); həmin tapşırıq mətn nəticələrində təkrarlanmır, onun şərhləri isə qalır | `QA-12` yazan adam həmin tapşırığı axtarır |
| Mətn heç nə tapmasa (ilk səhifə, açar da yoxdursa) `pg_trgm` ilə başlıqlarda yazı xətası axtarışı: `search_fold(q) <% title_fold`, `word_similarity` ≥ 0.5, bir səhifə, `match: "fuzzy"` | "plenlama" → "Planlama". Yalnız ehtiyat yol: hər sorğuda işlətmək trigram skanının xərcini hər yazışa əlavə edərdi. İndeks `title_fold gin_trgm_ops WHERE kind = 'item'` |
| API: `GET /api/v1/workspaces/{ws}/search?q=&limit=&cursor=`. `q`: kəsilmiş, 1–200 simvol (rune), ən azı bir 2+ simvollu söz və ya açar, əks halda 422; `limit` 1–50 (default 20), səhv `limit`/`cursor`/təkrar `q` 400. Cavab: `hits`, `next_cursor`, `counts {items, comments}` (yalnız ilk səhifədə) | ADR-0002 qaydaları. Kursor `(rank float32 bitləri, id)` — base64url, dəqiq round-trip |
| Hər nəticə: `kind`, `match` (`key`/`text`/`fuzzy`), layihə açarı və adı, tapşırıq nömrəsi/açarı/başlığı, status, `title` və `snippet` **seqment** kimi: `[{text, match}]`; şərhlərdə müəllif və vaxt | **API HTML qaytarmır.** "Escape edib sonra `<mark>` əlavə etmək" əvəzinə daha sərt yol: mətn yalnız düz mətn seqmentləridir, React onu mətn düyünü kimi göstərir, `dangerouslySetInnerHTML` yoxdur — XSS üçün səth qalmır. Vurğulama Go-da `search_fold`-un güzgüsü ilə (testlə sinxron) original sözlərin üzərində edilir |
| Snippet ~180 simvol, ilk uyğun sözün ətrafında, söz sərhədində kəsilir (`…`); təsvirdə uyğunluq yoxdursa snippet yoxdur. Şərhlərdə `<@uuid>` mention-ları **yalnız workspace üzvlərinin** adı ilə `@Ad` olur, tanınmayan `@?` | Xam id göstərilmir; mention başqa workspace-dəki adamın varlığını açmır |
| **Məxfilik:** əvvəlcə görünən layihələr hesablanır (`visibleToViewer`, ADR-0013-ün `canSeeProject` predikatı ilə eyni) və **hər** sorğu (mətn, açar, trigram, saylar) `project_id = ANY(görünənlər)` ilə məhdudlaşır; nəticələr göstərilməzdən əvvəl `items`-in özündən (canlı + görünən layihədə) yenidən yoxlanılır | Gizli layihə sorğuya heç vaxt daxil olmur — saylar, snippet-lər, açar və yazı xətası yolları da. Köhnəlmiş sənəd (yarış, gələcək xəta) belə heç nə aça bilmir |
| Saylar eyni skanda window aggregate ilə (`count(*) FILTER … OVER ()`), yalnız ilk səhifədə | Əlavə sorğu yoxdur; sonrakı səhifələrdə saylar qalan hissəni sayardı |
| Sorğu read-only transaksiyada, `statement_timeout = 3s` (`SET LOCAL`) | Patoloji sorğu bağlantını tutmur, 503 qaytarır |
| Rate limit: ümumi istifadəçi limitinə əlavə olaraq axtarışa ayrıca **3 sorğu/s, burst 15** (istifadəçi başına); UI 250 ms debounce edir | Axtarış adi oxumadan bahadır (GIN skanı, sıralama, snippet) və yazdıqca göndərilir. 429 + `Retry-After` (UI-da mövcud təkrar siyasəti) |
| Veb: başlıqda "Axtar" düyməsi (telefonda ikon, 44 px) və **Cmd/Ctrl+K** (yalnız workspace daxilində); `<dialog>` içində combobox (`role="combobox"`, `aria-activedescendant`, `listbox` + `group`-lar: "Tapşırıqlar", "Şərhlər" sayları ilə); ↑/↓ seçir, Enter açır, Esc bağlayır və fokusu düyməyə qaytarır; nəticə tapşırıq panelini açır (`/w/{ws}/p/{açar}?item={n}`); "Daha çox nəticə" kursorla; telefonda tam ekran, safe-area (ADR-0019) | Klaviatura və ekran oxuyucusu ilə tam istifadə; nəticə sayı `aria-live` ilə elan olunur |

## Miqrasiya 00018: kilidlər

Miqrasiya `NO TRANSACTION` işləyir və `items`/`comments`-i yenidən yazmır:

| Addım | Kilid | Təsir |
|---|---|---|
| `CREATE EXTENSION pg_trgm`, funksiyalar, boş `search_docs` və indeksləri | yalnız yeni obyektlər | yoxdur |
| 4 trigger (`items`, `comments`) | `SHARE ROW EXCLUSIVE`, millisaniyələr; `lock_timeout = 5s` | bu müddətdə həmin cədvəllərə yazma gözləyir; uzun transaksiya varsa miqrasiya 5 s sonra xəta verir (növbədə qalıb hamını bloklamır) — təkrar işə salmaq təhlükəsizdir |
| Backfill: `id` üzrə keyset, 2000 sətirlik partiyalar, hər partiya ayrıca `COMMIT` | partiyadakı sətirlərə `FOR SHARE` | yazmalar davam edir; yalnız həmin anda partiyada olan sətrin yeniləməsi o partiyanın sonunu gözləyir (onlarla ms). Trigger-lər backfill-dən **əvvəl** yaradılır, `ON CONFLICT DO NOTHING` trigger-in yazdığı (daha təzə) sənədi saxlayır; yarımçıq qalsa təkrar işə salmaq davam etdirir |

`items.search` və `items_search_idx` (00001) əvvəlki release üçün saxlanılır (expand/contract); layihə
siyahısındakı `?q=` hələ də onu işlədir. Növbəti miqrasiya həmin sütunu silə və siyahı axtarışını da
`search_docs`-a keçirə bilər.

## Benchmark

`internal/store/search_bench_integration_test.go` (opt-in: `NEBULA_SEARCH_BENCH=1`) atılan ayrıca test
bazasında 100 000 tapşırıq (2%-i silinmiş) və 200 000 şərh yaradır (20 layihə, 2-si gizli), 00018-in
backfill-ini ölçür və gizli layihələri görməyən üzv kimi sorğuları 35 dəfə (5 isinmədən sonra) işlədir.
Lüğət 40 sözdən ibarət, Zipf-ə bənzər paylanır, yəni ən çox yayılmış sözlər real həyatdakından **daha**
tez-tez rast gəlinir (pis hal). Yerli Docker (Apple Silicon), PostgreSQL 19, default ayarlar:

| Sorğu | Uyğunluq (tapşırıq / şərh) | p50 | p95 |
|---|---|---|---|
| nadir söz `zümrüd` | 100 / 0 | 3.3 ms | 4.8 ms |
| tam açar `P7-120` | 1 / 0 | 1.9 ms | 3.5 ms |
| yazı xətası `zumrut` (trigram) | 20 / 0 | 3.8 ms | 5.6 ms |
| uyğunluq yoxdur `qwertyuiop` (+ trigram) | 0 / 0 | 2.1 ms | 3.5 ms |
| prefiks `kom` | 38 694 / 42 480 | 87 ms | 130 ms |
| 2 hərfli prefiks `ba` | 61 217 / 77 198 | 129 ms | 170 ms |
| iki söz `müştəri kredit` | 64 400 / 65 850 | 144 ms | 165 ms |
| çox yayılmış söz `ödəniş` | 89 971 / 170 406 | 193 ms | 258 ms |
| eyni söz ASCII ilə `odenis` | 89 971 / 170 406 | 195 ms | 229 ms |
| `ödəniş`-in növbəti 10 səhifəsi | | 230 ms | 355 ms |
| **bütün sorğular** | | **87 ms** | **210 ms** |

- İki ardıcıl qaçışda ümumi p95 186 ms və 210 ms oldu (yerli maşında səs-küy ±20%).
- Backfill: 295 000 sənəd **22–26 s**, `search_docs` indekslərlə **169 MB**.
- Yazma xərci (trigger-lər): ardıcıl `CreateItem` trigger-siz 2.43 ms, trigger-lə 2.65 ms (**+0.22 ms**, ~9%).
- Seçici sorğular (real axtarışların əksəriyyəti) GIN sayəsində 5 ms-dən azdır. Xərc **uyğunluq sayı** ilə
  xətti artır: bütün uyğunluqlar sıralanmalı (və ilk səhifədə sayılmalı) olur. Sənədlərin ~90%-ində olan söz
  100k-da ~0.2–0.35 s-dir. Sonrakı səhifələr saymır (bu ölçüdən sonra əlavə edildi), amma sıralama yenə hamısı üzərindədir.

## Nəticələr

- Bir workspace-də milyonlarla sənəd və çox yayılmış söz olanda sorğu saniyəyə yaxınlaşa bilər; `statement_timeout`
  (3 s) yuxarı həddir. O həddə çatmadan əvvəl seçimlər: (a) namizədləri ~10 000-də kəsmək (saylar "10 000+"),
  (b) `rum` indeksi (indeksdən sıra ilə oxuma; rəsmi imicdə yoxdur), (c) `workspace_id` üzrə partition.
  Hər biri ya dəqiqliyi, ya əməliyyat sadəliyini azaldır — indiki həcmdə lazım deyil.
- GIN bütün workspace-lər üçün ortaqdır; `workspace_id` + `project_id` süzgəci indeksdən sonra tətbiq olunur.
  Çox tenantlı böyük quraşdırmada başqa tenantın yayılmış sözləri də skan olunur (yenə 3 s həddi var).
- `search_fold` dəyişərsə, saxlanmış leksemlər köhnə qalır: dəyişiklik edən miqrasiya `search_docs`-u partiyalarla
  yenidən qurmalıdır (`UPDATE … SET doc = search_item_doc(…)`) və Go güzgüsü (`internal/store/searchfold.go`)
  eyni dəyişikliyi almalıdır; `TestSearchFoldMatchesSQL` uyğunsuzluğu tutur.
- Stemming yoxdur: "ödənişlər" "ödəniş" prefiksi ilə tapılır, amma "ödəmə" yox. Sinonimlər yoxdur.
- Yazı xətası axtarışı yalnız tapşırıq başlıqlarındadır və yalnız mətn heç nə tapmayanda işləyir.
- Fərdi sahələr, əlavələr və aktivlik indekslənmir; lazım olarsa `search_docs`-a yeni `kind` kimi əlavə oluna bilər.
- Trigger-lər tapşırıq/şərh yazmasına bir `INSERT … ON CONFLICT` əlavə edir (~0.2 ms);
  `search_docs` silmələri `item_id`/`comment_id` indeksləri ilə FK cascade olunur.
- Layihə səviyyəsində `?q=` hələ qatlamır ("odenis" "ödəniş"i tapmır) — növbəti miqrasiyada `search_docs`-a keçəcək.
