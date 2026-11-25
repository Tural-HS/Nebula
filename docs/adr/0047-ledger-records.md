# ADR-0047: Reyestr qeydləri: rəqəm vahidi, əsas məlumat kartı, telefon linkləri

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0003](0003-custom-fields.md) (xüsusi sahələr), [ADR-0014](0014-saved-views-and-table-totals.md) (saxlanmış görünüşlər, cəm sətri),
  [ADR-0019](0019-mobile-web-ui.md) (telefon), [ADR-0035](0035-design-and-theme.md) (Siqnal sütunu, panel), [ADR-0044](0044-money-fields.md) (pul sahələri)

## Kontekst

İlk müştərinin Mattermost Boards lövhələri (yığım, müştəri aylıq hesabatı) Nebula-ya idxal olundu. Bunlar tapşırıqdan çox
**reyestrdir**: hər kart bir müştəridir, vacib olan sahələrdir (telefon, NV nömrəsi, aylıq ödəniş, qalıq). İlk istifadədə
görünən problemlər:

- tapşırıq panelində sahələr sağ sütunda, input-lar arasında itir; sol sütunu boş bölmələr tutur ("Təsvir yoxdur",
  "Alt tapşırıq yoxdur", "Əlaqə yoxdur", "Fayl yoxdur");
- məbləğlərin valyutası görünmür ("2 665"); idxal olunmuş sahələr `number` tipindədir (Boards-da belə idi), pul
  sahəsinə (ADR-0044) çevirmək tipin dəyişməzliyi səbəbindən yeni sahə və dəyər köçürməsi tələb edir;
- yığım komandası zəngə telefon nömrəsini kopyalayıb yığır.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **`options.unit`**: yalnız `number` sahələri üçün göstərmə şəkilçisi (≤ 8 simvol, rəqəmsiz, idarəetmə simvolsuz; boş = yoxdur). Admin yaradanda və ya sonra (`PATCH …/fields/{key}` `{"options":{"unit":"₼"}}`) qoyur, dəyişir, silir; audit `options.unit`. Dəyər dəyişmir, yalnız göstərmə: hüceyrə, cəm sətri, panel ("2 665 ₼", bölünməz boşluqla) | İdxal olunmuş `number` sahələri miqrasiyasız düzgün oxunur; `%`, `ədəd`, `km` üçün də işləyir | Pul sahəsinə çevirmək: hər sahə üçün yeni sahə, dəyər köçürməsi, görünüş/filtr/formula istinadlarının yenilənməsi. Valyuta konversiyası və ya çoxvalyutalı məntiq `number` üçün yoxdur — bunun üçün pul sahəsi var |
| **Əsas məlumat kartı**: paneldə başlıq və iş axınından dərhal sonra, hər iki sütunun üstündə, doldurulmuş sahələr (uzun mətn istisna) böyük şriftlə, yalnız oxumaq üçün; redaktə sağdakı sahə siyahısında qalır | Telefonda ilk ekranda görünür, kompüterdə tam en; qeydi açanın axtardığı budur | Sahə redaktorlarını yuxarı köçürmək: input-lar oxunuşu ağırlaşdırır, panelin ADR-0035 düzülüşü pozulur |
| **Boş bölmələr bir sətrə yığılır**: təsvir, alt tapşırıq, əlaqə və fayl boşdursa, "Əlavə et: + Təsvir · + Alt tapşırıq · + Əlaqə · + Fayl" sətri; klik bölməni açır. Redaktə hüququ olmayana boş bölmə göstərilmir. Məzmunu olan bölmə həmişə görünür | Qeyd öz məlumatı ilə açılır; heç bir funksiya itmir | Bölmələri tamamilə gizlətmək: fayl/əlaqə əlavə etmək yolu itir. Akkordeon: hər bölmə üçün əlavə başlıq yenə yer tutur |
| **Telefon linki** (`tel:`): yalnız mətn sahəsi adı/açarı telefon olanda (`telefon`, `phone`, `mobil`, `əlaqə nömrəsi`, `номер`, …) **və** dəyər 7–15 rəqəmdirsə. Paneldə həmişə; cədvəldə yalnız xana yerində redaktə olunmayanda (toxunma ekranı, oxuma hüququ) — redaktə düyməsinin içində link olmaz | Hesab/sənəd nömrəsi təsadüfən zəng linkinə çevrilmir; telefonda bir toxunuşla zəng | Dəyərə görə tanımaq: rəqəmli hesab nömrələri də link olardı. Ayrıca `phone` sahə tipi: miqrasiya və idxal xəritəsi dəyişikliyi — tələb yaranarsa sonra |
| **Cədvəl**: geniş cədvəldə (kompüter) açar və başlıq sağa sürüşdürəndə sabit qalır; başlıq sütunu 18 rem; heç bir yüklənmiş sətirdə siqnal yoxdursa Siqnal sütunu göstərilmir; bəli/xeyr sütunları 6 rem, mətn 10 rem; uzun başlıq sütunun içində qısalır, tam mətn `title`-da | 15+ sahəli reyestrdə əsas sütunlar ekrana sığır, kimin sətri olduğu itmir | Sütun eninin istifadəçi tərəfindən dəyişdirilməsi: faydalıdır, saxlanması (görünüşdə) əlavə model tələb edir — növbəti addım |

## Nəticələr

- Miqrasiya yoxdur: `field_definitions.options` artıq `jsonb`-dir; köhnə image `unit`-i oxumur və sadəcə göstərmir.
- İdxal skripti (müştəri tərəfi, repoda deyil) Boards-dakı sütun sırası ilə "Əsas cədvəl" (açıq tapşırıqlar, son tarixə
  görə) və "Tamamlananlar" görünüşlərini yaradır; məbləğ sahələrinə `₼` vahidi qoyur.
- Testlər: inteqrasiya (vahidin validasiyası, rollar), vitest (telefon tanıma, siqnal), E2E (vahid hüceyrədə və cəmdə,
  kart, `tel:` linki, yığılmış bölmələr, sabit başlıq, ayarlarda vahid, qonaq).

## Açıq suallar

- Sütun eninin dəyişdirilməsi və görünüşdə saxlanması.
- Əsas məlumat kartında hansı sahələrin göstəriləcəyini admin seçsin (indi: doldurulmuş, uzun mətn istisna)?
- Toplu "zəng et" növbəsi (yığım üçün gündəlik siyahı) — reyestr işinin növbəti addımı.
