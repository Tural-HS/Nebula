# ADR-0010: Mattermost/Slack bildirişləri və etibarlı proxy-lər

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

FinTez Mattermost Boards-dan Nebula-ya keçir. Orada status dəyişiklikləri şöbə kanallarına
yazılır. Nebula webhook-ları (ADR-0005) isə öz JSON hadisəsini HMAC imzası ilə göndərir;
Mattermost və Slack-in "incoming webhook"-u başqa gövdə gözləyir (`{"text": ...}`, markdown) və
sorğunu imza ilə yox, **URL-in içindəki gizli token** ilə tanıyır.

İkinci problem: rate limit (ADR-0004) client IP olaraq yalnız TCP peer-i (`RemoteAddr`) götürür.
Caddy və ya Kubernetes ingress arxasında bütün istifadəçilər proxy-nin bir IP "vedrəsini" bölüşür:
bir aktiv komanda hamını 429-a sala bilər.

## 1. Webhook formatı

| Qərar | Səbəb |
|---|---|
| Hər webhook-un `format`-ı var: `nebula` (default, davranış dəyişməyib) və ya `slack` | Mattermost Slack formatını olduğu kimi qəbul edir; ayrıca `mattermost` formatına ehtiyac yoxdur |
| Format yaradılarkən seçilir, sonra **dəyişmir** (`PATCH`-də `format` → 400) | Format dəyişəndə URL-in saxlanma qaydası da dəyişir (aşağıda); "sil və yenisini yarat" daha aydındır |
| `slack` gövdəsi: `{"text", "username": "Nebula", "icon_url"}`; `icon_url` yalnız `NEBULA_PUBLIC_URL` varsa | Mattermost-da "override username/icon" söndürülübsə bu sahələr sadəcə nəzərə alınmır |
| Hər hadisə üçün bir-iki sətir azərbaycanca mətn: yaradıldı, status (`🔄 QA-12 "Başlıq": To Do → In Progress — Alice`), icraçı, sahə dəyişikliyi, şərh (+ qeyd edilənlər), silinmə, test ping. Naməlum hadisələr üçün ümumi sətir | Kanal oxunaqlı qalır; adlar (aktor, status, istifadəçi) çatdırılma anında DB-dən oxunur |
| Tapşırıq keçidi `NEBULA_PUBLIC_URL/w/{ws}/p/{layihə}?item={n}`, Slack sintaksisi ilə `<url\|QA-12>` (Mattermost bunu markdown keçidinə çevirir) | Hər iki serverdə işləyir. `NEBULA_PUBLIC_URL` boşdursa keçid yoxdur. Konfiqurasiyada mütləq https URL (http yalnız development), credential/query/fragment qadağandır |
| `slack` üçün imza başlıqları (`Nebula-Signature`, `Nebula-Event-*`) göndərilmir | Chat serverləri onları yoxlamır; mesajın həqiqiliyini URL-in gizliliyi təmin edir |
| Çatdırılma jurnalı, retry/backoff, 50 uğursuzluqdan sonra söndürmə, SSRF qorunması **eynidir** | Eyni `deliver_webhook` işi, yalnız gövdə fərqlidir |

### İstifadəçi mətninin təhlükəsizliyi (escaping)

Başlıq, şərh, status və istifadəçi adları istifadəçi tərəfindən yazılır. Onlar kanalda hamını
çağıran mention (`@channel`, `@all`, `@here`, `<!here>`) və ya maskalanmış keçid (`[Giriş](http://evil)`,
`<http://evil|Giriş>`) yarada bilməməlidir. Hər istifadəçi mətni üçün:

- `&`, `<`, `>` → `&amp;`, `&lt;`, `&gt;` (Slack bunu tələb edir); `&lt;`-dən sonra görünməz
  WORD JOINER (U+2060) qoyulur ki, entity-ləri əvvəlcə açan server də `<!channel>` və ya `<url|mətn>` görməsin;
