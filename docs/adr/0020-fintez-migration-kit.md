# ADR-0020: Mattermost Boards-dan keçid dəsti (Azərbaycanca şablon, layihə şablonları, Boards importu)

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

FinTez komandaları işlərini Mattermost Boards-da aparır: hər şöbənin öz gizli board-u, sütunlar
"Görüləcək → İcrada → Yoxlamada → Tamamlandı", kartlarda icraçı, son tarix, select xassələri, mətn bloku
və şərhlər. Nebula-da bunun üçün lazım olanlar artıq var (gizli layihələr ADR-0013, təkrarlanan
tapşırıqlar ADR-0012, gecikmə xülasəsi ADR-0009, CSV/xlsx importu ADR-0011), amma keçidin özü əl işidir:

- yeni workspace ingiliscə statuslarla ("To Do", "In Progress"…) yaranır və hər dəfə adlar dəyişdirilməlidir;
- hər yeni şöbə layihəsi üçün eyni üzvlər, təkrarlanan tapşırıqlar və ümumi görünüşlər yenidən qurulur;
- Boards-dan çıxmaq üçün hər board əllə Excel-ə köçürülməlidir; şərhlər və kart mətni itir.

## Qərarlar

### 1. Workspace şablonu (`template: "az" | "en"`)

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `POST /workspaces` `template` qəbul edir; API-də default `en` (köhnə davranış), **veb formada default `az`** | Mövcud API müştəriləri və testlər dəyişmir; interfeys azərbaycancadır, ona görə forma yeni workspace-i azərbaycanca açır | API default-u `az`: mövcud skriptlərin gözlədiyi statuslar dəyişərdi |
| `az`: statuslar Görüləcək (başlanğıc) → İcrada → Yoxlamada → Tamamlandı, Ləğv edildi; iş növləri Tapşırıq, Xəta; qaydaların adları və email mətni azərbaycanca; saat qurşağı `Asia/Baku` | FinTez board-larının adları ilə eynidir, import zamanı status adları birbaşa uyğun gəlir | — |
| İş növlərinin **açarları** (`task`, `bug`) ingiliscə qalır, yalnız adlar tərcümə olunur | Açarlar URL-də, API-də, qaydalarda və importda identifikatordur; dilə görə dəyişməməlidir | Açarları da tərcümə etmək: inteqrasiyalar dil seçimindən asılı olardı |
| Şablon yalnız yaradılma anında tətbiq olunur; sonra hər şey Ayarlarda dəyişdirilir | Sadə, miqrasiya tələb etmir | Workspace-də "dil" sütunu: indiki ehtiyac üçün artıqdır (dil dəyişdirici ayrıca iş olacaq) |

### 2. Layihə şablonu (mövcud layihədən kopya)

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `POST /projects` `template: {from, members, recurrences, views}` qəbul edir: başqa layihənin **qurulması** kopyalanır, tapşırıqlar heç vaxt | Workflow, iş növləri və sahələr artıq workspace səviyyəsindədir; şöbə layihəsini fərqləndirən yalnız üzvlər, təkrarlanan tapşırıqlar və görünüşlərdir | Ayrı "şablon" obyekti və cədvəli: miqrasiya + idarəetmə ekranı, fayda isə eynidir — istənilən layihə şablon ola bilər |
| Hamısı **bir tranzaksiyada**: kopyalama alınmasa layihə də yaranmır | Yarımçıq layihə qalmır | — |
| Üzvlər `project_members`-dən; təkrarlanan tapşırıqlar eyni cədvəl, icraçı və **`next_due`** ilə (mənbə harada davam edəcəksə kopya da oradan); yalnız **ümumi** görünüşlər, sahibi yaradan admin olur | Şəxsi görünüşlər başqasınındır. `next_due`-nu yenidən hesablamaq keçmiş tarixləri "bərpa" edib çoxlu tapşırıq yarada bilərdi | — |
| Hər kopyalanan təkrarlanan tapşırıq üçün `recurrence.created`, layihə üçün `project.created` hadisəsində `template` (nə qədər kopyalandı) | Audit: layihənin haradan yarandığı görünür | — |
| Yalnız admin (layihə yaratma onsuz da admindir); workspace təkrarlanan tapşırıq limiti (1000) kopyaya da aiddir | Mövcud qaydalarla eyni | — |
| Gizli icraçı qaydası dəyişmir: gizli layihədə icraçı layihə üzvü olmalıdır, buna görə üzvləri kopyalamadan təkrarlanan tapşırıqlar yaradılanda icraçı xətası verə bilər (scheduler qaydanı söndürüb `recurrence.failed` yazır, ADR-0012) | Məxfilik qaydası (ADR-0013) istisnasızdır | — |

### 3. Mattermost Boards importu (`.boardarchive`)

