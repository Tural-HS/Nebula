# ADR-0059: Çat kanalı üçün yalnız status dəyişiklikləri və xülasənin dəqiqəsi

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0010](0010-chat-notifications-and-trusted-proxies.md) (Mattermost/Slack formatı), [ADR-0009](0009-due-dates-and-time-automation.md) (səhər xülasəsi)

## Kontekst

FinTez Mattermost-un "Tapşırıqlar" kanalında köhnə botun mesajlarını görməyə öyrəşib: ödəniş günü gələndə,
ödəniş edildikdə ("✅ … → Hazırdır") və gecikmə olanda; mesajlar səhər 09:30-da gəlir. Nebula-nın
`item.updated` abunəsi isə hər sahə redaktəsini kanala göndərirdi (bir toplu düzəliş 66 mesaj yaratdı), səhər
xülasəsi isə yalnız tam saatda qurula bilirdi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Yeni abunə nümunəsi **`item.status_changed`**: statusu dəyişən `item.updated` hadisələri. Ayrıca hadisə yaranmır, yalnız uyğunlaşdırma qaydasıdır (`MatchEventOf`) | Kanal yalnız vacib anları alır; mövcud hadisə modeli və integrasiyalar dəyişmir | Yeni hadisə növü yazmaq: audit və webhook-larda təkrar yaranardı |
| Çat mesajı: status "done" kateqoriyasına keçəndə **✅**, digər hallarda 🔄 | Köhnə botdakı kimi ödəniş bir baxışda görünür | Status adına görə: iş axınından asılı olardı |
| Xülasəyə **dəqiqə** (`overdue_digest_minute`, 0–59); vaxt yerli saat `hour:minute`-a çatanda göndərilir. Ayarlarda yarım saat addımı | 09:30 kimi vaxtlar | Saatı dəqiqə ilə bir sütunda saxlamaq: mövcud API-ni pozardı |
| İllik tarix xatırlatmaları (ADR-0054) eyni vaxtı işlədir | Bir səhər vaxtı | — |

## Nəticələr

- Miqrasiya 00042: `workspaces.overdue_digest_minute smallint NOT NULL DEFAULT 0` (CHECK 0–59, millisaniyəlik kilid).
- API: `PATCH /workspaces/{ws}` `overdue_digest_minute` qəbul edir.
- FinTez: MAH webhook-u `item.status_changed` və `workspace.overdue_digest` alır; xülasə 09:30.
