# ADR-0031: Təsdiqlər Mattermost/Slack kanalında (addım 1: kart və "Nebula-da aç")

- Status: Qəbul edilib
- Tarix: 2026-11-24
- Əlaqəli: ADR-0005 (webhook-lar), ADR-0010 (chat formatı, escaping), ADR-0012/0013 (layihə filtri, gizli layihələr),
  ADR-0025 (UI dilləri), ADR-0026 (dörd göz təsdiqi)

## Kontekst

FinTez komandaları gün ərzində Mattermost-da yaşayır. ADR-0026-dan sonra təsdiq hadisələri chat webhook-una bir
sətir kimi gedirdi (`🔏 Carol FIN-7 "Ödəniş" üçün təsdiq istədi (0/2)`), amma: kimin təsdiq edə biləcəyi, hansı
statusa keçid, şərh və rədd səbəbi görünmürdü; sətir tapşırığı açırdı, təsdiq panelini yox; yeni chat webhook-unun
default hadisələrində (`item.*, comment.created`) `approval.*` yox idi, yəni kart heç kimə getmirdi.

İstifadəçi ilə razılaşdırılmış plan iki addımdır:

1. **Bu ADR:** təsdiq hadisələri kanalda aydın, rəngli kart kimi görünür, **"Nebula-da aç"** keçidi birbaşa tapşırığın
   təsdiq panelinə aparır. Qərar Nebula-da verilir: **yeni etibar sərhədi yoxdur**.
2. **Sonra (bu ADR-də dizayn edilib, həyata keçirilməyib):** kanalda "Təsdiqlə / Rədd et" düymələri.

