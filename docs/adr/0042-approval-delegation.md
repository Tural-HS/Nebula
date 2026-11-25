# ADR-0042: Təsdiq səlahiyyətinin ötürülməsi (məzuniyyət və əvəzləmə)

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0026](0026-four-eyes-approvals.md) (dörd göz), [ADR-0034](0034-approval-inbox-reapproval-deadlines.md) (qutu, müddət,
  xatırlatma), [ADR-0031](0031-chat-approval-cards.md) (chat kartları), [ADR-0029](0029-audit-evidence-export.md) (sübut paketi),
  [ADR-0027](0027-working-day-calendar.md) (workspace saat qurşağı), [ADR-0013](0013-project-privacy.md) (gizli layihələr),
  [ADR-0019](0019-mobile-web-ui.md) (telefon), [ADR-0025](0025-ui-languages.md) (4 dil)

## Kontekst

Pilotda (fintech, bek-ofis) "Ödənildi" statusu konkret şəxslərə bağlı dörd göz siyasəti ilə qorunur (`approver_user_ids`: maliyyə
direktoru Elmar və s.). Elmar məzuniyyətə və ya xəstəliyə çıxanda sorğular onun qayıtmasını gözləyir, müddət keçir (ADR-0034),
komanda isə ya siyasəti müvəqqəti dəyişir (izsiz və risklidir), ya da gözləyir. Lazım olan: **müəyyən günlər üçün təsdiq
səlahiyyətini başqa üzvə ötürmək**, auditdə kimin kimin adına qərar verdiyi görünsün və dörd göz heç bir kombinasiyada
zəifləməsin.

