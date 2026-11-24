# ADR-0038: Workspace üzrə SMTP ayarları

- Status: Qəbul edilib
- Tarix: 2026-11-24
- Əsas: [ADR-0005](0005-automation-webhooks-email.md) (email, SSRF), [ADR-0007](0007-collaboration.md) (şifrələmə açarının rotasiyası),
  [ADR-0016](0016-invitations.md) (dəvət məktubları), [ADR-0018](0018-production-hardening.md) (`*_FILE` sirləri), [ADR-0025](0025-ui-languages.md) (4 dil)

## Kontekst

Email indiyə qədər yalnız server konfiqurasiyasından gəlirdi: `NEBULA_SMTP_*` mühit dəyişənləri və `nebula_smtp_password`
faylı. Poçt serverini dəyişmək üçün VPS-ə SSH ilə girmək, faylı dəyişmək və konteyneri yenidən yaratmaq lazım idi.
Maliyyə/bek-ofis komandaları (FinTez pilotu) məktubların öz domenlərindən, öz poçt hesablarından getməsini istəyir
(Google Workspace, Microsoft 365, hostinqin cPanel poçtu), bunu isə adətən workspace admini qurur, server operatoru yox.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Workspace üzrə** ayarlar: `workspace_email_settings` cədvəli (miqrasiya 00030), workspace başına bir sətir: host, port, `tls_mode` (`starttls`/`tls`/`none`), istifadəçi adı, **şifrələnmiş** parol, From, istəyə bağlı Reply-To, `is_enabled`, `updated_by`/`updated_at` | Hər komanda öz domenindən göndərir; bir instance-da bir neçə şirkət/şöbə ola bilər | Bir qlobal UI ayarı: instance operatorunun işidir, workspace adminlərinin yox; `workspaces`-ə sütunlar: parol və host workspace oxunuşunun hər yerinə düşərdi |
| **Ehtiyat sıra**: workspace-in aktiv ayarı → yoxdursa (və ya söndürülübsə) instance konfiqurasiyası → o da yoxdursa məktub göndərilmir. Workspace konteksti olmayan məktub həmişə instance ilə gedir | Mövcud quraşdırmalar dəyişmir; ayar silinəndə (`DELETE`) hər şey əvvəlki kimi işləyir | Ayar yoxdursa göndərməmək: indiki davranışı pozardı |
| Seçim **hər məktub üçün DB-dən** (`internal/mailroute`): email job-unun payload-ında `workspace_id` var; worker göndərən anda ayarı oxuyur. Qlobal dəyişən vəziyyət, keş yoxdur | Ayar dəyişikliyi növbəti məktuba dərhal tətbiq olunur, replikalar arasında fərq yoxdur; seçim bir funksiyadadır və testlə yoxlanır | Prosesdə keş: invalidasiya replikalar arasında lazım olardı; hər məktubda bir PK sorğusu ucuzdur |
| Köhnə job-larda `workspace_id` yoxdur → instance | Yeniləmədən əvvəl növbəyə düşmüş məktublar əvvəlki kimi gedir (geriyə uyğun payload) | — |
| **Pozulmuş sətirdə səssiz ehtiyat yoxdur**: parol açılmırsa (açar səhvdir) job xəta ilə təkrar cəhd edir, instance-a keçmir | Məktub başqa From ünvanından getsəydi, alıcı üçün çaşdırıcı və təhlükəli olardı | Instance-a keçmək: gizli, çətin aşkarlanan davranış |
| Parol `internal/secretbox` ilə (AES-256-GCM), əlavə məlumat `smtp-password:<workspace_id>` — başqa sətrə və ya webhook sütununa köçürülmüş şifrə mətni açılmır. API parolu **heç vaxt qaytarmır**, yalnız `password_set: bool` | Webhook URL-ləri ilə eyni nümunə (ADR-0010); DB dump-ı və backup parolu açıq göstərmir | Açıq mətn: backup və DB rolu sızması = poçt hesabının oğurlanması |
| `PUT`-da `password`: **yoxdur** = saxla, **`null`** = sil, dəyər = əvəz et. UI-də "•••• (saxlanılıb)" və "Dəyiş" | Formanı yenidən saxlamaq parolu silmir; parol brauzerə heç vaxt qayıtmır | Parolu formaya qaytarmaq: XSS/ekran görüntüsü ilə sızma |
| `ReencryptSecrets` (`nebula rotate-secrets`, ADR-0007) yeni sütunu da əhatə edir | Açar rotasiyası bir əmrdir; köhnə açar silinəndə SMTP parolları itmir (test) | Ayrıca əmr: unudulardı |
| **SSRF**: host saxlananda yoxlanır (IP literalı, `localhost`, `.internal`, `.local`, obfuskasiya olunmuş IPv4 rədd edilir) **və göndərən anda** dialer-in `Control` funksiyası hər həll olunmuş IP-ni yoxlayır (loopback, private, link-local/metadata, CGNAT, sənəd və rezerv aralıqları) — webhook-larla eyni `safehttp.IsPublic`/`safehttp.Control` | DNS rebinding: saxlananda ictimai, göndərəndə daxili IP-yə həll olunan ad da bloklanır. Workspace admini Nebula vasitəsilə daxili şəbəkəni (DB, metadata servisi) skan edə bilmir | Yalnız saxlama anında yoxlamaq: rebinding ilə keçilir |
| **Portlar**: standart `25, 465, 587, 2525` (`NEBULA_SMTP_WORKSPACE_PORTS`) | SMTP relay, implicit TLS, submission və geniş yayılmış alternativ; ixtiyari portlar daxili xidmətlərə çıxış yoludur | Bütün portlar: port skanı |
| `tls_mode=none` yalnız `NEBULA_SMTP_WORKSPACE_ALLOW_PLAINTEXT=true` ilə; private hostlar yalnız `NEBULA_SMTP_WORKSPACE_ALLOW_PRIVATE=true` ilə. **Hər ikisi productionda qadağandır** (konfiqurasiya startda rədd edir), dev compose Mailpit üçün açır | İnternet üzərindən açıq mətnli autentifikasiya qəbul edilmir. `none` rejimində istifadəçi adı/parol saxlamaq da rədd edilir ("credentials are never sent without TLS"); `net/smtp` əlavə olaraq localhost-dan başqa ünvana şifrəsiz `AUTH PLAIN` göndərmir | Daxili relay üçün production istisnası: operator instance konfiqurasiyasından istifadə edir (onun hostu yoxlanmır, o operatorun öz serveridir) |
| Siyasət **göndərən anda yenidən** yoxlanır (`EmailPolicy.CheckTransport`) | Operator bayrağı sərtləşdirəndə əvvəl saxlanmış ayar da dayanır; job daimi xəta ilə bitir, instance-a keçmir | — |
| Bağlantı vaxt limiti `internal/mail`-dəki kimi (30 s, job konteksti); sınaq göndərişi 20 s (sorğu limitindən az) | Asılı qalan SMTP serveri worker-i bloklamır | — |
| `POST …/email-settings/test {to?}`: standart olaraq **sorğu edənin təsdiqlənmiş emailinə**; başqa ünvan yalnız **workspace-in aktiv üzvünün** emaili ola bilər (DB-dəki ünvana göndərilir) | Endpoint açıq relay və spam topu olmur | İxtiyari ünvan: istənilən kəsə Nebula serverindən məktub |
| Sınaq göndərişlərinə **workspace başına 5 / 10 dəqiqə** limiti (replika başına, yaddaşda; 429 + `Retry-After`) | Düyməni basmaqla poçt qutusunu doldurmaq və serverin reputasiyasını korlamaq olmur | DB-də limit: bir VPS üçün artıqdır |
| Sınaq saxlanmış ayarı **söndürülmüş olsa da** sınayır; ayar yoxdursa instance-ı sınayır; cavab `source` (`workspace`/`instance`) göstərir | Admin ayarı yandırmazdan əvvəl yoxlaya bilir | — |
| SMTP xətası **təmizlənmiş** qaytarılır (`mail.Describe`): şəbəkə xətaları kateqoriyaya çevrilir (IP heç vaxt göstərilmir), SMTP cavabı kod + 200 simvola qədər mətn, parol və istifadəçi adı maskalanır, idarəedici simvollar silinir | Admin nə baş verdiyini görür, amma cavab daxili şəbəkənin xəritəsini və ya sirləri vermir | Xam xəta: `dial tcp 10.x.x.x:25` daxili IP-ləri sızdırır |
| **Audit**: `email_settings.updated` (`fields` — dəyişən sahələrin adları, `host`, `created`), `email_settings.deleted` (`host`), `email_settings.tested` (`source`, `to`, `ok`, workspace serveri üçün `host`). Parol heç vaxt; instance hostu workspace audit-inə yazılmır | Kim nə vaxt nəyi dəyişdi görünür; webhook-lara gedən hadisələrdə sir yoxdur (test bütün `events` və `jobs`-u yoxlayır) | — |
| Workspace serveri ilə gedən job-un xətası da təmizlənir; instance xətası (operatorun öz serveri) olduğu kimi loglanır | Admin tərəfindən idarə olunan serverin cavabı loglara/job xətasına sirr yazmasın | — |
| Hazır doldurma: **Gmail / Google Workspace** `smtp.gmail.com:587` STARTTLS (App password), **Microsoft 365** `smtp.office365.com:587` STARTTLS (mailbox üçün SMTP AUTH aktiv), **cPanel** `mail.<From domeni>:465` SSL/TLS | Provayderlərin sənədlərindəki dəyərlər; admin yalnız host/port/şifrələməni yazmır, istifadəçi adı və parol yenə onundur | Provayder siyahısı API-də: dəyişəndə deploy lazım olardı |
| UI: Ayarlar → **Email** (yalnız admin; API də yoxlayır), "server konfiqurasiyası istifadə olunur" qeydi, yazılan parol sahəsi, sınaq kartı, 4 dil, telefon üçün uyğun | ADR-0025, ADR-0035 | — |

