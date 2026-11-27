# ADR-0067: Bu gün ödəməli olanların yaşıl parıltısı

- Status: Qəbul edilib
- Tarix: 2026-11-27
- Əsas: [ADR-0061](0061-project-rules.md) (gecikmə sahələri), [ADR-0057](0057-overdue-bands.md) (gecikmə pillələri)

## Kontekst

FinTez istədi ki, ödəniş günü bu gün olan müştərilərin cədvəldəki adı, qalığı və son tarixi parıldasın,
gözə dərhal dəysin.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Bu gün = son tarix bu gündür, gecikmir, status "done"/"canceled" deyil | Ödənilmiş kart parıldamamalıdır | Statusdan asılı olmadan: ödəyənlər də yanardı |
| Eyni xanalar: ad, layihənin gecikmə sahələri (`late_fields`), üstəlik son tarix sütunu | Yeni ayar lazım deyil; gecikmə rəngləri ilə eyni yer | Ayrıca sahə siyahısı: artıq konfiqurasiya |
| Yaşıl (emerald): mətn 800 / qaranlıqda 200, yalnız işıq kölgəsi (text-shadow) yumşaq döyünür, 1.2 s | Gecikmə rəngləri sarı/narıncı/qırmızıdır, yaşıl qarışmır; mətnin kontrastı (AA) dəyişmir | Neon yaşıl mətn: açıq fonda oxunmur; sərt yanıb-sönmə: yorucudur |
| `prefers-reduced-motion`-da animasiya yoxdur, parıltı sabit qalır | Hərəkətə həssas istifadəçilər | — |

## Nəticələr

- Yalnız veb; server və miqrasiya dəyişmir.
- Animasiya bütün gün davam edir (WCAG 2.2.2 baxımından güzəşt); sistemdə "hərəkəti azalt" açılsa dayanır.
