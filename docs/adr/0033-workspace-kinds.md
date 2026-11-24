# ADR-0033: Komanda növünə görə workspace şablonları (Maliyyə, İT, HR)

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Nebula maliyyə və back-office komandaları üçün əməliyyat platformasıdır, amma eyni quraşdırmada İT və HR komandaları da
işləyir. Bu gün yeni workspace yalnız dil şablonu ilə yaranır (ADR-0020, ADR-0025): statuslar və iki iş növü. Maliyyə
komandası qalanını — sahələri, düsturu, "dörd göz" siyasətini, iş təqvimini, layihəni, görünüşləri və formanı — hər dəfə
əl ilə qurur; bu, yarım saatlıq iş və səhv mənbəyidir (məsələn, siyasəti unutmaq).

İstifadəçi qlobal "İT rejimi / Maliyyə rejimi" açarını **rədd etdi**: bu, iki məhsulu saxlamaq deməkdir (iki menyu,
iki davranış, hər funksiya üçün iki test yolu). Bunun əvəzinə yeni workspace komanda növünə uyğun **başlanğıc
konfiqurasiya** ilə yaranır; sonra hər şey adi qaydada Ayarlarda dəyişdirilir və məhsul bir olaraq qalır.

## Qərarlar

### 1. İki ox: dil və növ

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `POST /workspaces {slug, name, template, kind}`. `template` dildir (az/en/tr/ru, dəyişməyib); yeni `kind`: `general` (default, bugünkü davranış), `finance`, `it`, `hr`. Naməlum növ — 422 (`field: kind`) | Köhnə müştərilər `kind` göndərmir və eyni nəticəni alır; dil adları, növ isə məzmunu müəyyən edir — 4 × 4 kombinasiya bir cədvəldən yaranır | Hər kombinasiya üçün ayrıca şablon (`az-finance`, …): 16 şablon, tərcümə təkrarlanardı |
| Növ yalnız **yaradılma anında** tətbiq olunur və workspace-də saxlanılmır (yalnız `workspace.created` hadisəsində `kind`) | Sonra hər şey adi konfiqurasiyadır; "növ" davranışı dəyişmir, ona görə sütun və miqrasiya lazım deyil | `workspaces.kind` sütunu: heç bir kod onu oxumazdı — "rejim"ə aparan ilk addım olardı |
| Şablon məzmunu **koddur** (`internal/store/workspacekinds.go`), admin şablon redaktoru yoxdur | Məzmun versiyalanır, review-dan keçir, testlə yoxlanılır; 3 növ üçün redaktor ekranı və cədvəllər artıq yükdür | Verilənlər bazasında redaktə olunan şablonlar: miqrasiya, idarəetmə UI, şablonun özünün validasiyası və versiyası |
| Növlər həmişə dil şablonunun **avtomatlaşdırma qaydalarını** və **saat qurşağını** saxlayır; `general`-dan fərqli növ workflow-u, statusları və iş növlərini öz siyahısı ilə əvəz edir | Bildiriş qaydaları və saat qurşağı dilə/ölkəyə aiddir, komanda növünə yox (az → `Asia/Baku`) | — |
| İş növlərinin və sahələrin **açarları** ingiliscədir (`payment`, `vat`, `priority`), adlar dilə görə | ADR-0020 ilə eyni: açarlar API-də, düsturda, filtrdə identifikatordur | Açarları tərcümə etmək: `ru` workspace-də düstur başqa yazılardı |

### 2. Bir tranzaksiya, mövcud store funksiyaları

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Hər şey workspace ilə **bir tranzaksiyada** yaranır; bir addım alınmasa workspace də yaranmır | Yarımçıq workspace (siyasətsiz maliyyə workspace-i) olmur | Asinxron "quraşdırma" işi: yarımçıq vəziyyət görünərdi |
| Sahələr, bağlamalar, siyasət, təqvim, layihə, görünüşlər və formalar **API-nin istifadə etdiyi funksiyalarla** yaranır: `CreateField` → `createField(tx)`, `BindField`, `SetApprovalPolicy`, `AddAzerbaijanFixedHolidays`, `CreateProject`, `CreateView`, `CreateForm` tranzaksiya daxilində işləyən variantlara bölündü (ictimai funksiyalar onları `inTx` ilə çağırır, davranış dəyişmir) | Eyni validasiya (düstur kompilyasiyası, forma ↔ iş növü uyğunluğu, görünüş filtrləri), eyni limitlər və **eyni hadisələr**: `field.created`, `item_type.field_bound`, `approval_policy.set`, `calendar.preset_applied`, `project.created`, `view.created`, `form.created` — admin bunu əl ilə etmiş kimi | Birbaşa `INSERT`-lər: validasiyanın ikinci nüsxəsi və hadisəsiz dəyişikliklər (audit boşluğu) |
| Hadisələrin sahibi workspace-i yaradandır (owner); `workspace.created` birinci yazılır | Audit jurnalında ardıcıllıq təbiidir | Sistem aktoru (`actor_id NULL`): dəyişikliyi kimin başlatdığı itərdi |
| Statuslar və iş növləri əvvəlki kimi `installDefaultTemplate` ilə (hadisəsiz, `workspace.created` onları əhatə edir) | Dil şablonu ilə eyni yol; ayrıca status hadisələri yeni workspace üçün səs-küydür | — |