ADR-0026-da təsdiq "kiməsə təyin olunmur": siyasətin rolları və/və ya istifadəçi siyahısı uyğun gələn hər kəs təsdiqçidir və hər
təsdiqçinin bir "yeri" (slotu) var. Ona görə "məzuniyyətdəki istifadəçinin gözlədiyi sorğular" = onun uyğun təsdiqçi olduğu və
hələ yerinin boş olduğu gözləyən sorğulardır.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **`workspace_absences`** cədvəli: üzv, istəyə bağlı **əvəzləyən**, `starts_on`/`ends_on` (**workspace saat qurşağında yerli tarixlər, hər ikisi daxil**), qeyd (≤ 500), kim qeyd edib, `ended_at`/`ended_by` (vaxtından əvvəl bitirmə və ya gələcək məzuniyyətin ləğvi). Aktivlik: `ended_at IS NULL AND starts_on ≤ bu gün ≤ ends_on`, "bu gün" = `now() AT TIME ZONE workspaces.timezone` (ADR-0027 ilə eyni) | Bakıda 28-i son gündürsə Bakı vaxtı ilə 28-in sonuna qədər əvəzləmə işləyir; server UTC-də olsa belə. Tarix aralığı insanın düşündüyü kimidir ("25–28 noyabr") | `timestamptz` aralığı: UI-da saat seçimi, qurşaq səhvləri; qlobal (workspace-siz) məzuniyyət: əvəzləyən hər workspace-də fərqli ola bilər, üzvlük workspace-ə bağlıdır |
| **Workspace üzrə, bir üzvə bir vaxtda bir məzuniyyət** (bitirilməmiş məzuniyyətlər üst-üstə düşə bilməz) — yoxlama tətbiqdə, üzvün `workspace_members` sətri `FOR UPDATE` kilidlənəndən sonra; yaratma/dəyişmə/bitirmə eyni ardıcıllıqla kilidləyir | İki paralel sorğu iki üst-üstə düşən məzuniyyət yarada bilmir; "kim kimi əvəz edir" həmişə birmənalıdır | `EXCLUDE USING gist` (btree_gist): idarə olunan PostgreSQL-də genişlənmə hər yerdə icazəli deyil; kilid sadədir və testlə yoxlanır |
| **Qaydalar**: əvəzləyən özü ola bilməz, aktiv, ən az `member` rolunda workspace üzvü olmalıdır; məzuniyyət yalnız `member`+ üçün (qonaq təsdiq vermir — ötürüləcək bir şey yoxdur); yeni başlanğıc bu gün və ya sonra, son gün bu gün və ya sonra, ən çox 367 gün, ən çox bir il qabaqcadan; **başlamış məzuniyyətin başlanğıcı dəyişmir** (onun vasitəsilə qərarlar verilmiş ola bilər) | Keçmişə yazılan məzuniyyət artıq verilmiş qərarların əsasını dəyişərdi; limitlər səhv daxiletməni tutur | Keçmişə məzuniyyət: audit mənasızdır — qərar anında əvəzləmə ya var idi, ya yox |
| **Kim idarə edir**: üzv özünü; **admin/sahib istənilən üzvü** (qəfil xəstəlik) — qeyd etmək, dəyişmək, bitirmək. Bütün marşrutlar `member`+ (`requireRole`), arxivlənmiş workspace-də yazma yoxdur (ADR-0039), yazma istifadəçi başına limitlidir (60 ardıcıl, sonra 10 s-də bir) | Adminin müdaxiləsi real ehtiyacdır; hər əməliyyat audit hadisəsidir | Əvəzləyənin özünün idarəsi: kimin kimə səlahiyyət verdiyi tərsinə çevrilərdi |
| **Zəncir yoxdur**: əvəzləyən **özü də həmin gün məzuniyyətdədirsə, heç kimi əvəz etmir** ("əvəzləyən yoxdur"a düşür); A→B, B→C olsa da A-nın sorğularını C edə bilməz. Yaradanda/dəyişəndə üst-üstə düşmə varsa cavabda `delegate_absent: true` — UI xəbərdarlıq edir, siyahıda da görünür | Səlahiyyət kimin əlində olduğunu bilmək üçün bir addım kifayətdir; zəncir dövrə (A→B→A) və gözlənilməz səlahiyyət yığımı yaradır | Kaskad: auditor üçün izah olunmaz, dövrə yoxlaması lazımdır; məzuniyyət yaratmağı rədd etmək: real həyatda əvəzləyən də sonradan xəstələnə bilər |
| **Əvəzləyən təsdiqçinin səlahiyyətini götürür, kimliyini yox.** Səlahiyyət (siyasətin rolları/istifadəçi siyahısı, `approverEligible` — qərarla eyni predikat) **məzuniyyətdəki üzvə** yoxlanır; qalan hər şey **hərəkət edən insana**: aktiv, qonaq deyil, layihəni görür (ADR-0013), **sorğunu göndərən deyil**, bu sorğuda heç bir rolda qərar verməyib, özü məzuniyyətdə deyil | Məqsəd adla bağlı siyasəti (Elmar) icra etməkdir — əvəzləyən siyasətdə olmaya bilər. Amma vəzifələrin bölünməsi (separation of duties) insana aiddir: sorğu verən heç vaxt, heç kimin adına təsdiq edə bilməz | Əvəzləyənin də siyasətə uyğun olmasını tələb etmək: adla bağlı siyasətdə əvəzləmə mənasız olardı |
| **Dörd göz bütövlüyü (DB səviyyəsində)**: `approval_decisions.user_id` həmişə **real icraçıdır**, yeni `on_behalf_of` — yerini doldurduğu məzuniyyətdəki təsdiqçi. (1) Mövcud PK `(request_id, user_id)`: **bir insan bir sorğuda yalnız bir qərar** verir — öz qərarı + kiminsə adına qərar birləşə bilməz (hansı ardıcıllıqla olursa olsun); (2) yeni unikal indeks `(request_id, COALESCE(on_behalf_of, user_id))`: **hər təsdiqçinin yeri bir dəfə** dolur — əvəzləyən Elmarın adına təsdiq edibsə, Elmar qayıdıb bir də təsdiq edə bilmir və əksinə; (3) `CHECK (on_behalf_of <> user_id)`; (4) sorğunu göndərənin adına qərar yoxdur (onun yeri heç vaxt sayılmır). Tətbiq əvvəlcə aydın xəta verir (`already_decided`, `self_approval`, `not_a_delegate`), indekslər isə yarış halında son müdafiədir | `min_approvals = 2` olan sorğunu bir nəfər "özü + əvəzlədiyi" kimi tək bağlaya bilməz; hər `approvals` sayı fərqli insanların qərarıdır | Qərarda yalnız icraçını saxlamaq: Elmarın yeri boş görünərdi, o qayıdanda ikinci dəfə sayılardı; yalnız Elmarı saxlamaq: real icraçı audit izindən itərdi |
| **Açıq seçim**: API-də `on_behalf_of` göndərilmirsə qərar həmişə icraçının **öz adına**dır; əvəzləmə yalnız açıq `on_behalf_of: <user_id>` ilə. Rədd etmək də əvəzləmə ilə mümkündür (səbəb məcburi) | Səhvən başqasının adına qərar verilmir; UI-da düymələr ayrıdır ("Təsdiqlə" / "Elmar adına təsdiqlə") | Avtomatik seçim: eyni klik fərqli mənalar verərdi |
| **Məzuniyyətdəki üzv özü də qərar verə bilər** (məzuniyyət onu bloklamır); yalnız yeri artıq doldurulubsa (`already_decided`) | Məzuniyyətdə olan maliyyə direktoru təcili ödənişi özü təsdiqləmək istəyə bilər | Məzuniyyətdə bloklamaq: əlavə qadağa, fayda yoxdur |
| **Qutu (ADR-0034)**: "Mənim qərarımı gözləyənlər" = öz yeri boş olan uyğun sorğular **və ya** indi kiminsə adına qərar verə biləcəyi sorğular; hər sətirdə `on_behalf_of` (UI: "🌴 Elmar adına"), nişan sayı eyni predikatla. Tapşırıq siyahısının `?approval=mine` filtri yalnız öz adına olanları göstərir (yer məntiqi yenilənib) | Əvəzləyən hər şeyi bir yerdə görür; SQL predikatı qərarın özü ilə eynidir (`delegatorsFrom`/`delegateUsable`), sızma və uyğunsuzluq yoxdur | Siyahı filtrinə də əlavə etmək: layihə siyahısı "kimin adına" sütunu tələb edərdi — qutu kifayətdir |
| **Bildirişlər**: `approval.requested` uyğun təsdiqçilərə (məzuniyyətdəki də — qayıdanda gözləyir) **və** indi əvəzləyə bilən şəxslərə; `approval.overdue` xatırlatması **əvəzləyənə, məzuniyyətdəkinə yox** ("narahat etmə"), sorğu verənə əvvəlki kimi. Yeni bildiriş növü yoxdur (miqrasiyada `notifications_kind_check` dəyişmir) | Əvəzləyən sorğunu vaxtında görür; məzuniyyətdəki insan xatırlatma ilə narahat edilmir, amma tarixçəsi tam qalır | Məzuniyyətdəkinə heç nə göndərməmək: qayıdanda nə gözlədiyini bilməzdi (qutu bunu göstərir, amma bildiriş siyahısı da tarixçədir) |
| **Chat kartları (ADR-0031)**: "Təsdiq edə bilənlər" sahəsində əvəzləyənlər "Nəzirə (Elmar adına)" kimi; təsdiq/rədd kartında və mətnində "Təsdiqləyən: Nəzirə (Elmar adına)". Qərar yenə yalnız Nebula-da verilir | Kanal kimin hərəkət edəcəyini düzgün göstərir | — |
| **Audit**: `approval.approved` / `approval.rejected` / `approval.applied` hadisəsinin **actor-u real icraçıdır**, payload-da `on_behalf_of` və `absence_id`. Məzuniyyətlər üçün `absence.created` / `absence.updated` (`changes`, `previous_delegate_id`) / `absence.ended` (`began`) — üzv, əvəzləyən, tarixlər, `has_note`, `set_by_admin`; **qeyd heç vaxt hadisəyə yazılmır** (xəstəlik kimi şəxsi məlumat ola bilər) | Auditor "kim, kimin adına, hansı səlahiyyətlə" sualına hadisədən cavab alır; məzuniyyət dəyişiklikləri izlənir | Qeydi hadisəyə yazmaq: audit log və webhook-lar şəxsi səbəbi yayardı |
| **Sübut paketi (ADR-0029)**: `data/approvals.json`-da qərarın `on_behalf_of` sahəsi, insanlar siyahısında hər iki şəxs, HTML hesabatda "Alice <…> (Elmar <…> adına)" (4 dil); əhatə ixracında `absence.*` hadisələri "Layihə, təsdiq qaydası və əvəzləmə hadisələri" bölməsində | Paket özü "bu təsdiq kimin səlahiyyəti ilə verilib" sualına cavab verir | — |
| **Qeydin görünməsi**: məzuniyyəti `member`+ hər kəs görür (təsdiq panelində lazımdır), **qeydi** yalnız üzvün özü, əvəzləyən və adminlər | Məxfilik minimumu; banner/panel üçün tarix və əvəzləyən kifayətdir | — |
| **UI**: istifadəçi menyusunda (masaüstündə avatar, telefonda "Hesab") **"Məzuniyyət və əvəzləmə"** → `/w/:ws/absence` (siyahı, əlavə et/dəyiş/bitir, saat qurşağı yazılır); başlığın altında banner — məzuniyyətdəkinə "Siz … qədər məzuniyyətdəsiniz. Təsdiqlərinizi Nəzirə əvəzləyir", əvəzləyənə "Bu gün Elmar adına təsdiq qərarı verə bilərsiniz" + qutuya keçid; təsdiq panelində "🌴 Elmar … qədər məzuniyyətdədir, əvəzləyən: Nəzirə" (və ya "əvəzləyən yoxdur" — məsələn əvəzləyən layihəni görmür), təsdiqçilər siyahısında "məzuniyyətdə", qərarda "(Elmar adına)"; üzvlər ayarlarında admin bölməsi "Məzuniyyətlər" və hər üzvün altında "… qədər məzuniyyətdə · əvəzləyən: …". Bitirmə `Dialog`-da adla təsdiqlənir; 4 dil, 320 px | Hər rol öz vəziyyətini bir baxışda görür | Ayrı "əvəzləmələr" səhifəsi: banner və qutu kifayətdir |

