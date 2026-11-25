# ADR-0045: Sorğu və tapşırıqlar üçün SLA hədəfləri (iş günlərində)

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0009](0009-due-dates-and-time-automation.md) (vaxt hadisələri, idempotent skanlar), [ADR-0017](0017-in-app-notifications.md) (bildirişlər),
  [ADR-0027](0027-working-day-calendar.md) (iş təqvimi), [ADR-0032](0032-intake-forms.md) (qəbul formaları),
  [ADR-0034](0034-approval-inbox-reapproval-deadlines.md) (təsdiq müddəti və xatırlatma), [ADR-0035](0035-design-and-theme.md) (Siqnal sütunu, növbələr),
  [ADR-0039](0039-archive-and-delete.md) (arxiv)

## Kontekst

Maliyyə/bek-ofis komandaları daxili sorğulara (ödəniş, sənəd, hesabat sorğusu — çox vaxt qəbul formasından gəlir) xidmət
səviyyəsi vəd edir: "3 iş günü ərzində həll", "1 iş günü ərzində ilk cavab". Son tarix (`due_date`) tək bir tarixdir və
əl ilə qoyulur; "sorğu nə vaxt gəldi və vədə nə qədər qaldı" sualına cavab vermir. Komandaya lazım olan: hər sorğuda vaxtın
avtomatik sayılması, pozulmuş və riskli sorğuların növbəsi və vaxtında xəbərdarlıq.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Layihə üzrə siyasət** (`project_sla_policies`, layihə başına bir sətir): istəyə bağlı `response_days` (1–60) və `resolution_days` (1–120), cavab ≤ həll; ikisi də `null` = siyasət silinir. Yalnız **admin+** dəyişir (`PUT …/projects/{key}/sla`), layihəni görən hər kəs oxuyur (`GET`). Audit: `project.sla_updated` (from/to) | Bir layihə = bir növbə/komanda; MVP üçün sadə. Hədəflər konfiqurasiyadır, avtomatlaşdırma ilə eyni rol | Tip üzrə hədəflər: faydalıdır, amma UI və prioritet qaydaları ikiqat artır — növbəti addım (sətir `item_type_id` ilə genişlənə bilər). Workspace üzrə: komandalar fərqli vəd verir |
| **Vahid: iş günü**, iş saatı yox. Müddət = sorğunun gəldiyi **yerli** günündən (workspace saat qurşağı) sonrakı N-ci iş gününün **sonu** (yerli gecə yarısı). Cümə 15:00 + 1 iş günü = bazar ertəsinin sonu; şənbə gələn sorğu bazar ertəsindən sayılır. Hesablama `recur.Calendar.AddWorkdays` ilə (ADR-0027: həftə sonu, bayramlar, köçürülmüş iş günləri) | Maliyyə əməliyyatları belə danışır ("3 iş günü"); iş saatı modeli (iş vaxtı 09–18, nahar, qısaldılmış bayramqabağı günlər) workspace-də olmayan məlumat tələb edir və istifadəçiyə izahı çətindir | İş saatları: iş vaxtı cədvəli + saat qurşağı + bayramqabağı qısa günlər; dəqiq, amma MVP üçün ağır. Təqvim günləri: bayramda "pozulma" — yanlış siqnal |
| **Saat** `max(items.created_at, policy.since)`-dən başlayır | Siyasəti yandırmaq köhnə backlog-u bir anda "pozulmuş" etmir və bildiriş seli yaratmır | Yaranma anından: siyasət yandırılan kimi yüzlərlə pozulma bildirişi |
| **İlk cavab** = statusun ilk dəfə `todo` kateqoriyasından çıxması **və ya** sorğu sahibindən (`reporter_id`) başqasının ilk şərhi (hansı əvvəldirsə). **Həll** = status `done` və ya `canceled` kateqoriyasına keçir; yenidən açılanda `resolved_at` silinir, saat əvvəlki müddətlə davam edir. Həll cavabı da sayır | Qəbul formasında sorğu sahibi öz şərhini yazanda bu "cavab" deyil; komandanın şərhi və ya işə götürmə cavabdır | Yalnız status: şərhlə cavab verən komanda "cavabsız" görünür. Yalnız şərh: işə götürmə sayılmır |
| **Dayandırma yoxdur**: status kateqoriyaları `todo/in_progress/done/canceled`-dır, "gözləmə" kateqoriyası yoxdur, ona görə saat heç vaxt dayanmır (sənədləşdirilib) | Kateqoriya əlavə etmək bütün iş axını, lövhə və hesabat məntiqinə toxunur | Status adına görə ("Gözləyir"): dil və adla bağlı kövrək qayda |
| **Saxlanma**: `item_sla` (tapşırıq başına bir sətir: `started_at`, `response_due`, `resolution_due`, `responded_at`, `resolved_at`, `computed_at`, dörd xatırlatma markeri). Siyahı sorğusu ona PK ilə `LEFT JOIN` edir; növbə filtrləri və skanlar qismən indekslərlə (`… WHERE resolved_at IS NULL`) | Filtr, sıralama, növbə sayğacı və skan ucuz qalır; `items` cədvəlinə toxunulmur (miqrasiya yüngül, Down təmiz) | `items`-ə sütunlar: böyük cədvəldə `ALTER` və hər yazıda geniş sətir. Oxunuşda hesablama: təqvim SQL-də yoxdur, filtr mümkün olmur |
| `responded_at`/`resolved_at`-i **DB triggerləri** yazır (`items` `AFTER UPDATE OF status_id`, `comments` `AFTER INSERT`) | Status hər yoldan dəyişir: UI, toplu redaktə, avtomatlaşdırma, təsdiqin tətbiqi, idxal, API tokenləri, chat kartları — bir yer unudulmur | Hər Go yolunda çağırış: unudulan yol = səhv SLA |
| **Hesablama**: tapşırıq yarananda eyni tranzaksiyada (`createItemTx`: UI, forma, təkrarlanma, şablon), siyasət dəyişəndə eyni tranzaksiyada ən çox 2000 tapşırıq; qalanı (idxal, köhnə tapşırıqlar, köçürülmüş tapşırıqlar, təqvim dəyişikliyi) worker-in dəqiqəlik tick-ində partiyalarla (500 × 5). Təqvim yazanı `project_sla_policies.changed_at`-i yeniləyir; `computed_at < changed_at` sətirləri yenidən hesablanır. Hesablayan workspace sətrini `FOR SHARE` kilidləyir (ADR-0027 kilid ardıcıllığı) — eyni anda dəyişən təqvim köhnə müddət yazdırmır | Siyasəti saxlayan admin nəticəni dərhal görür; ağır iş sərhədlidir | Hamısı sinxron: böyük layihədə uzun tranzaksiya. Hamısı worker-də: nişan bir dəqiqə gec görünür |
| **Vəziyyət** (serverdə, hər oxunuşda): hədəf başına `on_track` / `at_risk` (vaxtın **25%-dən azı** qalıb) / `breached` (müddət keçib və ya gec yerinə yetirilib) / `met`; ümumi vəziyyət ən pisidir. Növbə filtrləri yalnız **açıq** hədəfləri sayır (gec həll olunan tapşırıq tarixçədir, növbədə deyil) | Sabit, izahı asan hədd; eyni ifadə SQL-də (filtr, skan) və Go-da (nişan) | Sabit "1 gün qalıb": 1 günlük və 20 günlük hədəfdə eyni məna vermir |
| **UI**: Siqnal sütununda mətnli nişan (`SLA`, `SLA !`, `SLA ✕`, `SLA ✓` + `role="img"`, tam ad və müddətlər `aria-label`/`title`-da), paneldə vəziyyət sözlə və hər hədəfin müddəti; "SLA pozulub" və "SLA riski" növbələri (`?sla=breached|at_risk`, linkdə, saxlanmış görünüşdə yox) yalnız boş olmayanda və ya seçiləndə görünür; layihə ayarlarında "SLA hədəfləri" bölməsi | Rəng tək məlumat daşıyıcısı deyil; siyasəti olmayan layihədə əlavə səs-küy yoxdur | Ayrı SLA sütunu: Siqnal sütunu artıq bu rolu oynayır (ADR-0035) |
| **Bildirişlər**: `item.sla_at_risk` və `item.sla_breached` hadisələri (sistem, `target: response|resolution`, `due`), mövcud pipeline ilə: riskdə **icraçıya**, pozulmada icraçıya və workspace **admin/sahiblərinə** (ən çox 20; adminlər hər layihəni görür). Dedup: hər hədəf üçün "hansı müddət üçün göndərilib" markeri (`*_for`), `UPDATE … WHERE marker IS DISTINCT FROM due … FOR UPDATE SKIP LOCKED` — bir neçə worker replikası və təkrar tick ikinci hadisə yaratmır; müddət dəyişərsə (təqvim) yenisi üçün bir xatırlatma. Arxivlənmiş workspace/layihə skan olunmur | ADR-0034 ilə eyni model; webhook və avtomatlaşdırma hadisələri də görür | Hər tick yenidən göndərmək: spam. Ayrı marker cədvəli: bir sətir kifayətdir |

