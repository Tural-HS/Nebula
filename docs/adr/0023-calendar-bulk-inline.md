# ADR-0023: Təqvim görünüşü, toplu və sətirdaxili redaktə

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

FinTez komandası ödəniş və hesabat tapşırıqlarını son tarixə görə planlayır: "bu ay nə var" sualına cədvəl
(sıralama ilə belə) zəif cavab verir. Eyni zamanda onlarla tapşırığın statusunu, icraçısını və ya son tarixini
bir-bir tapşırıq panelində dəyişmək yavaşdır; Jira/YouTrack-dakı kimi toplu dəyişiklik və cədvəl xanasında
birbaşa redaktə gözlənilir. Mövcud qaydalar dəyişməməlidir: workflow keçidləri, icraçının layihəyə girişi
(ADR-0013), sahə validasiyası, optimistik kilid (`version`), hər dəyişiklik üçün `item.updated` hadisəsi
(webhook-lar, avtomatlaşdırma, aktivlik) və 429-da təkrar (ADR-0002).

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Yeni görünüş rejimi `calendar` (`?view=calendar`), ay `?cal=YYYY-MM` (yoxdursa — workspace saat qurşağında bu ay). Ay **görünüş vəziyyətinə daxil deyil**: aylar arası keçid saxlanılan görünüşü "dəyişdirilib" etmir | Görünüş = filtrlər + rejim; ay naviqasiyadır, `?item` kimi | Ayı görünüşdə saxlamaq: hər ay keçidində "dəyişikliyi saxla" təklifi |
| Saxlanılan görünüşün `kind`-i `calendar` ola bilər. DB-də `saved_views.kind` CHECK-i (00011) **miqrasiya 00015** ilə genişləndirilir (`NOT VALID` + `VALIDATE`, məlumat dəyişmir). Görünüş **ayı saxlamır**: açılanda həmişə bu ay (və ya linkdəki `?cal`) göstərilir. Down `calendar` görünüşü varsa aydın xəta ilə dayanır | API müqaviləsi dürüst qalır (`kind` nə görünürsə odur) | `kind=table` + config-də gizli bayraq: köhnə klient təqvimi cədvəl kimi göstərər, API yalan danışar |
| Ay şəbəkəsi **bazar ertəsindən**, tam həftələr (5–6 sətir). Tarix hesabı UTC tarixləri üzərindədir (`lib/calendar.ts`), "bu gün" `Intl` ilə workspace saat qurşağındadır | `due_date` saat qurşağısız təqvim tarixidir (ADR-0009); brauzerin yerli saatı və DST günü sürüşdürməməlidir | `Date` yerli vaxtla: DST gecəsi və başqa qurşaqdakı istifadəçi üçün bir gün sürüşmə |
| Server: `GET …/items?due_from=&due_to=` (daxil olmaqla, `YYYY-MM-DD`), **ən çox 366 gün**, sıra yoxlanılır, hər parametr bir dəfə. Digər filtrlərlə AND; `counts` və `sums` də eyni filtri alır | Şəbəkə bir sorğu ilə (səhifə-səhifə) yüklənir; mövcud `items_project_due_number_idx (project_id, due_date, number)` indeksi aralıq skanını verir — **yeni indeks və miqrasiya lazım deyil** | Hər gün üçün ayrı sorğu: 42 sorğu |
| Təqvim aralığı `sort=due`, 200-lük səhifələrlə, **ən çox 1000 tapşırıq** (5 sorğu); çox olarsa "filtrlə daraldın" qeydi. Gündə 3 yer: 2 kart + "+N daha" (dialoqda hamısı) | Sıx ayda belə sabit yük; "+1 daha" heç vaxt tək kartı gizlətmir | Limitsiz yükləmə: 10k tapşırıqlı layihədə brauzer donur |
| Son tarixsiz tapşırıqlar şəbəkənin yanında ayrı siyahıdadır (`due=none`, səhifəli). Karta sürükləyib günə buraxmaq tarixi təyin edir, siyahıya qaytarmaq silir | Planlama axını: "tarixsizdən günə" | Ayrı səhifə: kontekst itir |
| Sürükləmə ADR-0019 sensorları ilə (siçan 6 px, toxunma 250 ms, klaviatura Space/oxlar/Enter). Dəyişiklik **optimistikdir**: kart dərhal yeni günə keçir, səhv olarsa (409, 422) geri qayıdır və xəbərdarlıq çıxır | Lövhə ilə eyni hiss; versiya yoxlaması tək redaktədəki kimidir | — |
| Telefonda (`useCompact`) şəbəkə əvəzinə **ayın gündəmi**: yalnız tapşırığı olan günlər, sticky gün başlıqları, sonda açılan "Son tarixi yoxdur" bölməsi. Gündəmdə sürükləmə yoxdur — tarixi panel dəyişir | 375 px-də 7 sütun oxunmur (ADR-0019) | Kiçildilmiş şəbəkə: 50 px-lik xanalar |
| Toplu redaktə: `POST …/items/bulk`, gövdə `{items: [{number, version}], status_id?, assignee_id?, due_date?, fields?}` və ya `{items, delete: true}`. **Ən çox 500** tapşırıq, dublikat və boş dəyişiklik 422 | Açıq limit: 500 tapşırıq bir tranzaksiyada bir neçə min ifadədir — sorğu (`RequestTimeout`) və `statement_timeout` (30 s) daxilində | Limitsiz: uzun kilidlər, timeout |
| **Hamısı-və-ya-heç-nə**, bir tranzaksiya; hər tapşırıq öz **savepoint**-ində işlənir ki, bütün xətalar bir dəfəyə toplansın. Xəta varsa hər şey geri alınır və cavab `bulk_failed` + tapşırıq-tapşırıq siyahı (`number, code, message, field`) olur: hər hansı `version_conflict` varsa **409**, yoxsa **422** | İstifadəçi "nə oldu, nə olmadı" fikirləşmir; avtomatlaşdırma yarımçıq vəziyyət görmür. Tam siyahı bir düzəlişlə hamısını həll etməyə imkan verir | Qismən uğur (207): UI-da və webhook-larda yarımçıq vəziyyət; ilk xətada dayanmaq: xətaları bir-bir tapmaq |
| Hər tapşırıq **tək redaktə ilə eyni koddan** keçir (`updateItemTx` / `deleteItemTx`): workflow keçidi, statusun tipin workflow-una aidliyi, icraçının layihəyə girişi, sahə validasiyası, `version` yoxlaması. Hər dəyişən tapşırıq üçün ayrıca `item.updated` (silinmə üçün `item.deleted`) hadisəsi yazılır; dəyişməyən tapşırıq hadisə yazmır | Webhook-lar, avtomatlaşdırma və aktivlik toplu dəyişikliyi tək-tək dəyişiklikdən ayırmır | Bir "item.bulk_updated" hadisəsi: bütün inteqrasiyalar yeni formatı öyrənməlidir |
| Audit: hər hadisənin payload-ında `bulk: {id, size}`; server logunda bir `bulk item change` sətri (`bulk_id`, actor, say, `request_id`). Audit log mövcud hadisələr cədvəlidir | Toplu dəyişikliyin bütün izləri bir id ilə tapılır | Ayrı audit cədvəli: miqrasiya, ikinci həqiqət mənbəyi |
| Sətirlər **nömrə sırası ilə** kilidlənir | Üst-üstə düşən iki toplu dəyişiklik deadlock deyil, növbə gözləyir | — |
| İcazələr: toplu redaktə — `member`+ (tək redaktə kimi); **toplu silmə — yalnız `admin`+**. Layihəyə giriş `loadProject`-dədir (gizli layihə 404) | Bir kliklə yüzlərlə tapşırıq silinməsi tək silmədən təhlükəlidir | Member üçün toplu silmə: səhv seçimdə böyük itki |
| **429-da təkrar təhlükəsizdir**: rate limiter sorğunu işləmədən rədd edir, üstəlik hər tapşırıq `version` daşıyır — artıq commit olunmuş sorğu təkrar göndərilsə bütövlükdə `version_conflict` ilə rədd olunur, iki dəfə tətbiq olunmur. Ona görə ayrıca idempotency açarı lazım deyil | Optimistik kilid artıq idempotentlik verir | `Idempotency-Key` cədvəli: miqrasiya və saxlama müddəti, fayda yoxdur |
| UI: cədvəldə sətir checkbox-ları və "Hamısını seç" — cari filtrlənmiş nəticə, lazım olsa növbəti səhifələri yükləyərək, **500-ə qədər**. Filtr və ya rejim dəyişəndə seçim sıfırlanır. Alətlər paneli: status, icraçı, son tarix (təyin/sil), sahə dəyəri, silmə (admin, təsdiqlə). Uğursuzluqda dialoq hər xətalı tapşırığı və səbəbini göstərir | Seçim həmişə görünən siyahıya aiddir; gizli qalmış seçim yoxdur | Serverdə "filtrə uyğun hamısı": istifadəçi nəyi dəyişdiyini görmür, limit də itir |
| Sətirdaxili redaktə: status (yalnız icazəli keçidlər, `/transitions`), icraçı (layihəni görə bilənlər), son tarix və `select`/`user`/`number`/`text`/`date`/`url`/`email` sahələri. Xana düymədir (`aria-label`: "QA-1: Status dəyiş"); Enter/xanadan çıxma saxlayır, **Escape ləğv edir**, fokus xanaya qayıdır. Siyahıda dərhal yeni dəyər görünür, xəta olarsa (409/422) əvvəlki keşə qayıdır. Başlıq düyməsi tapşırığı açır, sətirin qalan hissəsinə klik də | Panel açmadan sürətli redaktə; klaviatura ilə tam istifadə | `contenteditable` cədvəl: virtualizasiya və a11y ilə çətin |
| Klaviatura ilə `<select>`-də oxlar dəyəri dərhal saxlamır (bəzi brauzerlərdə oxlar bağlı select-i dəyişir) — Enter və ya xanadan çıxma lazımdır; siçanla seçim dərhal saxlanılır | Təsadüfi status dəyişikliyi olmasın | Hər `change`-də saxlamaq: Windows-da hər ox bir PATCH |
| Checkbox-lar və sətirdaxili redaktorlar yalnız **dəqiq göstəricidə** (`pointer: fine`) və geniş ekranda. Toxunma ekranında sətir bir toxunma hədəfidir (ADR-0019) və redaktə panelindədir | Toxunmada kiçik xanalar səhv toxunuşa səbəb olur, sətir açılmır | Planşetdə də: mobil E2E-də sətirdə bir neçə düymə, təsadüfi redaktə |