## Nəticələr

- Miqrasiya **00032**: `workspace_absences` (+ iki qismən indeks), `approval_decisions.on_behalf_of` + `CHECK` + unikal indeks
  `approval_decisions_principal_idx`. Kilid qeydləri faylın başlığında; Down cədvəli, sütunu və indeksi silir (kimin adına olduğu
  hadisələrdə qalır), deploy README §9-da sətir.
- API: `GET, POST /workspaces/{ws}/absences` (`?user_id=`), `PATCH /absences/{id}`, `POST /absences/{id}/end`;
  `POST …/approvals/{id}/approve|reject` gövdəsində `on_behalf_of`; `ApprovalRequest`-də `on_behalf_of` (baxanın kimin adına qərar
  verə biləcəyi), `absences` (bu gün məzuniyyətdə olan təsdiqçilər və real əvəzləyən), qərarda `on_behalf_of`; qutu sətrində
  `on_behalf_of`. Xəta kodları: `absence_overlap`, `absence_closed` (409), `not_a_delegate` (403).
- Kod: `internal/store/absences.go` (predikatlar `absenceActive`, `delegateUsable`, `delegatorsFrom` — qərar, qutu, bildiriş, chat və
  panel eyni SQL-i istifadə edir), `approvals.go` (`checkDecider`), `internal/server/absences_api.go`; web: `api/absences.ts`,
  `lib/absences.ts` (forma yoxlaması, banner seçimi), `components/Absence.tsx`, `pages/AbsencePage.tsx`, lüğət `absences` (4 dil).
