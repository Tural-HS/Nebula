# ADR-0051: Aylıq dövr — ayın bağlanması, tarixçə və sahələrin yenidən doldurulması

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0009](0009-due-dates-and-time-automation.md) (scheduler), [ADR-0012](0012-recurring-items.md) (təkrarlanan tapşırıqlar), [ADR-0023](0023-calendar-bulk-inline.md) (toplu redaktə), [ADR-0047](0047-ledger-records.md) (reyestr)

## Kontekst

Köhnə Boards quruluşunda "Müştəri Aylıq Hesabat" lövhəsini hər ayın 1-də ayrıca skript sıfırlayırdı: əvvəlcə keçən
ayın surətini "Hesabat arxiv — <ay>" lövhəsinə köçürürdü, sonra "Ödədi", "Portal", "WhatsApp" işarələrini silir və
"Qalıq"-a "Aylıq ödəniş" yazırdı. Nebula-da bu yox idi: avtomatlaşdırmalar yalnız hadisə ilə (yaradılma,
dəyişiklik, gecikmə) işə düşür, təqvimlə yox. Skript birbaşa verilənlər bazasına yazırdı — yoxlama, tarixçə və
audit yox idi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Layihə üzrə **bir konfiqurasiya** (`period_cycles`): ayın günü (1–28), silinən sahələr, "hədəf ← mənbə" cütləri (eyni tip, düstur sahəsi yox), açıq/söndürülmüş | Ümumi həll: istənilən aylıq reyestr (ödəniş, hesabat, yoxlama) eyni qayda ilə işləyir | Avtomatlaşdırmaya "təqvim" tətiyi: hər tapşırıq üçün ayrı icra, ayın snapshot-ı və "bir ay bir dəfə" zəmanəti olmazdı |
| Bağlanış **əvvəlcə snapshot** yazır (`period_snapshots`: hər tapşırığın başlığı, statusu, icraçısı, bütün sahələri), sonra **yalnız açıq** tapşırıqlarda (status kateqoriyası done/canceled deyil) dəyərləri dəyişir | Keçmiş ay Nebula-da cədvəl kimi oxunur, ayrıca "arxiv layihəsi" lazım deyil; bağlanmış müştərinin son dəyərləri qalır | Layihəni hər ay kopyalamaq (köhnə üsul): tapşırıqlar, şərhlər, linklər ikiləşir, axtarış çirklənir |
| Standart olaraq yalnız açıq tapşırıqlar sıfırlanır; **"bağlı tapşırıqları da sıfırla"** və **"statusu bu statusa qaytar"** seçimləri var | FinTez MAH-da status Yığım-dakı həmin ayın vəziyyətidir ("Tamamlandı" = bu ay ödənib): yeni ayda hamısı sıfırlanmalı və "Görüləcək"-ə qayıtmalıdır. Status tapşırığın workflow-unda deyilsə və ya keçid qadağandırsa, həmin tapşırıq atlanır və siyahıda göstərilir | Həmişə hamısını sıfırlamaq: həqiqətən bağlanmış işlərin (müştəri krediti bağlayıb) son dəyərlərini pozardı |
| Hər dəyişiklik **adi redaktə yolu** ilə gedir (`updateItemTx`, savepoint ilə): yoxlama, təsdiq kilidi, `item.updated` hadisəsi (Aktivlik, webhook, avtomatlaşdırma); hadisədə `cycle: {close_id, period}`. Uğursuz tapşırıq (məs. təsdiq gözləyir) atlanır və bağlanışda siyahılanır | Kənar yol yoxdur: audit və qaydalar eynidir; bir tapşırıq bütün ayı dayandırmır | Birbaşa `UPDATE`: sürətli, amma Aktivlik, webhook və təsdiq kilidini keçir |
| **Bir layihə, bir ay — bir bağlanış** (`UNIQUE (project_id, period)`). Scheduler hər dəqiqə, bütün replikalarda: `next_run <= bu gün` (iş yerinin saat qurşağı), `FOR UPDATE SKIP LOCKED`; bağlanan ay — `next_run`-dan əvvəlki ay. **3 gündən çox gecikmiş** bağlanış avtomatik tətbiq olunmur: `last_error` və `period.close_failed` ("əl ilə bağlayın"), növbəti tarix qoyulur | İkiqat sıfırlama mümkün deyil; gecikmiş sıfırlama bu ay qoyulmuş işarələri silər və onları köhnə ayın snapshot-ına yazardı | Buraxılmış hər ayı bağlamaq: snapshot səhv olardı |
| Bağlanışlar tick-in **sonunda, öz büdcəsi ilə** (2 dəq; bir bağlanış ≤ 2000 tapşırıq, ~15 s); bir layihənin gözlənilməz xətası onun `last_error`-una yazılır və digər layihələr davam edir | Uzun bağlanış səhər xülasəsini və digər skanları gecikdirməməlidir; bir sınıq layihə hamını bloklamamalıdır | Ümumi 30 s içində: böyük layihə hər dəqiqə yarımçıq qalardı |
| Əl ilə yalnız **keçən ay və ya cari ay**, və ən son bağlanmış aydan əvvəlki ay yox | Köhnə ayı indi bağlamaq bugünkü dəyərləri köhnə ayın adı ilə yazar və bu ayın işarələrini silər | İstənilən keçmiş ay: tarixçəni korlayardı |
| Mattermost/Slack webhook-u bağlanışın hər tapşırıq üzrə `item.updated`-ini **almır**, yalnız `period.closed` xülasəsini; `nebula` formatlı inteqrasiyalar hər dəyişikliyi alır | Kanal yüzlərlə sətirlə dolmasın | Hamısını göndərmək: 1-də gecə kanal spam olur |
| Açanda növbəti tarix **bu gündən sonrakı** uyğun gündür (ayın ortasında açmaq keçmiş ayı dərhal bağlamır); yalnız sahələri dəyişmək tarixi saxlayır | Gözlənilməz kütləvi dəyişiklik olmur | — |
| Admin **"İndi bağla"** (istənilən keçmiş və ya cari ay, `YYYY-MM`): əvvəlcə önizləmə (neçə tapşırıq snapshot-a düşür, neçəsi dəyişəcək), sonra təsdiq dialoqu. Əl ilə bağlanmış ayı scheduler atlayır | Buraxılmış bağlanışı bərpa etmək və ya ayı tez bağlamaq | — |
| İcazələr: oxumaq (konfiqurasiya, aylar, snapshot) — üzv; dəyişmək, önizləmə, bağlamaq — admin (təkrarlanan tapşırıqlar kimi) | Aylıq sıfırlama çoxlu tapşırığı dəyişir | — |
| Hədd: bir bağlanış ≤ 2000 tapşırıq; böyük layihə aydın xəta ilə dayanır (`last_error`, `period.close_failed` hadisəsi) | Bir tranzaksiya statement timeout-a çatmamalıdır | Partiyalarla bağlamaq: snapshot-ın atomarlığı itir |

