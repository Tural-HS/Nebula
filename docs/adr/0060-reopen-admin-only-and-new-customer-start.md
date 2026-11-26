# ADR-0060: Bağlanmışı yalnız admin açır; yeni müştərinin ilk ödənişi gələn ay

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0055](0055-cycle-due-dates.md) (aylıq dövrün son tarixləri)

## Kontekst

FinTez-də "Tamamlandı" (ödənildi) statusundan kartı geri qaytarmaq yalnız rəhbərliyin qərarı olmalıdır. Aylıq
hesabat cədvəli status dəyişəndə kartı gizlədirdi (görünüş filtri). Yeni müştəri: kredit yanvarın 5-də
verilibsə, ilk ödəniş fevralın 5-dir və müştəri yanvar cədvəlində olmamalıdır. Ödəniş günü bəzi müştərilərdə
kredit günündən fərqlidir ("Ödəniş günü" sahəsi).

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| İş axınında **`reopen_admin_only`**: done/canceled → todo/in_progress keçidini yalnız admin (və sahib) edir; üzvə 403 `reopen_admin_only`, keçidlər siyahısında açıq statuslar göstərilmir | Ödənilmiş kart təsadüfən geri qaytarılmır | Keçid qaydaları: rola baxmır; təsdiq (ADR-0026): hər geri qaytarmaya sorğu çox ağırdır |
| Sistem dəyişiklikləri (aktor yoxdur: aylıq dövr, avtomatlaşdırma) qaydadan azaddır | Ayın 1-də sıfırlanma işləməlidir | — |
| Son tarix filtri **`month`**: son tarixi bu ayın sonuna qədər olanlar (əvvəlki aylar daxil) | Ayın cədvəli: gələn aydan başlayan yeni müştəri görünmür, keçmiş aydan gecikən görünür | Status filtri: Tamamlandı/Gecikir gizlənirdi |
| Dövrdə **`due_start_field`** (məs. kredit tarixi): ilk son tarix bu tarixin ayından sonrakı aya düşür; ödəniş günü sahəsi boşdursa, bu tarixin günü işlədilir | Yeni müştəri əlavə olunanda əl ilə ödəniş günü yazmaq lazım deyil; istisnalar "Ödəniş günü" ilə | Ödəniş gününü həmişə əl ilə yazmaq: unudular |
| Server xəta kodları istifadəçinin dilində göstərilir (`errorCodes`) | İngiliscə mesaj işçiyə aydın deyil | — |

## Nəticələr

- Miqrasiyalar: 00043 `workflows.reopen_admin_only`, 00044 `period_cycles.due_start_field`.
- API: `PATCH /workflows/{id}` `reopen_admin_only`; `PUT /cycle` `due_start_field`; `?due=month`.
- FinTez: "Əsas" iş axınında açıq; MAH dövrü `due_day_field = payment_day`, `due_start_field = loan_date`;
  "Əsas cədvəl" görünüşü status filtrsiz, `due=month`.
