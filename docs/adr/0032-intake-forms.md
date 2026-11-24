# ADR-0032: Qəbul formaları (intake forms) və "Tələblərim"

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Maliyyə və back-office komandalarına (ilk müştəri — FinTez) tələblər başqa şöbələrdən gəlir: ödəniş tələbi, sistemə
giriş tələbi, xərcin təsdiqi. Bu gün belə tələblər email, chat və ya şifahi gəlir; komanda onları əl ilə tapşırığa
çevirir, tələb edən isə "nə oldu?" deyə təkrar soruşur. Lazım olan: admin layihə üçün forma qurur, tələb edən onu
doldurur, hər göndəriş sahələri doldurulmuş tapşırıq olur, tələb edən öz tələblərinin statusunu görür.

Forma yeni sahə sistemi deyil: ADR-0003-ün fərdi sahələrinin **seçilmiş alt çoxluğudur** (öz etiketi və kömək mətni
ilə) və eyni validatorlardan keçir. Tapşırıq adi tapşırıqdır: gizlilik (ADR-0013), təsdiqlər (ADR-0026), fayllar
(ADR-0022), avtomatlaşdırma və webhook-lar (ADR-0005) onun üzərində dəyişmədən işləyir.

## Təhlükəsizlik qərarları

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| v1-də formalar **yalnız daxil olmuş istifadəçilər** üçündür (Keycloak OIDC, mövcud auth middleware) | Hər göndərişin sahibi var: rate limit istifadəçiyə bağlanır, audit jurnalında `actor_id` real şəxsdir, sui-istifadə edən hesab söndürülür (ADR-0002) | Anonim ictimai forma: bax aşağıda — ayrıca, təxirə salınmış qərardır |
| Auditoriya (`audience`) iki cürdür: **`members`** — formanın workspace-inin istənilən üzvü (qonaq daxil); **`accounts`** — bu Nebula-ya daxil ola bilən hər kəs. Standart `members` | Şöbələrarası tələbdə tələb edən çox vaxt back-office workspace-inin üzvü deyil; onu üzv etmək (hətta qonaq kimi) ona layihələri göstərər | Yalnız üzvlər: hər tələb edən üçün qonaq dəvəti lazım gələrdi, qonaq isə workspace-in bütün açıq layihələrini oxuyur |
| Üzv olmayan tələb edən **heç bir workspace girişi almır**: `workspace_members`-ə yazılmır, layihə/tapşırıq/axtarış/üzv siyahısı 404-dür; yalnız `/me/requests` (öz göndərişləri) və formanın özü | "Tələblərim" ayrıca, workspace-dən kənar marşrutdur və yalnız `submitter_id = mən` sətirlərini oxuyur | Tələb edəni tapşırığın oxucusu etmək: tapşırığın şərhləri, sahələri, faylları və tarixçəsi daxili məlumatdır |
| Auditoriyaya daxil olmayan üçün forma **mövcud olmayan forma ilə eyni** 404 cavabını alır (həm `GET`, həm göndəriş) | Üzv olmayanlar `members` formalarının ünvanlarını yoxlayaraq workspace-ləri kəşf edə bilməz | 403: formanın varlığını açır |
| Tələb edənə göstərilən forma (`PublicForm`) yalnız formanın adı, giriş mətni, workspace-in **adı**, etiketlər, kömək mətnləri, sahə tipləri və aktiv seçimlərdir. Layihənin açarı/adı, digər tapşırıqlar və üzv kataloqu yoxdur. **`user` tipli sahələr formaya qoyula bilməz** (formula kimi hesablanan tiplər də: ağ siyahı yalnız daxil edilə bilən tiplərdir) | `user` sahəsi üzv seçicisi tələb edir — üzv siyahısı sızardı. İcraçını admin `default_assignee_id` ilə təyin edir | Üzv seçicisini auditoriyaya görə süzmək: mürəkkəb və hələ tələb yoxdur |
| Forma **gizli layihəni** (ADR-0013) hədəfləyə bilər. Tələb edən layihəni görmür; uğurlu cavabda tapşırığın açarı (`SEC-1`) qaytarılır | Gizli back-office layihəsinə tələb qəbul etmək əsas ssenaridir. Açar tələb edənin istinad nömrəsidir; formanı admin bilərəkdən dərc edib | Açarı gizlətmək: tələb edən komandaya hansı tələbdən danışdığını deyə bilməz |
| `accounts` auditoriyası Keycloak realm-ında **özünüqeydiyyat açıqdırsa** praktiki olaraq "qeydiyyatdan keçən hər kəs" deməkdir; production realm-ında özünüqeydiyyat bağlı saxlanmalıdır (`deploy/production/README.md`) | Admin auditoriyanın real mənasını bilməlidir | Realm parametrini Nebula-dan yoxlamaq: Keycloak admin API-sinə giriş tələb edir |