### 3. Maliyyə (`finance`)

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Statuslar: **Hazırlanır → Yoxlamada → Təsdiq gözləyir → Ödənildi**, Ləğv edildi; iş növləri: Ödəniş, Hesab-faktura, Hesabat | Ödəniş prosesinin real mərhələləri | — |
| Sahələr (Ödəniş və Hesab-faktura növlərinə bağlı, məcburi deyil): Məbləğ (`amount`, ədəd), **ƏDV dərəcəsi, %** (`vat_rate`, ədəd), ƏDV (`vat`, düstur), Kontragent (`counterparty`, mətn), Ödəniş tarixi (`payment_date`, tarix). Hesabat növünə sahə bağlanmır | Məcburilik formada tələb olunur; bağlamada məcburi sahə idxalı və toplu redaktəni çətinləşdirərdi | Hamısını məcburi bağlamaq: CSV idxalı və avtomatlaşdırma ilk gündən xəta verərdi |
| **ƏDV**: `az` workspace-də `round(amount * coalesce(vat_rate, 18) / 100, 2)`; digər dillərdə `round(amount * vat_rate / 100, 2)` (dərəcə yazılmayınca boş) | Azərbaycanda standart dərəcə **18%**-dir (Vergi Məcəlləsi, maddə 173.1: "hər vergi tutulan əməliyyatın və idxalın ƏDV dərəcəsi 18 faizdir"; [taxes.gov.az](https://www.taxes.gov.az/az/page/103-1), PwC Worldwide Tax Summaries 2026 ilə təsdiqlənib). Amma sıfır dərəcəli (ixrac, m. 165) və azad (m. 164) əməliyyatlar var: sabit `amount * 0.18` onlar üçün yanlış ƏDV göstərərdi. Dərəcə sahəsi boşdursa 18, azad ödəniş üçün 0 yazılır. Dil ölkəni bildirmir (`tr`/`ru` workspace Türkiyə və ya Rusiyada ola bilər, dərəcələr fərqlidir), ona görə orada default yoxdur | Sabit `round(amount * 0.18, 2)`: azad əməliyyatlarda səhv; dərəcə dəyişəndə bütün workspace-lərdə düsturu redaktə etmək lazım gələrdi |
| ƏDV məbləğə **əlavə** kimi hesablanır (məbləğ ƏDV-siz) | Hesab-fakturada adətən belə yazılır; ƏDV daxil məbləğ üçün admin düsturu `amount * r / (100 + r)` kimi dəyişə bilər | — |
| "Dörd göz": **Ödənildi**-yə daxil olmaq üçün siyasət, təsdiqçilər `admin` və `owner` rolları, `min_approvals = 1` (ADR-0026) | Ödənişin ikinci şəxs tərəfindən təsdiqi maliyyə nəzarətinin minimumudur | Siyasətsiz: ən çox unudulan addımdır |
| Nəticə: **təkbaşına sahib öz sorğusunu təsdiqləyə bilməz** (sorğu göndərən heç vaxt təsdiqçi deyil). Dialoq bunu xəbərdarlıq kimi göstərir: ən azı bir admin dəvət edin | ADR-0026 qaydası istisnasızdır | Tək sahib üçün istisna: dörd göz prinsipini pozardı |
| `az` workspace-də **cari ilin** (workspace saat qurşağında) rəsmi iş təqvimi, bu versiyada varsa (2026: 25 gün), yoxdursa sabit tarixli bayramlar (11 gün) — `AddAzerbaijanFixedHolidays` ilə eyni məntiq. Saat qurşağı dil şablonundan: `Asia/Baku` | Ödəniş və hesabat tarixləri iş günlərinə bağlıdır (`workdays()`, ayın son iş günü) | Növbəti ili də əlavə etmək: dekabrda rəsmi təqvim hələ dərc olunmaya bilər; admin Ayarlarda bir düymə ilə əlavə edir |
| Layihə **FIN** "Maliyyə" (workspace üçün açıq); ümumi görünüşlər: **Gecikənlər** (`due=overdue`, son tarixə görə) və **Təsdiq gözləyir** (status filtri, Məbləğ/ƏDV/Kontragent sütunları) | Komandanın gündəlik iki sualı | — |
| Forma **"Ödəniş tələbi"** (`<workspace>-payment-request`, auditoriya `members`, 5 faylaqədər): Kontragent, Məbləğ, Ödəniş tarixi məcburi; ƏDV dərəcəsi və təyinat isteğe bağlı; başlıq "Ödəniş: {{field.counterparty}}" | ADR-0032 ilə eyni; hesab-faktura fayl kimi əlavə olunur | — |

### 4. İT (`it`) və HR (`hr`)

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| İT: Növbədə (Backlog) → İcrada → Yoxlamada → Tamamlandı, Ləğv edildi; iş növləri Xəta, Tapşırıq, Sorğu; sahələr Prioritet (select: Aşağı/Orta/Yüksək/Kritik, rəngli) və Komponent (mətn); layihə **IT**; görünüşlər **Mənim işlərim** (`assignee=me`) və **Kritik** (Prioritet = Kritik seçiminin ID-si, yaradılanda müəyyən olunur); forma **"İT giriş sorğusu"** (Sistem, Əsaslandırma məcburi) | Kiçik helpdesk üçün kifayətdir; siyasət yoxdur | SLA, növbə, CMDB sahələri: tələb yoxdur, sonra əlavə etmək asandır |
| HR **kiçikdir**: Yeni → İcrada → Tamamlandı, Ləğv edildi; iş növləri İşə qəbul, Məzuniyyət, Tapşırıq; sahələr Əməkdaş, Vəzifə, Məzuniyyət növü (select), Başlama və Bitmə tarixi; formalar **"Məzuniyyət sorğusu"** və **"Yeni əməkdaş"**; görünüşlər Mənim işlərim, Gecikənlər; `az`-da iş təqvimi | Ən çox rast gələn iki HR prosesi | Əmək haqqı, qiymətləndirmə: ayrıca iş |
| HR layihəsi **gizlidir** (ADR-0013) | Kadr məlumatı (məzuniyyət, işə qəbul) həssasdır; qonaq kimi əlavə olunan əməkdaş açıq layihələri oxuyur, gizlini yox. Forma gizli layihəyə tələb qəbul edir (ADR-0032), tələb edən yalnız öz tələbini görür | Açıq layihə: hər üzv başqalarının məzuniyyətini görərdi |
| HR-də təsdiq siyasəti yoxdur | Məzuniyyət təsdiqi rəhbərdən asılıdır və təşkilata görə dəyişir; admin lazım olsa siyasəti özü əlavə edir | Default siyasət: tək HR adminli workspace-də sorğular ilişib qalardı |

### 5. Forma ünvanları

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Forma ünvanı `<workspace slug>-<şəkilçi>`; slug uzundursa kəsilir (≤ 63 simvol), ünvan artıq tutulubsa sonuna 4 təsadüfi hex əlavə olunur | Forma ünvanları bütün Nebula üzrə unikaldır; toqquşma workspace yaradılmasını pozmamalıdır | Toqquşmada xəta: istifadəçi "slug tutulub" görərdi, amma workspace slug-ı boşdur |

### 6. İnterfeys

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| "Yeni workspace" dialoqunda **"Komanda növü"**: radio kartlar (ad + nə yaradılacağının qısa təsviri), telefonda bir, `sm`-dən iki sütun; default **Ümumi** | Seçimin nəticəsi seçimdən əvvəl görünür; kartlar toxunma üçün böyükdür; dialoq onsuz da sürüşür. Başlığa heç nə əlavə olunmayıb (ADR-0019) | Select: təsvirlər sığmır |
| Maliyyə seçiləndə "ikinci admin lazımdır" xəbərdarlığı; `az` + Maliyyə/HR seçiləndə "rəsmi iş təqvimi əlavə olunur" qeydi | Sürprizlər əvvəlcədən deyilir | — |
| `general` sorğuda göndərilmir (`createWorkspaceBody`) | Sorğu köhnə serverlərin qəbul etdiyi forma ilə eynidir | — |
| Sətirlər yeni lüğət modulundadır (`locales/<dil>/kinds.ts`, `i18n-kinds.ts`), 4 dildə (ADR-0025) | — | — |

## Nəticələr

- **Miqrasiya yoxdur** — 00026 nömrəsi istifadə olunmadı; `CHECK` dəyişiklikləri də yoxdur.
- API: `POST /api/v1/workspaces` yeni `kind` sahəsi qəbul edir; yeni endpoint yoxdur.
- Testlər: `internal/server/workspace_kinds_integration_test.go` (hər növ üçün statuslar, iş növləri, sahələr və bağlamalar,
  siyasət, layihə, görünüşlər, formalar, təqvim günləri, hadisələr; formadan göndəriş → ƏDV = 180, təsdiqsiz "Ödənildi" → 409;
  `ru` + `finance` adları, `general` dəyişməyib, tutulmuş forma ünvanı, 63 simvollu slug), `web/src/lib/workspaceKinds.test.ts`,
  E2E `web/e2e/workspace-kinds.spec.ts`.
- Şablon məzmununu dəyişmək kod dəyişikliyidir və yalnız **yeni** workspace-lərə təsir edir; mövcud workspace-lər toxunulmur.
- Açıq suallar: maliyyə formasının auditoriyası `members`-dir — digər şöbələrdən tələb edənlər qonaq kimi əlavə olunsa, açıq
  FIN layihəsini oxuyacaqlar; belə halda admin formanı `accounts` auditoriyasına keçirməli və ya layihəni gizli etməlidir.
  Növbəti ilin rəsmi təqvimi dərc olunanda onu mövcud workspace-lərə avtomatik əlavə etmək ayrıca qərardır.
