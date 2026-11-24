# ADR-0034: Təsdiq qutusu, dəyişiklikdə yenidən təsdiq və təsdiq müddətləri

- Status: Qəbul edilib
- Tarix: 2026-11-24
- Əsas: [ADR-0026](0026-four-eyes-approvals.md) ("dörd göz" təsdiqi)

## Kontekst

ADR-0026 təsdiqi statusa daxil olmağa bağladı. İstifadədə üç boşluq qaldı (ADR-0026-nın açıq sualları):

1. **"Məndən nə gözlənilir?"** yalnız layihə daxilində filtrdir (`?approval=mine`). Beş layihədə işləyən maliyyə
   müdiri hər layihəni ayrıca açmalıdır.
2. **Köhnəlmiş təsdiq.** Keçid tətbiq olunandan sonra məzmun (məsələn, məbləğ) yenidən redaktə oluna bilir:
   tapşırıq "Ödənildi"də qalır, amma təsdiqçinin imzaladığı məbləğ artıq yoxdur. Maliyyə auditində bu, nəzarətin
   tam yan keçilməsidir.
3. **Müddət yoxdur.** Sorğu həftələrlə gözləyə bilər; nə gecikmə görünür, nə də kiməsə xatırlatma gedir.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Workspace qutusu**: `GET /workspaces/{ws}/approvals?state=pending&mine=true\|false&limit&cursor` və nişan üçün `GET /workspaces/{ws}/approvals/count?mine=true` (`{count, overdue, cap}`, ən çox 100) | Bir səhifədə bütün layihələrdən; nişan sorğusu yüngüldür, siyahını yükləmir | Hər layihəyə `?approval=mine` sorğusu göndərən UI: N sorğu, gizli layihələri UI özü süzməli olardı |
| Görünürlük: yalnız baxanın **görə bildiyi** layihələr (`visibleProjectFor` = `canSeeProject`); `mine=true` — eyni `approverEligible` predikatı ilə, yəni **qərarın özünü yoxlayan qayda ilə** (aktiv üzv, rol ≥ member, snapshot-dakı rol/istifadəçi, layihəni görür, sorğunu göndərən deyil, hələ səs verməyib) | Bir qayda — bir yer: `eligibleApproversSQL` indi `approverEligible(...)`-dən qurulur, qutu, bildirişlər, chat kartı və qərar eyni mətni istifadə edir; qutu heç vaxt gizli layihəni sızdıra bilməz (test: `TestApprovalInboxEligibilityPrivacyAndPagination`) | Qutu üçün ayrıca, sadələşdirilmiş şərt (`items.go`-dakı layihə filtri kimi): iki yerdə qayda, fərqlənmə riski |
| Sıra: **ən köhnə əvvəl** (`created_at, id` keyset, `approval_requests_inbox_idx`), UI müddəti keçənləri yuxarı çıxarır | Ən çox gözləyən sorğu ən təcilidir; keyset səhifələmə sabitdir (yeni sorğu səhifəni sürüşdürmür) | `due_on` üzrə sıra: müddətsiz sorğular üçün mənasız, kursor mürəkkəbləşir |
| UI: `/w/:ws/approvals` səhifəsi ("Mənim qərarımı gözləyənlər" / "Bütün gözləyənlər"), workspace səhifəsində nişanlı link; sətir `?item=n&focus=approval` ilə tapşırıq panelini təsdiq bölməsində açır (ADR-0031) | Telefon başlığı doludur (ADR-0019): yeni başlıq elementi yoxdur. Dərin link artıq var | Başlıqda zəng kimi ikinci nişan: 375 px-də sığmır |
| **Yenidən təsdiq** siyasətdə seçimdir: `reapprove_on_change` + `reapprove_fields` (`title`, `description`, `fields.<key>`; boş = başlıq, təsvir və bütün fərdi sahələr). Formula sahəsi izlənmir (422): onun girişləri izlənir | Admin "yalnız məbləğ" deyə daralda bilər; standart ən təhlükəsizdir. Formula girişlər dəyişəndə dəyişir, ikiqat sayılmaz | Həmişə açıq: mövcud workflow-ların davranışı xəbərdarlıqsız dəyişərdi |
| Seçim də **snapshot**-dur (sorğuda `reapprove_on_change`, `reapprove_fields`): təsdiq hansı qayda ilə verilibsə, sonrakı dəyişikliyi o qayda yoxlayır. Siyasət sonradan silinibsə (status artıq qorunmur) — yenidən təsdiq yoxdur | ADR-0026-nın prinsipi: admin artıq verilmiş təsdiqin şərtini geriyə doğru dəyişmir. Siyasət silinibsə, o statusa daxil olmaq üçün təsdiq lazım deyil, deməli dəyişmək üçün də | Cari siyasəti oxumaq: admin seçimi söndürüb məbləği dəyişib yenidən yandıra bilərdi |
| **Variant (a) seçildi**: izlənən məzmun dəyişəndə **eyni tranzaksiyada** köhnə təsdiq `invalidated_at` alır, tapşırıq təsdiqdən **əvvəlki statusa qayıdır** və eyni keçid üçün **yeni sorğu açılır** (`reopened_from`), sorğunu göndərən — dəyişikliyi edən şəxs | Status həmişə doğrunu deyir: "Ödənildi" = **indiki** məzmun təsdiqlənib; hesabatlar, lövhə, avtomatlaşdırma və webhook-lar köhnəlmiş təsdiqi "təsdiqlənmiş" kimi görmür. Dəyişdirən şəxs özü təsdiqləyə bilmir — dörd göz yenidən işləyir (təsdiqçi özü məbləği dəyişsə belə). Yeni sorğu gözləyərkən məzmun yenə kilidlidir | (b) yalnız "köhnəlib" nişanı, status yerində: statusa əsaslanan hər şey (hesabat, `done` kateqoriyası, avtomatlaşdırma "Ödənildi-yə keçəndə") yalan deyir; nişanı görməmək asandır, audit bunu nəzarət zəifliyi sayır |
| Geri qayıdış sistem addımıdır: `checkTransition` və qayıdılan statusun öz siyasəti tətbiq olunmur (təsdiqlənmiş keçidin tərsidir). Əvvəlki status silinibsə — dəyişiklik **`409 reapproval_required`** ilə rədd olunur (əvvəlcə tapşırığı statusdan çıxarın) | Qayıdış "geri al"dır, yeni keçid deyil; enforcement olan workflow-da tərs keçid adətən yoxdur. Hara qayıdacağı bilinməyəndə məzmunu səssizcə dəyişməkdənsə dayanmaq təhlükəsizdir | İlkin statusa qayıtmaq: təsdiqdən əvvəlki addımları (yoxlama) atlayar |
| İnsan olmayan və toplu dəyişikliklər **rədd edilir** (`409 reapproval_required`): avtomatlaşdırma (daimi xəta, təkrar yoxdur) və toplu redaktə (ADR-0023, hamısı-və-ya-heç-nə). İdxal yalnız yaradır, təsiri yoxdur | Yeni sorğunun insan "hazırlayanı" olmalıdır (`requested_by NOT NULL`, dörd göz); bir toplu klik 500 ödənilmiş tapşırığı geri göndərməməlidir | Avtomatlaşdırma adından sorğu: kim hazırlayıb sualı cavabsız qalır |
| Status dəyişikliyi ilə birlikdə gələn redaktə yenidən təsdiq tələb etmir; son tarix, icraçı, əsas tapşırıq heç vaxt məzmun sayılmır; faylların sonradan əlavəsi də (bax: açıq suallar) | Tapşırıq qorunan statusdan onsuz da çıxır; qalanlar əməliyyatdır, imza deyil (ADR-0026 kilid siyahısı ilə eyni məntiq) | — |
| Hadisələr: `approval.invalidated` (köhnə sorğu: `request_id`, `changed`, `reverted_to_status_id`, `reopened_request_id`), sonra yeni sorğunun `approval.requested`-i (`reopened_from`, `due_on`); `item.updated`-in `approval` sahəsi `{invalidated_request_id, request_id}`. Sıra: `item.updated` → `approval.invalidated` → `approval.requested` | Audit "kim, nə vaxt, nəyi dəyişdi və təsdiqi etibarsız etdi" sualına cavab verir; webhook-lar və chat (`approval.*`) yeni hadisələri dəyişmədən alır | — |
| **Müddət**: siyasətdə `due_in_working_days` (1..30, boş = yoxdur). Sorğu açılanda `due_on` = workspace-in yerli "bu gün"ündən sonra N-ci **iş günü** (ADR-0027 təqvimi; yeni `recur.(*Calendar).AddWorkdays`, `WorkdaysBetween`-in tərsi). Tarix snapshot-dur: sonrakı təqvim dəyişikliyi onu dəyişmir | "2 iş günü" maliyyədə adi SLA dilidir; bayram və köçürülmüş iş günləri nəzərə alınır (test: bayramdan keçən müddət). Snapshot — audit üçün sabit tarix | Saatla müddət: iş günü/bayram nəzərə alınmaz, saat qurşağı qarışıqlığı. Hər dəfə yenidən hesablamaq: tarix "sürüşür" |
| Gecikmə: `due_on < yerli bu gün` olanda sorğu **gecikmiş** sayılır — tapşırığın nişanı (`approval.overdue`, qırmızı), paneldə, qutuda və nişan sayında görünür | Son gün bütövlükdə qərar üçündür | `due_on`-un başlanğıcında gecikmə: son günü itirmək |
| **Bir xatırlatma**: planlayıcının dəqiqəlik `Tick`-i `EmitApprovalOverdue` ilə hər gecikmiş sorğu üçün **bir dəfə** `approval.overdue` yazır. İdempotentlik: `UPDATE … SET overdue_notified_at = now() WHERE overdue_notified_at IS NULL` (bir neçə worker replikası, təkrar işə salma — yenə bir hadisə; test: iki paralel tick). `FOR UPDATE SKIP LOCKED`: qərar tranzaksiyası tutan sətir növbəti tick-ə qalır, tapşırıq kilidini gözləmədiyi üçün deadlock mümkün deyil | Mövcud `item.overdue` mexanizmi ilə eyni yanaşma (ADR-0009), ayrıca marker cədvəli lazım deyil | Hər gün xatırlatma: spam; email: SMTP söndürülə bilər, sonra əlavə oluna bilər |
| Bildiriş alanlar (yeni növ `approval_overdue`): **hələ qərar verməmiş uyğun təsdiqçilər** (ən çox 100) və **sorğunu göndərən**. **Admin eskalasiyası edilmədi** | Qərar verə bilən adamlar və gözləyən adam — xatırlatmanın mənası budur. Adminlər çox vaxt onsuz da təsdiqçi rollarındadır (ikiqat bildiriş); gizli layihədə də admin görür, amma ona "qərar verin" demək mənasızdır; qutu və qırmızı nişan hamıya görünür. Eskalasiya sonra siyasətdə ayrıca seçim kimi əlavə oluna bilər | Hər gecikmədə bütün admin/owner-lərə: kiçik komandada eyni adama iki bildiriş, böyükdə səs-küy |
| Bildiriş `approval_invalidated`: əvvəlki sorğunu göndərənə və **təsdiq vermiş** şəxslərə | Onların imzası artıq qüvvədə deyil — bilməlidirlər. Yeni sorğunun təsdiqçiləri adi `approval_requested` alır | — |
| Miqrasiya **00027**: `approval_policies` və `approval_requests`-ə sütunlar (sabit default — cədvəl yenidən yazılmır), `approval_requests_invalidated_check` (yalnız təsdiqlənmiş sorğu etibarsız ola bilər), qutu/müddət/örtük indeksləri, `notifications_kind_check` — 00019 və 00021-in **tam birləşməsi** + `approval_overdue`, `approval_invalidated` (`NOT VALID` + `VALIDATE`) | Mövcud sorğular `reapprove_on_change = false` alır: köhnə təsdiqlər heç vaxt etibarsız olmur | Yeni `state = 'invalidated'`: sorğu həqiqətən təsdiqlənib və keçid tətbiq olunub; ayrıca sütun tarixçəni düzgün saxlayır |
| Audit sübut paketi (ADR-0029): `approvals.json`-da `due_on`, `invalidated_at`, `reopened_from`; HTML hesabatda vəziyyət "təsdiqlənib · məzmun dəyişdi, təsdiq etibarsızdır (vaxt)", tələb sütununda son gün. Chat kartları (ADR-0031): "♻️ Məzmun dəyişdi, yenidən təsdiq lazımdır" və "⏰ Təsdiq müddəti keçib" (təsdiq edə bilənlərin adları ilə) | Yeni hadisələr mövcud ixrac və webhook yollarından keçir; paket imzası dəyişmir | — |
| UI (ADR-0025, az/en/tr/ru, yeni lüğət modulu `approvalInbox`): siyasət redaktorunda "Təsdiqdən sonrakı dəyişiklik yenidən təsdiq tələb edir", izlənən məzmun, "Qərar müddəti, iş günü"; paneldə son gün çipi, etibarsız təsdiq və "yenidən təsdiq istənilir" qeydi, təsdiqlənmiş tapşırıqda "Diqqət: Məbləğ dəyişsə, tapşırıq yenidən təsdiqə göndəriləcək" | Redaktədən **əvvəl** xəbərdarlıq — istifadəçi statusun niyə dəyişdiyini anlayır. Panel UI-si `components/Approvals.tsx`-dadır; `ItemDrawer.tsx` dəyişmədi | — |

