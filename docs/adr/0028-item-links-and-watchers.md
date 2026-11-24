# ADR-0028: Tapşırıq əlaqələri və izləyicilər

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Maliyyə və back-office komandaları üçün iş nadir hallarda tək olur: "ay sonu bağlanışı" bank üzləşməsi bitməyincə
başlaya bilməz, eyni ödəniş xətası iki dəfə qeydə alınır, audit sorğusu başqa şöbənin tapşırığına bağlıdır. Jira və
YouTrack-da bunun üçün "issue links" və "watchers" var; istifadəçilər hər izləyicidə bunları gözləyir. İndiyə qədər
Nebula-da yalnız alt tapşırıqlar (ADR-0021: bir səviyyə, eyni layihə) var idi, bildirişlər isə yalnız icraçıya,
müəllifə və qeyd olunanlara gedirdi (ADR-0017).

Əlaqə alt tapşırıqdan fərqli anlayışdır: iyerarxiya deyil, iki bərabər tapşırıq arasında münasibətdir, layihələr
arası ola bilər və bir tapşırığın çoxlu əlaqəsi ola bilər. Ona görə ayrıca cədvəldir, `parent_id`-yə toxunulmur.

## Qərarlar — əlaqələr

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Sabit növlər: `blocks` / `blocked_by`, `relates`, `duplicates` / `duplicated_by` | Maliyyə proseslərində lazım olan üç münasibət; UI və bildirişlər sadə qalır | Workspace-in öz növləri (YouTrack kimi): hələ tələb yoxdur, sxem (`type text` + CHECK) sonra cədvələ çevrilə bilər. **İndi lazım deyil** |
| `item_links (source_id, target_id, type)` — hər əlaqə **bir dəfə**, "aktiv" tərəfdən saxlanılır; tərs ad (`blocked_by`) yalnız API-dədir | Bir sətir = bir əlaqə: iki tərəfin sinxron saxlanması problemi yoxdur | Hər tərəf üçün ayrı sətir: silmə/yaratma iki yazı, uyğunsuzluq riski |
| Unikal indeks `(type, LEAST(source, target), GREATEST(source, target))`; `CHECK (source_id <> target_id)` | Eyni cüt və növ hər iki istiqamətdə bir dəfə: "A B-ni bloklayır" varsa "B A-nı bloklayır" yaradıla bilmir (iki addımlıq dövr də yox olur). Qayda DB-dədir, API-dən yan keçən yazı da poza bilməz | Yalnız tətbiqdə yoxlama: yarışda dublikat yarana bilərdi |
| Layihələr arası, amma **workspace daxilində**: kompozit FK `(workspace_id, source_id/target_id) → items (workspace_id, id)` | Tenant izolyasiyası DB səviyyəsində (ADR-0001); test bunu xam SQL ilə də yoxlayır | — |
| Hədəf açarla verilir (`"target": "OPS-12"`), workspace daxilində axtarılır | Layihələrarası olduğu üçün nömrə kifayət deyil; UI hədəfi mövcud axtarış API-si ilə (ADR-0024) tapır | — |
| **Gizlilik**: əlaqə yalnız **hər iki** tapşırığı görə bilənə göstərilir. Siyahı, "bloklanıb" nişanı, `?blocked=true` filtri, aktivlik lenti eyni predikatdan (`visibleProjectFor`, `canSeeProject` ilə eyni) keçir | Gizli layihənin (ADR-0013) tapşırığı nə başlıqla, nə sayla, nə də bir bitlik "bloklanıb" bayrağı ilə sızmır | Gizli tərəfi "gizli tapşırıq" kimi göstərmək: varlığı sızdırır |
| Yazma: mənbə tapşırıqda redaktə hüququ (`member` + layihəni görmək) və hədəfi görmək. Görünməyən hədəf mövcud olmayanla **eyni** 422 cavabını alır | Hədəf açarlarını yoxlayaraq gizli layihələri kəşf etmək mümkün deyil | 403 / fərqli mesaj: varlığı açır |
| Silmə: əlaqənin istənilən tərəfindən, `member`; digər tərəfi görünməyən əlaqə 404 | — | — |
| Tapşırıq silinəndə (soft delete) əlaqələri **silinir**; sağ qalan tərəfin tarixçəsinə `link.removed` (`reason: item_deleted`). Fiziki silmədə FK CASCADE | Silinmiş tapşırığa əlaqə qalmır; tək və toplu silmə eyni koddan keçir | Əlaqəni saxlamaq: hər oxumada silinmişləri süzmək lazım olardı, bərpa funksiyası da yoxdur |
| Hadisələr: hər əlavə/silmədə **hər iki** tapşırığa `link.added` / `link.removed`; payload-da həmin tərəfdən növ, `link_id`, `other_item_id`, `source` | Hər iki tapşırığın tarixçəsi, webhook və avtomatlaşdırma mövcud mexanizmlə görür | — |
| Payload-da digər tapşırığın açarı və başlığı **yoxdur**; aktivlik lenti onları oxuma anında, baxan şəxs üçün (`data.other_item`) əlavə edir | Hadisə webhook-lara və gizli tərəfi görməyənlərə də çatır; açar (`SEC-3`) gizli layihənin varlığını açardı | Açarı payload-a yazmaq: sızma |
| Limit: tapşırıq başına **100 əlaqə** (hər iki tərəf, görünməyənlər daxil); hər iki tapşırıq `FOR UPDATE` ilə (sabit sırada) kilidlənir | Limit yarışda da keçilmir; silmə ilə yarış əlaqəni silinmiş tapşırığa bağlaya bilmir; əks istiqamətli iki əlaqə deadlock yaratmır | — |
| "Bloklanıb": açıq (`todo` / `in_progress`), canlı, görünən bir tapşırıq onu `blocks` edirsə. Siyahı cavabında `blocked` sahəsi səhifə üçün **bir** toplu sorğu ilə doldurulur; `GET item`-də yoxdur | N+1 yoxdur; panel bloklayanları onsuz da "Əlaqələr" bölməsində göstərir | `itemSelect`-ə subquery: baxanı bilmir, gizliliyi poza bilərdi |
| `?blocked=true` siyahı və `/items/counts` filtri (EXISTS, indeks `item_links_target_idx`) | Ucuzdur, eyni predikatdır | — |