Miqrasiya: **yoxdur** (00024 nömrəsi istifadə olunmayıb). Hər şey mövcud `webhooks.events` modelinə sığır.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `slack` formatlı webhook-da `approval.*` hadisələri üçün gövdəyə **bir attachment** (rəngli kart) əlavə olunur; `text` əvvəlki kimi bir sətirdir | `text` Slack-də mobil bildirişin mətni, attachment-i göstərməyən klientdə isə yeganə mətndir (plain-text fallback); attachment-in `fallback`-i də eyni sətirdir. Digər hadisələr dəyişmir | Yalnız attachment, `text` boş: mobil bildiriş boş qalardı. Mattermost Blocks / Slack Block Kit: iki server üçün iki fərqli gövdə və hələ yoxlanılmamış versiya tələbləri; Slack-in "secondary attachments"-i Mattermost-un "message attachments"-i ilə **eyni** JSON-dur |
| Kart sahələri (hər ikisinin sənədləşdirdiyi): `fallback`, `color`, `title`, `title_link`, `text`, `fields[{title,value,short}]`, `footer`, `mrkdwn_in: ["text","fields"]` | Mattermost `text` və `fields.value`-da Markdown-u həmişə göstərir; Slack yalnız `mrkdwn_in`-də sadalananları formatlayır. `ts` Mattermost-da dəstəklənmir, istifadə olunmur | `author_*`, `image_url`: lazım deyil; `pretext`: `text` ilə təkrar |
| Rənglər: gözləyir və aralıq təsdiq (`1/2`) — kəhrəba `#F59E0B`; son təsdiq və tətbiq — yaşıl `#059669`; rədd — qırmızı `#DC2626`; ləğv — boz `#71717A` | Kanalda vəziyyət bir baxışda görünür. Aralıq təsdiq hələ "gözləyir"dir, yaşıl yalnız `approvals ≥ min_approvals` olanda | Hər `approved` üçün yaşıl: 1/2-ni "bitdi" kimi göstərərdi |
| Kartın məzmunu. **requested**: təsdiq istəyən, `Status: X → Y`, `Təsdiqlər: 0/2`, şərh, **təsdiq edə bilənlərin adları** (ən çox 10, "və daha N nəfər"). **approved**: təsdiqləyən, irəliləyiş, şərh. **rejected**: rədd edən, **səbəb**. **canceled**: ləğv edən. **applied**: son irəliləyiş (status dəyişikliyinin özü əvvəlki kimi `item.updated` sətiri ilə gedir) | ADR-0026 payload-u (`requested_by`, `from/to_status_id`, `approvals`, `min_approvals`, `comment`/`reason`) kifayətdir; adlar çatdırılma anında DB-dən oxunur | Adları hadisəyə yazmaq: audit jurnalına artıq şəxsi məlumat; ad dəyişəndə köhnəlir |
| Təsdiqçi adları `Store.PendingApprovers` ilə — qərarın özünün istifadə etdiyi predikat (`eligibleApproversSQL`: aktiv üzv, rol ≥ member, snapshot-dakı rol/istifadəçi, **layihəni görə bilir**, sorğunu göndərən deyil, hələ qərar verməyib). Yalnız `display_name`, **heç vaxt email** | Gizli layihənin sorğusu yalnız o layihəni görənlərin adını çəkir (test). Çatdırılma gecikibsə və sorğu artıq bağlanıbsa ad siyahısı boşdur | UI-dakı `approvers` siyahısını təkrar yazmaq: ikinci qayda mənbəyi |
| Başlıq (`title`) yalnız bizim mətnimizdir: `FIN-7 · Təsdiq gözləyir`; istifadəçinin yazdığı başlıq `text`-də, `“…”` içində və escape olunmuş | Mattermost attachment `title`-ını Markdown kimi sənədləşdirmir; orada escape `\(` kimi görünərdi. Layihə açarı `[A-Z0-9]`-dur, escape tələb etmir | İstifadəçi başlığını `title`-a qoymaq: ya escape görünür, ya da escape-siz inyeksiya riski |
| Bütün istifadəçi mətni (başlıq, şərh, səbəb, adlar, status adları) ADR-0010-un `escapeChat`-ından keçir: `@` + WORD JOINER (`@channel`, `@all`, `@here` işləmir), `&lt;` + WORD JOINER (`<!channel>`, `<url\|mətn>` yoxdur), Markdown simvolları escape, yeni sətir və bidi simvolları boşluq. Bu ADR yeni escape qaydası **əlavə etmir**, amma kartın hər sahəsinə tətbiq edildiyini test edir | Kartda yeni sahələr var — inyeksiya testi `text`, `title`, `fallback`, `footer` və hər `field`-i yoxlayır: yeganə `<…\|…>` tokenləri bizim keçidlərimizdir | — |
| **Keçid**: `NEBULA_PUBLIC_URL/w/{ws}/p/{KEY}?item={n}&focus=approval` — həm `title_link`, həm də `text`-də `<url\|🔗 Nebula-da aç>` | `title_link` hər iki serverdə işləyən rəsmi yoldur; `<url\|mətn>` ADR-0010 ilə eyni sintaksisdir (Mattermost onu keçidə çevirir). `NEBULA_PUBLIC_URL` boşdursa keçid yoxdur (ADR-0010) | `#approval` fraqmenti: girişdən keçən istifadəçi onu itirir — OIDC `state`-ə yalnız `pathname + search` yazılır (`safeReturnPath`, `web/src/auth/oidc.ts`). Query parametri girişdən sonra da qalır |
| Veb: `?item=n` drawer-i açır (mövcud davranış); `focus=approval` olanda `ApprovalPanel` sorğular yüklənəndən sonra panelə scroll edir, fokus verir (`role=region`, `tabIndex=-1`) və parametri `replace` ilə silir | Yeniləmə və ya drawer-in yenidən açılması təkrar "atlamır"; geri düyməsi tarixçəni korlamır. Giriş etməyən istifadəçi Keycloak-dan keçib eyni URL-ə qayıdır | — |
| **Düymə yoxdur, keçid var.** "Nebula-da aç" attachment `actions` düyməsi kimi göndərilmir | Slack Block Kit `url` düyməsi də klikdə interaktivlik payload-u göndərir və tətbiqdən cavab gözləyir (incoming webhook-un "Request URL"-i yoxdur); Mattermost-un yeni link düyməsi (`mm_blocks` + `openURL`) `props` içində ayrı formatdır, minimum versiyası sənəddə göstərilmir; köhnə attachment `actions`-ı hər iki sənəddə "legacy"dir və URL-düyməsi sənədləşdirilməyib. `title_link` + keçid hər yerdə işləyir | Mattermost üçün `mm_blocks` düyməsi: ayrı gövdə, FinTez serverinin versiyası təsdiqlənməyib. Addım 2-də düymələr onsuz da interaktiv olacaq |
| **Opt-in hər webhook üçün = hadisə siyahısı.** UI-da "Təsdiq hadisələrini göndər" qutusu `approval.*` nümunəsini əlavə edir / bütün `approval.…` nümunələrini silir. Yeni **chat** webhook-unun default-u `item.*, comment.created, approval.*`; `nebula` formatının default-u dəyişmir (`item.*`) | ADR-0005/0010-un modeli: nə göndərildiyini yalnız `events` müəyyən edir, ikinci bayraq (və miqrasiya) yaranmır. `*` seçilibsə qutu işarəlidir və deaktivdir; yeganə nümunə `approval.*`-dırsa söndürmək olmur (boş siyahı 422-dir) | Ayrı `send_approvals` sütunu: `events` ilə ziddiyyətli iki həqiqət mənbəyi |
| Mövcud chat webhook-larına `approval.*` **avtomatik əlavə olunmur** | Admin kanala nə gəldiyini özü seçib; səssizcə yeni mesaj növü açmaq sürprizdir. Qutu bir kliklə açır | Data miqrasiyası: admin-in seçimini dəyişərdi |
| Gizlilik: dəyişiklik yoxdur — gizli layihənin təsdiq hadisəsi yalnız o layihəni **açıq seçən** webhook-a gedir (ADR-0012/0013, `DeliveryTargets`). İnteqrasiya testi bunu təsdiq kartları ilə yoxlayır | Filtr çatdırılmadan əvvəl işləyir; renderer gizli layihəni ümumiyyətlə görmür | — |
| **Dil: azərbaycanca qalır**, hər webhook üçün `language` əlavə olunmadı | Server tərəfində chat mətnləri (`chat.go`, ~40 sətir, rəqəm/cəm qaydaları, tarix formatı) ADR-0025-in UI lüğətlərindən kənardadır. Dil sahəsi = miqrasiya + 4 dildə bütün chat sətirlərinin tərcüməsi + Go-da cəm qaydaları + test matrisi — "ucuz" deyil. FinTez kanalları azərbaycancadır | `language: az\|en\|tr\|ru` sütunu (00024): ayrıca iş kimi ROADMAP-da qalır |
| Mesaj limiti: `text` 3000 simvol (ADR-0010), kartda adlar 10 ilə məhdud, şərh/səbəb DB-də ≤ 2000 (ADR-0026) | Mattermost 16k, Slack 40k limitindən xeyli aşağı | — |