### Anonim ictimai formalar — təxirə salınıb (təhdid modeli)

Giriş tələb etməyən forma (xarici təchizatçı, müştəri) ayrıca qərar olacaq. Hazırkı modelin üstünə qoyula bilməz, çünki:

- **Kimlik yoxdur → limit yalnız IP-yədir.** NAT arxasında bir ofis bir IP-dir, IPv6 və botnet-lər IP-ni ucuz dəyişir.
  Per-user rate limit, hesabı söndürmək və audit `actor_id` işləmir.
- **Gücləndirmə (amplification).** Hər göndəriş tapşırıq, `item.created` webhook-u, avtomatlaşdırma (email, chat
  mesajı) və bildirişlər yaradır: bir bot dəqiqədə komanda kanalını və SMTP kvotasını doldurar.
- **Fayllar.** Anonim yükləmə zərərli fayl yayımı və workspace kvotasının bitməsi deməkdir (ADR-0022 yalnız daxil olmuş
  istifadəçilərə hesablanıb).
- **Sosial mühəndislik.** Göndərişin mətni avtomatlaşdırma emailləri və chat mesajlarında komandaya "daxili" mesaj kimi
  çatır (fişinq linkləri, saxta ödəniş tələbi).
- **Auth middleware-dən kənar endpoint** lazımdır: indi bütün `/api/v1` (versiyadan başqa) token tələb edir.

Lazım olacaqlar: CAPTCHA (öz hostumuzda, məs. proof-of-work; üçüncü tərəf skripti CSP-ni pozur), email təsdiqi və ya
birdəfəlik link, honeypot sahə, IP + forma üzrə limit və gündəlik kvota, anonim göndərişlərin "moderasiya" statusu
(avtomatlaşdırma və webhook-lar təsdiqdən sonra), faylsız rejim, şəxsi məlumatların emalına razılıq mətni. Bunlar
olmadan anonim forma açılmır.

