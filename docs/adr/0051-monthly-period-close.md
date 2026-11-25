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
| Hər dəyişiklik **adi redaktə yolu** ilə gedir (`updateItemTx`, savepoint ilə): yoxlama, təsdiq kilidi, `item.updated` hadisəsi (Aktivlik, webhook, avtomatlaşdırma); hadisədə `cycle: {close_id, period}`. Uğursuz tapşırıq (məs. təsdiq gözləyir) atlanır və bağlanışda siyahılanır | Kənar yol yoxdur: audit və qaydalar eynidir; bir tapşırıq bütün ayı dayandırmır | Birbaşa `UPDATE`: sürətli, amma Aktivlik, webhook və təsdiq kilidini keçir |
| **Bir layihə, bir ay — bir bağlanış** (`UNIQUE (project_id, period)`). Scheduler hər dəqiqə, bütün replikalarda: `next_run <= bu gün` (iş yerinin saat qurşağı), `FOR UPDATE SKIP LOCKED`; bağlanan ay — `next_run`-dan əvvəlki ay. Fasilədən sonra yalnız son ay bağlanır | İkiqat sıfırlama mümkün deyil; köhnə ayları bugünkü dəyərlərlə "bağlamaq" yanlış tarixçə olardı | Buraxılmış hər ayı bağlamaq: snapshot səhv olardı |
| Açanda növbəti tarix **bu gündən sonrakı** uyğun gündür (ayın ortasında açmaq keçmiş ayı dərhal bağlamır); yalnız sahələri dəyişmək tarixi saxlayır | Gözlənilməz kütləvi dəyişiklik olmur | — |
| Admin **"İndi bağla"** (istənilən keçmiş və ya cari ay, `YYYY-MM`): əvvəlcə önizləmə (neçə tapşırıq snapshot-a düşür, neçəsi dəyişəcək), sonra təsdiq dialoqu. Əl ilə bağlanmış ayı scheduler atlayır | Buraxılmış bağlanışı bərpa etmək və ya ayı tez bağlamaq | — |
| İcazələr: oxumaq (konfiqurasiya, aylar, snapshot) — üzv; dəyişmək, önizləmə, bağlamaq — admin (təkrarlanan tapşırıqlar kimi) | Aylıq sıfırlama çoxlu tapşırığı dəyişir | — |
| Hədd: bir bağlanış ≤ 5000 tapşırıq; böyük layihə aydın xəta ilə dayanır (`last_error`, `period.close_failed` hadisəsi) | Bir tranzaksiya statement timeout-a çatmamalıdır | Partiyalarla bağlamaq: snapshot-ın atomarlığı itir |

## Nəticələr

- Miqrasiya 00037: `period_cycles`, `period_closes`, `period_snapshots` (FK-lər kaskad ilə; tapşırıq silinəndə
  snapshot sətri də silinir).
- API: `GET/PUT …/projects/{key}/cycle`, `POST …/cycle/preview`, `POST …/cycle/close`, `GET …/cycle/closes`,
  `GET …/cycle/closes/{YYYY-MM}`. Hadisələr: `cycle.updated`, `period.closed`, `period.close_failed`.
- UI: layihənin "Aylıq dövr" səhifəsi (ayarlar, növbəti tarix, əl ilə bağlama, bağlanmış aylar və hər ayın cədvəli
  cəmlərlə).
- Geri qaytarma: bağlanışın özü geri alınmır (köhnə dəyərlər snapshot-dadır, lazım olsa əl ilə və ya toplu
  redaktə ilə). Konfiqurasiyanı söndürmək növbəti bağlanışları dayandırır.

## Açıq suallar

- Snapshot-dan "bu ayın dəyərlərini geri yaz" düyməsi.
- Bağlanışdan sonra Mattermost/Telegram-a xülasə (ay cəmləri).
