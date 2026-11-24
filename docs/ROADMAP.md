# Yol xəritəsi

Hər mərhələ yalnız testlər, miqrasiyalar (up və down) və CI yaşıl olduqda tamamlanmış sayılır.

## Phase 0: Təməl (tamamlandı)
- [x] Arxitektura qərarları (ADR-0001)
- [x] Əsas sxem: workspace-lər, istifadəçilər, layihələr, item tipləri, fərdi sahələr, workflow-lar, item-lər, şərhlər, hadisələr
- [x] Go servis skeleti: konfiqurasiya, strukturlu loglama, graceful shutdown, health/readiness, metriklər
- [x] Docker image, docker-compose, Makefile, CI
- [x] OIDC autentifikasiya (istənilən OIDC IdP; dev mühitində Keycloak), ADR-0002
- [x] Workspace-lər, üzvlər, rollar, layihələr
- [x] Items API: yaratmaq/oxumaq/yeniləmək/silmək/siyahı; keyset pagination, filtrlər, tam mətnli axtarış və optimistic locking
- [x] Hər dəyişikliyin eyni tranzaksiyada hadisə loguna yazılması

## Phase 0.5: Fərdiləşdirmə nüvəsi (tamamlandı)
- [x] Fərdi sahələr: workspace səviyyəsində təriflər, item tipinə bağlama (məcburilik bağlamada), tipli validasiya, sahəyə görə filtr (ADR-0003)
- [x] Workflow, status və item tipi idarəetmə API-si (ADR-0004)
- [x] Rate limiting: IP başına (autentifikasiyadan əvvəl) və istifadəçi başına (ADR-0004)

## Phase 1: Workflow və avtomatlaşdırma (tamamlandı, ADR-0005)
- [x] Status keçidlərinin məcburi tətbiqi və növbəti mümkün statuslar endpoint-i
- [x] Transactional outbox + PostgreSQL job növbəsi (SKIP LOCKED, retry/backoff, `nebula worker`)
- [x] Çıxan webhook-lar: HMAC imza, şifrələnmiş secret, SSRF qoruması, çatdırılma logu, avtomatik söndürmə
- [x] Avtomatlaşdırma qaydaları (trigger, şərt, əməliyyat), dövrə qoruması, hadisə anındakı snapshot
- [x] SMTP bildirişləri (məcburi TLS, header injection qoruması), dev mühitində Mailpit

## Veb interfeys v1 (tamamlandı, ADR-0006)
- [x] OIDC + PKCE ilə giriş (token yalnız yaddaşda), workspace və layihə siyahıları, yaratma formaları
- [x] Virtual cədvəl (sonsuz sürüşdürmə, axtarış, filtrlər), kanban lövhəsi (sürüklə-burax, workflow qaydalarına hörmət)
- [x] Tapşırıq paneli: başlıq, təsvir, status, icraçı, fərdi sahələr; optimistic locking konfliktlərinin idarəsi

## Phase 1.5: Əməkdaşlıq (tamamlandı, ADR-0007)
- [x] Şərhlər (tombstone silmə, yalnız müəllif redaktəsi) və @mention email bildirişləri
- [x] Tapşırıq aktivlik lenti və workspace audit log
- [x] Webhook "test ping", fasiləsiz şifrələmə açarı rotasiyası (`nebula rotate-secrets`)
- [x] LISTEN/NOTIFY: fon işləri ~23 ms-də emal olunur
- [x] UI: tapşırıq panelində "Şərhlər" və "Aktivlik" bölmələri, @ ilə avtomatik tamamlama

## Phase 2a: Admin ekranları (tamamlandı)
- [x] Workspace ayarları: üzvlər (rol dəyişmə, çıxarma, "son sahib" qoruması), sahələr və seçimlər, tapşırıq tipləri və sahə bağlamaları
- [x] Workflow redaktoru: statuslar, sıralama, başlanğıc status, keçidlər, keçid məcburiliyi
- [x] Webhook-lar (secret bir dəfə göstərilir, test, çatdırılma logu, secret rotasiyası), vizual avtomatlaşdırma qayda qurucusu, audit log