## Forma tərifi

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `forms` cədvəli: layihə, tapşırıq növü, `slug`, ad, giriş mətni, başlıq şablonu, sıralı `fields` (JSON massiv: `{key, label, help, required}`), `default_assignee_id`, `initial_status_id`, `audience`, `is_enabled`, `max_files` | Bir sətir = bir forma; sahələr formadan kənarda mənasızdır, ayrıca cədvələ ehtiyac yoxdur | `form_fields` cədvəli: sıra və etiket üçün əlavə JOIN, fayda yoxdur |
| Sahə açarı: fərdi sahənin açarı, `description` və ya `due_date`. Tapşırıq növünə bağlı, arxivləşdirilməmiş, daxil edilə bilən tipli olmalıdır; açar formada bir dəfə | Tapşırıq API-si ilə eyni model; başqa sahə adları yoxdur | — |
| Tapşırıq növünün **məcburi** sahələri formada olmalı və məcburi işarələnməlidir (saxlananda 422) | Əks halda heç bir göndəriş uğurlu olmaz | — |
| Formada `required` sahə növündə məcburi olmayanı da məcburi edə bilər; məcburi `boolean` = "işarələnməlidir" (razılıq qutusu) | Formalarda gözlənilən semantika | — |
| Başlıq şablonu: `{{field.<açar>}}` (formadakı fərdi sahə), `{{requester}}`, `{{form}}`; başqa `{{…}}` saxlananda 422. Dəyər bir sətrə çevrilir, nəticə boşdursa formanın adı, 500 simvoldan uzundursa kəsilir | "Ödəniş tələbi: {{field.vendor}}" komandanın siyahısında oxunaqlı başlıqdır; xəta saxlanma anında görünür | Şablonsuz (həmişə formanın adı): siyahıda bütün tələblər eyni adlanar |
| Ünvan: `slug` bütün quraşdırmada unikal (`citext UNIQUE`, `^[a-z0-9][a-z0-9-]{2,62}$`), paylaşılan link `/f/<slug>`; `/f/<id>` də işləyir | Qısa, oxunaqlı link; tək VPS quraşdırmasında (ADR-0015) qlobal ad məkanı problem deyil | Workspace daxilində unikal + `/f/<ws>/<slug>`: daha uzun link, fayda azdır |
| `initial_status_id`: tapşırıq növünün workflow-undan; **təsdiq tələb edən statusa** (workflow-un başlanğıc statusu istisna) icazə yoxdur — tapşırıq yaratmaqla eyni qayda (ADR-0026). Status silinərsə `NULL` → workflow-un başlanğıc statusu | Formadan qorunan statusa düşmək təsdiqi yan keçmək olardı | — |
| `default_assignee_id`: layihəni görən üzv (`canSeeProject`). Göndəriş anında girişi yoxdursa tapşırıq icraçısız yaradılır (xəta yox) | Tələb edən düzəldə bilməyəcəyi xəta almamalıdır; gizli layihə tapşırığı layihəni görməyənə bildirilmir | — |
| CRUD yalnız **admin+** (layihə daxilində, `loadProject` gizliliyi yoxlayır); `PUT` bütün tərifi əvəz edir (builder həmişə tam göndərir). Limit: workspace-də 100 forma, formada 50 sahə | Formanın dərci şöbəni xarici istifadəçilərə açır — admin qərarıdır | Üzvlərə icazə: hər üzv workspace-i `accounts` auditoriyasına aça bilərdi |
| Hadisələr: `form.created`, `form.updated`, `form.deleted` (`entity_type = form`, payload-da `project_id` və tərifin snapshot-u). `form` layihəyə bağlı hadisə növüdür (`eventscope`): gizli layihənin forma hadisələri yalnız o layihəni seçən webhook-lara gedir | Audit "formanı kim, nə vaxt açdı/dəyişdi"ni göstərir; ADR-0013 filtrindən sızma yoxdur | — |
| Forma silinəndə göndərişlər və tapşırıqlar qalır (`form_id = NULL`, `form_title` snapshot) | Tələb edən öz tarixçəsini itirmir | — |