## Nəticələr

- Siyasətdə yeni seçimlər yandırılmayıbsa heç nə dəyişmir; mövcud təsdiqlər (00027-dən əvvəl) heç vaxt etibarsız olmur.
- Yenidən təsdiq zəncir boyunca deyil, **bir addım** geri qaytarır: "Yoxlandı" (qorunan) → "Ödənildi" (qorunan)
  zəncirində məbləğ dəyişəndə tapşırıq "Yoxlandı"ya qayıdır, "Yoxlandı" təsdiqi yerində qalır.
- Avtomatlaşdırma qaydası təsdiqlənmiş tapşırığın izlənən sahəsini dəyişə bilmir (daimi xəta, qayda logunda görünür).
- Gecikmə xatırlatması qeyri-iş günündə də gedə bilər (müddət keçən kimi); bu, tətbiqdaxili bildirişdir, email deyil.
- Nişan sayı 100-də kəsilir ("99+"); müddəti keçənlər sayda əvvəl sayılır.
- E2E: `web/e2e/approvals-plus.spec.ts`. Rollback: `deploy/production/README.md` §9, 00027 sətri.

## Açıq suallar

- Təsdiqdən sonra **fayl** (qəbz) əlavə etmək/silmək yenidən təsdiq tələb etsinmi? Hazırda yox (fayllar `item.updated`
  dəyişikliyi deyil); izlənən məzmuna `attachments` tokeni kimi əlavə oluna bilər.
- Admin eskalasiyası (müddətdən N gün sonra admin-ə) siyasətdə ayrıca seçim kimi.
- Email ilə gündəlik "qərarınızı gözləyənlər" xülasəsi.
