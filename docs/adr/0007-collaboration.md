# ADR-0007: Əməkdaşlıq: şərhlər, qeydetmələr (@mention), aktivlik lenti, audit log

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Şərhlər

| Qərar | Səbəb |
|---|---|
| Şərhlər tapşırığa bağlıdır, `workspace_id` + composite FK ilə (`items(workspace_id, id)`) | Digər cədvəllər kimi tenant izolyasiyası bazada təmin edilir |
| Yazmaq: member+; oxumaq: guest+ | Rol modelinə uyğun (ADR-0002) |
| Redaktə: **yalnız müəllif** (admin də başqasının sözünü dəyişə bilməz) | Söz sahibinin mətni saxtalaşdırıla bilməz |
| Silmə: müəllif və ya admin+ (moderasiya) | |
| Silinmiş şərh "tombstone" kimi qalır: `deleted: true`, `body: null` | Mövzunun axını pozulmur; mətn API-dən bir daha qaytarılmır |
| Silinmiş şərhin mətni bazada audit üçün qalır | Mübahisəli hallarda bərpa; tam silmə (GDPR) ayrıca admin əməliyyatı kimi gələcəkdə |
| Mətn düz mətndir, maks. 20 000 simvol; UI-da HTML kimi render edilmir | XSS mümkün deyil |
| Sıralama: köhnədən yeniyə, keyset pagination `(created_at, id)` | Sabit və sürətli |

## Qeydetmə (@mention)

- Format: `<@istifadəçi-uuid>`. Adlar unikal və sabit olmadığı üçün ID istifadə olunur.
- UI istifadəçiyə `@Ad Soyad` göstərir, göndərəndə token-ə çevirir (`web/src/lib/mentions.ts`, testlərlə).
- Yalnız workspace-in aktiv üzvləri qeyd edilə bilər. Digərləri **səssizcə nəzərə alınmır**,
  ona görə qeydetmə ilə kimin mövcud olduğunu yoxlamaq mümkün deyil.
- Şərh başına ən çox 20 nəfər qeyd edilə bilər (spam qoruması).
- Qeyd olunan hər üzvə email göndərilir. Müəllifin özünə göndərilmir. Email şərhlə **eyni tranzaksiyada** növbəyə yazılır (outbox).
- Redaktədə yalnız **yeni** qeyd olunanlara email gedir. Dedupe açarı `mn:<şərh>:<istifadəçi>` təkrar göndərməni bloklayır.

## Aktivlik lenti və audit log

- Tapşırığın tarixçəsi `events` cədvəlindən gəlir. Ayrıca cədvəl yoxdur, ona görə tarixçə heç vaxt itmir və ya uyğunsuz olmur.
- `GET .../items/{n}/activity`: dəyişikliklər, şərhlər; yenidən köhnəyə, `before` (hadisə ID-si) ilə səhifələnir. guest+.
- `GET .../audit-log`: workspace-in bütün hadisələri; `action` prefiksi (`webhook.`) və `actor` filtrləri. **Yalnız admin+**.
- Daxili `snapshot` (qaydaların qiymətləndirilməsi üçün) lentdə qaytarılmır.
- Şərh hadisələri `entity_type = item` ilə yazılır (`comment.created/updated/deleted`). Beləliklə tapşırığın lentinə düşür və
  webhook-larda `comment.*` filtri ilə izlənə bilir.

## Webhook "test ping"

`POST .../webhooks/{id}/ping` (admin+) yalnız həmin webhook-a `webhook.ping` hadisəsi göndərir. Hadisə filtri və aktiv olub-olmaması
nəzərə alınmır, digər webhook-lara yayılmır. Nəticə `.../deliveries`-də görünür.

## Şifrələmə açarının rotasiyası (fasiləsiz)

1. Yeni açar yaradın: `openssl rand -base64 32`.
2. Deploy: `NEBULA_ENCRYPTION_KEY=<yeni>`, `NEBULA_ENCRYPTION_KEYS_OLD=<köhnə>` (vergüllə bir neçə köhnə açar ola bilər).
   Bu vaxt hər iki açarla şifrələnmiş dəyərlər oxunur, yeni dəyərlər yalnız yeni açarla yazılır.
3. `nebula rotate-secrets` işə salın. Köhnə açarla şifrələnmiş secret-lər yeni açarla yenidən şifrələnir.
   Əmr idempotentdir: təkrar işə salmaq təhlükəsizdir.
4. Bütün replikalar yeni konfiqurasiya ilə işləyəndən sonra `NEBULA_ENCRYPTION_KEYS_OLD`-u silin.

Konfiqurasiya yoxlaması köhnə açarın formatını və cari açarla eyni olmadığını yoxlayır.

## LISTEN/NOTIFY ilə ani emal

- `jobs` cədvəlinə yazılan hər tranzaksiya `pg_notify('nebula_jobs')` göndərir (statement-level trigger, COMMIT-də çatdırılır).
- Hər worker prosesi bir ayrıca bağlantı ilə `LISTEN` edir və bildiriş gələn kimi növbəni oxuyur.
- Ölçülmüş gecikmə: email işi yaradılandan **~23 ms** sonra tamamlanır. Əvvəl polling ilə bu ~1 s idi.
- Bağlantı kəsilərsə 1 s-dən 30 s-yə qədər artan intervalla yenidən qoşulur. Polling ehtiyat kimi qalır, ona görə heç bir iş itmir.
  LISTEN vəziyyətli bağlantı pool-a qaytarılmır (`Hijack`).

## Nəticələr

- Müsbət: tarixçə və audit tək mənbədən gəlir; qeydetmələr və emaillər atomikdir; webhook və email gecikməsi praktik olaraq sıfırdır.
- Mənfi: hər worker replikası bir əlavə daimi bağlantı istifadə edir (pool ölçüsündən kənar).
