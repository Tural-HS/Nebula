# ADR-0008: Lövhədə status sütunları və sütun-sütun yükləmə

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

Lövhə (kanban) əvvəl tapşırıqları 4 kateqoriyaya (Görüləcək / İcrada / Hazır / Ləğv edilib) qruplaşdırırdı
və layihənin **ilk 500 tapşırığını** brauzerə yükləyib orada bölürdü. İki problem:

1. Komandalar öz statuslarını görmək istəyir (məs. "Plan", "Gözləyir", "Gecikmə"), kateqoriyanı yox.
2. Böyük layihədə (FinTez-in Yığım lövhəsi kimi, ~1800 kart) 500-dən sonrakı kartlar lövhədə görünmürdü,
   sütun sayları da yalnız yüklənmiş kartları göstərirdi.

Tapşırıq tipləri və workflow-lar workspace səviyyəsindədir: bir layihədə fərqli workflow-lu tiplər ola bilər.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| Default qruplaşdırma: **status üzrə**; "Kateqoriya üzrə" seçimi qalır (`?group=category`) | Gündəlik iş statuslarla gedir; kateqoriya bir neçə komandanın ümumi mənzərəsi üçün faydalıdır |
| Eyni ad + eyni kateqoriyalı statuslar (fərqli workflow-larda) **bir sütunda birləşir**; ad müqayisəsi böyük/kiçik hərfə və boşluqlara həssas deyil | "Done" bug və task workflow-larında eyni sütundur. Fərqli kateqoriyalar heç vaxt birləşmir |
| Sıra: kateqoriya → workflow-dakı mövqe → tiplərin sırası | Proqnozlaşdırıla bilən, admin ekranındakı sıraya uyğun |
| Hər sütun **öz sorğusu** ilə yüklənir (`?status=a,b&limit=50`), aşağı sürüşdürdükcə növbəti səhifə gəlir | Lövhə ölçüsündən asılı olmayaraq sürətli; heç bir kart "kəsilmir" |
| Sütun sayları üçün yeni endpoint: `GET …/items/counts` — siyahı ilə **eyni filtrlər**, status üzrə say | Başlıqda yüklənmiş deyil, həqiqi say göstərilir |
| Yeni indeks `(project_id, status_id, number DESC) WHERE deleted_at IS NULL`, köhnə `(project_id, status_id)` silinir; `CONCURRENTLY` ilə | Hər sütun səhifəsi indeksdən birbaşa oxunur; böyük cədvəldə yazmalar bloklanmır |
| Kart yalnız **öz workflow-unun** statusuna keçə bilər; sürükləmə başlayanda keçidlər yoxlanır, icazəsiz sütunlar solğunlaşır | Workflow qaydaları UI-da da görünür; server onsuz da yoxlayır (ADR-0005) |
| Arxivlənmiş tiplərin statuslarında qalan kartlar üçün "Digər" sütunu (yalnız oxumaq) | Heç bir kart lövhədən səssizcə yox olmur |
| Taşıma optimistikdir: kart dərhal yeni sütunda görünür, sütunlar serverdən yenilənənə qədər orada qalır | Titrəmə yoxdur; xəta olsa kart geri qayıdır və bildiriş çıxır |

## Rate limit ilə işləmə (bu iş zamanı tapılan xətalar)

- **429 heç vaxt təkrar edilmirdi** (bütün 4xx "düzəlməz" sayılırdı). Nəticə: bir 429 üzv siyahısını
  və ya başlıq dəyişikliyini birdəfəlik itirirdi; UI isə dəyişikliyi saxlanmış kimi göstərirdi.
- Həll (`web/src/lib/retry.ts`):
  - Oxuma: 429, 5xx və şəbəkə xətası 3 dəfəyə qədər təkrar edilir; `Retry-After` gözlənilir (maks. 30 s).
  - Yazma: **yalnız 429** təkrar edilir. Limiter handler-dən əvvəl işləyir, yəni 429 alan sorğu tətbiq
    olunmayıb. 5xx və şəbəkə xətası təkrar edilmir (POST iki dəfə yarana bilər).
- Bir taşımada sütunlar iki dəfə yenilənirdi (ikinci invalidasiya birincini ləğv edirdi);
  indi ikincisi birinciyə qoşulur (`cancelRefetch: false`).

## Nəticələr

- Lövhədə kart sayı limiti yoxdur; sorğu sayı = sütun sayı + 1 (counts). 10–15 sütunda problem deyil;
  çox böyük workflow-lar üçün gələcəkdə görünməyən sütunları gecikdirərək yükləmək olar.
- Hər kart dəyişikliyi bütün sütunları yeniləyir (başlıq kimi dəyişikliklər istənilən sütuna aid ola bilər).
  Real vaxt yeniləmələri (SSE/WebSocket) gələndə bu hədəfli olacaq.
- Default workflow statusları hələ ingiliscədir ("To Do", "In Progress"…). Şablonlar mərhələsində
  workspace dilinə görə yaradılacaq.

## Açıq məsələ (ayrıca iş)

Rate limiter müştərini yalnız `RemoteAddr` ilə tanıyır (`X-Forwarded-For` saxtalaşdırıla bildiyi üçün qəsdən
nəzərə alınmır). Reverse proxy (Caddy, ingress) arxasında bütün istifadəçilər proxy-nin bir IP-si kimi görünür
və IP limitini paylaşır. Production-dan əvvəl etibarlı proxy-lər (`NEBULA_TRUSTED_PROXIES`, CIDR) konfiqurasiyası
lazımdır: yalnız onlardan gələn `X-Forwarded-For` oxunmalıdır.
