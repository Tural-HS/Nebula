# ADR-0021: Alt tapşırıqlar

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Böyük işi (buraxılış, auditə hazırlıq, işə qəbul prosesi) kiçik addımlara bölmək və irəliləyişi bir baxışda görmək
lazımdır. Jira-da "sub-task", YouTrack-da "subtask of" bunun üçündür. Nebula-da indiyə qədər yalnız düz siyahı var
idi: addımlar ayrıca tapşırıq kimi yaradılır, aralarındakı əlaqə yalnız başlıqda yazılırdı.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| `items.parent_id` (nullable) — tapşırığın əsas (parent) tapşırığı | Ayrıca əlaqə cədvəli yox: bir tapşırığın ən çox bir əsas tapşırığı olur, sütun kifayətdir və sorğular sadədir |
| Əsas tapşırıq **eyni workspace və eyni layihədə** olmalıdır: kompozit FK `(workspace_id, project_id, parent_id) → items (workspace_id, project_id, id)` | Tenant izolyasiyası DB səviyyəsində qorunur (ADR-0001); layihələrarası əlaqə gizli layihənin (ADR-0013) başlığını başqa layihəyə sızdıra bilərdi. Fərqli layihələr arasında əlaqə lazım olsa, ayrıca "əlaqəli tapşırıqlar" xüsusiyyəti olacaq |
| **Yalnız bir səviyyə**: alt tapşırığın öz alt tapşırığı ola bilməz; alt tapşırıqları olan tapşırıq başqasının alt tapşırığı ola bilməz | Bax: "Niyə bir səviyyə" |
| Qaydalar DB-də: `items_check_parent` trigger-i (dərinlik, silinmiş əsas tapşırıq) və `CHECK (parent_id <> id)` | API-dən yan keçən yazı (skript, gələcək import, avtomatlaşdırma) da qaydanı poza bilməz. Bir səviyyədə dövr (A → B → A) yalnız "özü özünün əsası" ola bilər, onu CHECK bağlayır |
| Trigger əsas tapşırığın sətrini `FOR SHARE` ilə kilidləyir | İki eyni anda dəyişiklik ("X → Y altına" və "Z → X altına") ayrılıqda keçərli, birlikdə ikinci səviyyə yaradır. Kilid onları ardıcıl edir: ikincisi birincinin nəticəsini görür və rədd olunur (READ COMMITTED-də plpgsql hər sorğuda yeni snapshot alır). Test: `TestSubtasksConcurrentDepth` |
| Store eyni yoxlamaları əvvəlcədən edir və `422 validation_failed, field: parent` qaytarır; trigger xətası da eyni cavaba çevrilir | İstifadəçi aydın mesaj alır; trigger yalnız son müdafiə xəttidir |
| Əsas tapşırıq silinəndə (soft delete) alt tapşırıqlar **ayrılır** (yuxarı səviyyəyə qalxır), hər biri üçün `item.updated` hadisəsi (`changes.parent: {from: "OPS-1", to: null}`) | Alt tapşırıqları da silmək gözlənilməz məlumat itkisi olardı (silmə geri qaytarılmır). Ayırma tarixçədə görünür və avtomatlaşdırmalar onu görür |
| FK `ON DELETE SET NULL (parent_id)` | Fiziki silmə yalnız bütün layihə silinəndə baş verir; o zaman da davranış soft delete ilə eynidir. Sütun siyahısı vacibdir: adi `SET NULL` kompozit FK-da `workspace_id`/`project_id`-ni də NULL etməyə çalışardı (PG 15+) |
| İstinad: nömrə (`12`), nömrə sətri (`"12"`) və ya açar (`"OPS-12"`); başqa layihənin açarı 422 | UI nömrə göndərir, insanlar və skriptlər açar yazır |
| API: `POST .../items` `parent` qəbul edir; `PATCH` `parent: <ref>` qoyur, `parent: null` ayırır; `GET` `parent: {key, number, title}` və `subtasks: {count, done}` qaytarır; siyahıda `?parent=<ref>` və `?top_level=true` (birlikdə 400) | Alt tapşırıq siyahısı adi siyahı endpoint-idir: filtrlər, sıralama, səhifələmə, gizli layihə yoxlaması (`loadProject` → `canSeeProject`) avtomatik eynidir |
| `subtasks.done` = kateqoriyası `done` **və ya** `canceled` olan alt tapşırıqlar | Ləğv edilmiş addım işi bitməyə qoymur: 4/5 deyil, 5/5 |
| Əsas tapşırıq dəyişikliyi `item.updated` hadisəsində `changes.parent` kimi (açarlarla) yazılır; snapshot-a `parent` əlavə olundu, avtomatlaşdırma şərtlərində `parent` sahəsi istifadə oluna bilər | Webhook/avtomatlaşdırma/aktivlik lenti mövcud mexanizmlə işləyir, yeni hadisə növü yoxdur |
| İcazələr: alt tapşırıq yaratmaq/köçürmək üçün `member`, oxumaq üçün `guest` | Adi tapşırıq qaydaları ilə eyni (ADR-0002) |
| İdxal və təkrarlanan tapşırıqlar dəyişmir (həmişə yuxarı səviyyəli tapşırıq yaradır) | Əhatə dairəsi kiçik qalır; lazım olsa sonra "Əsas tapşırıq" sütunu əlavə olunacaq |

