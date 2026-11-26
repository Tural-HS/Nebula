# ADR-0056: Avtomatlaşdırma əməliyyatı — sahələri başqa layihəyə köçür (`copy_fields`)

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0005](0005-automation-webhooks-email.md) (avtomatlaşdırma qaydaları), [ADR-0051](0051-monthly-period-close.md) (aylıq dövr)

## Kontekst

FinTez-də eyni müştərilər iki layihədədir: **REY** (lizinq reyestri) və **MAH** (aylıq müştəri hesabatı). Müştəri
hər iki layihədə `nv_number` mətn sahəsi ilə tanınır. REY-də qalıq, aylıq ödəniş, telefon və s. dəyişəndə MAH-dakı
eyni `nv_number`-li kartda da həmin dəyərlər olmalıdır. İndi bunu əllə edirlər və dəyərlər ayrılır.

## Qərar

Yeni əməliyyat növü:

```json
{"type": "copy_fields", "project": "MAH", "match": "nv_number", "fields": ["balance", "monthly_payment"]}
```

Qayda işləyəndə tətikləyən tapşırığın `match` dəyəri oxunur, hədəf layihədə həmin dəyərə malik canlı (silinməmiş)
tapşırıqlar tapılır və onlara `fields`-dəki sahələrin dəyərləri yazılır.

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Hədəf layihə **açarla** saxlanılır (`project: "MAH"`) | Layihə açarı dəyişmir; qaydanı JSON-da oxumaq asandır | Layihə ID-si: oxunmur, API-nin qalanı açarla işləyir |
| Uyğunlaşdırma sahəsi yalnız **mətn və ya rəqəm** sahəsi ola bilər, dəqiq bərabərlik (`fields @> {"k": v}`, GIN indeksi) | Müqavilə/NV nömrəsi kimi identifikatorlar bu tiplərdədir; rəqəmlər dəyərlə müqayisə olunur (`5` = `5.0`) | Seçim/tarix/istifadəçi: identifikator deyil; hərf variantlarına görə uyğunluq: səhv müştəriyə yazmaq riski |
| Köçürülən sahələr: 1–30, təkrarsız, mövcud və arxivlənməmiş; düstur sahəsi və uyğunlaşdırma sahəsinin özü olmaz | Düstur hesablanır, yazıla bilməz; uyğunlaşdırma sahəsini köçürmək mənasızdır | — |
| Qayda başına hər hədəf layihəyə **bir** `copy_fields` | İki əməliyyat eyni sahəyə fərqli yaza bilməzdi | — |
| Hədəf layihə saxlanarkən mövcud və arxivlənməmiş olmalıdır; icra anında arxivlənib/silinirsə əməliyyat logla ötürülür | Arxivlənmiş layihə yalnız oxunandır (ADR-0039) | Job-u xəta ilə bitirmək: təkrar cəhd kömək etməz |
| Mənbə dəyərləri tapşırığın **icra anındakı** vəziyyətindən oxunur (eyni qaydanın `set_*` əməliyyatlarından sonra), şərtlər isə əvvəlki kimi hadisə snapshot-ı ilə | Hədəf həmişə mənbənin son vəziyyətinə yaxınlaşır; ardıcıl iki redaktədə köhnə dəyər yazılmır | Snapshot dəyərləri: gecikən job köhnə dəyəri yaza bilərdi |
| Mənbədə `match` boşdursa — heç nə edilmir | Boş dəyər bütün "boş" kartlara uyğun gələrdi | — |
| Eyni layihəyə köçürmə edilmir (mənbə = hədəf layihə), tətikləyən tapşırıq heç vaxt hədəf deyil | Layihədaxili dublikatları sinxronlaşdırmaq bu ADR-in məqsədi deyil | — |
| Ən çox **20** uyğun tapşırıq (nömrəyə görə), artığı logda xəbərdarlıq ilə ötürülür | Səhv qayda (məs. `match` hamıda eyni dəyər) yüzlərlə kartı yenidən yazmasın | Limitsiz: kütləvi səhv yazılış |
| Yalnız **fərqlənən** dəyərlər yazılır; patch boşdursa yeniləmə edilmir (hadisə, versiya artımı yoxdur) | Sinxron cütlük heç nə yazmır; tarixçə səs-küysüz qalır | Hər dəfə yazmaq: hər redaktədə boş `item.updated` |
| Mənbədə boş (yoxdur/`null`) dəyər hədəfdəki dəyəri **silir** | "Eyni dəyərlər" tələbi: REY-də silinən telefon MAH-da qalmamalıdır | Boşu ötürmək: dəyərlər yenə ayrılırdı |
| Hədəfin tapşırıq tipi sahəni istifadə etmirsə, həmin sahə həmin hədəf üçün ötürülür (silmə üçün bağlılıq lazım deyil) | Bir sahə xətası bütün patch-i rədd etməsin | — |
| Yazılış adi `UpdateItem` ilə: API ilə eyni sahə yoxlaması (pul, rəqəm, tələb olunan sahələr, üzvlük), audit hadisəsi, webhook-lar | Avtomatlaşdırma API-dən yan keçə bilməz | Birbaşa SQL: yoxlama və audit itərdi |
| Versiya konfliktində tapşırıq yenidən oxunur və **bir dəfə** təkrar cəhd edilir; yenə konfliktdirsə, qalan hədəflər bitəndən sonra job təkrarlanır (sinxron hədəflər o zaman heç nə yazmır) | İnsan redaktəsi ilə paralel işləmə, dəyər itmir | Konflikti udmaq: hədəf köhnə qalardı |
| Hədəf səviyyəli rədd (yoxlama xətası, təsdiq kilidi `approval_pending` / `approval_required` / `reapproval_required`, silinmiş tapşırıq) logla ötürülür, digər hədəflər davam edir, job uğurla bitir | Təkrar cəhd eyni nəticəni verərdi; bir kart bütün sinxronizasiyanı dayandırmasın | Job-u qalıcı xəta ilə bitirmək |
| Yazılışlar `source = automation` və qaydanın ID-si ilə qeyd olunur, ona görə **heç bir qaydanı tətikləmir** (ADR-0005) | Sonsuz dövrə mümkün deyil: REY→MAH və MAH→REY qaydaları birlikdə olsa da, avtomatik yazılış qarşı qaydanı işə salmır | Dərinlik sayğacı: mövcud qoruma kifayətdir |
| Aylıq bağlanışın (ADR-0051) `item.updated` hadisələri üçün `copy_fields` işləmir (digər əməliyyatlar işləyir) | Bağlanış bütün layihəni sıfırlayır; bu sıfırlama başqa layihəyə (məs. MAH→REY qaydası ilə reyestrə) köçməməlidir | — |
| `create: true` (2026-11-26): uyğun hədəf yoxdursa, hədəf layihədə yeni tapşırıq yaradılır — mənbənin adı, uyğunlaşdırma dəyəri və siyahıdakı sahələrlə; növ hədəf layihədə ən çox işlədilən növdür. Uyğunlaşdırma dəyəri boşdursa heç nə yaranmır. Eyni dəyər üçün advisory kilid və tranzaksiya daxilində yenidən yoxlama iki paralel işin iki tapşırıq yaratmasının qarşısını alır | Reyestrə yeni müştəri əlavə olunanda aylıq hesabatda da avtomatik yaransın | Ayrı "yarat" əməliyyatı: eyni uyğunlaşdırma məntiqini təkrarlayardı |