## Phase 2b: Növbəti
- [x] Playwright E2E testləri (real Keycloak girişi, PKCE, rollar, admin ekranları, @mention email-i); CI-də hər PR-da işləyir
- [x] Lövhədə status sütunları (fərqli workflow-ların eyni statusları birləşir), sütun-sütun sonsuz yükləmə, həqiqi sütun sayları, icazəsiz keçidlərin solğunlaşması (ADR-0008)
- [x] 429-un düzgün idarəsi: `Retry-After` ilə təkrar; yazmalarda yalnız 429 (dəyişiklik itkisi aradan qaldırıldı)
- [x] CSV və Excel (.xlsx) importu: önbaxış, sütun uyğunlaşdırması (başlıq adına görə təklif), dry run ilə sətir xətaları, hamısı-və-ya-heç-nə idxal, audit qeydi (ADR-0011)
- [x] Tətbiqdaxili bildiriş mərkəzi: @mention, təyinat, gecikmə və şərh bildirişləri, başlıqda zəng düyməsi (oxunmamış sayı, 30 s polling), gizli layihələrdə oxuma anında giriş yoxlaması, 90/180 günlük saxlama (ADR-0017, miqrasiya 00014)
- [x] Son tarix (`due_date`), workspace saat qurşağı, avtomatik "gecikir" nişanı və filtr; `item.overdue` qayda trigger-i, gündəlik gecikmə xülasəsi (`workspace.overdue_digest`), "Ümumi" ayarlar bölməsi (ADR-0009)
- [x] Mattermost/Slack bildiriş hədəfi: webhook `format: slack`, azərbaycanca qısa mesajlar, tapşırıq keçidi, URL şifrələnmiş və maskalanmış (ADR-0010)
- [x] Etibarlı proxy-lər üçün rate limit: `NEBULA_TRUSTED_PROXIES`, `X-Forwarded-For` yalnız etibarlı CIDR-lərdən, sağdan sola (ADR-0010)
- [x] Təkrarlanan tapşırıqlar: gündəlik/həftəlik/aylıq (ayın son günü daxil)/illik, yalnız növbəti tapşırıq `lead_days` əvvəl, buraxılmış tarixlərin bərpası, xətalı qaydanın söndürülməsi (ADR-0012)
- [x] Layihə səviyyəsində məxfilik: gizli layihələr (yalnız üzvlər + workspace adminləri), layihə üzvləri, icraçı və @mention yalnız layihəni görə bilənlərə (ADR-0013)
- [x] Saxlanılan görünüşlər (şəxsi/ümumi, filtrlər, sütunlar, sıralama, qruplaşdırma), fərdi sahə sütunları, bütün filtrlənmiş nəticə üzrə cəm sətri, son tarixə görə sıralama, kod bölünməsi (ADR-0014)
- [x] Production: tək VPS-də Docker Compose (Caddy + TLS/CSP, Keycloak production rejimi, PG 19 + pgBackRest → S3, PITR və aylıq avtomatik restore testi, Alloy + alertlər → Mattermost, digest ilə deploy/rollback skriptləri) (ADR-0015, `deploy/production/README.md`)
- [x] Webhook-lara layihə filtri: boş = açıq layihələr, gizli layihə yalnız açıq seçiləndə; çatdırılmada təkrar yoxlama; gündəlik xülasə hər webhook üçün süzülür (ADR-0013, migrasiya 00012)
- [x] Email dəvətləri: tək istifadəlik link (yalnız heş saxlanılır), IdP-nin təsdiqlədiyi email uyğun gəlməlidir, 7 gün, ləğv/yenidən göndər, limitlər (ADR-0016)
- [x] Production hardening: `*_FILE` sirləri, ayrı miqrasiya/runtime DB rolları (runtime yalnız DML), `nebula healthcheck`, access log-da `client_ip`, app-dan növbə metrikləri, release-də Trivy gate (ADR-0018)
- [x] Telefon və planşet: lövhədə toxunma ilə sürükləmə (basılı saxla, scroll ilə vuruşmur), sütunlarda snap, telefonda bir sətirlik alətlər paneli + "Filtrlər" vərəqi, sabit başlıqlı cədvəl, tam ekran tapşırıq paneli (klaviatura və safe-area), 44 px toxunma hədəfləri, heç bir səhifədə yana sürüşmə yoxdur; Playwright cihaz emulyasiyası ilə E2E (ADR-0019)
- [x] Mattermost Boards-dan keçid: azərbaycanca workspace şablonu (Görüləcək → İcrada → Yoxlamada → Tamamlandı, Asia/Baku), mövcud layihədən layihə şablonu (üzvlər, təkrarlanan tapşırıqlar, ümumi görünüşlər), `.boardarchive` importu (kart mətni + köhnə şərhlər təsvirdə, Mattermost istifadəçilərinin üzvlərə uyğunlaşdırılması), keçid runbook-u (ADR-0020, `docs/runbooks/mattermost-boards-migration.md`)
- [x] Təqvim görünüşü (bazar ertəsindən ay şəbəkəsi, workspace saat qurşağı, "+N daha", sürükləyərək son tarix, son tarixsiz siyahı, telefonda gündəm), `due_from`/`due_to` aralıq filtri; toplu redaktə (500-ə qədər, hamısı-və-ya-heç-nə, tapşırıq-tapşırıq xəta siyahısı, admin üçün toplu silmə) və cədvəldə sətirdaxili redaktə (ADR-0023)
- [x] Alt tapşırıqlar: bir səviyyə (DB trigger-i ilə qorunur, eyni layihə), paneldə siyahı + inline əlavə + irəliləyiş "2/5", əsas tapşırıq keçidi, cədvəl/lövhədə nişan, `?parent=`/`?top_level=true` filtrləri, əsas tapşırıq silinəndə alt tapşırıqlar ayrılır (ADR-0021, miqrasiya 00016)
- [x] Fayl əlavələri (lokal volume və ya S3, baytlardan tip təyini, həmişə `attachment` + sandbox CSP, yalnız raster şəkil önizləməsi, gizli layihə icazələri, kvota və rate limit, yetim blob təmizliyi, audit) və təsvir/şərhlərdə təhlükəsiz Markdown (GFM, xam HTML yox, təhlükəsiz linklər, önizləmə tabı) (ADR-0022, miqrasiya 00017)
- [x] Workspace üzrə axtarış: tapşırıqlar (açar, başlıq, təsvir) və şərhlər bir yerdə, Azərbaycan hərflərinin qatlanması ("odenis" ↔ "ödəniş"), prefiks və yazı xətası (pg_trgm) axtarışı, `QA-12` tam açarı birinci, gizli layihələr heç bir formada sızmır, Cmd/Ctrl+K dialoqu klaviatura ilə; 100k tapşırıq + 200k şərhdə seçici sorğular p95 < 5 ms, ən pis halda (sənədlərin 90%-i uyğun) ~0.25 s (ADR-0024, miqrasiya 00018)
- [x] İnterfeys dilləri: azərbaycanca, ingiliscə, türkcə, rusca; başlıqda dil seçici, brauzer dilinin təyini, yalnız seçilmiş dilin yüklənməsi, tsc ilə yoxlanılan lüğətlər, dilə görə tarix və cəm formaları, Keycloak giriş səhifəsi eyni dildə, türkcə və rusca workspace şablonları (ADR-0025)
- [x] "Dörd göz" təsdiqi: statusa daxil olmaq üçün siyasət (rollar və/və ya istifadəçilər, 1–5 təsdiq), sorğunu göndərən təsdiqləyə bilməz, gözləyərkən status/başlıq/təsvir/sahələr/fayllar kilidli, rədd üçün məcburi səbəb, son təsdiq keçidi eyni tranzaksiyada bir dəfə tətbiq edir, toplu redaktə/idxal/avtomatlaşdırma yan keçə bilmir, gizli layihədə yalnız layihəni görənlər təsdiqçidir, `approval.*` hadisələri, bildirişlər, nişan və "Təsdiqimi gözləyənlər" filtri (ADR-0026, miqrasiya 00019)
- [x] Workspace iş təqvimi: həftə sonu günləri, bayramlar və köçürülmüş iş günləri (Ayarlar, il üzrə), Azərbaycanın sabit tarixli bayramları üçün köməkçi (hərəkətli bayramlar hər il əl ilə); təkrarlanmada "ayın son / N-ci iş günü" və qeyri-iş gününə düşən tarixin əvvəlki/növbəti iş gününə köçürülməsi, təqvim dəyişəndə növbəti tarixlərin yenidən hesablanması (keçmişə tapşırıq yaratmadan), təqvim görünüşündə bayramlar, qeyri-iş günlərində gecikmə xülasəsi göndərilmir (ADR-0027, miqrasiya 00020)
- [x] Tapşırıq əlaqələri (bloklayır / bloklanır, əlaqəlidir, təkrarlayır; layihələr arası, workspace daxilində, DB-də dublikat və öz-özünə əlaqə qadağası, gizli tərəf heç bir formada görünmür, cədvəl/lövhədə "Bloklanıb" nişanı, `?blocked=true`) və izləyicilər (avtomatik izləmə, status/icraçı/son tarix/şərh/əlaqə bildirişləri dublikatsız, toplu redaktə izləyicilərə bildiriş vermir) (ADR-0028, miqrasiya 00021)
- [x] Təsdiqlər Mattermost/Slack kanalında, addım 1: rəngli kart (gözləyir / təsdiq / rədd / ləğv / tətbiq), təsdiq istəyən, `X → Y` status, irəliləyiş, şərh və səbəb, təsdiq edə bilənlərin adları (email yox), "Nebula-da aç" keçidi təsdiq panelinə (`?focus=approval`, girişdən sonra da qalır), hər webhook üçün "Təsdiq hadisələrini göndər" (chat üçün default), gizli layihə filtri dəyişməz; addım 2 (kanalda təsdiq düymələri) dizayn edilib, təxirə salınıb (ADR-0031, miqrasiya yoxdur)
- [x] Audit sübut paketi: bir tapşırıq və ya layihələr + tarix aralığı (ən çox 366 gün, "yalnız təsdiqdən keçənlər") üçün imzalı ZIP — oxunaqlı HTML hesabat (az/en/tr/ru, PDF kimi çap), JSON və CSV məlumatlar (hadisələr, tapşırıqlar, təsdiqlər və qərarlar, şərhlər, əlaqələr, faylların SHA-256-sı), Ed25519 ilə imzalanmış manifest; `nebula evidence verify` (dəyişdirilmiş fayl/manifest, `--key` ilə açar, `--against` ilə iki ixrac arasında silinmiş və ya redaktə edilmiş hadisələr); yalnız ixrac edənin gördüyü, limitlər (kəsmədən), rate limit, hər ixrac `evidence.exported` hadisəsidir (ADR-0029, miqrasiyasız)
- [x] Düstur sahələri: Excel tipli ifadələr (`amount - paid`, `round(amount * 0.18, 2)`, `due - today()`, `workdays(today(), @due_date)`), əl ilə yazılmış təhlükəsiz parser (limitlər, fuzz testi), `math/big` ilə dəqiq onluq hesab və "yarısı sıfırdan uzağa" yuvarlaqlaşdırma, sıfıra bölmə xəta dəyəri kimi, dövrsüz formula zəncirləri, yazma anında hesablama + fon keçidi (tərif, iş təqvimi, saat qurşağı dəyişəndə və hər yerli gün), yalnız oxunan dəyərlər (API/idxal/toplu redaktə rədd edir), cəm, `sort=field.<key>` / `-field.<key>` sıralaması və filtr, canlı önizləməli redaktor, dilə görə rəqəm formatı (ADR-0030, miqrasiya 00023)
- [x] Qəbul formaları: admin layihə üçün forma qurur (fərdi sahələrin seçilmiş alt çoxluğu, etiket və kömək mətni, sürükləyərək sıra, önizləmə, başlıq şablonu, standart icraçı, başlanğıc status, fayllar), `/f/<ünvan>` linki yalnız daxil olmuş istifadəçilər üçün (workspace üzvləri və ya hesabı olan hər kəs), göndəriş tapşırıqla eyni validatorlardan keçir və atomik tapşırıq yaradır (`source = form`, avtomatlaşdırma işləyir), gizli layihəyə tələb, təsdiq siyasəti yan keçilmir, istifadəçi başına rate limit; "Tələblərim" — yalnız öz tələbləri, status, komandanın mesajı, yenilik nişanı; anonim formalar təhdid modeli ilə təxirə salınıb (ADR-0032, miqrasiya 00025)
- [x] Komanda növünə görə workspace şablonları (dil ilə yanaşı ikinci ox, qlobal "rejim" yoxdur): **Maliyyə** — Hazırlanır → Yoxlamada → Təsdiq gözləyir → Ödənildi, Ödəniş/Hesab-faktura/Hesabat, Məbləğ, ƏDV dərəcəsi və ƏDV düsturu (az-da default 18%, Vergi Məcəlləsi 173.1), Kontragent, Ödəniş tarixi, "Ödənildi" üçün dörd göz siyasəti, az-da rəsmi iş təqvimi, "Maliyyə" layihəsi, ümumi görünüşlər və "Ödəniş tələbi" forması; **İT** — Prioritet/Komponent, "Mənim işlərim", "Kritik", giriş sorğusu forması; **HR** — gizli layihə, məzuniyyət və yeni əməkdaş formaları; hamısı bir tranzaksiyada, mövcud store funksiyaları və hadisələrlə, 4 dildə (ADR-0033, miqrasiya yoxdur)
- [x] Təsdiqlərin təkmilləşdirilməsi: workspace üzrə "Təsdiq gözləyənlər" qutusu (bütün görünən layihələr, qərarla eyni uyğunluq qaydası, keyset səhifələmə, nişan sayı, panelə dərin link), təsdiqdən sonra izlənən məzmun (standart: başlıq, təsvir, bütün sahələr; və ya yalnız seçilənlər) dəyişəndə təsdiq etibarsız olur, tapşırıq əvvəlki statusa qayıdır və yeni sorğu açılır (dəyişdirən təsdiqləyə bilməz; toplu redaktə və avtomatlaşdırma rədd edilir), iş günü ilə qərar müddəti (1–30, iş təqvimi), gecikmə nişanı və planlayıcıdan bir dəfəlik xatırlatma, `approval.invalidated` / `approval.overdue` hadisələri, sübut paketi və chat kartları (ADR-0034, miqrasiya 00027)
- [x] Rəng rejimi: Açıq / Tünd / Sistem, brauzer başına, yenidən yükləmədən, ilk boyanmadan əvvəl (CSP ilə uyğun same-origin skript), OS dəyişikliyini canlı izləyir; workspace vurğu rəngi: 7 seçilmiş, WCAG AA üçün testlə yoxlanmış şkala, yalnız workspace daxilində, admin seçir, `workspace.updated`-də iz qalır; tapşırıq siyahısı və paneli "Ledger" dizaynında: tez filtr sayları (bir sorğu), filtr menyusu və çiplər, vəziyyət zolağı və "Siqnal" sütunu, cədvəlin yanında iki sütunlu panel, iş axını addımları və kim təsdiqlədi/kim gözlənilir (ADR-0035, miqrasiya 00028)
- [x] Giriş səhifəsi Nebula brendində: iki panel (qradiyentli brend paneli: böyük loqo, ad, dörd dildə şüar; telefonda zolaq), firuzəyi düymələr və linklər, tətbiqin şriftləri, OS-a görə tünd rejim, telefon üçün uyğun; `keycloak.v2`-nin CSS genişlənməsi, şablonlar dəyişmir, xarici resurs yoxdur (ADR-0036)
- [x] Layihənin standart görünüşü: layihə bu ümumi görünüşlə açılır (admin seçir; Maliyyə/IT/HR şablonları "Bütün tapşırıqlar" görünüşünü məbləğ/prioritet sütunları ilə standart edir); telefonda siqnallar açar sətrində, ayrıca sütunsuz (ADR-0014 əlavəsi, miqrasiya 00029)

## Phase 2: Şablonlar və görünüşlər
- Proqram təminatı / HR / Satış / Marketinq şablonları; cədvəl, kanban, təqvim, forma görünüşləri

## Phase 3: İnteqrasiyalar
- IMAP ilə email-dən item yaratmaq, xarici baza konnektorları (yalnız oxuma, sandbox-da), Git/CI, Jira/YouTrack/CSV importu

## Phase 4: Korporativ
- Ətraflı RBAC, row-level security, SSO/LDAP, hesabatlar, WASM plaginləri, multi-tenant SaaS, paylanmış (Redis) rate limiting