### Niyə bir səviyyə

- **Kifayətdir.** Real istifadə "iş → addımlar"dır. Daha dərin struktur (epic → story → task → sub-task) Jira-da
  ayrı-ayrı tiplərlə qurulur və çox komandada qarışıqlıq yaradır; bizdə qruplaşdırma üçün layihələr, fərdi sahələr
  və saxlanılan görünüşlər var.
- **Sadə və sürətli.** Dövr yoxlaması, rekursiv CTE, ağac boyunca irəliləyiş hesabı lazım deyil: xülasə bir indeksli
  `count(*)`-dır (`items_parent_idx`), siyahıda hər sətir üçün ucuzdur.
- **UI sadə qalır.** Telefonda (ADR-0019) iç-içə ağac göstərmək çətindir; bir səviyyə = bir siyahı və bir breadcrumb.
- **Genişlənə bilər.** Sonra dərinlik lazım olsa, trigger-dəki şərt "N səviyyə + dövr yoxlaması" ilə əvəz olunur;
  sxem (`parent_id`) dəyişmir.

## UI

- Tapşırıq panelində "Alt tapşırıqlar" bölməsi: siyahı (açar, başlıq, status), irəliləyiş zolağı və "2/5",
  inline əlavə etmə (Enter). Alt tapşırığın panelində bölmə yoxdur, yerinə "Əsas tapşırıq: ↑ OPS-1 …" keçidi və
  "Əsas tapşırıqdan ayır" düyməsi var.
- Cədvəldə başlığın yanında, lövhədə kartda kiçik "☑ 2/5" nişanı (hamısı bitəndə yaşıl).
- Bütün sətirlər `i18n-subtasks.ts`-dədir; toxunma hədəfləri 44 px (`pointer-coarse:min-h-11`).

## Miqrasiya 00016

`NO TRANSACTION`, bütün addımlar idempotent (yarımçıq qalsa yenidən işə salınır). Yeni unikal indeks
`(workspace_id, project_id, id)` və `items_parent_idx` `CONCURRENTLY` qurulur, sonra constraint indeksdən
(`USING INDEX`) yaradılır: böyük `items` cədvəlində yazmalar uzun müddət bloklanmır. Yeni sütun boş olduğu üçün FK-nın
yoxlanması dərhal bitir.

Down: trigger, constraint-lər, indekslər və sütun silinir — **bütün alt tapşırıq əlaqələri itir** (tapşırıqların
özü qalır, hamısı yuxarı səviyyəli olur). Köhnə image yeni sxemlə də işləyir (sütunu tanımır), amma köhnə kod əsas
tapşırığı silərkən alt tapşırıqları ayırmır; yenidən yeni versiyaya keçəndə belə tapşırıqlar API-də əsas tapşırıqsız
görünür.

## Nəticələr

- Siyahı sorğusu hər sətir üçün bir `LATERAL count` edir; partial indeks sayəsində alt tapşırığı olmayan sətir üçün
  bu, boş indeks axtarışıdır.
- Alt tapşırığın sayına limit qoyulmayıb; panel ilk 100-ü göstərir, qalanı "Daha N alt tapşırığı göstər" ilə yüklənir.
- Layihələr arası köçürmə hələ yoxdur; olsa, alt tapşırıqlar əsas tapşırıqla birlikdə köçməlidir (FK bunu tələb edir).
