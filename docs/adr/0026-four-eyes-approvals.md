# ADR-0026: Status keçidlərində "dörd göz" təsdiqi

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Maliyyə və back-office komandaları (ilk müştəri — FinTez) üçün iki qayda məcburidir: **işi hazırlayan öz işini
təsdiqləyə bilməz** və **tapşırıq N təsdiq olmadan "Tamamlandı" (və ya seçilmiş başqa) statusa keçə bilməz**.
İndiyədək workflow-larda yalnız statuslar və icazəli keçidlər var idi (ADR-0005): statusu dəyişə bilən hər kəs
ödənişi "Ödənildi"yə keçirə bilirdi. Lazım olan: server tərəfində məcburi, yan keçilə bilməyən (toplu redaktə,
idxal, avtomatlaşdırma, birbaşa API), hər addımı audit jurnalında "kim, nə vaxt, niyə" ilə yazılan təsdiq axını.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Siyasət **statusa daxil olmağa** bağlanır: `approval_policies (workflow_id, status_id UNIQUE, min_approvals 1..5, approver_roles, approver_user_ids)`, miqrasiya **00019** | "Tamamlandı-ya N təsdiqsiz keçmək olmaz" qaydasının özüdür; `enforce_transitions` söndürülü workflow-da da işləyir və "istənilən statusdan" keçidləri də əhatə edir | Keçidə (`workflow_transitions`) bağlamaq: enforcement olmayan workflow-da keçid yoxdur; eyni statusa aparan hər keçid üçün siyasəti təkrarlamaq lazım gəlir, birini unutmaq yan keçid deməkdir |
| Təsdiqçilər: workspace **rolları** (`member`, `admin`, `owner`) və/və ya **konkret istifadəçilər** (≤ 50). `guest` heç vaxt təsdiqləmir (DB `CHECK` + yoxlama). Yalnız istifadəçilər seçilibsə `min_approvals ≤ istifadəçi sayı` | Kiçik komandada "istənilən üzv", böyükdə "maliyyə müdiri + baş mühasib"; heç vaxt ödənilə bilməyən siyasət yaradılmır | Ayrıca "təsdiqçi qrupları" cədvəli: indi lazımsız mürəkkəblik, rollar + siyahı kifayətdir |
| **"Hazırlayan" = sorğunu göndərən** (`requested_by`). O heç vaxt təsdiqləyə və ya rədd edə bilməz (`403 self_approval`), təsdiqçi siyahısında da göstərilmir | Statusu "hazırdır" deyə irəli aparan şəxs məhz təsdiq istəyən şəxsdir. A hazırlayıb B-yə "sorğu göndər" desə və A özü təsdiqləsə, prosesdə yenə **iki fərqli adam** iştirak edir — dörd göz prinsipi qorunur | Reporter (`reporter_id`): rəhbər tapşırığı yaradıb sonra icraçının işini təsdiqləyə bilməzdi — ən çox rast gəlinən axın pozulardı. İcraçı (assignee): çox komanda tapşırığı yoxlamaq üçün məhz təsdiqçiyə təyin edir |
| Qorunan statusa keçid `PATCH`-də **`409 approval_required`**; keçid yalnız `POST …/items/{n}/approvals {to_status_id, version, comment?}` ilə sorğu kimi yaradılır. Tapşırıq yerindən **tərpənmir** | Açıq, ayrı əməliyyat: klient nə baş verdiyini bilir, `PATCH`-in semantikası dəyişmir (başlıq + status birlikdə gəlsə yarım tətbiq olunmaz) | `PATCH`-in səssizcə sorğu yaratması: 200 qaytarıb statusu dəyişməmək API-ni yalançı edir |
| "Təsdiq gözləyir" **status deyil, vəziyyətdir**: tapşırıqda `approval: {id, to_status, requested_by, approvals, min_approvals}` (yoxdursa `null`); hər tapşırıq cavabında, siyahıda, lövhədə gəlir | Statuslar workflow-un işidir; saxta "Gözləyir" statusu hər workflow-a əlavə edilməli, hesabatları (kateqoriyalar) pozmalı olardı | Saxta status: bax sol tərəf |
| Bir tapşırıqda eyni anda **ən çox bir** gözləyən sorğu (`UNIQUE (item_id) WHERE state = 'pending'`) | Sadə model: kilid və nişan birmənalıdır | Bir neçə paralel sorğu: hansının qalib gələcəyi qeyri-müəyyəndir |
| Sorğu siyasətin **snapshot**-unu saxlayır (`min_approvals`, rollar, istifadəçilər). Siyasəti dəyişmək/silmək açıq sorğulara təsir etmir | Admin artıq gözləyən ödənişin qaydasını "yumşalda" bilmir; audit "hansı qayda ilə təsdiqləndi" sualına cavab verir | Cari siyasəti oxumaq: təsdiqə yaxın sorğu üçün `min_approvals`-ı 1-ə endirmək boşluqdur |
| Gözləyən sorğu zamanı **kilidli**: status, başlıq, təsvir, fərdi sahələr, tapşırığın öz faylları (əlavə/silmə) və tapşırığın silinməsi → **`409 approval_pending`**. **Açıq qalır**: icraçı, son tarix, əsas tapşırıq, şərhlər və şərh faylları | Təsdiqçi məzmuna imza atır: məbləğ (sahə), təsvir və qəbz (fayl) sonradan dəyişə bilməz. İcraçı/son tarix əməliyyatdır, təsdiqin mənasını dəyişmir; şərh müzakirə üçündür. Eyni dəyəri yenidən göndərmək (no-op) dəyişiklik sayılmır | Hər şeyi kilidləmək: təsdiqçiyə təyin etmək belə mümkün olmazdı. Heç nəyi kilidləməmək: təsdiqdən sonra məbləği dəyişib "təsdiqlənib" saxlamaq olardı |
| Təsdiq: `POST …/approvals/{id}/approve {comment?}`; rədd: `…/reject {reason}` — **səbəb məcburidir** (1–2000 simvol, DB `CHECK`); ləğv: `…/cancel` — sorğunu göndərən və ya admin+ | Rədd səbəbi audit üçündür; DB qaydası API-dən yan keçən yazını da tutur | Səbəbsiz rədd: maliyyə auditində "niyə" sualı cavabsız qalır |
| Uyğunluq **qərar anında** yoxlanılır: aktiv üzv, rol ≥ `member`, snapshot-dakı rol və ya istifadəçi siyahısında, **layihəni görə bilir** (ADR-0013 `canSeeProject` ilə eyni predikat), sorğunu göndərən deyil, hələ qərar verməyib (`approval_decisions` PK) | Gizli layihənin tapşırığını görə bilməyən adam təsdiqçi ola bilməz və sorğunun varlığını bilməz: layihə 404 verir, bildiriş getmir, "Təsdiqimi gözləyənlər" filtri layihə daxilindədir | Siyasət yaradılanda yoxlamaq: layihə üzvlüyü sonradan dəyişir |
| `min_approvals`-a çatan təsdiq keçidi **eyni tranzaksiyada** tətbiq edir: tapşırıq sətri `FOR UPDATE` (əvvəl tapşırıq, sonra sorğu — sabit sıra), versiya kilid altında oxunur, dəyişiklik adi `updateItemTx` yolundan keçir (workflow keçidi yoxlanılır, `item.updated` yazılır) | Paralel iki təsdiq ardıcıl olur: keçid **bir dəfə** tətbiq olunur, gec gələn `409 approval_closed` alır (test: `TestApprovalConcurrentApprovesApplyOnce`). Avtomatlaşdırma və webhook-lar status dəyişikliyini adi `item.updated` kimi görür | Ayrı "apply" addımı (worker): təsdiq ilə keçid arasında pəncərə yaranır |
| Yan keçid yoxdur: toplu redaktə (ADR-0023) hər tapşırıq üçün `approval_required` / `approval_pending` xətası verir (hamısı-və-ya-heç-nə); idxal (ADR-0011) qorunan statusdakı sətiri `status_id` xətası ilə rədd edir; tapşırığı birbaşa qorunan statusda **yaratmaq** 422-dir (**başlanğıc status istisnadır** — yeni tapşırıq onsuz da oradan başlayır); avtomatlaşdırmanın `set_status`-u daimi xəta ilə dayanır (təkrar cəhd yoxdur); hədəf statusu gözləyən sorğu varkən silinmir (`409 approval_pending`) | Qayda bir yerdədir (`updateItemTx` + `prepareItem`): bütün yollar oradan keçir | Hər giriş nöqtəsində ayrıca yoxlama: birini unutmaq kifayətdir |
| Hadisələr (`entity_type = item`): `approval.requested`, `approval.approved`, `approval.rejected`, `approval.canceled`, `approval.applied`; payload: `request_id`, `requested_by`, `from_status_id`, `to_status_id`, `approvals`, `min_approvals`, `comment`/`reason`. Keçidin `item.updated`-i `approval.request_id` daşıyır. Siyasət dəyişiklikləri: `approval_policy.set` / `approval_policy.deleted` (workflow üzərində) | Kim/nə vaxt `events.actor_id` + `occurred_at`-dadır; aktivlik lenti, audit log və webhook-lar mövcud mexanizmlə işləyir (`approval.*` filtri artıq keçərlidir). Gizli layihə hadisələri yalnız o layihəni seçən webhook-lara gedir (ADR-0013) | Ayrı audit cədvəli: ikinci həqiqət mənbəyi |
| Avtomatlaşdırma qaydalarına yeni trigger əlavə olunmadı | Tətbiq olunan keçid adi `item.updated`-dir — "Tamamlandı-ya keçəndə email" kimi qaydalar dəyişmədən işləyir | `approval.*` trigger-ləri: indi tələb yoxdur, sonra əlavə oluna bilər |
| Bildirişlər (ADR-0017): yeni növlər `approval_requested` (uyğun təsdiqçilərə, ən çox 100, sorğu iş başlayana qədər bağlanıbsa — heç kimə), `approval_applied`, `approval_rejected`, `approval_canceled` (sorğunu göndərənə; öz ləğvinə bildiriş yoxdur). `notifications.kind` CHECK-i `NOT VALID` + `VALIDATE` ilə genişləndi | Təsdiqçi "məndən nə gözlənilir"i zəngdə görür; çox böyük rol siyahısında spam olmur | Email: SMTP söndürülmüş quraşdırmalarda heç nə getməz; sonra əlavə oluna bilər |
| Chat webhook-ları (ADR-0010) üçün azərbaycanca sətirlər: "🔏 … təsdiq istədi (0/2)", "⛔ … rədd etdi: «səbəb»" və s. | Mattermost kanalında təsdiq axını görünür | — |
| Versiya: sorğu, rədd və ləğv `items.version`-ı **artırmır**; yalnız tətbiq olunan keçid artırır | Versiya məzmun dəyişikliyinin kilididir; kilidli məzmun onsuz da dəyişə bilməz, icraçını dəyişən paralel redaktə səbəbsiz konflikt almır | Hər vəziyyət dəyişikliyində artırmaq: lazımsız `version_conflict` |
| API (ADR-0002, ümumi rate limit): `GET/POST …/items/{n}/approvals`, `POST …/approvals/{id}/approve|reject|cancel` (cavab: `{approvals, item}`), `PUT/DELETE …/workflows/{wf}/statuses/{st}/approval-policy` (admin+), `GET …/transitions`-da `requires_approval`, siyahıda `?approval=pending|mine` | Mövcud konvensiyalar; UI bir cavabla həm paneli, həm tapşırığı yeniləyir | — |
| UI: workflow parametrlərində hər status üçün siyasət redaktoru; tapşırıq panelində "Təsdiq" bölməsi (vəziyyət, irəliləyiş, təsdiq edə bilənlər, qərarlar və şərhlər, Təsdiqlə / Rədd et (səbəb dialoqu) / Ləğv et), qorunan status seçiləndə sorğu dialoqu, kilidli sahələr deaktiv; cədvəl və lövhədə "⏳ 1/2" nişanı; alətlər panelində "Təsdiq" filtri (telefonda "Filtrlər" vərəqində). Bütün mətnlər az/en/tr/ru (ADR-0025) | Düymələr serverin `can_approve` / `can_cancel` bayraqlarına əsaslanır: UI qayda təkrarlamır, yalnız göstərir | — |

