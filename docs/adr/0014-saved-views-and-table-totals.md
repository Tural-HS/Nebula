# ADR-0014: Saxlanılan görünüşlər, cədvəl sütunları və cəm sətri

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

FinTez Mattermost Boards-dan köçür. Orada adlı, filtrlənmiş görünüşlər ("Plan 2027", "Gecikənlər"), əlavə
sütunlar (aylıq ödəniş, qalıq) və aylıq ödəniş sütununun **cəmi** gündəlik iş alətidir. Nebula-da filtrlər
yalnız URL-də idi, cədvəlin sütunları sabit idi, cəm yox idi. Bundan əlavə JS paketi 570 kB-a çatmışdı
(Vite xəbərdarlığı 500 kB).

## Qərarlar

| Qərar | Səbəb |
|---|---|
| Görünüş layihəyə bağlı `saved_views` sətridir: ad, növ (`table` / `board`), `config` (filtrlər, sütunlar, sıralama, lövhə qruplaşdırması), sahib, `shared` | Bir cədvəl, sadə model; `config` JSONB-dir, amma API onu tam tipli struktura (`store.ViewConfig`) çevirir və **naməlum açarları rədd edir** (400) |
| Filtrlər `GET …/items`-in eyni anlayışlarıdır: `q`, `assignee` (`me` baxana görə), `due`, status id-ləri, `field.<key>` | Görünüş yeni sorğu dili deyil, mövcud filtrlərin adlı dəstidir; "Mənim işlərim" ümumi görünüşü hər kəsə öz işlərini göstərir |
| Hər şey serverdə yoxlanılır: ad 1–100 simvol, sütun açarları (daxili sütunlar və ya **aktiv** fərdi sahələr), ən çox 30 sütun, statuslar workspace-ə aid, icraçı üzv, sahə filtrləri tipinə görə | UI yoxlaması yalnız rahatlıq üçündür |
| Görünürlük: **şəxsi** (yalnız sahibi görür; admin də görmür) və ya **ümumi** (layihəni görən hər kəs) | Şəxsi filtrlər məxfi qalır; başqasının şəxsi görünüşü 404-dür, mövcudluğu da açılmır |
| Layihəni oxuya bilən hər kəs (qonaq da) şəxsi görünüş yarada bilər; ümumi görünüş yaratmaq və ya şəxsini paylaşmaq **admin+**; ümumi görünüşü dəyişmək/silmək admin+ və ya onun sahibi | Paylaşılan konfiqurasiya admin işidir; sahib sonradan admin olmasa da öz görünüşünə nəzarəti saxlayır |
| Ümumi görünüş dəyişiklikləri `view.created` / `view.updated` / `view.deleted` hadisələri kimi audit jurnalına düşür; şəxsilər yox | Hamını təsir edən dəyişiklik izlənir; şəxsi kliklər jurnalı doldurmur |
| İstifadəçi və layihə başına ən çox 100 görünüş (advisory lock ilə paralel sorğularda da) | Səhv və ya zərərli klientin sonsuz sətir yaratmasının qarşısı |
| Tenant izolyasiyası: `(workspace_id, project_id)` kompozit FK, hər sorğuda `workspace_id` + `project_id` | Digər cədvəllərlə eyni model; başqa workspace-in layihəsinə görünüş bağlana bilmir |
| Yeni sıralama: `sort=due` → `due_date ASC NULLS LAST, number ASC`; default `number DESC` qalır | Hər iki sıra tamdır (nömrə bərabərliyi pozur), ona görə keyset pagination nə təkrarlayır, nə buraxır; yeni tapşırıqlar həmişə böyük nömrə alır |
| Kursor sıralamanı daşıyır: `v1:<n>` (köhnə format dəyişmir) və `d1:<tarix və ya boş>:<n>`; başqa sıralamanın kursoru 400 | Kursoru başqa sorğuya "yapışdırmaq" səssiz səhv nəticə vermir |
| İndeks `items (project_id, due_date, number) WHERE deleted_at IS NULL`, `CREATE INDEX CONCURRENTLY` ilə, transaksiyasız idempotent miqrasiyada (00011) | Böyük cədvəldə yazmaları bloklamadan; yarımçıq qalan miqrasiya təkrar işə salına bilər |
| Cəm: `GET …/items/sums?fields=a,b&<eyni filtrlər>` → hər sahə üçün `sum` (dəqiq onluq, mətn) və `count`; `itemFilter` təkrar istifadə olunur | Cəm yüklənmiş səhifənin deyil, **bütün filtrlənmiş nəticənin** cəmidir |
| Yalnız `number` tipli sahələr cəmlənir (sorğu başına ən çox 10); `jsonb_typeof = 'number'` olmayan dəyərlər nəzərə alınmır, `numeric` ilə toplanır | Köhnə/əl ilə yazılmış pis data sorğunu sındırmır; `float64` yuvarlaqlaşdırması yoxdur (1e15 + 0.1 dəqiq qalır) |
| Veb: səhifə vəziyyəti (filtrlər, sütunlar, sıralama, qruplaşdırma, cədvəl/lövhə) **URL-dədir**; görünüş onun adlı ilkin dəstidir, `?saved=<id>` hansı görünüşdən gəldiyini göstərir | Hər vəziyyət paylaşıla bilən keçiddir (görünüşü görməyən adam da eyni filtrləri alır); dəyişiklik "dəyişdirilib" kimi görünür və "Dəyişiklikləri saxla" ilə yazılır |
| `?view=` cədvəl/lövhə üçün qalır (`?view=board`), görünüş id-si `?saved=`-dədir | Köhnə keçidlər və E2E testlər sınmır |
| Cədvəl: daxili sütunlar + seçilmiş fərdi sahələr, tipinə görə formatlanır ("20 noy 2026", seçim adları, istifadəçi adları, "1 234,5"); virtual qalır; başlıq, ilk sütun və cəm sətri yapışqandır, dar ekranda üfüqi sürüşmə | 100 min sətirdə də sürətli; telefonda da istifadə oluna bilir |
| Lövhə status filtrini sütunlara tətbiq edir (seçilməyən statusların sütunu boş, sayı 0) | Ümumi "Plan 2027" lövhə görünüşü yalnız lazımi statusları göstərir |
| Kod bölünməsi: ayarlar səhifələri, idxal, təkrarlanan tapşırıqlar, lövhə (dnd-kit) və tapşırıq paneli `React.lazy` + `Suspense` ilə; React/router/TanStack/OIDC ayrıca `vendor` chunk-ındadır | Əsas chunk 570.56 kB → 69.41 kB (+ vendor 395.99 kB); heç bir chunk 500 kB-dan böyük deyil, buraxılışda vendor keşdə qalır |

## Nəticələr

- `sort=due` ilə səhifələmə zamanı tapşırığın son tarixi dəyişərsə, o tapşırıq bir dəfə buraxıla və ya iki dəfə
  görünə bilər (hər keyset pagination-un xüsusiyyəti); yeni yaradılan tapşırıqlar bu problemi yaratmır.
- Görünüşdə sonradan arxivlənmiş və ya silinmiş sahə sütunu qalarsa, UI onu göstərmir; yeni sütun kimi əlavə
  etmək isə mümkün deyil.
- Görünüşlərdə optimistik kilid yoxdur: eyni ümumi görünüşü iki admin eyni anda dəyişərsə, sonuncu qalır.
- Layihə səviyyəsində məxfilik (ADR-0013) gəldikdə "layihəni görən hər kəs" avtomatik o qaydaya tabe olur,
  çünki bütün marşrutlar `loadProject`-dən keçir.
- Cəm sorğusu filtrlənmiş bütün sətirləri skan edir (indeks yalnız filtrlərə kömək edir); çox böyük layihələrdə
  lazım olarsa, cəmlər üçün qısa müddətli keş əlavə oluna bilər.
