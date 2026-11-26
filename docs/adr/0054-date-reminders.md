# ADR-0054: İllik tarix xatırlatması (ad günləri)

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0017](0017-in-app-notifications.md) (tətbiqdaxili bildirişlər), [ADR-0009](0009-due-dates-and-time-automation.md) (planlayıcı)

## Kontekst

FinTez müştərilərin ad günlərini Excel-də saxlayır və yadda saxlamaq çətindir. Ad günü bir tarix sahəsidir
(`birthday`); lazım olan, hər il həmin gün komandaya tətbiqdə xəbər verməkdir.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Layihə** bir tarix sahəsi seçir (Layihə ayarları → "Ad günü xatırlatması"); cədvəl `date_reminders` | Eyni müştəri bir neçə layihədə ola bilər (MAH, Reyestr): sahə səviyyəsində açsaq, hər gün təkrar bildiriş gələrdi. Admin hansı siyahının "əsas" olduğunu seçir | Sahə üzrə (`options.yearly`): bütün layihələrdə təkrarlanır |
| Gün və ay **bu günə** düşəndə (workspace saat qurşağı); 29 fevral adi ildə **28 fevralda** | Heç bir ad günü buraxılmır | 1 mart: adətə uyğun deyil |
| Saat: səhər xülasəsinin saatı, söndürülübsə **09:00** | İş gününün əvvəli, xülasə ilə eyni vaxtda | Gecə yarısı: bildiriş gecə gəlir |
| **Gündə bir dəfə**: `last_sent` (yerli tarix) sətir kilidi ilə (`FOR UPDATE SKIP LOCKED`) eyni tranzaksiyada | Bir neçə replika və hər dəqiqəlik tick təkrar göndərməsin; sahəni yenidən saxlamaq da təkrarlamır | Hər element üçün işarə cədvəli: bu miqyas üçün artıqdır |
| Alıcılar: layihəni **görən hər üzv** (qonaqlar yox), `visibleProjectFor` ilə; ≤ 500 nəfər | Gizli layihənin (MAH) müştərisi yalnız onun üzvlərinə görünür | İcraçı/izləyici: MAH kartlarında icraçı yoxdur |
| Hər element ayrıca bildiriş (`anniversary`), gündə layihə başına ≤ 200 | Bildiriş kartı birbaşa müştərini açır | Bir xülasə bildirişi: bildiriş modeli elementə bağlıdır |
| Bağlı (Tamamlandı) kartlar da daxildir, silinmişlər yox | MAH-da "Tamamlandı" ayın ödənişidir, müştəri qalır | — |
| Hadisə `item.anniversary` (audit/webhook üçün də), mətn sahə adı ilə | Digər planlayıcı hadisələri kimi | — |

## Nəticələr

- Miqrasiya 00040: `date_reminders`; bildiriş növləri siyahısına `anniversary` (CHECK). Down seçimi və bu növ
  bildirişləri silir.
- API: `GET/PUT /workspaces/{ws}/projects/{p}/date-reminder` (`{field: "birthday" | null}`; oxumaq üzv+,
  dəyişmək admin+).
- FinTez: MAH layihəsində `birthday` seçiləcək (Reyestrdə yox — eyni müştərilər).
