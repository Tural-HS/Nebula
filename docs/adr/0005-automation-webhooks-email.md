# ADR-0005: Status keçidləri, avtomatlaşdırma, webhook-lar və email

- Status: Qəbul edilib
- Tarix: 2026-11-23

## 1. Status keçidləri

- Hər workflow-un `enforce_transitions` bayrağı var. Söndürülü olduqda (default) item eyni workflow-un istənilən
  statusuna keçə bilər. Aktiv olduqda yalnız təyin edilmiş keçidlərə icazə verilir.
- Keçid `from_status_id → to_status_id` cütüdür; `from_status_id: null` "istənilən statusdan" deməkdir (məs. "Ləğv et").
- İcazəsiz keçid `422` (`field: status_id`) qaytarır. Qayda avtomatlaşdırmaya da tətbiq olunur.
- Keçidlər olmadan enforcement-i aktiv etmək (`409 no_transitions`) və enforcement aktiv ikən sonuncu keçidi
  silmək (`409 last_transition`) qadağandır: əks halda heç bir item statusunu dəyişə bilməzdi.
- `GET .../items/{number}/transitions` item-in növbəti mümkün statuslarını qaytarır (UI düymələri üçün).

## 2. Hadisə çatdırılması: transactional outbox

```
 API sorğusu ──► tranzaksiya ┬─► items / workflows / ... dəyişikliyi
                             ├─► events (audit)
                             └─► jobs: event.dispatch          ◄── hamısı ya commit, ya rollback
                                      │
 worker (SKIP LOCKED) ◄───────────────┘
   event.dispatch ─► fan-out (dedupe açarı ilə, idempotent):
        ├─► webhook.deliver   (hər uyğun webhook üçün bir dəfə)
        └─► automation.run    (hər uyğun qayda üçün bir dəfə)
                 └─► email.send
```

- Hadisə ilə onun job-u **eyni tranzaksiyada** yazılır: dəyişiklik commit olunubsa, çatdırılma mütləq baş verəcək.
  Rollback olunubsa, nə hadisə, nə də job qalır. Server çöksə belə heç nə itmir.
- Worker-lər `FOR UPDATE SKIP LOCKED` ilə iş götürür. İstənilən sayda replika paralel işləyə bilər və eyni işi iki dəfə götürmür.
  Ölən worker-in işi `lease` (2 dəq) bitəndən sonra başqası tərəfindən yenidən götürülür.
- Təkrar cəhdlər eksponensial artır (15s, 30s, 1d ... maks. 6 saat, ±20% jitter). Qalıcı xətalar dərhal `failed` olur
  (məs. 4xx cavab, silinmiş hədəf, SSRF siyasəti).
- Çatdırılma **ən azı bir dəfə** (at-least-once) zəmanətlidir. Qəbul edən tərəf `Nebula-Event-Id` ilə dublikatları ayırmalıdır.
- Bitmiş job-lar 7 gündən, webhook çatdırılma logları 30 gündən sonra avtomatik silinir.
- Worker `nebula serve` daxilində işləyir (`NEBULA_WORKER_CONCURRENCY`, default 4). Ayrıca miqyaslamaq üçün
  `nebula worker` əmri var: API-də `NEBULA_WORKER_CONCURRENCY=0` qoyulur, worker isə ayrıca deployment kimi işə salınır.

## 3. Webhook-lar

### Təhlükəsizlik

| Təhdid | Tədbir |
|---|---|
| SSRF: admin webhook URL-i ilə daxili şəbəkəyə (`169.254.169.254`, `localhost`, `10.x`, `.internal`) çatmaq | URL yoxlaması + **qoşulma anında** IP yoxlaması (`net.Dialer.Control`, DNS həllindən sonra). DNS rebinding də işləmir |
| Gizlədilmiş IP yazılışları (`0x7f000001`, `2130706433`, `0177.0.0.1`) | URL səviyyəsində rədd edilir |
| Yönləndirmə (redirect) ilə daxilə keçmək | Redirect-lər heç vaxt izlənmir |
| Proxy ilə yoxlamadan yan keçmək | Mühit proxy-ləri söndürülüb |
| Daxili admin portları | Yalnız 80, 443 və 8000-8999 portlarına icazə var |
| Saxta webhook (qəbul edən tərəf üçün) | Hər sorğu HMAC-SHA256 ilə imzalanır |
| Secret-in sızması | Secret bazada AES-256-GCM ilə şifrələnir (sətir ID-si ilə bağlı), yalnız yaradılanda bir dəfə göstərilir, audit loguna yazılmır |
| Productionda http | Productionda yalnız https; `NEBULA_WEBHOOK_ALLOW_PRIVATE=true` productionda qadağandır |

### İmzanın yoxlanması (qəbul edən tərəf)

Başlıq: `Nebula-Signature: t=<unix vaxt>,v1=<hex>`. Burada `v1 = HMAC-SHA256(secret, "<t>.<xam gövdə>")`.

1. `t` və `v1`-i ayırın.
2. HMAC-ı **xam** gövdə üzərində hesablayın (JSON-u yenidən serializasiya etməyin).
3. Sabit vaxtlı müqayisə edin (`hmac.compare_digest`, `crypto/subtle`).
4. 5 dəqiqədən köhnə `t`-ni rədd edin (replay qoruması).

Digər başlıqlar: `Nebula-Event-Id`, `Nebula-Event-Type`, `Nebula-Delivery-Attempt`.

### Sağlamlıq

