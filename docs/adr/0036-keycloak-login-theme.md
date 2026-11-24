# ADR-0036: Nebula brendli Keycloak giriş səhifəsi

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Veb interfeys round 7-də yeni dizayn aldı (ADR-0035: "Ledger", firuzəyi brend, açıq/tünd rejim). Giriş səhifəsi isə
Keycloak-ın standart `keycloak.v2` mövzusu idi: Keycloak fon şəkli, mavi düymələr, böyük hərflərlə realm adı. FinTez
pilotunda istifadəçinin Nebula-da gördüyü ilk ekran budur. ADR-0025 yalnız dilləri əlavə etmişdi.

Birinci versiya (rəngləri dəyişmiş standart kart) istifadəçiyə "standart Keycloak-a çox oxşar, loqo zəif, rəng solğun"
göründü; ikinci versiya brendi açıq göstərən iki panelli səhifədir (aşağıda).

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Mövzu `nebula` (yalnız `login` tipi), `parent=keycloak.v2`, `import=common/keycloak`; **yalnız CSS, şəkillər və mesaj faylları**, heç bir FreeMarker şablonu dəyişdirilmir | Keycloak yeniləmələri öz şablon düzəlişlərini (təhlükəsizlik daxil) gətirir; bizdə saxlanacaq kopya yoxdur | Keycloakify (React): ayrıca build, npm asılılıqları, hər Keycloak versiyasında uyğunluq yoxlaması. Şablonları kopyalamaq: hər upgrade-də əl ilə birləşdirmə |
| Stillər: `styles=css/styles.css css/nebula.css?v=N` (hazırda `v=2`) — əvvəl `keycloak.v2`-nin öz `styles.css`-i (valideyndən), sonra Nebula qatı | PatternFly 5 qlobal tokenləri `:where(:root)` və `:where(.pf-v5-theme-dark)` ilə (sıfır spesifiklik) elan edir — `:root`/`.pf-v5-theme-dark` qaydaları `!important` olmadan qalib gəlir | PatternFly CSS-ini əvəz etmək: böyük, Keycloak versiyası ilə ayrışır |
| Rənglər `web/src/index.css`-dəki brend şkalası (OKLCH → sRGB hex) və zinc neytralları. Əsas düymə brand-600-ün açıqlığında **daha doymuş firuzəyi** `#007b89` (oklch 0.52 0.12 205; ağ mətn 5,0:1), hover `#00657c` (6,7:1); linklər `#00657c` (açıq) / brand-300 (tünd, 10,6:1); sahə fokusda bütün çərçivə firuzəyi + yumşaq halo, fokus halqası ≥ 3:1. Tünd rejimdə brend paneli eyni qalır, forma fonu tətbiqin zinc-900/950-i. PatternFly-ın kontekst variantları (`--primary-color--dark-100`, `--link--Color--light` və s.) da yönləndirilir — əks halda forma sahələrinin fokus xətti mavi qalırdı | Tətbiqlə eyni görünüş; kontrast AA | — |
| Status rəngləri (xəta/xəbərdarlıq/info alert-ləri, sahə xətası) PatternFly-ınkı qalır | Onlar iki rejimdə AA-dır (ölçülüb: minimum 5,46:1); qırmızı = xəta mənası tətbiqlə eynidir | — |
| **Düzən:** ≥ 900 px — iki panel: solda (42%) tam hündürlükdə **brend paneli** (loqonun navy → indiqo → firuzəyi qradiyenti, yumşaq radial parıltılar və bir neçə "ulduz", hamısı CSS qradiyenti), ortada 112 px loqo, böyük "Nebula" və bir sətirlik şüar; sağda neytral fonda forma kartı. < 900 px — yuxarıda qradiyent zolaq (loqo + "Nebula" + şüar; telefonda "ulduz"suz, çünki mətn zolağı tutur), altında forma. Keycloak-ın `#kc-header`-i panelə çevrilir (CSS grid) | Brend ilk baxışda görünür, forma sadə qalır; 375 px-də üfüqi daşma yoxdur | Fon şəkli: əlavə fayl və ölçü; şablonu dəyişmək: aşağıya bax |
| **Panelin mətni mesajdandır:** `messages/messages_{az,en,tr,ru}.properties`-də `loginTitleHtml` (`keycloak.v2`-nin `template.ftl`-i onu `#kc-header-wrapper`-də `kcSanitize` ilə çıxarır; Keycloak-ın sanitizer siyasəti `div/span/strong/small` və `class`, `title` atributlarına icazə verir). Şüar: az "Komandanızın işləri bir yerdə", en "Your team's work in one place", tr "Ekibinizin işleri tek yerde", ru "Все задачи команды в одном месте". Loqo `#kc-header-wrapper::before` ilə (`img/logo-mark.png`, tətbiqin faylı); favicon tətbiqin `favicon.png`-indən ICO | Tərcümə digər Keycloak mətnləri kimi mesaj faylındadır, şablon override lazım olmadı. Mesaj MessageFormat-dır: apostrof `''` yazılır | `template.ftl` və ya `login.ftl` override: hər Keycloak upgrade-də birləşdirmə; CSS `content:` + `:lang()`: mətn CSS-də, tərcümə faylsız |
| Realm-ın display name-i (`{0}`) formanın altında **kiçik başlıq** kimi; `title` atributu "Nebula"-dırsa (production) CSS gizlədir | Dev ("Nebula (local development only)") və prod ayırd edilir, prod-da təkrar yoxdur | — |
| Şrift: tətbiqin sistem şrift yığını; PatternFly-ın RedHat şriftləri istifadə edilmir (yüklənmir) | Xarici resurs yoxdur, tətbiqlə eyni | Self-host şrift: ADR-0035-də açıq sual |
| Tünd rejim: `darkMode=true` — `keycloak.v2`-nin öz skripti OS `prefers-color-scheme`-ə görə `<html>`-ə `pf-v5-theme-dark` qoyur; Nebula tokenləri bu sinfə bağlanır | Keycloak mexanizmi, öz JS-imiz yoxdur. Tətbiqdəki "Açıq/Tünd" seçimi (ADR-0035, `localStorage` başqa origin-dədir) giriş səhifəsinə keçmir — giriş səhifəsi həmişə OS-u izləyir | — |
| Telefon (375 px): kart kənarlardan 12 px; dil seçicisi masaüstündə öz enində (maks. 11rem) | ADR-0019 | — |
| Element id-ləri dəyişmir (`#username`, `#password`, `#kc-login`) | E2E (`web/e2e/fixtures.ts`, `auth.spec.ts`) | — |
| **Keş:** Keycloak mövzu fayllarını `/resources/<versiya>/...` altında `Cache-Control: max-age=2592000` (30 gün) ilə verir və `<versiya>` yalnız Keycloak versiyasından və build seçimlərindən asılıdır (eyni 26.8.2 + eyni seçimlər → eyni yol, `--no-cache` build ilə yoxlanıb). Ona görə `nebula.css` dəyişəndə `theme.properties`-də `?v=N` **artırılmalıdır** | Əks halda brauzerlər 30 günə qədər köhnə CSS-i göstərir | Faylın adını dəyişmək: eyni effekt, daha çox diff |
| Production: mövzu image-ə `/opt/keycloak/themes/nebula` kimi kopyalanır, **`kc.sh build`-dən sonra** (mövzular runtime-da oxunur; CSS dəyişikliyi build-i yenidən işə salmır). Keycloak-ın mövzu keşi açıq qalır. Dev: eyni qovluq `docker-compose.yml`-də read-only mount; `start-dev` mövzuları keşləmir — CSS-i dəyişib səhifəni yeniləmək kifayətdir | Bir mənbə (`deploy/production/keycloak/themes/nebula`), iki mühit | — |
| Realm: hər iki realm JSON-da `"loginTheme": "nebula"` | Yeni quraşdırmalar avtomatik | — |

