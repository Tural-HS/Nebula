# ADR-0019: Telefon və planşet üçün veb interfeys

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

Mobil tətbiq yoxdur və olmayacaq: FinTez komandası Mattermost Boards-dan da telefon brauzerində istifadə edirdi.
Nebula-nın veb interfeysi isə masaüstü üçün qurulmuşdu: lövhədə sürükləmə yalnız `PointerSensor` (6 px) ilə
işləyirdi, ona görə telefonda kartın üstündən sürüşdürmək lövhəni deyil, kartı tuturdu; alətlər paneli 375 px-də
4–5 sətrə bükülürdü; başlıq və bəzi səhifələr yana sürüşürdü; iOS Safari 14 px mətnli sahəyə fokusda səhifəni
böyüdürdü; tapşırıq paneli klaviatura altında qalırdı.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Lövhədə ayrı sensorlar: `MouseSensor` (6 px məsafə), `TouchSensor` (**250 ms basılı saxlama, 8 px tolerans**), `KeyboardSensor` (Space qaldırır, oxlar, Space/Enter buraxır, Esc ləğv; Enter kartı açır) | Sürətli sürüşdürmə həmişə scroll-dur, kartı götürmək üçün barmaq bir an dayanmalıdır — Trello/Jira mobil davranışı. Gözləmə zamanı 8 px-dən çox hərəkət scroll sayılır və sürükləməni ləğv edir | `PointerSensor` + `touch-action: none`: kartların üstündə scroll tamam itir. Tək `PointerSensor` + delay: siçan üçün də gecikmə yaradır |
| `DragOverlay`: qaldırılan kart sütunların üstündə çəkilir, orijinalı solğun yer tutucu kimi qalır | Sütunun öz scroll konteyneri `transform` ilə hərəkət edən kartı kəsirdi; barmaq altında kart görünməlidir | — |
| Toqquşma: `pointerWithin`, boşdursa `rectIntersection` | Dar ekranda kart iki sütunu örtür; düşmə barmağın altındakı sütuna olmalıdır. Klaviatura ilə sürükləmədə göstərici yoxdur, örtüşməyə qayıdır | Yalnız `rectIntersection`: telefonda təsadüfi sütun |
| Telefonda sütunlar ekranın ~85%-i enində, `scroll-snap` ilə; **sürükləmə vaxtı snap söndürülür** | Növbəti sütun görünür (davamı olduğunu göstərir); snap dnd-kit-in auto-scroll-u ilə vuruşmasın | — |
| Layout iki cür dəyişir: üslub fərqləri CSS-dədir (`max-md:`, `pointer-coarse:`), **DOM fərqləri** (`md`-dən dar ekranda alətlər paneli və vərəq) `useMediaQuery` ilə yalnız bir variant render olunur | Hər iki variantı render edib birini CSS ilə gizlətmək eyni etiketli iki idarəetmə yaradır: ekran oxuyucuları və E2E testləri (strict locator) çaşır | Yalnız CSS: dublikat idarəetmələr |
| Toxunma ölçüləri `pointer-coarse:` ilə: düymələr, sahələr, tablar ən azı **44 px**, sahələrdə **16 px** mətn | Siçan istifadəçisi üçün sıxlıq dəyişmir; planşet də (geniş, amma toxunma) böyük hədəf alır. 16 px iOS-un fokusda zoom etməsinin qarşısını alır | Ekran eninə görə (`sm:`): toxunmalı planşetdə kiçik hədəflər, siçanlı dar pəncərədə lazımsız böyük |
| Telefon alətlər paneli bir sətirdir: axtarış, "Filtrlər" (aktiv filtr sayı ilə) və Cədvəl/Lövhə; görünüşlər, icraçı, son tarix, status və sahə filtrləri, sıralama, qruplaşdırma və sütunlar aşağıdan açılan **vərəqdədir** (`Dialog variant="sheet"`, native `<dialog>`) | Axtarış ən çox istifadə olunur və görünür qalır; qalanı bir toxunuş uzaqdadır. Native `<dialog>` fokus tələsini və Esc-i pulsuz verir | Üfüqi scroll olunan panel: gizli idarəetmələr tapılmır |
| Cədvəl telefonda: başlıq sütunu birinci və **sabitdir** (sticky), açar ikinci sətir kimi başlığın içindədir; qalan sütunlar yana sürüşür. Başlıq, sətirlər və cəm sətri eyni sütun siyahısını işlədir (`displayColumns`) | 375 px-də açar + başlıq + növbəti sütunun başlanğıcı görünür; virtualizasiya dəyişmir | Kart siyahısına keçmək: fərdi sahə sütunları və cəm itir |
| Tapşırıq paneli telefonda tam ekrandır, başlığı sticky, bağla düyməsi 44 px; safe-area padding. Klaviatura: Android üçün `interactive-widget=resizes-content`, iOS üçün panelin alt kənarı Visual Viewport API ilə klaviaturanın üstünə qaldırılır və fokusdakı sahə görünən hissəyə gətirilir | iOS layout viewport-u klaviatura üçün kiçiltmir; `dvh` də kömək etmir | — |
| Viewport: `viewport-fit=cover` (safe-area), zoom **qadağan edilmir** | `maximum-scale=1` / `user-scalable=no` WCAG 1.4.4-ü pozur | — |
| E2E: `web/e2e/mobile.spec.ts` üç Playwright layihəsində (Pixel 7, 375 px iPhone 13 mini, iPad mini landşaft), toxunma CDP `Input.dispatchTouchEvent` ilə | Real toxunma hadisələri və brauzerin öz scroll-u; CI yalnız Chromium quraşdırır | WebKit layihəsi: CI-də əlavə brauzer və vaxt. Safari-nin özü real cihazda yoxlanılmalıdır |

## Nəticələr

- Siçan və klaviatura ilə sürükləmə əvvəlki kimi işləyir; kartın öz `onKeyDown`-u artıq klaviatura sensorunu
  udmur (əvvəl Enter yalnız kartı açırdı, klaviatura ilə sürükləmə mümkün deyildi).
- Toxunma ilə kartı götürmək üçün 250 ms basılı saxlamaq lazımdır; bu, ilk dəfə istifadə edənə görünmür. Kart
  tutulanda solğunlaşır və kölgəli nüsxə barmağı izləyir.
- Telefonda filtr üçün bir toxunuş artıq lazımdır; aktiv filtr sayı düymədə görünür.
- `pointer-coarse` brauzerin bildirdiyinə əsaslanır: toxunma ekranlı noutbukda siçan istifadə olunsa da
  hədəflər böyük ola bilər — qəbul edilən kompromis.
- Real iOS Safari və Android Chrome-da əl ilə yoxlama hələ lazımdır (klaviatura, safe-area, uzun basma menyusu).