Format (mattermost-plugin-boards mənbə kodundan yoxlanılıb): zip, içində `version.json` (`{"version":2}`) və
hər board üçün `<boardID>/board.jsonl` — hər sətir `{"type":"board"|"block"|"boardMember","data":{…}}`.
Kart xassələri seçim/istifadəçi **identifikatorları** ilə saxlanılır; arxivdə ad və email yoxdur.

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Arxiv **mövcud idxal boru xəttinə** çevrilir: hər kart bir sətir; sütunlar "Başlıq", kart xassələri (board-dakı sırası ilə), "Təsvir". Sonra eyni önizləmə, sütun uyğunlaşdırma, quru yoxlama və tranzaksiyalı import (ADR-0011) işləyir | Limitlər, validasiya, xəta hesabatı, audit və interfeys artıq var və test olunub; yeni təhlükəsizlik səthi minimumdur | Ayrı Boards import API-si: eyni yoxlamaların ikinci nüsxəsi |
| Dəyər çevrilməsi: select → seçimin adı; multiSelect → adlar vergüllə; date (`{"from":ms}`) → `YYYY-MM-DD` (vaxt varsa RFC 3339, UTC); checkbox → true/false; createdTime → kartın yaranma vaxtı; file/updatedTime/updatedBy atılır | Mövcud çeviricilər (ParseDate, ParseBool, seçim adları) bunu birbaşa oxuyur | — |
| "Təsvir" = kartın mətn blokları `contentOrder` sırası ilə (checkbox → `- [x]`, ayırıcı → `---`, şəkil/fayl → qeyd) + sonda "**Mattermost Boards-dan köçürülmüş şərhlər**" (müəllif, vaxt, mətn), ən çox 100 000 simvol | Şərhlər itmir və kontekst saxlanılır. Şərhləri ayrıca şərh obyekti kimi yaratmaq üçün müəllif Nebula istifadəçisi olmalıdır və yaranma vaxtı saxtalaşdırılmalı olardı | Ayrı şərhlər: müəllif/vaxt uyğunsuzluğu, @mention bildirişləri köhnə şərhlər üçün yenidən gedərdi |
| **İnsanlar**: arxivdə yalnız Mattermost istifadəçi kodu var. Önizləmə hər kodu neçə kartda göründüyü və 3 nümunə kart adı ilə göstərir; admin hər birini workspace üzvünə uyğunlaşdırır (`mapping.people: {kod: email}`). Uyğunlaşdırılmayanlar boş qalır | Admin kartlara baxıb kimin kim olduğunu bilir. Kodu Mattermost API-dən sorğulamaq Nebula-ya Mattermost token-i vermək deməkdir | Mattermost API inteqrasiyası: əlavə sir, şəbəkə girişi və icazə |
| `people` dəyərləri yalnız **workspace üzvlərinin email-i** ola bilər (yoxsa 422) | Dəyər icraçıya və şərh mətninə yazılır; ixtiyari mətn yeridilə bilməz | — |
| Atılanlar: şablon board (xəta), şablon kartlar, silinmiş bloklar, görünüşlər (`view`), board üzvləri, şəkil və fayllar | Görünüşlər və üzvlər Nebula-da ayrıca qurulur; fayl əlavələri ayrı işdir (ADR-0022) | — |
| Arxivdə bir neçə board varsa, mövcud "vərəq" seçimi board seçimi kimi işləyir (adla) | Yeni parametr yoxdur | — |

### Təhlükəsizlik və limitlər (Boards)

- Fayl **güvənilməz verilənlərdir**: yalnız `version.json` və `<bir qovluq>/board.jsonl` adlı girişlər oxunur
  (`..`, mütləq yol və dərin yollar nəzərə alınmır — fayl sistemə heç nə yazılmır); şəkillər açılmır.
- Zip bomba: oxunan girişlərin elan olunmuş açılmış ölçüsü cəmi ≤ 64 MiB (`archive/zip` elan olunandan artıq
  açmağa icazə vermir); giriş sayı ≤ 2000; sətir ≤ 4 MiB; kart ≤ 20 000; blok ≤ 400 000; 15 san. parse vaxtı.
- Yükləmə limiti `.boardarchive` üçün **50 MiB** (arxivdə board-un şəkilləri də olur), qalan fayllar üçün
  10 MiB qalır; eyni vaxtda ən çox 2 import (yaddaş: 2 × (50 + 64) MiB-dan az).
- Yalnız admin; nəticə adi import kimi audit olunur (`source: import`, avtomatlaşdırma və webhook-lar işə düşmür, ADR-0011).
- Fuzz testi (`FuzzBoardsArchive`) panic olmadığını yoxlayır.

## Nəticələr

- Keçid ardıcıllığı və yoxlama siyahısı: `docs/runbooks/mattermost-boards-migration.md`.
- Status uyğunlaşdırması adla gedir: Boards-dakı status adları Nebula workflow-undakı adlarla eyni olmalıdır
  (az şablonu FinTez adları ilə eynidir). Fərqlidirsə, quru yoxlama hər sətri göstərir; ya Nebula-da status adını,
  ya da faylı düzəltmək lazımdır. Ayrıca "dəyər uyğunlaşdırması" ekranı lazım olarsa sonradan əlavə olunacaq.
- Migrasiya yoxdur (00015 nömrəsi istifadə olunmadı).
- Dil dəyişdirici (interfeysin ingiliscəsi) bu ADR-in hissəsi deyil.
