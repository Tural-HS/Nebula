# ADR-0012: Təkrarlanan tapşırıqlar

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

FinTez-in Mattermost Boards qurulumunda təkrarlanan işlər (məs. "hər ayın 9-u müştərinin ödənişi") Python cron
skripti ilə edilirdi: qayda kartları ayrıca lövhədə, skript isə **2 il qabağa** kartlar yaradırdı. Nəticə:
bir lövhədə 1777 kart, brauzerin çökməsi, sahə sinxronizasiyası üçün əlavə məntiq, birbaşa bazaya yazma.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| Təkrarlanma layihəyə bağlı ayrıca obyektdir (`recurrences`): başlıq şablonu, təsvir, tip, icraçı, fərdi sahə dəyərləri, cədvəl | Kart deyil, qayda; tapşırıq siyahısını çirkləndirmir |
| Cədvəl növləri: hər N gündə; hər N həftədə (həftənin günləri); hər N ayda (ayın günü və ya **ayın son günü**); hər N ildə (ay + gün) | FinTez və tipik HR/maliyyə işlərini əhatə edir. Ayın 31-i olmayan aylarda son günə endirilir (31 yanvar → 28/29 fevral) |
| Başlanğıc tarixi, istəyə görə bitmə tarixi | Müqavilə/kredit müddəti bitəndə avtomatik dayanır |
| **Yalnız növbəti** tapşırıq yaradılır, son tarixindən `lead_days` gün əvvəl (default 3) | Yüzlərlə gələcək kart yaranmır; lövhə və cədvəl sürətli qalır. Gələcək tarixlər qaydanın "növbəti tarixlər" önbaxışında görünür |
| Yaradılan tapşırığın `due_date`-i = təkrarın tarixi | Gecikmə sistemi (ADR-0009) dərhal işləyir: ödəniş edilməyibsə "gecikir" + xülasə |
| Təkrarlanmanın qarşısı: `recurrence_runs (recurrence_id, due_date)` əsas açar; `next_due` hər dəfə irəli çəkilir | Bir neçə replika və ya təkrar işləmə eyni tarix üçün ikinci tapşırıq yaratmır |
| Server dayanıbsa, buraxılmış tarixlər növbəti dövrdə yaradılır (dövr başına məhdud say) | Heç bir ödəniş "itmir"; keçmiş tarixlilər dərhal gecikmiş kimi görünür |
| Yaratma eyni doğrulamadan keçir (`prepareItem`): tip, məcburi sahələr, workflow, icraçı üzvlüyü | Tək yaratma, idxal və təkrarlanma eyni qaydalara tabedir |
| Qayda artıq etibarlı deyilsə (məs. sonradan məcburi edilmiş sahə, icraçı workspace-dən çıxıb), təkrarlanma **söndürülür**, `last_error` yazılır və `recurrence.failed` hadisəsi yaranır | Hər dəqiqə eyni xətanın təkrarlanması yoxdur; admin UI-da səbəbi görür, düzəldib yenidən aktivləşdirir |
| Yaradılan tapşırıq adi `item.created` hadisəsidir (`source = 'system'`, payload-da `recurrence_id`) | Webhook-lar (Mattermost kanalına "yeni tapşırıq") və `item.created` qaydaları işləyir — FinTez-in səhər bildirişinin əvəzi |
| Başlıq şablonu: `{{date}}` (gg.aa.iiii), `{{month}}` (azərbaycanca ay adı), `{{year}}` | "Ofis icarəsi — {{month}} ödənişi" → "Ofis icarəsi — noyabr ödənişi" |
| İdarəetmə: admin+ (avtomatlaşdırma qaydaları kimi) | Avtomatik iş yaradan hər şey eyni rol səviyyəsindədir; layihə səviyyəsində rollar gələndə yenidən baxılacaq |
| Limitlər: workspace başına 1000 təkrarlanma, gündəlik təkrarlanmada `interval ≥ 1` | Səhv konfiqurasiya ilə sonsuz iş yaradılmasının qarşısı |

## Nəticələr

- "Plan" görünüşü (gələcək ödənişlərin siyahısı) kart yaratmadan, qaydalardan hesablanaraq göstərilə bilər;
  bu mərhələdə yalnız qayda başına növbəti 5 tarix önbaxışı var.
- Qaydanı dəyişmək artıq yaradılmış tapşırıqlara toxunmur (FinTez-dəki sahə sinxronizasiyası yoxdur):
  açıq tapşırıq yeni sahə dəyərini almaz, növbəti tarixdən etibarən yeni dəyərlər tətbiq olunur.