## Göndəriş

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Server tərəfində **tapşırıqla eyni validatorlar**: `canonicalValue` (tiplər, seçimlər, uzunluqlar), `optionalText`, `dueDateField`, sonra adi `createItemTx` (`prepareItem` + `applyFields`). Xəta sahəsi `values.<açar>` | Formadan keçən dəyər API ilə yaradılmış tapşırıqdakı ilə eyni kanonik formadadır | Ayrı forma validatoru: iki həqiqət mənbəyi |
| Formada olmayan açar 422 (`values.x: is not on this form`); boş/`null`/boş massiv = verilməyib | Tələb edən formada olmayan sahəni (məs. daxili `approver`) doldura bilmir | — |
| Tələb edənin düzəldə bilmədiyi xəta (sonradan məcburi edilmiş sahə, indi təsdiq tələb edən status, arxivləşdirilmiş növ) → **`409 form_misconfigured`**, tapşırıq yaradılmır | Problem formanın sahibindədir; tələb edənə aydın mesaj | 422 `fields.x`: tələb edən formada olmayan sahəni görür |
| Söndürülmüş forma (və ya arxivləşdirilmiş layihə/növ): `GET` auditoriyaya yalnız adı və `open: false` qaytarır (sahələr yox), adminlər önizləmə üçün sahələri görür; göndəriş `409 form_closed` | Link paylaşılıb: "bağlıdır" mesajı 404-dən yaxşıdır | — |
| Tapşırıq, fayllar və `form_submissions` sətri **bir tranzaksiyada**; `reporter_id` = tələb edən | Yarımçıq göndəriş yoxdur | — |
| Hadisə: `item.created`, `source = 'form'`, payload-da `form_id`, `submission_id`. `events.source` CHECK-inə `form` əlavə olundu | Aktivlik lentində "tapşırığı formadan yaratdı"; webhook alıcısı mənbəni ayırd edir | `source = user` + yalnız payload: integrasiyalar mənbəni süzə bilməzdi |
| **`form` mənbəli hadisələr adi istifadəçi dəyişikliyi kimi göndərilir**: webhook-lar, avtomatlaşdırma qaydaları (`item.created`) və bildirişlər işləyir. İdxal (`import`) isə göndərilmir (ADR-0011) | İdxal minlərlə sətirlik **toplu** əməliyyatdır; göndəriş isə bir şəxsin bir qərarıdır — komanda məhz "formadan gələni avtomatik təyin et / chat-a yaz" qaydalarını istəyir. Dövr riski yoxdur (yalnız `automation` mənbəyi atlanır) | Avtomatlaşdırmanı söndürmək: forma ilə gələn tələb əl ilə yaradılandan fərqli işləyərdi |
| Webhook və avtomatlaşdırmada `actor_id` tələb edəndir və **workspace üzvü olmaya bilər** | İnteqrasiyalar actor-u üzv siyahısında tapmaya bilər; tapşırığın `reporter_id`-si də belədir | — |
| Rate limit: göndəriş üçün istifadəçi başına ayrıca **burst 10, sonra 5 saniyədə 1** (ümumi limitə əlavə). JSON gövdə ≤ 1 MiB (ümumi qayda), 30 s son müddət; multipart ≤ 1 MiB + `max_files` × fayl limiti | Hər göndəriş tapşırıq, webhook, avtomatlaşdırma və bildiriş yaradır; ofisdə 10 tələbi ardıcıl göndərmək mümkün qalır | — |
| Fayllar (ADR-0022 boru xətti): multipart-da bir `values` hissəsi + ən çox `max_files` (≤ 10) `file` hissəsi. Fail-fast (forma, auditoriya, kvota), açar əvvəlcədən rezerv, ölçü limiti axında, tip baytlardan, bloblar tranzaksiyadan əvvəl yazılır; tranzaksiya uğursuzdursa bloblar silinir/purge növbəsinə düşür. Tapşırıq başına limit və workspace kvotası eyni kilid altında yoxlanır (`insertAttachmentTx` tək yükləmə ilə ortaqdır) | Bir göndəriş = bir sorğu, fayllı tələb yarımçıq qalmır; təhlükəsizlik qaydaları təkrarlanmır | İki addım (əvvəl göndər, sonra yüklə): faylı məcburi etmək mümkün olmazdı, yarımçıq tələblər qalardı |