## Mövcud realm-lar

Realm yalnız mövcud deyilsə import olunur, ona görə artıq yaradılmış production realm-da mövzu **bir dəfə** əl ilə
seçilir (yeni image deploy edildikdən sonra): admin konsol → Realm settings → Themes → Login theme: `nebula`, və ya

```bash
docker compose exec -it keycloak /opt/keycloak/bin/kcadm.sh update realms/nebula -s loginTheme=nebula \
  --no-config --server http://localhost:8080 --realm master --user <admin>
```

(`deploy/production/README.md` §3). Geri qaytarmaq: eyni komanda `loginTheme=keycloak.v2` ilə — dərhal qüvvəyə minir,
restart lazım deyil. Dev realm üçün eyni komanda (`docker compose exec keycloak … --user admin`), ya da realm-ın
yenidən import edilməsi.

## Yoxlama

Ayrıca Keycloak 26.8.2 konteynerində (start-dev, mövzu mount ilə) Playwright: giriş, səhv parol, "Səhifə vaxtı
bitmişdir", çıxış təsdiqi — açıq/tünd × 1280/375 × az/ru; dil dəyişməsi (az → ru) və uğurlu giriş. Xarici sorğu
yoxdur, konsolda CSP xətası yoxdur (Keycloak-ın CSP-si: `frame-src 'self'; frame-ancestors 'self'; object-src
'none'`), üfüqi daşma yoxdur. Forma mətninin kontrastı ≥ 5,0:1; qradiyent üzərindəki mətn (ad, şüar) **piksellə**
ölçülüb (mətn gizlədilib, arxadakı ən pis piksel götürülüb) — ≥ 6,5:1. Hesablanmış stillərdə PatternFly mavisi
qalmayıb; production adı "Nebula" olanda realm başlığı gizlənir. Production image
`start --optimized` ilə: mövzu yüklənir, `kcadm` komandası realm-ı `keycloak.v2` → `nebula` dəyişir.

## Nəticələr

- Keycloak versiyası yüksəldiləndə: `keycloak.v2` markup-ında dəyişiklik varsa `nebula.css`-də seçicilər
  (`#kc-header`, `#kc-header-wrapper`, `.pf-v5-c-login__container`/`__main`, `#kc-logout-confirm .instruction`)
  yoxlanmalıdır. Ən həssas nöqtə: `template.ftl`-in başlıqda `loginTitleHtml`-i `kcSanitize` ilə çıxarması və
  sanitizer-in `div/span/strong/small` + `class/title`-a icazəsi — bunlardan biri dəyişsə panel mətni düz mətnə
  çevrilir (loqo və qradiyent qalır). PatternFly 6-ya keçən `keycloak.v3` olarsa mövzu yenidən baxılmalıdır.
- Tətbiqin rəng rejimi seçimi giriş səhifəsinə ötürülmür (OS-u izləyir).
- Workspace vurğu rəngi (ADR-0035) giriş səhifəsində yoxdur — giriş workspace kontekstindən kənardır, həmişə firuzəyi.

## Açıq suallar

- Production realm-ın `displayName`-i "Nebula"-dır; müştəri adı ("FinTez — Nebula") lazımdırsa, realm parametri ilə
  dəyişdirilir, mövzu dəyişmir.
- Email mövzusu (`emailTheme`) hələ standartdır — ayrıca iş.