- `@`-dan sonra WORD JOINER: heç bir mention işləmir (şərhdəki `<@uuid>` `@Ad` kimi göstərilir, amma heç kimi çağırmır);
- `[`, `]`, `(`, `)`, `*`, `_`, `~`, `|`, `\` və backtick əks-xətt ilə escape olunur (keçid və formatlama yoxdur);
- yeni sətir, idarəetmə və bidi (U+202E və s.) simvolları boşluğa çevrilir: mətn yeni blok aça və ya
  mesajı vizual olaraq tərsinə çevirə bilməz;
- mesaj 3000 simvolla məhdudlaşır.

Çılpaq URL (`https://...`) mətndə olduğu kimi görünür və chat serveri onu keçidə çevirə bilər; bu,
gizli keçid deyil (istifadəçi ünvanı görür). Məlum məhdudiyyət: Slack əks-xətt escape-ni tanımır,
ona görə Slack-də istifadəçi mətnindəki `*_~[]|` simvollarından əvvəl `\` görünə bilər. Əsas hədəf Mattermost-dur.

### URL gizli açardır

| Qərar | Səbəb |
|---|---|
| `slack` URL-i yalnız AES-256-GCM ilə şifrələnmiş saxlanılır (`webhooks.url_encrypted`, `internal/secretbox`), AAD = `webhook-url:` + webhook ID | Signing secret ilə eyni qoruma; AAD ciphertext-in başqa sətrə və ya secret sütununa köçürülməsinin qarşısını alır |
| `url` sütununda və API-də yalnız maska: `https://host/…` + son 4 simvol (yol ≥ 16 simvol olduqda) | Webhook-ları bir-birindən ayırmaq olur, token görünmür. Audit hadisəsi (`webhook.created/updated`) də maskanı yazır |
| `nebula rotate-secrets` həm secret-ləri, həm şifrələnmiş URL-ləri yenidən şifrələyir | Açar rotasiyası (ADR-0007) dəyişmədən işləyir |
| Çatdırılma xətalarından URL çıxarılır (`Post "https://…": ...` → `Post: ...`) — jurnal, `jobs.last_error` və loglar üçün | `net/http` xətaları tam URL-i ehtiva edir; bu, token-in admin UI-a və loglara sızması demək olardı. Bu, `nebula` formatına da tətbiq olunur |
| `slack` webhook-un secret-i yoxdur: yaradılarkən `secret: null`, `rotate-secret` → 409 `not_signed` | Credential-ı yeniləmək üçün Mattermost-da yeni URL yaradılır və `PATCH {url}` edilir (yenidən şifrələnir) |
| Miqrasiya `00007`: `format` (CHECK `nebula`/`slack`), `url_encrypted`, `format = 'slack'` ⇔ `url_encrypted IS NOT NULL` | DB səviyyəsində ardıcıllıq. `Down` əvvəl `slack` webhook-larını söndürür (`disabled_reason`), sonra sütunları silir: maskalanmış URL-ə Nebula JSON göndərilməməlidir. Rollback-dan sonra həmin webhook-ların URL-i itir, yenidən yaradılmalıdır |

## 2. Etibarlı proxy-lər (`NEBULA_TRUSTED_PROXIES`)

`X-Forwarded-For`-u (XFF) istənilən client saxtalaşdıra bilər. Ona görə:

- Default boşdur: davranış əvvəlki kimidir, yalnız TCP peer.
- Dəyər: vergüllə CIDR-lər və ya tək IP-lər (`10.0.0.0/8, fd00::/8, 192.0.2.1`). Səhv CIDR, host bitləri
  qoyulmuş prefiks (`10.0.0.1/8`) və ya zona ilə IP → **işə düşmə xətası**.
- Alqoritm:
  1. Peer (`RemoteAddr`) etibarlı deyilsə → peer istifadə olunur, XFF **tamamilə nəzərə alınmır**.
  2. Peer etibarlıdırsa → bütün XFF sətirləri (bir neçə başlıq sətri ardıcıl birləşdirilir) **sağdan sola**
     gəzilir; etibarlı olmayan ilk ünvan client-dir. Ondan soldakı hər şey client-in öz yazdığıdır və nəzərə alınmır.
  3. Gəzilən hissədə səhv element (IP deyil, boş, zona ilə), başlıqda 128-dən çox element, ardıcıl 32-dən çox
     etibarlı hop və ya XFF yoxdursa → peer.
     `ip:port` və `[ipv6]:port` qəbul edilir; IPv4-mapped IPv6 (`::ffff:a.b.c.d`) IPv4 kimi müqayisə olunur.
- Sol tərəfdən götürmək (`X-Forwarded-For`-un birinci elementi) **seçilmədi**: client onu istədiyi kimi yazır və
  hər sorğuda yeni "vedrə" ala bilərdi. `X-Real-IP` və `Forwarded` (RFC 7239) də dəstəklənmir: bir mənbə,
  bir qayda.
- Access log client IP yazmır (əvvəl də yazmırdı); IP-ni yalnız rate limiter istifadə edir.

### Əməliyyat qeydləri

- Yalnız **həqiqətən öz proxy-nizin** ünvanlarını yazın. Bütün pod şəbəkəsini (`10.0.0.0/8`) yazmaq o deməkdir ki,
  klasterdəki istənilən pod XFF ilə istədiyi IP-ni seçə bilər. Mümkünsə ingress controller pod-larının CIDR-i və
  NetworkPolicy ilə API-yə yalnız ingress-dən giriş.
- Proxy XFF-i **əlavə etməlidir** (append), əvəz etməməlidir; Caddy və ingress-nginx default olaraq belədir.
  ingress-nginx-də `use-forwarded-headers` yalnız ondan əvvəl başqa etibarlı proxy (məs. bulud LB) varsa açılmalıdır.

## Nəticələr

- FinTez status bildirişlərini Mattermost kanallarına Nebula-dan ala bilər; hər kanal üçün ayrıca webhook və hadisə filtri.
- Real Mattermost/Slack serverinə çatdırılma avtomatik testlərdə yoxlanılmır (fake receiver ilə inteqrasiya testi var).
- Sonrakı addım: Mattermost "attachments" (rəngli kartlar) və gecikmə xülasələri (ADR-0009 ilə birlikdə).
