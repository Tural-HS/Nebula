# ADR-0039: Layihə və workspace-lərin arxivlənməsi, bərpası və birdəfəlik silinməsi

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0013](0013-project-privacy.md) (gizli layihələr, webhook layihə filtri), [ADR-0022](0022-attachments-markdown.md) (blob təmizliyi),
  [ADR-0029](0029-audit-evidence-export.md) (sübut paketi), [ADR-0032](0032-intake-forms.md) (formalar), [ADR-0009](0009-due-dates-and-time-automation.md) (planlayıcı)

## Kontekst

Workspace və ya layihəni silmək, gizlətmək mümkün deyildi: `DELETE`/arxiv marşrutları yox idi. `projects.archived_at` sütunu
00001-dən var idi və siyahılar onu süzürdü, amma onu heç nə təyin etmirdi. Pilotda (maliyyə komandası) bitmiş layihələr
(illik bağlanış, köhnə müqavilələr) siyahıları doldurur, sınaq workspace-ləri isə yalnız DB-də SQL ilə silinə bilirdi
(`web/e2e/cleanup.sql`). Maliyyə məlumatı olduğu üçün silmə həm geri dönməz, həm də audit baxımından izlənən olmalıdır.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **İki addım**: əvvəl **arxiv** (geri qaytarılan), sonra yalnız arxivlənmiş obyekt üçün **birdəfəlik silmə** (DB `CHECK`: `deletion_requested_at` yalnız `archived_at` ilə) | Səhv kliklə məlumat itmir; arxiv gündəlik "təmizlik" ehtiyacını ödəyir, silmə nadir və şüurlu addımdır | Birbaşa silmə: bir səhv = itki; yalnız soft-delete: sınaq/şəxsi məlumatı həqiqətən silmək mümkün olmazdı |
| **Layihə arxivi — admin+**, bərpa — admin+ | Admin layihə yaradır və bütün (gizli) layihələri görür; arxiv geri qaytarılır | Yalnız sahib: gündəlik təmizliyi bir nəfərə bağlayardı |
| **Layihənin birdəfəlik silinməsi — yalnız workspace sahibi** (admin yox), yazılmış layihə açarı ilə | Geri dönməz əməliyyat maliyyə məlumatında ən az imtiyaz prinsipinə tabe olmalıdır; admin arxivləyə bilir (məlumat görünməz olur), silməni isə sahib təsdiqləyir — "dörd göz"ə yaxın ayrılıq. Workspace silinməsi də yalnız sahibdədir — qayda ardıcıldır | Admin: daha rahat, amma tək admin hesabının oğurlanması bütün layihələrin izsiz itməsi demək olardı |
| **Workspace arxivi — yalnız sahib**, bərpa — admin+, silmə — yalnız sahib (yazılmış slug ilə) | Workspace-in arxivi bütün komandanın işini dayandırır; bərpa təhlükəsiz istiqamətdir, arxiv bölməsini görən adminlər də edə bilər | — |
| Arxivlənmiş **layihə**: bütün `/projects/{key}/…` marşrutları **410 `project_archived`** (silinərkən `project_deleting`); layihəni görə bilməyən (gizli layihə) üçün 404 qalır. Yalnız `POST …/restore` və `DELETE …/{key}` ayrıca loader ilə arxivlənmiş layihəyə çatır | Dərin link "arxivlənib" deyə bilir və admin üçün "Bərpa et" göstərir; gizli layihənin varlığı sızmır (ADR-0013) | Arxivlənmiş layihəni yalnız oxunan saxlamaq: hər oxuma yolunu (cədvəl, lövhə, panel) "yalnız oxu" rejiminə salmaq lazım olardı; arxiv = "gizlət" daha sadə və gözlənəndir |
| Arxivlənmiş **workspace**: **yalnız oxunan** — `GET` işləyir, digər metodlar **409 `workspace_archived`**; istisnalar: `POST /restore`, `DELETE /`, sübut ixracları (`POST /evidence/exports`, `POST …/items/{n}/evidence` — POST-dur, amma yalnız oxuyur). Silinən workspace — 410 `workspace_deleting` | Bir yoxlama nöqtəsi (`loadWorkspace`), marşrutlar sonradan əlavə olunsa da yazma qadağası avtomatik tətbiq olunur (allowlist, deny-by-default); sahib silməzdən əvvəl sübut paketini endirə bilir | Hər handler-də yoxlama: yeni marşrutda unudulardı |
| Gizlədilir: layihə/workspace siyahıları, axtarış, ana səhifə (sayğaclar, siyahılar, kartlar), təsdiq qutusu və nişanı, bildiriş siyahısı və sayı, əlaqə hədəfi kimi seçmə. Formalar bağlanır (`closed_reason: "archived"`, aydın mesaj), dəvət qəbulu 409 | Tələb: "siyahılardan, axtarışdan, ana səhifədən, növbələrdən gizli" | — |
| **Fon işləri dayanır**: gecikmə skanı, gündəlik xülasə, təkrarlanan tapşırıqlar, təsdiq müddəti xatırlatması, düstur yenidən hesablanması (workspace) — sorğularda `archived_at IS NULL`. Hadisələr **fan-out olunmur** (webhook, avtomatlaşdırma, bildiriş), çatdırılma və avtomatlaşdırma anında **yenidən yoxlanır** (arxivdən əvvəl növbəyə düşmüş job-lar da); arxivlənmiş/silinmiş workspace-in məktubu atılır (instance serverinə keçmir) | "Arxivlənmiş layihənin webhook/avtomatlaşdırma/təkrarlanan işi işləməməlidir"; növbədəki köhnə job-lar da | Yalnız fan-out-da yoxlamaq: arxivdən əvvəl növbəyə düşmüş job-lar yenə gedərdi |
| Bərpadan sonra təkrarlanan qaydalar buraxılmış tarixləri **mövcud tutma limiti ilə** yaradır | Maliyyə üçün buraxılmış aylıq bağlanış tapşırığı görünməlidir; limit (ADR-0012) seli kəsir | Bərpada `next_due`-nu bu günə çəkmək: unudulmuş tapşırıqlar səssizcə itərdi |
| **Silmə asinxrondur**: sorğu `deletion_requested_at` qoyur və audit qeydini yazır, sonra eyni sorğuda ~3 s ərzində silməyə başlayır (kiçik layihə dərhal: 200 `deleted`), qalanını worker-in hər dəqiqəlik tick-i bitirir (202 `deleting`). Silmə **500 tapşırıq / 5000 hadisə** partiyaları ilə, hər partiya öz tranzaksiyasında; sonda bir tranzaksiyada qalan sətirlər və layihə/workspace sətri | Böyük workspace-in bir tranzaksiyada kaskad silinməsi uzun kilid, nəhəng WAL və HTTP timeout deməkdir. Hər addım idempotentdir (`EXISTS deletion_requested_at` qoruyucusu ilə — heç vaxt silinməsi istənməyən obyektə toxunmur), çökmə və ya iki replika yalnız zərərsiz işi təkrarlayır | Ayrı job növü: dedupe və davam etdirmə mürəkkəbləşirdi; tick skanı blob təmizliyi ilə eyni nümunədir |
| **FK yolları yoxlanıb**: tapşırıq silinməsi kaskadla şərhləri, əlavələri (trigger `attachment_blob_purges`-ə yazır — blob mövcud gecikmə ilə silinir, ADR-0022), əlaqələri (digər layihələrdəkilər daxil), izləyiciləri, təsdiq sorğularını və qərarları, bildirişləri, axtarış sənədlərini, gecikmə qeydlərini, təkrarlanma icralarını və forma göndərişlərini aparır; layihə sətri formaları, görünüşləri (standart görünüş daxil), təkrarlanmaları, üzvləri. `item_types`/`statuses`/`workflows` kaskadsız istinadlardır — workspace silinməsində `recurrences → items → forms → item_types` ardıcıllığı (`cleanup.sql` ilə eyni). `webhooks.project_ids` FK deyil: layihə massivdən çıxarılır, **massiv boşalırsa webhook söndürülür** (boş filtr = "bütün layihələr" olardı) | Heç nə qırılmır, yetim sətir qalmır (inteqrasiya testi cədvəl-cədvəl yoxlayır) | Webhook-u boş filtrlə saxlamaq: məxfi filtrin səssizcə bütün layihələrə genişlənməsi |
| **Audit**: `project.archived`, `project.restored`, `project.deleted` (açar, ad, tapşırıq və fayl sayları) workspace-in `events`-ində qalır (layihə silinsə də). `workspace.archived`, `workspace.restored`, `workspace.deletion_requested` workspace-in öz jurnalındadır — o, workspace ilə silinir. Ona görə workspace silinməsi **həm server loguna** (sorğu, sorğu edən, saylar; sonda "deletion completed"), **həm də workspace-dən kənar `workspace_deletions` cədvəlinə** yazılır (slug, ad, sorğu edən, vaxtlar, saylar, arxivdən sonra sübut ixracı olubmu). Cədvəlin workspace-ə FK-sı yoxdur, Down miqrasiyası onu saxlayır | Silinmiş obyektin qeydi silinmiş obyektin içində ola bilməz; server logu Loki-yə gedir (ADR-0015), cədvəl isə backup və SQL ilə sorğulanan daimi qeyddir | Yalnız log: saxlama müddəti qısadır; ayrı audit servisi: bir VPS üçün artıqdır |
| **Sübut paketi**: silmə dialoqu paketi endirmə linkini və xəbərdarlığı göstərir, API **`evidence_acknowledged: true` tələb edir** (onay qutusu); ixracın özü məcburi deyil, amma arxivdən sonra ixrac olub-olmadığı `workspace_deletions.summary`-də qeyd olunur | Server faylın saxlanıldığını yoxlaya bilməz — ixracı məcburi etmək "teatr" olardı və boş/sınaq workspace-lərini əngəlləyərdi; saxlama öhdəliyi müştərinin siyasətidir. Açıq təsdiq + qeyd hesabatlılığı təmin edir. Paket 366 gün limitlidir (ADR-0029): uzun tarixçə üçün bir neçə ixrac lazımdır | İxracı məcburi etmək (son N saatda `evidence.exported`): yoxlanmayan yük; heç nə tələb etməmək: maliyyə məlumatı üçün kifayət deyil |
| Silinmiş layihənin açarı və workspace-in slug-ı silmə bitəndən sonra **yenidən istifadə oluna bilər** | Sətir həqiqətən silinir; köhnə dərin linklər 404 verir | Tombstone ilə rezerv: sınaq workspace-lərində narahatçılıq |
| UI: workspace Ayarlar → Ümumi və layihə ayarlarında **"Təhlükəli zona"**; arxiv `Dialog`-la (obyektin adı ilə), silmə yazılmış açar/slug ilə (düymə uyğunluğa qədər deaktiv), ana səhifədə **"Arxivlənmiş workspace-lər"** (yalnız sahib/admin, Bərpa et), workspace səhifəsində **"Arxivlənmiş layihələr"** (admin), arxivlənmiş workspace-in hər səhifəsində banner, arxivlənmiş layihənin dərin linkində vəziyyət ekranı. 4 dil, 320 px | ADR-0019, ADR-0025; `window.confirm` yoxdur | — |