- `2xx` uğurlu sayılır. `408`, `429` və `5xx` təkrar cəhd edilir. Digər `4xx` qalıcı xətadır.
- Ardıcıl 50 uğursuz cəhddən sonra webhook avtomatik söndürülür (`disabled_reason`). Yenidən aktivləşdirmək sayğacı sıfırlayır.
- `GET .../webhooks/{id}/deliveries` son cəhdləri göstərir (status kodu, xəta, müddət).
- Workspace başına ən çox 20 webhook. Hadisə filtrləri: `*`, `item.*`, `item.updated`.

## 4. Avtomatlaşdırma qaydaları

"**Bu hadisə baş verəndə** və **bütün şərtlər** ödənəndə, **bu əməliyyatları** et."

- Trigger-lər: `item.created`, `item.updated`.
- Şərt sahələri: `type`, `project`, `status`, `status_category`, `assignee`, `reporter`, `title`, `fields.<açar>`
  və `changed` (yalnız `item.updated` üçün, `op: includes`).
- Operatorlar: `eq`, `neq`, `in`, `not_in`, `contains`, `is_empty`, `is_not_empty`.
- Əməliyyatlar: `set_status`, `set_assignee`, `set_field`, `send_email`, `copy_fields` (başqa layihəyə, [ADR-0056](0056-copy-fields-action.md)).

### Əsas qərarlar

| Qərar | Səbəb |
|---|---|
| Şərtlər **hadisə anındakı** item vəziyyətinə görə yoxlanılır (hadisədə saxlanan snapshot) | Eyni hadisəyə bağlı iki qaydanın nəticəsi icra sırasından asılı olmur (reqressiya testi var) |
| Avtomatlaşdırmanın yaratdığı hadisələr qaydaları **tetikləmir** (`events.source = automation`) | Sonsuz dövrələr mümkün deyil |
| Bu hadisələr yenə də webhook-lara çatdırılır və audit logunda qaydanın ID-si ilə qeyd olunur | İzlənə bilənlik |
| Bütün `set_*` əməliyyatları bir yeniləmədə tətbiq olunur, versiya konfliktində 3 dəfəyə qədər təkrar cəhd edilir | İnsan redaktəsi ilə paralel işləmə |
| Status keçid qaydaları avtomatlaşdırmaya da tətbiq olunur | Qayda workflow-u aşa bilməz |
| Email yalnız `assignee`, `reporter` və ya **workspace üzvlərinin** ünvanlarına göndərilir; üzvlük göndərmə anında yenidən yoxlanılır | Sistem spam relay-ə çevrilə bilməz |
| Dəyişikliyi edən şəxsə özü haqqında email göndərilmir | Lazımsız bildirişlər olmur |
| Şablonlar yalnız sabit yer tutucularla işləyir (`{{item.key}}`, `{{item.title}}`, `{{item.type}}`, `{{item.status}}`, `{{item.project}}`, `{{rule.name}}`, `{{workspace}}`) | Template injection mümkün deyil |
| Qaydalar yaradılanda ciddi yoxlanılır (status, istifadəçi, sahə mövcudluğu, ölçülər) | Səhvlər icra zamanı deyil, yaradılanda tutulur |
| Workspace başına ən çox 50 qayda, qayda başına 20 şərt və 10 əməliyyat | Sui-istifadəyə qarşı |

Hər yeni workspace iki standart qayda ilə yaradılır: item yaradılanda və ya yenidən təyin olunanda icraçıya email.
Bu qaydalar söndürülə və ya silinə bilər.

## 5. Email (SMTP)

- `NEBULA_SMTP_HOST` boşdursa email söndürülür: email job-ları xəta vermədən ötürülür.
- TLS rejimləri: `starttls` (587, default), `tls` (465), `none` (yalnız development).
  `starttls` rejimində server STARTTLS dəstəkləmirsə, göndərmə **rədd edilir** (downgrade yoxdur).
- Şifrələnməmiş bağlantıda şifrə heç vaxt göndərilmir.
- Header injection mümkün deyil: mövzu və ünvanlarda CR/LF rədd edilir, qeyri-ASCII mövzular RFC 2047 ilə kodlanır.
- Məktublar `Auto-Submitted: auto-generated` başlığı daşıyır, ona görə avtocavablar geri yazmır.
- Development-də bütün məktublar Mailpit-də görünür: http://localhost:8025.

## 6. Şifrələmə açarı

`NEBULA_ENCRYPTION_KEY` (32 bayt, base64) `serve` və `worker` üçün məcburidir. Yaratmaq üçün: `openssl rand -base64 32`.

- Açar secret store-da saxlanılmalıdır (External Secrets / OpenBao / Azure Key Vault). Git-də saxlanılmamalıdır.
- Açar itirilərsə, webhook secret-ləri bərpa oluna bilməz. Bu halda webhook-lar yenidən yaradılmalı və ya secret-ləri rotasiya edilməlidir.
- Açar rotasiyası (köhnə və yeni açarla paralel oxuma) gələcək ADR-in mövzusudur. Ciphertext formatında versiya baytı bunun üçün nəzərdə tutulub.

## Bilinən məhdudiyyətlər

- Webhook çatdırılma sırası zəmanətli deyil. `occurred_at` və `event_id` ilə sıralayın.
- Webhook-dakı `item` sahəsi item-in **çatdırılma anındakı** vəziyyətidir. Hadisə anındakı dəyişikliklər `data.changes`-dədir.
- Job növbəsi polling ilə işləyir (1s). Daha az gecikmə lazım olarsa `LISTEN/NOTIFY` əlavə ediləcək.
