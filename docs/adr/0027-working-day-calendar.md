# ADR-0027: Workspace iş təqvimi və iş gününə bağlı təkrarlanma

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Maliyyə və back-office komandalarının (ilk müştəri FinTez) son tarixləri iş günlərinə bağlıdır: vergi
bəyannaməsi, ödənişlər, aylıq hesabatlar. Ən çox rast gəlinən qaydalar "ayın son iş günü", "ayın 5-ci iş
günü" və "tarix bayrama düşərsə əvvəlki/növbəti iş gününə keçir"dir. ADR-0012-nin təkrarlanması yalnız
təqvim tarixlərini bilir: 31 yanvar şənbəyə düşəndə tapşırığın son tarixi şənbə olur, ADR-0009 isə onu bazar
ertəsi səhər "gecikmiş" kimi göstərir.

Azərbaycanda qeyri-iş günlərinin bir hissəsi sabitdir, bir hissəsi isə hər il elan olunur:

- **Mənbə:** Əmək Məcəlləsinin 105-ci maddəsi, Nazirlər Kabinetinin saytında dərc olunmuş mətni
  (<https://nk.gov.az/az/azerbaycan/bayramlar>, 2026-11-24 tarixində yoxlanılıb). 105.2-yə görə iş günü
  hesab edilməyən sabit tarixli bayramlar: **1–2 yanvar** (Yeni il), **8 mart** (Qadınlar günü), **9 may**
  (Faşizm üzərində qələbə günü), **28 may** (Müstəqillik Günü), **15 iyun** (Milli qurtuluş günü), **26 iyun**
  (Silahlı Qüvvələr günü), **8 noyabr** (Zəfər Günü), **9 noyabr** (Dövlət bayrağı günü), **31 dekabr**
  (Dünya azərbaycanlılarının həmrəyliyi günü). Novruz (5 gün), Ramazan (2 gün) və Qurban (2 gün) bayramları da
  qeyri-iş günüdür, amma günlərini müvafiq icra hakimiyyəti orqanı növbəti il üçün dekabrın sonunadək elan
  edir (105.4; 2026 üçün Nazirlər Kabinetinin 2 dekabr 2025-ci il, № 362 qərarı).
- 105.5: həftəlik istirahət günü qeyri-iş günü olan bayramla üst-üstə düşərsə, istirahət günü bayramdan sonrakı
  ilk iş gününə keçirilir; 105.6: Ramazan/Qurban başqa qeyri-iş günü ilə üst-üstə düşərsə, növbəti iş günü
  istirahət günüdür; 105.7: iş və istirahət günləri qərarla köçürülə bilər.
- 18 oktyabr, 12 noyabr və 17 noyabr 105.1-də bayram kimi sadalanır, amma 105.2-nin qeyri-iş günləri
  siyahısında yoxdur. Palata saytının (<https://www.chamber.az/static/holidays>) köhnə mətnində isə 105.2-də
  8 və 9 noyabr da yoxdur. Rəsmi konsolidasiya edilmiş mətn (<https://e-qanun.az/framework/46943>) JavaScript
  tələb etdiyi üçün avtomatik oxunmadı — **siyahı birləşdirmədən (merge) əvvəl e-qanun.az-da əl ilə təsdiqlənməlidir.**

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Təqvim **workspace-ə** aiddir: həftə sonu günləri (`workspaces.weekend_days`, default şənbə+bazar), qeyri-iş tarixləri (`kind = holiday`) və köçürülmüş iş günləri (`kind = workday`: həftə sonu tarixi iş günü elan edilir), hər tarixin adı var. Cədvəl `workspace_calendar_days (workspace_id, day)` — **bir tarix bir sətir**, ona görə eyni gün həm bayram, həm iş günü ola bilməz | Workspace artıq saat qurşağının (ADR-0009) sahibidir; bir şirkətin bütün layihələri eyni dövlət təqvimi ilə işləyir | Layihə səviyyəsində təqvim: eyni bayramları N dəfə daxil etmək; istifadəçi səviyyəsində: tapşırığın son tarixi kimin təqviminə görə? |
| Hər hansı xarici API və ya hər il üçün sərt kodlanmış hərəkətli bayram **yoxdur**. Yalnız **sabit tarixli** Azərbaycan bayramları üçün istəyə bağlı köməkçi: `POST …/calendar/presets/az-fixed {year}`. Artıq daxil edilmiş tarixlərə (adminin öz adı və ya köçürmə) toxunmur, təkrar çağırış heç nə əlavə etmir. Novruz, Ramazan, Qurban, 105.5–105.7 köçürmələri admin tərəfindən əl ilə əlavə olunur; UI bunu sarı qeyd ilə "hər il təsdiqlənməlidir" deyə açıq yazır | Hərəkətli günlər Nazirlər Kabinetinin illik qərarıdır, ay təqvimindən hesablanmır (qərar astronomik hesabdan fərqlənə bilər). Xarici API: yeni asılılıq, şəbəkə çıxışı, self-hosted quraşdırmada əlçatmazlıq və etibar problemi | Ay təqvimi alqoritmi: rəsmi elandan fərqli gün → səhv son tarixli vergi tapşırığı. İllik JSON paketi: hər il release tələb edir |
| 105.5 (bayram həftə sonuna düşəndə istirahət gününün köçürülməsi) **avtomatik tətbiq olunmur** | Hər il qərarla elan olunur və 105.6/105.7 ilə birlikdə istisnalar var; avtomatik qayda rəsmi qərarla ziddiyyət yarada bilər | Avtomatik köçürmə: qərar fərqli olarsa iki mənbə, hansı doğrudur? |
| **Rəsmi illik təqvim** (2026-11-24 əlavə): rəsmi təqvimi dərc olunmuş illər üçün eyni preset (`az-fixed`) bütün qeyri-iş günlərini əlavə edir — sabit bayramlar, Novruz/Ramazan/Qurban və 105.5/105.6 köçürmələri ("Köçürülmüş istirahət günü"); hadisədə `preset: az-official`. Məlumat kodda (`azOfficialCalendars`), mənbəsi ilə; hər il Nazirlər Kabinetinin qərarı və ƏƏSMN-in istehsalat təqvimi dərc olunandan (dekabr) sonra əl ilə əlavə olunur və release ilə gəlir. `TestAzOfficialCalendars` hər ili dərc olunmuş iş günü sayı ilə yoxlayır (2026: 241). `GET …/calendar` cavabında `az_official_years`; UI bu illərdə yaşıl "rəsmi təqvim" qeydini göstərir. **2026:** NK qərarı № 362 (02.12.2025: Novruz 20–24 mart, Ramazan 20–21 mart, Qurban 27–28 may) və <https://sosial.gov.az/az/faydali/istehsalat-teqvimi/2026-ci-ilin-istehsalat-teqvimi> (köçürmələr 9, 25, 26, 27, 30 mart, 11, 29 may, 10 noyabr; 20 yanvar Ümumxalq hüzn günü qeyri-iş günüdür — sabit tarixli siyahıya da əlavə olundu) | Yoxlanmış, git-də izlənən, xarici asılılıqsız məlumat; admin hər il əl ilə 8–10 tarix daxil etmək məcburiyyətində qalmır; səhv sayı test tutur | Xarici bayram API-si (Calendarific, Nager.Date): Azərbaycan üçün dəqiqliyi təsdiqlənmir, mənbələr bir-birinə ziddir, açar/şəbəkə tələb edir. Qərarın PDF-ni avtomatik oxumaq: format sabit deyil |
| Limitlər: həftə sonu **ən çox 3 gün**, ildə **ən çox 60** tarix (bayram + köçürmə), illər **2000–2100**, ad 1–100 simvol. `GET` aralığı ən çox 400 gün | İş günü axtarışı sərhədli qalır: ≤ 3 həftə sonu günü və ≤ 60 bayramla ən uzun qeyri-iş ardıcıllığı ~210 gündür, axtarış 400 gündür (`recur.MaxSearchDays`) — hər nəticə iş günüdür (fuzz testi ilə yoxlanılır) | Limitsiz: patoloji təqvim (bütün il bayram) hesablamanı sonsuz və ya mənasız edir |
| Təkrarlanma yeni seçimlər alır (`recurrences.work_day`, `recurrences.shift`): **ayın N-ci iş günü** (1..23) və **ayın son iş günü** (-1), aylıq və illik (ay + iş günü); adi tarix üçün **köçürmə siyasəti**: `none` / `previous` / `next` (qeyri-iş gününə düşərsə əvvəlki/növbəti iş günü). İş günü qaydasında `month_day` və `shift` olmur (DB CHECK) | Maliyyə qaydalarının əksəriyyətini əhatə edir; aylıq/illik ilə eyni ritm (`every`) işləyir | "N-ci iş günündən sonra +K gün" kimi ifadə dili: mürəkkəb, UI-da izah etmək çətin; ehtiyac yaranarsa ayrıca ADR |
| Ayda N iş günü yoxdursa (məs. fevralda 23-cü) — ayın **son iş günü** götürülür; ayda heç iş günü yoxdursa ay buraxılır | ADR-0012-dəki "31 → ayın son günü" ilə eyni məntiq: qayda heç vaxt səssizcə ayı itirmir | Ayı buraxmaq: aylıq hesabat yaranmır |
| Hesablama serverdə (`internal/recur`), workspace saat qurşağındakı "bu gün" ilə; təqvim tarixləri üzərində (saatsız). `next_due` semantikası dəyişmir | ADR-0009/0012 ilə eyni: tarix saat qurşağısızdır, "bu gün" workspace-indir | Brauzerdə hesablama: iki həqiqət mənbəyi |
| Hər təkrar **nominal** tarix (qaydanın adlandırdığı tarix; iş günü qaydasında — ayın 1-i) və **son tarix** (köçürmədən sonra) daşıyır. `recurrence_runs.nominal_date` saxlanılır (köhnə sətirlərdə NULL = `due_date`). Növbəti təkrar həm nominal, həm də son tarixə görə sonuncu yaradılmışdan sonradır; iki nominal tarix eyni son tarixə düşərsə (şənbə və bazar → bazar ertəsi) **bir** tapşırıq yaranır | Təqvim dəyişəndə eyni ay/tarix üçün ikinci tapşırıq yaranmır: "1-ci iş günü" 2-dən 5-ə keçsə də həmin ayın təkrarıdır | Yalnız `due_date` üzrə dublikat yoxlaması: bayram silinəndə köçürülmüş tarix "yeni" görünür və ikinci tapşırıq yaranır |
| Nominal tarixi keçməyibsə, əvvəlki iş gününə köçürülmüş təkrar hələ "gələcəkdir" (məs. bazar günü əlavə olunan bayram cümə gününə köçürür, son tarix dünəndir): tapşırıq yaranır və dərhal gecikmiş görünür | Gec edilən təqvim dəyişikliyi vergi son tarixini **səssizcə itirmir** | Köçürülmüş tarix keçmişdə qalırsa atlamaq: son tarix heç yerdə görünmür |
| `start_date` və `end_date` nominal tarixlərə aiddir | "31 dekabra qədər" qaydası dekabrın 31-ni əhatə edir, hətta son tarix yanvara keçsə belə | Son tarixə görə bitmə: köçürmə ilə son təkrar itir |
| Təqvim dəyişəndə (həftə sonu, tarix əlavə/silmə, preset) eyni tranzaksiyada təqvimdən asılı qaydaların (`work_day IS NOT NULL OR shift <> 'none'`) `next_due`-su **bu gündən** yenidən hesablanır, dəyişən hər qayda üçün `recurrence.rescheduled` hadisəsi (`reason: calendar`, `next_due` from/to). **Keçmiş tarixlər üçün tapşırıq yaradılmır** (qaydanı redaktə etmək kimi), artıq yaradılmış tapşırıqların son tarixi dəyişmir (ADR-0012: qayda dəyişikliyi yaradılmışlara toxunmur) | Proqnozlaşdırılan davranış; audit hər köçmənin izini saxlayır; rename (eyni növ) heç nəyi hesablamır | Mövcud tapşırıqların son tarixini də köçürmək: istifadəçinin əl ilə dəyişdiyi son tarixi əzər, gizli kütləvi redaktə |
| Kilid ardıcıllığı: təqvim yazanı workspace sətrini `FOR NO KEY UPDATE`, qaydanı saxlayan/işlədən `FOR SHARE` ilə kilidləyir, sonra qayda sətri. Planlaşdırıcı (`runRecurrence`) da əvvəl workspace, sonra qayda | Eyni anda təqvim dəyişikliyi və qayda hesablaması köhnə təqvimlə `next_due` yaza bilmir, deadlock yoxdur; `NO KEY UPDATE` FK yoxlamalarını (tapşırıq yazmaları) bloklamır | Kilidsiz: yarış vəziyyətində köhnə təqvimlə hesablanmış tarix |
| **Gecikmə xülasəsi (ADR-0009) qeyri-iş günlərində göndərilmir**: həmin günün `workspace_digest_runs` sətri yazılır (gün "bitib"), olay yaranmır | Bayramda/həftə sonu kanala gələn mesajı heç kim oxumur, bazar ertəsi isə köhnə xülasə yenisinin üstündə qalır. Heç nə itmir: növbəti iş gününün xülasəsi hələ də gecikmiş hər şeyi göstərir (xülasə anlıq görüntüdür, növbə deyil). Fərdi `item.overdue` hadisələri və qaydalar dəyişmir (avtomatlaşdırma bayramda da işləyir) | Həmişə göndərmək: bayram səs-küyü; istəyə bağlı ayar: indiki ehtiyac yoxdur, sonra əlavə etmək asandır |
| API (ADR-0002): `GET /workspaces/{ws}/calendar?year=` və ya `?from=&to=` (bütün üzvlər, qonaqlar da — təqvim görünüşü bayramları göstərir; layihə məlumatı yoxdur); yazmalar **admin+**: `PUT …/calendar/weekend {weekend_days}`, `PUT …/calendar/days/{date} {kind, name}`, `DELETE …/calendar/days/{date}`, `POST …/calendar/presets/az-fixed {year}`. Hadisələr: `calendar.weekend_updated`, `calendar.day_set` (əvvəlki dəyərlə), `calendar.day_deleted`, `calendar.preset_applied` (`entity_type = calendar`, workspace səviyyəli — webhook layihə filtri onları layihəyə bağlamır) | Təqvim tarixləri təkrarlanan qaydaları dəyişir — avtomatlaşdırma ilə eyni rol səviyyəsi | Workspace `PATCH`-inə əlavə etmək: hadisələr `workspace.updated` içində itir, tarix siyahısı PATCH semantikasına uymur |
| Gizli layihələr (ADR-0013): təqvim hadisələri heç bir layihəyə aid deyil; `recurrence.rescheduled` adi `recurrence` hadisəsidir və layihəsinin gizliliyi ilə filtrlənir | Gizli layihənin qayda adı başqa webhook-a sızmır | — |
| UI: Ayarlar → "İş təqvimi" (il naviqasiyası, həftə sonu düymələri, tarix siyahısı və əlavə forması, preset kartı); təkrarlanma formasında "Gün: ayın günü / iş günü", "Neçənci iş günü", "Qeyri-iş gününə düşərsə". Təqvim görünüşü həftə sonlarını **workspace təqvimindən** (sərt kodlanmış şənbə/bazar deyil) solğun, bayramları sarı fon və adla göstərir; adı `title` və `aria-label`-dadır ("Qeyri-iş günü: …"). Telefon gündəmində (ADR-0019) gün başlığında bayramın adı | Ekran oxuyucu və toxunma ekranında da (tooltip yoxdur) məlumat əlçatandır | Yalnız rəng: rəng korluğu və ekran oxuyucu üçün görünməz |
| `describeSchedule` mətnləri dörd dildə (`locales/*/workdays.ts`): "Hər ayın son iş günü", "Hər ayın 3-cü iş günü", "Monthly on the 2nd working day", "Каждые 2 месяца, в 3-й рабочий день" (rusca/ingiliscə üçün `plural()`) | ADR-0025 | — |

## Miqrasiya 00020

- `workspaces.weekend_days smallint[] DEFAULT '{0,6}'` (CHECK: 0..6, ≤ 3), `workspace_calendar_days`,
  `recurrences.work_day`, `recurrences.shift DEFAULT 'none'`, `recurrence_runs.nominal_date`. Sabit default ilə
  `ADD COLUMN` cədvəli yenidən yazmır; yeni CHECK yalnız `recurrences`-i skan edir (workspace başına ≤ 1000).
  `lock_timeout = 5s`.
- Down: iş günü seçimi və ya köçürmə istifadə edən qayda varsa aydın xəta ilə dayanır (köhnə image belə qaydanı
  "günsüz aylıq qayda" kimi oxuyardı); yoxdursa təqvim məlumatı (bayramlar, həftə sonu) silinir.

## Nəticələr

- Təkrarlanmadan kənar son tarixlər (əl ilə qoyulan) bayrama düşəndə avtomatik köçmür; UI-da xəbərdarlıq
  sonrakı addımdır.
- Təqvim dəyişikliyi mövcud tapşırıqların son tarixini dəyişmir; lazım olarsa toplu redaktə (ADR-0023) ilə
  köçürülür.
- Gecikmə hesabı (`due_date < bu gün`) iş günlərinə görə deyil, təqvim günlərinə görədir; "N iş günü gecikib"
  kimi metrika gələcək işdir.
- Azərbaycan bayramlarının sabit siyahısı Əmək Məcəlləsi dəyişəndə kodda (`store.azFixedHolidays`) və bu
  ADR-də yenilənməlidir.