## API

- `POST /workspaces/{ws}/archive` (sahib), `POST /workspaces/{ws}/restore` (admin+), `DELETE /workspaces/{ws}` `{confirm, evidence_acknowledged}` (sahib) → 200 `{"status":"deleted"}` / 202 `{"status":"deleting"}`
- `GET /workspaces?state=archived` — çağıranın admin/sahib olduğu arxivlənmiş workspace-lər
- `GET /workspaces/{ws}/archived-projects` (admin+)
- `POST /workspaces/{ws}/projects/{key}/archive` (admin+), `POST …/restore` (admin+), `DELETE /workspaces/{ws}/projects/{key}` `{confirm}` (sahib)
- Workspace və layihə JSON-una `archived_at` və `deleting` əlavə olundu; ictimai formaya `closed_reason`.

## Nəticələr

- Miqrasiya 00031: `workspaces.archived_at`, `workspaces.deletion_requested_at`, `projects.deletion_requested_at` (+ `CHECK`-lər), `workspace_deletions`.
  Down silmə davam edərkən imtina edir; arxivlənmiş workspace-lər köhnə image-də yenidən aktiv görünür.
- Başqa layihədəki tapşırığın silinmiş tapşırıqla əlaqəsi kaskadla yox olur; o tərəfin tarixçəsinə `link.removed` yazılmır
  (bir-bir silmədən fərqli olaraq) — iz `project.deleted` hadisəsindədir.
- Arxivlənmiş layihənin tapşırığına başqa layihədən olan əlaqə siyahıda görünməyə davam edir (tarixçədir); keçid arxiv ekranını açır,
  "Bloklanıb" nişanı arxivlənmiş bloklayıcını da sayır.
- Workspace arxivlənəndə onun gözləyən təsdiqləri, bildirişləri və iş qutuları gizlənir, amma silinmir — bərpa ilə qayıdır.
- Workspace silinəndə üzvlərin istifadəçi hesabları qalır (qlobal identiklik), yalnız üzvlük silinir.

## Açıq suallar

- `workspace_deletions` üçün instance admin ekranı (hazırda SQL və loglar).
- Arxivdən sonra minimum gözləmə müddəti (məs. 7 gün) silmədən əvvəl — pilot rəyinə görə.
- Çox böyük tarixçə üçün "bütün dövr" sübut paketi (bir neçə 366 günlük ixracı avtomatik birləşdirmək).
- Arxiv/bərpa hadisələrini webhook-lara çatdırmaq (hazırda arxivlənmiş obyektin heç bir hadisəsi, arxiv hadisəsinin özü də, getmir).
