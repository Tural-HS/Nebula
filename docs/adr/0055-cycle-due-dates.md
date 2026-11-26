# ADR-0055: Aylıq dövrlə idarə olunan ödəniş son tarixləri

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0051](0051-monthly-period-close.md) (aylıq dövr), [ADR-0009](0009-due-dates-and-time-automation.md) (son tarixlər və planlayıcı)

## Kontekst

MAH layihəsində hər kart bir müştəridir: `loan_date` (kreditin verildiyi tarix) və `paid` (bu ay ödədi) sahələri
var. Müştəri hər ay `loan_date`-in günündə ödəməlidir. İndiyə qədər son tarix (`due_date`) əl ilə yazılırdı və aylıq
bağlanış "Ödədi"-ni silərkən gecikmiş, ödəməmiş müştərini də "təmiz" vəziyyətə qaytarırdı — gecikmə itirdi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Dövrdə iki istəyə bağlı ayar: `due_day_field` (aktiv `date` sahəsi) və `paid_field` (aktiv `boolean` sahəsi, yalnız `due_day_field` ilə) | Son tarix dövrün işidir: bağlanış onsuz da hər ay kartları yeniləyir | Sahə səviyyəsində "aylıq təkrar": eyni sahə bir neçə layihədə ola bilər |
| Son tarix = yeni ayda tarixin **günü**, ayın uzunluğuna sıxılır (31 → 30 noyabr, 28/29 fevral) | Ödəniş heç vaxt buraxılmır | Növbəti ayın 1-i: müştəriyə yanlış tarix |
| Tarix cari ayda və ya sonradırsa (bu ay verilmiş kredit), ilk son tarix **tarixdən sonrakı ay** | Kredit verildiyi gün ödənmir; əks halda yeni kart dərhal gecikmiş olardı | Həmişə cari ay: yeni müştəri ilk gün "gecikmiş" görünür |
| Bağlanışda (N = bağlanan aydan sonrakı ay): `paid` true deyil, status `todo`/`in_progress` və `due_date` < icra günü olan kart **köçürülür**: sıfırlanmır (status, sahələr, son tarix qalır), snapshot-da var; sayı `period_closes.carried` və önizləmədə `will_carry` | Gecikmə serverin öz "gecikmiş" qaydası ilə eynidir və davam edir | Sıfırlayıb "borc" sahəsi yazmaq: əlavə sahə, mürəkkəb |
| Bağlı (`done`/`canceled`) kartlar heç vaxt köçürülmür, `paid` nə olursa olsun | Onlar gecikmiş sayılmır; köhnə kartlarda `paid` boş ola bilər | — |
| Qalan kartlarda adi sıfırlama + `due_date` = N-də gün; tarix boşdursa son tarix dəyişmir. Dəyişiklik `UpdateItemParams.DueDate` ilə | Audit, `item.updated`, avtomatlaşdırma və webhook-lar əl ilə dəyişiklik kimi işləyir | Birbaşa SQL: hadisəsiz |
| Gündəlik addım (`EmitCycleDueDates`, `EmitDateReminders`-dən sonra, hər tick): (a) `due_date` boş və tarixi olan kartlara cari ayda son tarix; (b) **roll**: `paid` true, `due_date` cari aydan əvvəl və **həmin ay artıq bağlanıb** (`period_closes.period` ≥ son tarixin ayı) olan karta dövrün sıfırlaması (eyni `cycleChanges`/`statusChange`, refill daxil) + cari ayda son tarix | Gecikən ödəyəndə növbəti ay üçün kart təmizlənir; ay bağlanmamış roll etmək snapshot-u itirərdi | Ödəniş anında trigger: snapshot qoruması yoxdur, əl ilə dəyişikliyə gizli yan təsir |
| Gündəlik addım idempotentdir (dəyişən kart artıq namizəd deyil); `period_cycles` sətri `FOR UPDATE SKIP LOCKED` (bağlanışla və digər replikalarla toqquşmur), kartlar `FOR UPDATE OF i SKIP LOCKED`; tick-də ≤ 500 redaktə, ≤ 50 layihə | Bir neçə replika və hər dəqiqəlik tick təhlükəsizdir | Gündə bir dəfə (`last_run`): yeni kartın son tarixi bir gün gecikərdi |
| Dəyişikliklər `item.updated`, `payload.cycle` ilə işarələnir (`{"due_date": true}` və ya `{"roll": true, "period": "YYYY-MM"}`), aktor sistem | `isCycleReset` Slack kanalına hər kart üçün sətir göndərmir (ilk açılışda yüzlərlə kart); avtomatlaşdırma qaydaları sistem hadisəsi kimi işləyir, nebula formatlı webhook-lar hər dəyişikliyi alır | İşarəsiz: kanal daşqını |
| Validasiya: `due_day_field` silinən/doldurulan sahə ola bilməz; `paid_field` hər ay silinməli və ya doldurulmalıdır | Əks halda son tarix itər və ya bir dəfə ödəyən heç vaxt gecikmiş olmaz | Xəbərdarlıq: səssiz səhv konfiqurasiya |

## Nəticələr

- Miqrasiya 00041: `period_cycles.due_day_field`, `period_cycles.paid_field` (NULL), `period_closes.carried`
  (`DEFAULT 0`). Down yalnız bu ayarları və sayları silir; kartlardakı son tarixlər qalır.
- API: `GET/PUT .../cycle` gövdəsində `due_day_field`, `paid_field` (`null` = söndürülüb, boş sətir də `null`);
  bağlanış və siyahıda `carried`, önizləmədə `will_carry`; `cycle.updated` auditində hər iki sahə.
- Bilinən məhdudiyyət: sıfırlanması daim alınmayan kart (məsələn, təsdiq kilidi, workflow-un qadağan etdiyi keçid)
  hər tick yenidən cəhd edilir; layihədə 500-dən çox belə kart olarsa, digərləri gözləyə bilər.
- FinTez: MAH-da `due_day_field = loan_date`, `paid_field = paid` (`paid` artıq `clear_fields`-dədir).