### Nümunə gövdə (`approval.requested`)

```json
{
  "text": "🔏 Carol <https://nebula.example.com/w/acme/p/FIN?item=7|FIN-7> “Ödəniş \\(USD\\)” üçün təsdiq istədi (0/2)",
  "username": "Nebula",
  "icon_url": "https://nebula.example.com/logo-mark.png",
  "attachments": [{
    "fallback": "🔏 Carol <https://nebula.example.com/w/acme/p/FIN?item=7|FIN-7> “Ödəniş \\(USD\\)” üçün təsdiq istədi (0/2)",
    "color": "#F59E0B",
    "title": "FIN-7 · Təsdiq gözləyir",
    "title_link": "https://nebula.example.com/w/acme/p/FIN?item=7&focus=approval",
    "text": "“Ödəniş \\(USD\\)”\n<https://nebula.example.com/w/acme/p/FIN?item=7&focus=approval|🔗 Nebula-da aç>",
    "fields": [
      {"title": "Təsdiq istəyən", "value": "Carol", "short": true},
      {"title": "Status", "value": "Yoxlamada → Ödənildi", "short": true},
      {"title": "Təsdiqlər", "value": "0/2", "short": true},
      {"title": "Şərh", "value": "“Qəbz əlavə olunub”", "short": false},
      {"title": "Təsdiq edə bilənlər", "value": "Erin, Eve", "short": false}
    ],
    "footer": "Nebula · qərar yalnız Nebula-da verilir",
    "mrkdwn_in": ["text", "fields"]
  }]
}
```

(Go `encoding/json` `<`, `>`, `&` simvollarını `<` və s. kimi yazır; bu, eyni JSON-dur.)

### Mənbələr (2026-11-24 tarixində oxunub)

- Mattermost message attachments: <https://docs.mattermost.com/developers/integrate/reference/message-attachments> —
  sahələr, `text`/`fields.value`-da Markdown, `ts` dəstəklənmir, footer ≤ 300 simvol.