## Nəticələr

- Workspace admini SSH olmadan komandanın poçt hesabını qoşur; instance konfiqurasiyası ehtiyat olaraq qalır.
- Email göndərən hər yol — avtomatlaşdırmanın `send_email` hərəkəti, dəvətlər (ADR-0016), mention məktubları — job-a `workspace_id` yazır.
  Gecikmə xülasəsi (ADR-0009) və təsdiqlər (ADR-0034) birbaşa məktub göndərmir: onlar hadisə/avtomatlaşdırma/chat ilə gedir,
  avtomatlaşdırma məktubu isə workspace ayarı ilə gedir.
- `store.EnqueueEmail` imzası `wsID` aldı; yeni çağırışlar ya workspace ID, ya `uuid.Nil` (instance) verməlidir.
- Nebula SMTP-də yalnız istifadəçi adı/parol (`AUTH PLAIN`) dəstəkləyir. Microsoft 2026-cı ilin sonundan mövcud tenant-larda
  SMTP AUTH Basic auth-u standart olaraq söndürür; Microsoft 365 istifadəçiləri admin tərəfindən yenidən aktivləşdirməli və ya
  relay/başqa provayderdən istifadə etməlidir. OAuth2 (XOAUTH2) gələcək iş kimi qalır.
- Rate limit replika başınadır (bir VPS-də kifayətdir).

## Açıq suallar

- Microsoft 365 və Google üçün OAuth2 (XOAUTH2) — refresh token saxlamaq, consent axını; Basic auth-un tam bağlanma tarixi 2027-də elan olunacaq.
- Daxili korporativ relay (private IP) productionda lazım olarsa: hostlar üzrə allowlist (`NEBULA_SMTP_WORKSPACE_ALLOWED_HOSTS`) ümumi `ALLOW_PRIVATE`-dən təhlükəsizdir.
- DKIM/SPF yoxlaması (From domeninin serverə uyğunluğu) UI-də xəbərdarlıq kimi.
