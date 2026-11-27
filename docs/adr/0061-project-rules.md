# ADR-0061: Layihə qaydaları — tamamlanma sahələri və gecikmə rəngi

- Status: Qəbul edilib
- Tarix: 2026-11-27
- Əsas: [ADR-0057](0057-overdue-bands.md) (gecikmə pillələri), [ADR-0060](0060-reopen-admin-only-and-new-customer-start.md)

## Kontekst

FinTez-in aylıq hesabatında müştəri kartı yalnız "Ödədi", "Portal" və "Etibarnamə" üçü də işarəli olanda
tamamlanmış sayılır: üçü olmadan "Tamamlandı" etmək, üçü işarəli ikən "Görüləcək"ə qaytarmaq olmamalıdır.
Əvvəl bunu avtomatlaşdırma "geri qaytarırdı" — çaşdırıcı idi. Gecikən müştərinin cədvəldə adı, qalığı və
son tarixi gecikmə rəngi ilə seçilməlidir.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Layihə ayarı `project_rules`: **tamamlanma sahələri** (1–5 bəli/xeyr) və **gecikmə rəngi sahələri** (≤ 10) | Qaydalar layihəyə aiddir (MAH); iş axını başqa layihələrlə paylaşılır | İş axınına bağlamaq: digər layihələrə də təsir edərdi |
| Done statusa keçid üçün hamısı **true** olmalıdır (**409 `complete_requires`**); done/canceled → açıq keçid hamısı true ikən rədd (**409 `close_lock`**); **admin üçün də**; "Ləğv edildi"yə keçid azaddır | Məlumat ziddiyyəti, icazə məsələsi deyil | Yalnız üzvləri bloklamaq: admin də səhv edə bilər |
| Yoxlama dəyişiklikdən sonrakı dəyərlərə baxır (sahəni və statusu bir addımda dəyişmək olur) | Rahat düzəliş | — |
| Sistem dəyişiklikləri (aktor yoxdur: aylıq dövr, avtomatlaşdırma) azaddır | Ayın 1-də sıfırlanma və avtomatik "Tamamlandı" işləməlidir | — |
| Cədvəldə gecikən tapşırığın **adı** və seçilmiş sahələrin xanaları pillə rəngi ilə dolur; son tarix nişanı onsuz da rənglidir | Bir baxışda görünür | Bütün sətri rəngləmək: oxunmur |
| Xəta mesajları istifadəçinin dilində (`errorCodes`) | Aydın göstəriş | — |

## Nəticələr

- Miqrasiya 00045: `project_rules`.
- API: `GET/PUT /workspaces/{ws}/projects/{p}/rules` (`{complete_fields, late_fields}`; oxumaq üzv+, dəyişmək admin+).
- FinTez MAH: tamamlanma `paid`, `portal`, `poa`; gecikmə rəngi `balance`. Avtomatlaşdırma: üçü işarəli olanda
  Tamamlandı; "Ödədi" işarələnəndə Qalıq silinir (ayın 1-də dövr onu yenidən aylıq ödənişə bərabər edir).