- Testlər: `absences_integration_test.go` — idarəetmə və icazələr (qonaq, üzv, admin, başqasının məzuniyyəti), validasiya, üst-üstə
  düşmə, vaxtından əvvəl bitirmə, qeydin görünməsi, audit (qeyd sızmır); əvəzləmə axını (qutu, panel, bildirişlər, hadisə və sübut
  sahələri); dörd göz halları (öz→əvəzləmə, əvəzləmə→öz, sorğu verən əvəzləyən, sorğu verənin adına, zəncir, bitmiş məzuniyyət, gizli
  layihə); pəncərənin sərhədləri workspace saat qurşağında (UTC+14 / UTC−11); xatırlatmalar. Engine və sübut hesabatı unit testləri,
  vitest (`lib/absences.test.ts`), Playwright `web/e2e/absence.spec.ts`.
- E2E üçüncü şəxs tələb edir (sorğu verən nə təsdiqçi, nə əvəzləyən olmalıdır): `apiAsCarol` dev Keycloak-da admin API ilə `carol`
  istifadəçisini yaradır (yalnız lokal, hər qaçışda təsadüfi parol, saxlanılmır).
- Rollback: yalnız image — köhnə image yeni sütunu və cədvəli oxumur; kənar hal deploy README-dədir.

## Açıq suallar

- Workspace-lər arası "bütün workspace-lərdə eyni məzuniyyət" qısayolu (indi hər workspace-də ayrıca qeyd olunur).
- Əvəzləyənin bütün sorğularda deyil, yalnız müəyyən layihə/siyasət üçün təyin edilməsi; bir neçə əvəzləyən (biri yoxdursa digəri).
- HR təqvimi və ya IdP-dən (Keycloak atributu, Outlook "Out of office") avtomatik məzuniyyət idxalı.
- Məzuniyyət bitəndə avtomatik "qayıtdınız, sizin adınıza N qərar verildi" xülasəsi.
- Chat kartında (ADR-0031 addım 2) əvəzləyən üçün birbaşa düymələr.
