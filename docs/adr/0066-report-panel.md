# ADR-0066: Aylıq hesabat paneli

- Status: Qəbul edilib
- Tarix: 2026-11-27
- Əsas: [ADR-0064](0064-summary-bar.md) (xülasə zolağı), [ADR-0057](0057-overdue-bands.md) (gecikmə pillələri)

## Kontekst

FinTez cədvəlin üstündə zəngin aylıq hesabat istədi: yığım faizi və proqnoz, plan/ödənilib/qalıq kartları,
müştərilərin vəziyyəti və gecikmə pillələri, bu gün ödəməli olanlar, ən böyük gecikmə, ödəniş növü üzrə bölgü.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Layihə qaydalarında `report`: `plan_field`, `remaining_field`, istəyə görə `breakdown_field` (seçim sahəsi) | Panel rolları adla bilinir; ümumi xülasə zolağından aydındır | Xülasə sətirlərinin sırası ilə: kövrəkdir |
| Hesablama vebdə, mövcud API-lərlə: cəmlər (`/items/sums`), status sayları (`/items/counts`), gecikənlər və bu gün (`?due=overdue|today`), bölgü (seçim üzrə cəm) | Yeni server məntiqi yoxdur; cədvəlin filtrlərinə uyğundur | Ayrıca hesabat endpoint-i: təkrar məntiq |
| Ödənilib = plan − qalıq; faiz = ödənilib / plan; **proqnoz = plan − gecikənlərin qalığı** | Ödəniş qrafiki son tarixlə bilinir: vaxtı gəlməyənlər gözlənilir, gecikənlər risk sayılır | Xətti temp: ödənişlər ay ərzində bərabər deyil |
| Keçən ayla müqayisə hələ yoxdur | İlk ay bağlanışı (snapshot) 1 dekabrdadır | — |
| Panel varsa xülasə zolağının yerini tutur | Bir hesabat bloku | — |
| Yığcam dizayn: faiz halqası (açıq qövs proqnoz), üç məbləğ bir sətirdə, alt sətirdə çiplər və qısa faktlar; bir sətrə **yığılır**, seçim brauzerdə qalır | Cədvələ yer qalır; gündəlik baxış üçün kifayətdir | Böyük rəngli kartlar: çox yer tutur, solğun görünür |

## Nəticələr

- Miqrasiya 00048: `project_rules.report jsonb`.
- FinTez MAH: plan `monthly_payment`, qalıq `balance`, bölgü `payment_method`.