## Nəticələr

- `UpdateItem` və `DeleteItem` tranzaksiya daxilində işləyən hissələrə bölündü (`checkUpdate`, `updateItemTx`,
  `deleteItemTx`); tək redaktənin davranışı dəyişmədi. Toplu silmə də `version` yoxlayır (köhnə seçim
  arada dəyişmiş tapşırığı silmir).
- 500 tapşırıqlı toplu dəyişiklik 500 hadisə və 500 dispatch işi yaradır; avtomatlaşdırma qaydaları hər biri
  üçün işləyir. Bu gözlənilən davranışdır, amma böyük workspace-də worker növbəsini qısa müddətə artırır
  (ADR-0018 növbə metrikləri ilə görünür).
- Təqvim ayda ən çox 1000 tapşırıq göstərir; daha çoxu üçün filtr lazımdır.
- Toplu alətlər panelindəki status siyahısı bütün tiplərin statuslarının birləşməsidir; başqa workflow-dakı
  tapşırıq üçün bu, tapşırıq-tapşırıq 422 verir və heç nə dəyişmir — xəta siyahısı bunu izah edir.
- E2E: `web/e2e/calendar.spec.ts` (yerləşmə, "+N daha", naviqasiya, gecikmə, sürükləmə, telefon gündəmi) və
  `web/e2e/bulk.spec.ts` (toplu status/tarix, köhnə versiya ilə hamısı-və-ya-heç-nə, admin silməsi,
  sətirdaxili redaktə, Escape). `E2E_API_URL` fixture-lərə əlavə edildi: E2E başqa API-yə (məs. branch
  build-ə) qarşı işlədilə bilər.