## Nəticələr

- Miqrasiya 00037: `period_cycles`, `period_closes`, `period_snapshots` (FK-lər kaskad ilə; tapşırıq silinəndə
  snapshot sətri də silinir).
- API: `GET/PUT …/projects/{key}/cycle`, `POST …/cycle/preview`, `POST …/cycle/close`, `GET …/cycle/closes`,
  `GET …/cycle/closes/{YYYY-MM}`. Hadisələr: `cycle.updated`, `period.closed`, `period.close_failed`.
- UI: layihənin "Aylıq dövr" səhifəsi (ayarlar, növbəti tarix, əl ilə bağlama, bağlanmış aylar və hər ayın cədvəli
  cəmlərlə).
- Bağlanmış ay **Excel/CSV** kimi yüklənir (`GET …/cycle/closes/{YYYY-MM}/export`): cədvəl ixracının (ADR-0040)
  parametrləri, limitləri, formula neytrallaşdırması və audit qeydi (`items.exported`, `period` ilə). Ay bağlananda
  Mattermost/Slack webhook-una qısa mesaj gedir ("📅 MAH: Noyabr 2026 bağlandı — …").
- Geri qaytarma: bağlanışın özü geri alınmır (köhnə dəyərlər snapshot-dadır, lazım olsa əl ilə və ya toplu
  redaktə ilə). Konfiqurasiyanı söndürmək növbəti bağlanışları dayandırır.

## Əlavə: səhər xülasəsində "bu gün son tarix"

Köhnə skript hər səhər hər şöbənin kanalına günü çatan kartları da göndərirdi. Gündəlik xülasə (ADR-0013) indi iki
siyahıdır: **📅 Bu gün son tarix** (açıq, son tarixi bu gün) və **⚠️ Gecikmədə olanlar**; ikisi də boşdursa, heç nə
göndərilmir. Layihəyə bağlı webhook yalnız öz layihələrinin sətirlərini alır — hər şöbənin kanalı üçün bir webhook
köhnə quruluşu əvəz edir. Payload-a `today`, `today_total`, `today_by_project` əlavə olundu (köhnə sahələr dəyişmir).
Mattermost/Slack formatlı yeni webhook-un standart hadisələrinə `workspace.overdue_digest` də daxildir.

## Açıq suallar

- Snapshot-dan "bu ayın dəyərlərini geri yaz" düyməsi.
- Bağlanışdan sonra Mattermost/Telegram-a xülasə (ay cəmləri).