## "Tələblərim"

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `form_submissions` sətri tələb edənindir: `GET /me/requests` (yeni → köhnə, keyset kursor), `GET /me/requests/{id}`, `POST /me/requests/{id}/seen`. Başqasının sətri **hər kəsə** (workspace admini də daxil) 404 | İzolyasiya bir predikatdadır: `submitter_id = mən` | Tapşırıq API-si üzərindən: üzv olmayanın girişi yoxdur, gizlilik qaydaları qarışardı |
| Tələb edən görür: açar, **göndərdiyi** başlıq, formanın adı, workspace adı, **cari status** (ad + kateqoriya), göndərdiyi cavablar (o anki etiketlərlə mətn snapshot-u), öz yüklədiyi faylların adları və ölçüləri, komandanın **mesajı** | Daxili redaktələr (başlığın dəyişdirilməsi, daxili sahələr, icraçı) tələb edənə sızmır | Tapşırığın cari başlığı və sahələri: daxili qeydlər görünərdi |
| **Şərhlər tələb edənə göstərilmir.** Yerinə bir açıq sahə: "tələb edənə mesaj" (`PUT …/items/{n}/request/message`, member+, ≤ 5000 simvol, boş = silmək), hadisə `request.message_set` (`from`/`to`) | Şərhlər daxili müzakirədir (məbləğ, təsdiqçi qeydləri); "ictimai/daxili şərh" bayrağı səhv kliklə sızma riski yaradır. Tək mesaj v1 üçün kifayətdir | Jira Service Management kimi ictimai şərhlər: ayrıca qərar (açıq suallar) |
| "Yenilik" bildirişi: `form_submissions.status_changed_at` **DB trigger-i** ilə (status və ya silinmə dəyişəndə) yenilənir; `updated_at = max(göndəriş, status, mesaj)`, `unseen = max(status, mesaj) > seen_at`. Başlıqda "Tələblərim" linki yeniliklərin sayı ilə | Trigger bütün yolları tutur (PATCH, toplu redaktə, təsdiq tətbiqi, avtomatlaşdırma) və mövcud kodu dəyişmir. Daxili redaktələr "yenilik" sayılmır | Hər yerdə tətbiq kodu: bir yolu unutmaq kifayətdir |
| **Tətbiqdaxili bildiriş cədvəlinə (ADR-0017) yeni növ əlavə olunmadı.** `notifications` sətri workspace üzvlüyünə bağlıdır (FK) — üzv olmayan tələb edən oraya yazıla bilməz. Layihəni görən üzv tələb edən isə reporter kimi avtomatik izləyicidir (ADR-0028) və status dəyişikliyi bildirişlərini onsuz da alır | Hamı üçün bir mexanizm ("Tələblərim" nişanı), üzvlər üçün əlavə olaraq mövcud bildirişlər; `notifications_kind_check`-ə toxunulmur | FK-ni dəyişmək: bildirişlərin üzvlük və gizlilik modeli pozulardı |
| `seen` qeydi hadisə yazmır | Oxunma qəbzidir, vəziyyət dəyişikliyi deyil (bildirişlərin "oxundu"su kimi) | — |
| Tapşırıq silinəndə tələb "Silinib" görünür (status `null`); layihə/workspace fiziki silinəndə sətir də silinir (CASCADE) | Tələb edən tələbinin taleyini bilir | — |

## API (ADR-0002)

| Endpoint | Kim | Cavab |
|---|---|---|
| `GET/POST …/projects/{p}/forms`, `GET/PUT/DELETE …/forms/{id}` | admin+ | forma; 409 `slug_taken`, `limit_reached`; 422 sahə adı ilə |
| `GET /api/v1/forms/{slug\|id}` | auditoriya | `{form: {id, slug, title, intro, workspace, open, max_files, fields, can_manage}, limits?}`; 404 |
| `POST /api/v1/forms/{slug\|id}/submissions` | auditoriya | `{values}` (JSON) və ya multipart; 201 `{id, key, title, created_at}`; 404, 409 `form_closed`/`form_misconfigured`, 413, 422 `values.*`/`files`, 429 |
| `GET /api/v1/me/requests?limit&cursor` | daxil olmuş hər kəs | `{requests, next_cursor, unseen}` |
| `GET /api/v1/me/requests/{id}`, `POST …/{id}/seen` | yalnız sahibi | detal / 204; başqasına 404 |
| `GET …/items/{n}/request` | member+ | `{form_title, requester: {display_name, email, member}, message, …}`; formadan deyilsə 404 |
| `PUT …/items/{n}/request/message` `{message}` | member+ | yenilənmiş `request` |

Tələb edənin emaili yalnız member+ üçündür (qonaqlar xarici əməkdaşlar ola bilər).

## UI

