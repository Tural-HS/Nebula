# Runbook: Mattermost Boards-dan Nebula-ya keçid

Bu sənəd bir komandanın (məs. FinTez) board-larını Mattermost Boards-dan Nebula-ya köçürmək üçündür.
Qərarlar və limitlər: ADR-0020. Production quraşdırma: `deploy/production/README.md`.

> **Məxfilik:** `.boardarchive` faylları şirkət məlumatıdır. Onları çatlara, email-ə və ya ümumi qovluqlara
> qoymayın; importdan sonra silin. Nebula faylı yalnız yaddaşda oxuyur, diskdə saxlamır.

## 0. Keçiddən əvvəl (bir dəfə)

1. Nebula production-da işləyir, backup və restore testi yaşıldır (`deploy/production/README.md` §4–5).
2. **Workspace**: "Yeni workspace" → **Başlanğıc şablonu: Azərbaycanca** (default). Statuslar:
   Görüləcək → İcrada → Yoxlamada → Tamamlandı, Ləğv edildi; saat qurşağı Asia/Baku.
3. Board-lardakı status sütunlarının adlarını yazın. Nebula-dakı adlardan fərqlidirsə
   (məs. "Gözləmədə"), əvvəlcə **Ayarlar → Workflow-lar**-da eyni adlı status əlavə edin. İdxal statusu **adla** tapır.
4. Board-larda istifadə olunan select xassələri (məs. "Filial", "Prioritet") üçün **Ayarlar → Sahələr**-də
   eyni seçimli `select` sahəsi yaradın və iş növünə bağlayın. Seçim adları board-dakı ilə eyni olmalıdır.
5. **İstifadəçilər**: hər əməkdaşa dəvət göndərin (Ayarlar → Üzvlər → Dəvət et, ADR-0016). İcraçı kimi təyin
   olunacaq hər kəs importdan əvvəl workspace üzvü olmalıdır.

## 1. Hər board üçün

| Addım | Harada | Yoxlama |
|---|---|---|
| 1. Layihə yaradın: "Yeni layihə" → açar (məs. `MAL`), **Gizli layihə** (şöbə board-u gizlidirsə). İkinci şöbədən başlayaraq **Şablon layihə** seçin: üzvlər, təkrarlanan tapşırıqlar, ümumi görünüşlər kopyalanır | Nebula | Layihə siyahıda 🔒 ilə görünür |
| 2. Layihə üzvlərini əlavə edin (Layihə ayarları). Gizli layihədə icraçı **layihə üzvü olmalıdır**, yoxsa idxal həmin sətri rədd edir | Nebula | Üzvlər siyahısı board üzvləri ilə eynidir |
| 3. Board-u ixrac edin: board menyusu (⋯) → **Export board archive** → `archive-….boardarchive` | Mattermost | Fayl ≤ 50 MB (böyükdürsə, board-dakı şəkil/faylları azaldın) |
| 4. Layihədə **İdxal et** → faylı seçin → "Faylı oxu" | Nebula | Sətir sayı = board-dakı kart sayı (şablon kartlar sayılmır) |
| 5. Sütunlar: Başlıq → başlıq, Status → status, icraçı xassəsi → İcraçı, tarix → Son tarix, Təsvir → təsvir, qalanları uyğun sahələrə və ya "nəzərə alma" | Nebula | Təkliflər adlara görə avtomatik doldurulur |
| 6. **Mattermost istifadəçiləri**: hər kodun yanında göstərilən kartlara baxıb Nebula üzvünü seçin | Nebula | Hər tanınan şəxs seçilib; tanınmayan boş qalır |
| 7. **Yoxla** (quru yoxlama). Xətalar sətir və sütunla göstərilir; düzəldin (status adı, seçim adı, üzvlük) və təkrar yoxlayın | Nebula | "Bütün N sətir düzgündür" |
| 8. **İdxal et** | Nebula | "N tapşırıq yaradıldı" və açar aralığı |
| 9. Nümunə yoxlaması: 3–5 kartı board ilə müqayisə edin (başlıq, status, icraçı, son tarix, təsvirin sonunda köhnə şərhlər) | Hər ikisi | Uyğundur |
| 10. Faylı silin | Kompüter | — |

İdxal hamısı-və-ya-heç nədir: bir sətir xətalıdırsa heç nə yazılmır. Təkrar idxal **dublikat yaradır** —
uğurlu idxaldan sonra eyni faylı yenidən yükləməyin.

## 2. Board-dan sonra qurulmalı olanlar

- **Təkrarlanan tapşırıqlar** (aylıq hesabatlar və s.): layihə → "Təkrarlanan" → yaradın (ADR-0012).
  Şablon layihədən yaradılıbsa artıq kopyalanıb; icraçıları yoxlayın.
- **Gecikmə xülasəsi**: Ayarlar → Ümumi → xülasə saatı; Mattermost kanalına göndərmək üçün
  Ayarlar → Webhook-lar → format **Mattermost/Slack**, layihə filtri ilə (ADR-0009, ADR-0010, ADR-0013).
- **Görünüşlər**: "Gecikənlər", "Mənim işlərim" kimi ümumi görünüşlər (ADR-0014).

## 3. Keçid (cut-over)

1. **Paralel dövr (1–2 həftə)**: yeni işlər yalnız Nebula-da açılır; Boards yalnız oxumaq üçündür
   (board icazələrini "viewer" edin). İkiqat giriş etməyin — məlumat ayrılır.
2. Komandadan geribildirim: çatışmayan status/sahə/görünüş varsa əlavə edin.
3. Son yoxlama: hər layihədə açıq tapşırıq sayı Boards-dakı açıq kart sayı ilə uyğundur.
4. Boards board-larını arxivləyin (silməyin): Mattermost retention siyasətinə uyğun saxlanılsın.

## 4. Geri qayıtma

İdxal yeni layihədə tapşırıq yaradır, mövcud məlumatı dəyişmir.

- Yanlış idxal (səhv board, səhv uyğunlaşdırma): layihəni arxivləyin, yenisini yaradın və təkrar idxal edin.
  İdxal ilə yaranan tapşırıqların `item.created` hadisələri `source: import` ilə qeyd olunur (tapşırığın fəaliyyət tarixçəsi).
- Keçid tamamilə dayandırılsa: Boards board-larının icazələrini geri qaytarın; Nebula-da Boards-dan sonra
  yaradılmış işlər əllə köçürülməlidir (paralel dövrü qısa saxlamağın səbəbi budur).

## 5. Tez-tez rast gəlinən xətalar

| Xəta | Səbəb | Həll |
|---|---|---|
| `status "…" does not exist in the workflow of "…"` | Board-dakı status adı Nebula workflow-unda yoxdur | Statusu əlavə edin və ya adını dəyişin (§0.3) |
| `user is not a member of this workspace or cannot see this project` | İcraçı workspace-də yoxdur və ya gizli layihənin üzvü deyil | Dəvət edin / layihəyə əlavə edin (§1.2) |
| `no workspace member has the email …` (`people`) | Seçilmiş şəxs workspace üzvü deyil | Əvvəl dəvət edin |
| `archive version N is not supported` | Köhnə Focalboard ixracı | Board-u Mattermost-un indiki versiyasından yenidən ixrac edin |
| `… is a board template, not a board` | Şablon ixrac edilib | Board-un özünü ixrac edin |
| `file_too_large` | Arxiv > 50 MB və ya board məlumatı > 64 MB | Böyük şəkil/faylları board-dan çıxarın və ya board-u bölün |