## Miqrasiya 00035

- Yeni cədvəllər `project_sla_policies`, `item_sla`, iki trigger funksiyası, `notifications_kind_check`-ə `sla_at_risk` və
  `sla_breached`. `lock_timeout = 5s`. `CREATE TRIGGER` `items`/`comments`-də qısa `SHARE ROW EXCLUSIVE` götürür (skan yoxdur);
  CHECK dəyişikliyi `notifications`-i bir dəfə yoxlayır (retensiya ilə məhdud).
- Down: cədvəllər, triggerlər və SLA bildirişləri silinir; hadisələr qalır. Hədəflər Down-dan əvvəl qeyd edilməlidir.

## Nəticələr

- Siyasəti olan layihədə hər tapşırıq SLA saatı alır — tip seçimi yoxdur (növbəti addım).
- Workspace saat qurşağı dəyişəndə müddətlər avtomatik yenidən hesablanmır (yalnız təqvim və siyasət dəyişikliyi); siyasəti
  yenidən saxlamaq kifayətdir. Növbəti addım: saat qurşağı dəyişikliyində də `markSLAStale`.
- Siyasət yarananda mövcud tapşırıqlar üçün `responded_at`/`resolved_at` təxminidir (`updated_at` və ya ilk komanda şərhi).
- Worker-in "sətirsiz tapşırıq" axtarışı siyasəti olan layihələrin tapşırıqları üzrə anti-join-dir: on minlərlə tapşırıqda
  millisaniyələr; yüz minlərdə qismən indeks və ya `items` üzrə marker lazım ola bilər.
- Hesabatlarda "SLA uyğunluğu %" KPI-ı və ana səhifə sayğacı bu ADR-də yoxdur (növbəti addım; məlumat `item_sla`-dadır).

## Açıq suallar

- "Gözləmə" kateqoriyası (müştəridən sənəd gözlənilir) əlavə olunmalıdırmı və saat orada dayanmalıdırmı?
- Pozulmada adminlər əvəzinə layihə üzrə "SLA sahibi" seçimi lazımdırmı (böyük workspace-lərdə 20 admin həddi)?
- İş saatı modeli (məs. "4 iş saatı ərzində ilk cavab") pilotda tələb olunacaqmı?