## Nəticələr

- Siyasət yaradılmayıbsa heç nə dəyişmir: mövcud workflow-lar əvvəlki kimi işləyir.
- "Təsdiq" filtri URL-in bir hissəsidir, **saxlanılan görünüşə yazılmır** (görünüş konfiqurasiyasının sxemi
  dəyişmədi). Belə görünüşə filtr əlavə etmək onu "dəyişdirilib" göstərir.
- Gözləyən sorğunun hədəf keçidi sonradan workflow-da qadağan edilərsə, son təsdiq 422 ilə rədd olunur və sorğu
  açıq qalır: ləğv edilməli və yenidən göndərilməlidir.
- Təsdiqdən sonra məzmun yenidən redaktə oluna bilər (tapşırıq artıq hədəf statusdadır). "Təsdiqlənmiş tapşırığı
  dondurmaq" ayrıca qərardır (bax: açıq suallar).
- Sorğu tarixçəsi silinmir; status silinəndə bağlı sorğularda status `NULL` olur (`ON DELETE SET NULL`), hadisələr
  qalır.
- E2E: `web/e2e/approvals.spec.ts`.

## Açıq suallar

- Təsdiqlənmiş tapşırığın məzmununu (məbləğ) sonradan dəyişmək yenidən təsdiq tələb etsinmi?
- Təsdiq üçün son tarix / eskalasiya (N saat cavab yoxdursa admin-ə bildiriş).
- Email bildirişləri və gündəlik "təsdiqinizi gözləyənlər" xülasəsi.
- Workspace üzrə "Təsdiqimi gözləyənlər" siyahısı (hazırda layihə daxilində filtr və bildirişlər var).
