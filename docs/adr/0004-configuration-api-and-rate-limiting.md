# ADR-0004: Konfiqurasiya API (workflow, status, item tipi) və rate limiting

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Konfiqurasiya API

Workflow-lar, statuslar və item tipləri adminlər tərəfindən API vasitəsilə idarə olunur.
Hər dəyişiklik `events` cədvəlində audit hadisəsi yaradır.

### Qaydalar

| Qayda | Səbəb |
|---|---|
| Hər workflow-un dəqiq bir başlanğıc (`is_initial`) statusu var; yeni workflow-da birinci status başlanğıcdır | Yeni item-in hansı statusda başlayacağı həmişə müəyyəndir |
| Başlanğıc statusu "söndürmək" olmaz, yalnız başqa statusu başlanğıc etmək olar | Workflow heç vaxt başlanğıcsız qalmır |
| İstifadə olunan, başlanğıc və ya sonuncu statusu silmək olmaz (409) | Item-lər yetim statusla qalmır; baza FK ilə də qoruyur |
| Workflow başına ən çox 50 status, workspace başına ən çox 100 aktiv item tipi | Sui-istifadə və UI-nin yüklənməsinə qarşı |
| Item tipinin workflow-u dəyişdirilmir | Mövcud item-ləri başqa workflow-a köçürmək status xəritələməsi tələb edir (gələcək iş) |
| Item tipi silinmir, **arxivlənir**: yeni item-lər üçün bağlanır, köhnə item-lər tipini saxlayır | Tarixçə və məlumat itmir |
| Arxivlənmiş tiplərin açarları rezerv qalır | Köhnə item-lər və inteqrasiyalar qarışmır |
| Sonuncu aktiv item tipini arxivləmək olmaz | Workspace həmişə item yarada bilir |
| Struktur dəyişiklikləri workflow sətrini kilidləyir (`FOR UPDATE`) | Paralel admin əməliyyatları limitləri və invariantları poza bilmir |
| Oxumaq bütün üzvlər üçün açıqdır, yazmaq admin+ tələb edir | Klientlər item-ləri göstərmək üçün konfiqurasiyanı oxumalıdır |

Status keçidlərinin məcburi tətbiqi (transitions) Phase 1-in mövzusudur. Hazırda status eyni workflow
daxilində istənilən statusa dəyişdirilə bilər, başqa workflow-un statusu isə rədd edilir (422).

## Rate limiting

İki qat token bucket istifadə olunur (`golang.org/x/time/rate`):

| Qat | Açar | Default | Nə vaxt |
|---|---|---|---|
| IP başına | TCP peer ünvanı | 100 rps, burst 200 | Autentifikasiyadan **əvvəl**: saxta token selləri imza yoxlamasına və bazaya çatmamış kəsilir |
| İstifadəçi başına | istifadəçi ID-si | 20 rps, burst 40 | Autentifikasiyadan sonra: bir istifadəçinin fərqli IP-lərdən gəlməsi limiti sıfırlamır |

- Limit aşıldıqda cavab `429 rate_limited` və `Retry-After` başlığıdır. Rədd edilən sorğu gələcək token-ləri istehlak etmir.
- `/healthz`, `/readyz` və `/api/v1/version` limitlənmir.
- `X-Forwarded-For` bilərəkdən nəzərə alınmır, çünki klient onu saxtalaşdıra bilər. Reverse proxy arxasında
  IP limitini ingress-də tətbiq edin, Nebula-da isə `NEBULA_RATE_LIMIT_IP_RPS=0` ilə söndürün.
- Yaddaş məhduddur: 10 dəqiqə istifadə olunmayan bucket-lər silinir, ümumi açar sayı 100 000 ilə məhdudlaşdırılıb.

### Bilinən məhdudiyyət

Limitlər hər replikanın yaddaşında saxlanılır: N replika ilə faktiki limit N dəfə çoxdur.
Bu, sui-istifadəyə qarşı qoruma kimi qəbul ediləndir. Dəqiq qlobal limit (məs. kvota, ödənişli plan)
lazım olduqda eyni middleware arxasında Redis əsaslı limiter qoşulacaq.
