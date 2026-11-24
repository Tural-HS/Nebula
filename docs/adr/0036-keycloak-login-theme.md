# ADR-0036: Nebula brendli Keycloak giriş səhifəsi

- Status: Qəbul edilib
- Tarix: 2026-11-24

## Kontekst

Veb interfeys round 7-də yeni dizayn aldı (ADR-0035: "Ledger", firuzəyi brend, açıq/tünd rejim). Giriş səhifəsi isə
Keycloak-ın standart `keycloak.v2` mövzusu idi: Keycloak fon şəkli, mavi düymələr, böyük hərflərlə realm adı. FinTez
pilotunda istifadəçinin Nebula-da gördüyü ilk ekran budur. ADR-0025 yalnız dilləri əlavə etmişdi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Mövzu `nebula` (yalnız `login` tipi), `parent=keycloak.v2`, `import=common/keycloak`; **yalnız CSS və şəkillər**, heç bir FreeMarker şablonu dəyişdirilmir | Keycloak yeniləmələri öz şablon düzəlişlərini (təhlükəsizlik daxil) gətirir; bizdə saxlanacaq kopya yoxdur | Keycloakify (React): ayrıca build, npm asılılıqları, hər Keycloak versiyasında uyğunluq yoxlaması. Şablonları kopyalamaq: hər upgrade-də əl ilə birləşdirmə |
| Stillər: `styles=css/styles.css css/nebula.css?v=N` — əvvəl `keycloak.v2`-nin öz `styles.css`-i (valideyndən), sonra Nebula qatı | PatternFly 5 qlobal tokenləri `:where(:root)` və `:where(.pf-v5-theme-dark)` ilə (sıfır spesifiklik) elan edir — `:root`/`.pf-v5-theme-dark` qaydaları `!important` olmadan qalib gəlir | PatternFly CSS-ini əvəz etmək: böyük, Keycloak versiyası ilə ayrışır |
| Rənglər `web/src/index.css`-dəki brend şkalası (OKLCH → sRGB hex) və zinc neytralları: əsas düymə brand-600 (ağ mətn 5,1:1), linklər brand-700 (açıq, 7,3:1) / brand-300 (tünd), fokus halqası brand-500/400. PatternFly-ın kontekst variantları (`--primary-color--dark-100`, `--link--Color--light` və s.) da yönləndirilir — əks halda forma sahələrinin fokus xətti mavi qalırdı | Tətbiqlə eyni görünüş; kontrast AA | — |
| Status rəngləri (xəta/xəbərdarlıq/info alert-ləri, sahə xətası) PatternFly-ınkı qalır | Onlar iki rejimdə AA-dır (ölçülüb: minimum 5,46:1); qırmızı = xəta mənası tətbiqlə eynidir | — |
| Başlıq: `#kc-header-wrapper::before` ilə loqo (`img/logo-mark.png`, tətbiqin faylı), altında realm-ın **display name**-i (böyük hərfsiz). Favicon tətbiqin `favicon.png`-indən ICO | Şablona toxunmadan; realm adı realm parametrindən gəlir (prod: "Nebula") | `displayNameHtml`-də `<img>`: realm datasında HTML, hər realm üçün ayrıca |
| Şrift: tətbiqin sistem şrift yığını; PatternFly-ın RedHat şriftləri istifadə edilmir (yüklənmir) | Xarici resurs yoxdur, tətbiqlə eyni | Self-host şrift: ADR-0035-də açıq sual |
| Tünd rejim: `darkMode=true` — `keycloak.v2`-nin öz skripti OS `prefers-color-scheme`-ə görə `<html>`-ə `pf-v5-theme-dark` qoyur; Nebula tokenləri bu sinfə bağlanır | Keycloak mexanizmi, öz JS-imiz yoxdur. Tətbiqdəki "Açıq/Tünd" seçimi (ADR-0035, `localStorage` başqa origin-dədir) giriş səhifəsinə keçmir — giriş səhifəsi həmişə OS-u izləyir | — |
| Telefon (375 px): 12 px kənar boşluq, kart sərhədi görünür; dil seçicisi masaüstündə öz enində (maks. 11rem), başlıq sıxılmır | ADR-0019 | — |
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
bitmişdir", giriş cəhdinin vaxtı bitməsi, çıxış təsdiqi, dil seçicisi — açıq/tünd × 1280/375 × az/ru. Xarici sorğu
yoxdur, konsolda CSP xətası yoxdur (Keycloak-ın CSP-si: `frame-src 'self'; frame-ancestors 'self'; object-src
'none'`), ölçülən mətn kontrastı ≥ 5,1:1, hesablanmış stillərdə PatternFly mavisi qalmayıb. Production image
`start --optimized` ilə: mövzu yüklənir, `kcadm` komandası realm-ı `keycloak.v2` → `nebula` dəyişir.

## Nəticələr

- Keycloak versiyası yüksəldiləndə: `keycloak.v2` markup-ında dəyişiklik varsa `nebula.css`-də seçicilər
  (`#kc-header-wrapper`, `.pf-v5-c-login__*`, `#kc-logout-confirm .instruction`) yoxlanmalıdır; PatternFly 6-ya
  keçən `keycloak.v3` olarsa mövzu yenidən baxılmalıdır.
- Tətbiqin rəng rejimi seçimi giriş səhifəsinə ötürülmür (OS-u izləyir).
- Workspace vurğu rəngi (ADR-0035) giriş səhifəsində yoxdur — giriş workspace kontekstindən kənardır, həmişə firuzəyi.

## Açıq suallar

- Production realm-ın `displayName`-i "Nebula"-dır; müştəri adı ("FinTez — Nebula") lazımdırsa, realm parametri ilə
  dəyişdirilir, mövzu dəyişmir.
- Email mövzusu (`emailTheme`) hələ standartdır — ayrıca iş.