## Qərarlar — izləyicilər

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `item_watchers (workspace_id, item_id, user_id)`; FK `(workspace_id, user_id) → workspace_members` CASCADE | Workspace-dən çıxan adam heç nəyi izləmir | — |
| Avtomatik izləmə: yaradan (reporter), təyin olunan icraçı, şərh yazan, şərhdə qeyd olunan (yalnız layihəni görənlər, ADR-0013) | Jira/YouTrack davranışı; "mənimlə əlaqəli işlər" əlavə klik olmadan izlənir | — |
| Avtomatik izləmə ayrıca hadisə yazmır; əl ilə izləmə/çıxarma `watcher.added` / `watcher.removed` yazır (audit jurnalında var, tapşırığın aktivlik lentində göstərilmir) | Səbəb hadisəsi (yaradılma, təyin, şərh) onsuz da qeydə alınır; lent səs-küysüz qalır | Hər avtomatik əlavəyə hadisə: lent ikiqat böyüyərdi |
| İzləmək üçün layihəni görmək lazımdır; **girişi itirən** (gizli layihədən çıxarılan) izləyicinin sətri qalır, amma siyahıda görünmür və bildiriş almır (hər bildirişdə və siyahıda yenidən yoxlanır) | Giriş bərpa olunanda izləmə özü qayıdır; girişin itirilməsi heç nə sızdırmır | Sətri silmək: layihə üzvlüyü dəyişikliyinə əlavə yan təsir, bərpa olunmur |
| Hər kəs (qonaq da) **özünü** izləyə bilər; başqasını əlavə etmək/çıxarmaq yalnız admin | İzləmə tapşırığı dəyişmir, oxumaq hüququ kifayətdir | — |
| Limit: tapşırıq başına **100 izləyici**; əl ilə əlavədə dəqiq (tapşırıq sətri kilidlənir), avtomatik əlavədə "ən çox"(eyni anda bir neçəsi keçə bilər) | Bir hadisənin fan-out-u məhdud qalır | — |
| Bildiriş növləri (ADR-0017-yə əlavə): `status_changed`, `assignee_changed`, `due_changed`, `link_added`; şərhdə izləyicilər mövcud `comment` növünü alır | Mövcud növlər, sayğac, saxlama müddəti, oxunmuş/oxunmamış məntiqi dəyişmir | — |
| Bir hadisədən bir nəfərə **bir** bildiriş, prioritet: `mention` → `assigned` → `comment`/`status_changed` → `assignee_changed` → `due_changed`. Yeni icraçı həm izləyicidirsə yalnız `assigned` alır. Müəllif öz dəyişikliyinə heç vaxt bildiriş almır | Dublikat yoxdur; `UNIQUE (event_id, recipient_id)` son müdafiə xəttidir | — |
| `link_added`: alıcı hər iki tapşırığı görməlidir; hər iki tapşırığı izləyən adam bir bildiriş alır (mənbə tərəfdən) | Gizli tərəf haqqında heç nə getmir; bir əlaqə = bir bildiriş | — |
| Bütün alıcılar **bir** `INSERT … SELECT FROM unnest(...)` ilə yazılır; aktiv üzvlük və layihə görünürlüyü SQL-də yoxlanır | Əvvəl alıcı başına bir giriş sorğusu idi; 100 izləyici ilə bu, 100 sorğu olardı | — |
| **Toplu redaktə (ADR-0023) izləyicilərə bildiriş vermir** (`bulk` payload-u olan hadisələr); yeni icraçıya `assigned` əvvəlki kimi gedir. **İdxal** (ADR-0011) heç bildiriş vermir və avtomatik izləyici yazmır | 500 tapşırıqlıq toplu status dəyişikliyi izləyici başına 500 bildiriş olmamalıdır | Toplu dəyişikliyi bir bildirişdə yığmaq: yeni növ və UI lazımdır, sonrakı iş |
| Rate limit: əlaqə və izləyici yazmaları üçün istifadəçi başına ayrıca 2/s, burst 20 (ümumi limitə əlavə) | Hər yazı iki tapşırığa hadisə, webhook və 100-ə qədər bildiriş yaradır | — |