- Layihə parametrlərində "Qəbul formaları" → `/w/:ws/p/:project/forms`: siyahı (status, link, göndəriş sayı,
  aktiv/söndür, redaktə, sil), redaktor (sahə seçicisi, sürükləmə və klaviatura üçün ↑/↓ ilə sıra, etiket, kömək,
  məcburi; növün məcburi sahələri kilidli), canlı **önizləmə** və başlıq şablonunun nəticəsi.
- `/f/:ref`: telefon üçün əvvəlcə (ADR-0019), bir sütun, 44 px hədəflər, `noValidate` + öz yoxlaması; əlçatan **xəta
  xülasəsi** (`role="alert"`, fokus xülasəyə, hər xəta sahəyə link), sahələrdə `aria-invalid`/`aria-describedby`;
  fayl seçimi limitlərlə; uğur səhifəsində tələbin açarı və "Tələblərim" linki.
- `/requests`, `/requests/:id`: siyahı ("Yeni" nişanı), detal (status, komandanın mesajı, göndərilmiş məlumat,
  fayllar); açılanda `seen`.
- Tapşırıq panelində "Tələb" bölməsi (member+): forma, tələb edən (üzv deyilsə qeyd), tələb edənə mesaj.
- Sətirlər `locales/<dil>/forms.ts`-dədir (`i18n-forms.ts`, az/en/tr/ru); formanın adı, etiketləri və kömək mətnləri
  istifadəçi məzmunudur, tərcümə olunmur.
- E2E: `web/e2e/forms.spec.ts`.

## Miqrasiya 00025

`forms`, `form_submissions`, `items` üzərində `items_form_submission_touch` trigger-i (yalnız `status_id`/`deleted_at`
dəyişəndə işləyir) və `events_source_check`-ə `form`. **Kilidlər:** trigger yaradılması `items`-ə millisaniyəlik
`SHARE ROW EXCLUSIVE`; constraint dəyişikliyi `events`-ə qısa `ACCESS EXCLUSIVE` (`NOT VALID`), `VALIDATE` isə
`SHARE UPDATE EXCLUSIVE` ilə skan edir (yazmalar davam edir); `lock_timeout` 5 s.

Down: bütün formalar və göndərişlər (tələb edənlərin "Tələblərim" tarixçəsi, mesajlar) silinir; tapşırıqlar, onların
faylları və hadisələr qalır. `events.source` CHECK-ində `form` qalır: audit jurnalı append-only-dir və yenidən
yazılmır; köhnə kod bu dəyəri yazmır. Köhnə image yeni sxemlə işləyir (yeni cədvəlləri tanımır, trigger zərərsizdir).

00022–00024 nömrələri paralel işlərə (ADR-0029–0031) ayrılıb; bu branch onları istifadə etmir.

## Nəticələr

- Tələb edən üçün email bildirişi yoxdur (SMTP söndürülmüş quraşdırmalar, spam riski) — yalnız başlıqdakı nişan və
  üzvlər üçün mövcud bildirişlər.
- Tələb edən öz fayllarını yükləyə bilmir, yalnız adlarını görür.
- Göndərişdən sonra tələb edən tələbini redaktə edə, ləğv edə və ya ona cavab yaza bilmir.
- `PUT` son yazan qalib gəlir (iki admin eyni anda redaktə edərsə); forma tərifində versiya yoxdur.
- Şərti sahələr (bir cavaba görə digərini göstərmək), çoxsəhifəli formalar və forma üzrə hesabatlar yoxdur.

## Açıq suallar

- Anonim ictimai formalar (yuxarıdakı təhdid modeli ilə) — hansı CAPTCHA, moderasiya axını?
- "İctimai şərhlər" (tələb edənlə yazışma) lazımdırmı, yoxsa tək mesaj kifayətdir?
- Tələb edənə email (status dəyişikliyi, mesaj) — SMTP-li quraşdırmalar üçün seçimlə?
- Tələb edən tələbini geri çəkə bilsinmi (ləğv statusuna keçid)?