- Mattermost incoming webhooks: <https://docs.mattermost.com/developers/integrate/webhooks/incoming> — `attachments`
  üst səviyyədə olmalıdır (`props` içində yox); `text` və ya `attachments` məcburidir; post ≤ 16383 simvol;
  Slack-in `mrkdwn`/`parse`/`link_names` parametrləri dəstəklənmir.
- Mattermost Blocks: <https://docs.mattermost.com/developers/integrate/reference/mm-blocks> — düymə + `openURL`
  əməliyyatı `props.mm_blocks_actions`-da (addım 1-də istifadə olunmur).
- Slack legacy secondary attachments: <https://docs.slack.dev/legacy/legacy-messaging/legacy-secondary-message-attachments.md>
  — `color`, `fallback`, `title_link`, `fields`, `mrkdwn_in`; Slack Block Kit-i tövsiyə edir; `actions` sahə
  siyahısında yoxdur.
- Slack incoming webhooks: <https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks>;
  formatlama və bildiriş mətni: <https://docs.slack.dev/messaging/formatting-message-text.md> ("mobil bildirişlər
  yalnız `message.text`-dən istifadə edir").
- Slack button element: <https://docs.slack.dev/reference/block-kit/block-elements/button-element.md> — `url` olan
  düymə də interaktivlik payload-u göndərir və təsdiq cavabı gözləyir.

Yoxlanılmayıb (real serverə qarşı test yoxdur, ADR-0010 kimi): Slack incoming webhook-unun legacy `attachments`-i
hazırda necə göstərdiyi (sənəd webhook səhifəsində yalnız `blocks` nümunəsi verir) və `<url|mətn>` sintaksisinin
Mattermost attachment `text`-ində keçidə çevrilməsi. Hər ikisi üçün `title_link` ehtiyat yoldur. Slack-də escape
simvolları (`\(`) görünür — ADR-0010-un məlum məhdudiyyəti.

## Addım 2 (dizayn edilib, təxirə salınıb): kanalda "Təsdiqlə / Rədd et"

### Nə lazım olardı

| Komponent | Təsvir |
|---|---|
| **Kimlik uyğunlaşdırması** | Chat istifadəçisi (Mattermost `user_id`, Slack `U…` + `team_id`) ↔ Nebula istifadəçisi. Email ilə avtomatik uyğunlaşdırma **yox**: chat serverində email-i istifadəçi özü dəyişə bilər. Bir dəfəlik bağlama: Nebula-da "Mattermost hesabımı bağla" → chat-də slash komanda/OAuth ilə təsdiq. Cədvəl `chat_identities (workspace_id, provider, team_id, chat_user_id UNIQUE, user_id)`, admin ləğv edə bilər |
| **Daxil olan əlaqə** | Chat serverindən Nebula-ya: Mattermost "interactive message" `integration.url` / Slack "Request URL" → `POST /api/v1/chat/actions`. Bu, Nebula-nın **chat serverindən əlçatan** olması deməkdir (bu gün yalnız çıxan əlaqə var). VPS-də (ADR-0015) Caddy-də ayrıca yol, rate limit, ölçü limiti |
| **Sorğunun həqiqiliyi** | Slack: `X-Slack-Signature` (HMAC-SHA256, signing secret, `X-Slack-Request-Timestamp` ±5 dəq). Mattermost: incoming webhook-un imzası yoxdur — action `context`-inə qoyulan **bizim** imzalı token yeganə sübutdur; əlavə olaraq Mattermost-un mənbə IP-si allowlist-də |
| **Bir dəfəlik action token** | Kartdakı hər düymə üçün `context.token` = HMAC(server açarı, `request_id`, `decision`, `exp`, `nonce`); DB-də `nonce` istifadə olunub kimi işarələnir (təkrar göndərmə yoxdur), `exp` ≤ 24 saat, sorğu bağlananda bütün token-lər etibarsızdır. Token **kimi təsdiqlədiyini** yox, **nəyi** təsdiqlədiyini daşıyır; kim — chat imzalı `user_id` → uyğunlaşdırma |
| **Səbəb dialoqu** | Rədd üçün səbəb məcburidir (ADR-0026). Mattermost interactive dialog / Slack modal (`views.open`, bot token tələb edir — incoming webhook kifayət deyil, Slack App lazımdır) |
| **Qərarın yolu** | Eyni `Store.DecideApproval` — self-approval, uyğunluq, gizli layihə yoxlaması, versiya kilidi dəyişmir. Chat yalnız başqa "giriş qapısı"dır, qayda mənbəyi yox |
| **Kartın yenilənməsi** | Qərardan sonra kartı "Təsdiqləndi — Eve" kimi əvəz etmək (Mattermost: action cavabında `update`; Slack: `response_url`) |
| **Audit** | Hadisədə `source: "chat"`, provider, chat `user_id`; audit jurnalında görünür |