## API (ADR-0002)

Hamısı `/api/v1/workspaces/{ws}/projects/{p}/items/{n}` altında; layihəni görməyən üçün 404.

| Endpoint | Rol | Cavab |
|---|---|---|
| `GET …/links` | guest+ | `{links: [{id, type, item: {key, project_key, number, title, status}, created_at}]}` |
| `POST …/links` `{type, target}` | member+ | 201 əlaqə; 409 `link_exists`; 422 `type` / `target` |
| `DELETE …/links/{linkID}` | member+ | 204; 404 |
| `GET …/watchers` | guest+ | `{watching, count, watchers: [{id, display_name}]}` |
| `PUT` / `DELETE …/watchers/me` | guest+ | 200 yeni siyahı |
| `PUT` / `DELETE …/watchers/{userID}` | admin+ | 200; 422 `user_id` (üzv deyil / layihəni görmür / limit) |

Siyahıda yeni: `blocked` (bool) hər sətirdə, `?blocked=true` filtri.

## UI

- Tapşırıq panelində "Əlaqələr" bölməsi: növə görə qruplar, açar, başlıq, status; bitmiş/ləğv edilmiş tapşırıq
  üstündən xətlə və solğun (ekran oxuyucusu üçün "(bağlıdır)"). "Əlaqə əlavə et": növ seçimi + açar/başlıq axtarışı
  (debounce 250 ms), artıq eyni növlə bağlı olanlar və tapşırığın özü siyahıda yoxdur. Hər sətirdə silmə düyməsi.
- Panelin başlığında göz düyməsi (izlə / izləmə, `aria-pressed`) və say; sayın üstünə klik izləyicilər siyahısını
  (avatarlar) açır, admin burada üzv əlavə edə və çıxara bilər.
- Cədvəldə və lövhədə kiçik "⛔ Bloklanıb" nişanı.
- Aktivlik lentində "əlaqə əlavə etdi: bloklanır OPS-4".
- Bütün sətirlər `locales/<dil>/links.ts`-dədir (`i18n-links.ts`), bildiriş mətnləri `notifications.ts`-də.

## Miqrasiya 00021

İki yeni cədvəl və `notifications_kind_check` yenidən yaradılır (dörd yeni növ). Yeni cədvəllər boşdur, böyük
cədvəllərdə uzun kilid yoxdur (`notifications` constraint-inin yoxlanması bir dəfəlik skandır).

Down: yeni növlü bildirişlər silinir, constraint köhnə siyahıya qayıdır, **bütün əlaqələr və izləyicilər itir**.
Köhnə image yeni sxemlə işləyir (cədvəlləri tanımır), amma köhnə kod tapşırıq silərkən əlaqələri silmir; yeni
versiyaya qayıdanda belə əlaqələr siyahıda görünmür (silinmiş tərəf süzülür), sətir isə tapşırıq fiziki silinənə qədər qalır.

## Nəticələr

- Əlaqə siyahısında səhifələmə yoxdur (ən çox 100).
- `blocked` yalnız siyahı cavablarındadır; tək tapşırıq cavabında yoxdur (`omitempty`).
- Webhook-lar `link.*` hadisələrində yalnız `other_item_id` alır; açar lazım olsa, inteqrasiya API ilə oxumalıdır.
- Workspace-in öz əlaqə növləri, "bloklanan tapşırığı bağlamaq olmaz" qaydası (workflow şərti), əlaqə qrafikinin
  vizuallaşdırılması və toplu dəyişikliklərin xülasə bildirişi gələcək işdir.