### Bilinən məhdudiyyətlər və risklər

- Avtomatik yazılış qaydaları tətikləmədiyi üçün zəncir işləmir: REY→MAH köçürməsindən sonra MAH-dakı
  "dəyişəndə" qaydaları (email, digər `copy_fields`) işə düşmür. Bu, dövrələrə qarşı qəsdən seçilib.
- REY→MAH və MAH→REY qaydaları birlikdə qurulubsa, son redaktə qalib gəlir. Bir istiqamət tövsiyə olunur.
- Mənbə və hədəf eyni workspace-dədir; workspace-lər arası köçürmə yoxdur.
- Hədəf layihənin gizli olması (ADR-0013) yazmağa mane olmur: qaydanı yalnız admin qurur.
- Qayda yaradılandan sonra sahə arxivlənsə və ya tipi dəyişsə, icra zamanı `UpdateItem` xətası logda görünür.

## Nəticələr

- Miqrasiya yoxdur: əməliyyatlar `automation_rules.actions` (jsonb) sütunundadır. `Action` strukturuna `project`,
  `match`, `fields` (hamısı `omitempty`) əlavə olundu.
- UI: Ayarlar → Avtomatlaşdırma → "Sahələri başqa layihəyə köçür": hədəf layihə, uyğunlaşdırma sahəsi və
  köçürüləcək sahələrin siyahısı (işarələmə ilə, ≤ 30); qaydalar siyahısında `MAH: Qalıq, Aylıq ödəniş`. 4 dil.
- FinTez: REY layihəsində `item.updated` (və istəyə görə `item.created`) qaydası, şərt `project = REY`,
  əməliyyat `copy_fields` → MAH, `match: nv_number`.
