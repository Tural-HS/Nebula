# ADR-0062: "Sahəni doldur" əməliyyatında dəyəri başqa sahədən götürmək

- Status: Qəbul edilib
- Tarix: 2026-11-27
- Əsas: [ADR-0005](0005-automation-webhooks-email.md) (avtomatlaşdırma)

## Kontekst

FinTez: "Ödədi" işarələnəndə ayın qalığı silinir; işarə götürüləndə qalıq yenidən aylıq ödənişə bərabər
olmalıdır. Aylıq ödəniş hər müştəridə fərqlidir, ona görə sabit dəyər yazan `set_field` kifayət etmir.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| `set_field` əməliyyatına `from` (sahə açarı): dəyər tapşırığın həmin sahəsinin **cari** dəyəridir; boşdursa hədəf təmizlənir | Bir sahə ilə həll olunur, mövcud əməliyyat genişlənir | Düstur sahəsi: qalıq əl ilə də dəyişdirilir, düstur buna imkan vermir |
| `value` və `from` birlikdə verilə bilməz; `from` mövcud, arxivlənməmiş sahə olmalı, hədəfin özü olmamalıdır | Aydın qayda | — |
| Veb-də "Dəyər mənbəyi": sabit dəyər və ya "… sahəsindən" | Qayda veb-də açılıb saxlananda itmir | — |

## Nəticələr

- Miqrasiya yoxdur (əməliyyatlar jsonb-dir).
- FinTez MAH: "Ödədi" götürüləndə Qalıq = Aylıq ödəniş (qayda "MAH: ödəniş geri alınanda Qalıq bərpa").