### Təhdid modeli (qısa)

| Təhdid | Addım 2-də qorunma |
|---|---|
| Kanal üzvü kartı başqasına yönləndirir / düymə ekran görüntüsü ilə paylaşılır | Token kimlik daşımır; qərar verən chat imzalı `user_id` → uyğunlaşdırılmış Nebula istifadəçisi + `DecideApproval`-ın uyğunluq yoxlaması |
| Saxta action sorğusu (Nebula-nın action URL-i məlumdur) | Slack imzası / Mattermost üçün bizim HMAC token + nonce + `exp`; imzasız və ya köhnə sorğu 401 |
| Təkrar (replay) | Nonce bir dəfəlik, vaxt pəncərəsi, sorğu bağlıdırsa `409 approval_closed` |
| Chat hesabının ələ keçirilməsi / admin-in chat-də başqasının adından yazması | Chat serverinin admin-i istənilən `user_id` adından action göndərə bilər — bu, **chat serveri Nebula-nın etibar sərhədinə daxil olur** deməkdir. Maliyyə təsdiqi üçün yüksək məbləğlərdə "yalnız Nebula-da təsdiq" siyasət bayrağı lazım ola bilər |
| Kanal tarixçəsində qalan token | `exp` + bağlanan sorğu; token heç vaxt Nebula sessiyası vermir |
| Gizli layihə | Kart onsuz da yalnız layihəni seçən webhook-a gedir; action endpoint-i layihəni görməyən istifadəçiyə 404 qaytarır |

### Niyə əvvəl addım 1

- Addım 1 **yeni etibar sərhədi yaratmır**: Nebula yalnız çıxan HTTP edir (SSRF qorunması ilə, ADR-0005), qərar
  Nebula sessiyası (Keycloak OIDC) altında verilir. Addım 2 chat serverini təsdiq zəncirinə daxil edir — maliyyə
  auditində bu ayrıca qərardır.
- Kanalda görünmə və birbaşa keçid dəyərin böyük hissəsini verir: təsdiqçi bildirişi kanalda görür, bir kliklə
  panelə düşür (telefonda da, ADR-0019).
- Addım 2 Slack App (bot token, modal) və ya Mattermost-un interaktiv mesajları üçün inbound reachability tələb edir;
  FinTez serverinin Nebula-ya çıxışı (şəbəkə qaydaları) hələ məlum deyil.
- Addım 1-in kartı, rəngləri və keçidi addım 2-də dəyişmədən qalır; düymələr eyni attachment-ə əlavə olunur.

## Nəticələr

- Yeni chat webhook-ları təsdiq kartlarını default olaraq alır; mövcudlar üçün admin "Təsdiq hadisələrini göndər"i işarələyir.
- `nebula` formatlı webhook-ların gövdəsi dəyişməyib.
- Testlər: `internal/engine/chat_approval_test.go` (hər hadisə üçün kart, JSON forması, inyeksiya, keçid),
  `internal/server/chat_approvals_integration_test.go` (fake receiver-ə çatdırılma, opt-out, gizli layihə filtri,
  email yoxdur), `web/src/lib/webhooks.test.ts`, E2E `web/e2e/chat-approvals.spec.ts`.
- Miqrasiya yoxdur → rollback cədvəlinə sətir əlavə olunmadı; geri qaytarmaq üçün əvvəlki image kifayətdir.

## Açıq suallar

- Hər webhook üçün mesaj dili (`language`), bütün chat sətirlərinin tərcüməsi ilə birlikdə.
- Mövcud chat webhook-larına `approval.*` əlavə etməyi admin-ə UI-da bir dəfə təklif etmək.
- Mattermost Blocks (`openURL` link düyməsi) FinTez-in Mattermost versiyası təsdiqlənəndən sonra.
- Addım 2: bax yuxarı.
