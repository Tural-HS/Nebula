# ADR-0050: Bir kliklə bəli/xeyr, geri qaytarma və tətbiqin öz təsdiq dialoqu

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0023](0023-calendar-bulk-inline.md) (yerində redaktə), [ADR-0047](0047-ledger-records.md) (reyestr cədvəli)

## Kontekst

UI auditi (2026-11-25): yığım komandası gün boyu "Ödədi", "Portal", "WhatsApp göndərildi" işarələyir, amma bəli/xeyr
sahələri cədvəldə redaktə olunmurdu — hər dəfə panel açılmalı idi. Heç bir redaktənin "geri qaytar"ı yox idi.
Silmə və digər riskli əməllər 13 yerdə brauzerin `window.confirm`-i ilə soruşulurdu: temaya uyğun deyil, obyekti
aydın adlandırmır, düymələri "OK/Cancel"-dir.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Cədvəldə bəli/xeyr xanası **bir kliklə** dəyişir: `role="checkbox"`, `aria-checked`; bəli = yaşıl dairədə ✓, xeyr = boş dairə, qeyd yoxdur = "–". Yazı optimistikdir və versiyalıdır (digər yerində redaktələr kimi, 409 konflikt) | Ən çox edilən əməl bir klikdir; "xeyr" ilə "qeyd yoxdur" fərqlənir | Redaktə rejimi (seçim siyahısı): iki klik, iki nəticə üçün artıqdır |
| Dəyişiklikdən sonra bildirişdə **"Geri qaytar"** (8 s): əvvəlki dəyəri növbəti versiya ilə yazır. Bildiriş komponenti bir əməl düyməsi daşıya bilir | Təsadüfi klik bahalı deyil; 8 s düyməyə çatmağa kifayətdir, səhv bildirişləri isə bağlanana qədər qalır (WCAG 2.2.1) | Tarixçədən geri qaytarma: ümumi həll, daha böyük iş — növbəti addım (toplu redaktə üçün) |
| **Tətbiqin öz təsdiq dialoqu** (`ConfirmProvider`, `useConfirm()` → `Promise<boolean>`): başlıq, obyekti adlandıran mətn, əməlin adı ilə düymə (məs. "Sil"), dağıdıcı əməldə qırmızı; native `<dialog>` (fokus içəridə, Esc ləğv). Bütün 13 `window.confirm` əvəzləndi | Tema, dil və əlçatanlıq bütün təsdiqlərdə eyni | Hər yerdə ayrıca `Dialog` state-i: 13 kopya |
| **Zəng nəticəsi** (telefon sahəsi olan tapşırıqlarda, redaktə hüququ ilə): "Cavab verdi", "Cavab vermədi", "Ödəməyə söz verdi" (tarix ilə — son tarix də həmin günə keçir), "Yanlış nömrə"; hər biri `📞 …` şərhi yazır. Paneldə **← n / cəmi →** və J/K ilə siyahının növbəti/əvvəlki tapşırığı (yazı yazarkən və dialoq açıqkən işləmir) | Yığım işi zəng siyahısıdır: nəticə bir klik, tarixçə şərhlərdə və Aktivlikdə axtarılır; "söz verdi" günü gecikmə xülasəsinə düşür | Ayrıca "zəng" modeli və cədvəli: hesabatlar üçün daha yaxşı, amma miqrasiya və yeni API — tələb təsdiqlənsə növbəti addım |

## Nəticələr

- Server dəyişikliyi yoxdur. E2E testləri native dialoq əvəzinə tətbiqin dialoqunu klikləyir.
- Cədvəldə valyuta vahidli (₼, AZN, USD, …) rəqəmlər həmişə 2 onluqla göstərilir ("622,20 ₼"); ayarlar menyusunda
  aktiv səhifə brend rəngi ilə seçilir.

## Əlavə (2026-11-26): toplu redaktədə geri qaytarma

Toplu dəyişiklikdən (status, icraçı, son tarix, bir sahə) sonra bildirişdə **"Geri qaytar"**: tapşırıqlar əvvəlki
dəyərlərinə görə qruplaşdırılır və hər qrup üçün bir toplu sorğu göndərilir (redaktənin qaytardığı versiyalarla —
arada başqası dəyişibsə, konflikt olur, heç nə üstünə yazılmır). Toplu silmə geri qaytarılmır (təsdiq dialoqu qalır).

## Açıq suallar

- Klaviatura ilə cədvəldə hərəkət (↑↓, Space ilə bəli/xeyr).
