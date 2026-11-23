# ADR-0017: Tətbiqdaxili bildiriş mərkəzi

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

İstifadəçilər indiyədək yalnız email alırdı (@mention, avtomatlaşdırma qaydaları). Email söndürülmüş
quraşdırmalarda heç nə gəlmir, gələndə də "kimin məndən nə gözlədiyini" tətbiqin içində görmək olmur.
Lazım olan: hər istifadəçi üçün, bütün workspace-lər üzrə bir bildiriş siyahısı, oxunmamış sayı ilə başlıqda
zəng düyməsi. Gizli layihələr (ADR-0013) burada da sızmamalıdır.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| Yeni `notifications` cədvəli (miqrasiya 00014): alıcı, workspace, mənbə hadisə (`event_id`), növ, `project_id` + `item_id`, `actor_id`, `read_at`, `created_at`; id `bigint identity` | Bildiriş hadisə jurnalından **törəyən** məlumatdır; id ardıcıl olduğu üçün keyset pagination tək sütunla (`id < kursor`) qurulur |
| Kompozit FK-lar: `(workspace_id, recipient_id)` → `workspace_members` (CASCADE), `(workspace_id, project_id)` → `projects`, `(workspace_id, item_id)` → `items` | Digər cədvəllərlə eyni tenant modeli: başqa workspace-in tapşırığına bildiriş bağlana bilmir; workspace-dən çıxan adamın bildirişləri silinir |
| Bildirişlər mövcud outbox ilə yaranır: `FanOut` bildiriş verə biləcək hər hadisə üçün **bir** `notifications.create` işi qoyur (dedupe açarı `nt:<event_id>`) | Əsas əməliyyatın transaksiyasına əlavə yük yoxdur; webhook və avtomatlaşdırma ilə eyni etibarlılıq (at-least-once) |
| İdempotentlik: `UNIQUE (event_id, recipient_id)` + `ON CONFLICT DO NOTHING` | Təkrarlanan iş (worker işi görüb `done` yazmadan ölsə) dublikat yaratmır; bir hadisədən bir nəfərə ən çox bir bildiriş |
| Növlər: `mention` (şərhdə yeni qeyd olunanlar; redaktədə yalnız əlavə olunanlar — `comment.updated` hadisəsi indi `mentions_added` daşıyır), `assigned` (yaradılarkən icraçı və ya icraçı dəyişikliyi), `overdue` (`item.overdue`-da icraçı), `comment` (icraçısı və ya müəllifi olduğunuz tapşırığa yeni şərh) | Email ilə eyni qeyd siyahısı; qeyd olunan adam eyni şərhə görə ikinci (`comment`) bildiriş almır — `mention` üstündür |
| Heç vaxt müəllifin öz hərəkətinə bildiriş yoxdur; özünə təyin etmək də | Səs-küy |
| Yaradılarkən alıcı aktiv workspace üzvü olmalı və layihəni görə bilməlidir (`canSeeProject`) | Gizli layihənin məzmunu kənara getmir; mention-lar onsuz da `resolveMentions`-da süzülür, amma `comment` / `overdue` alıcıları (müəllif, icraçı) sonradan girişini itirmiş ola bilər |
| Oxuyarkən giriş **yenidən** yoxlanılır (`visibleNotification`, `canSeeProject` ilə eyni predikat); silinmiş tapşırıqların bildirişləri göstərilmir | Layihə sonradan gizli olsa və ya adam layihədən çıxarılsa, köhnə bildirişlər gizlənir (silinmir: giriş bərpa olunanda yenə görünür) |
| İdxal olunan tapşırıqlar bildiriş vermir | İdxal sətir-sətir dispatch etmir (ADR-0011); 5000 sətirlik fayl 5000 bildiriş yaratmamalıdır |
| API: `GET /api/v1/me/notifications` (`?unread=true`, `?limit` ≤ 100, `?cursor`), `GET …/unread-count`, `POST …/{id}/read`, `POST …/read-all` (`?workspace=<slug>` ilə yalnız bir workspace) | İstifadəçiyə bağlıdır, workspace-ə yox: bir zəng düyməsi bütün workspace-ləri göstərir. Hər sətir UI-ın əlavə sorğusuz göstərməsi üçün kifayətdir: workspace slug/adı, layihə açarı, tapşırıq nömrəsi, açarı, **cari** başlığı, müəllifin adı, növ, vaxt, oxunub |
| Başqasının bildirişini oxunmuş etmək 404-dür | Mövcudluğu da açılmır |
| Oxunmamış sayı **100-də kəsilir** (`capped: true`); sorğu qismən indeksi (`recipient_id, id DESC WHERE read_at IS NULL`) gəzir və ən çox 100 sətir üçün PK axtarışı edir | Sayğac hər açıq tabdan 30 saniyədən bir sorğulanır; "99+" göstərmək üçün dəqiq say lazım deyil, xərc isə istifadəçinin bildiriş sayından asılı qalmır. Giriş yoxlaması lazım olduğundan sorğu tam index-only deyil |
| Saxlama müddəti: oxunmuşlar 90 gün, hamısı 180 gün; worker-in saatlıq housekeeping-ində (`PurgeFinished`), hər qayda üçün ən çox 10 000 sətir | Cədvəl sonsuz böyümür; partiyalı silmə uzun kilid yaratmır, geridə qalan hissə növbəti saatda silinir |
| Veb: başlıqda zəng düyməsi, oxunmamış nişanı ("99+"), sayğac hər 30 s və pəncərəyə fokusda yenilənir (gizli tabda sorğu dayanır); açılan paneldə son 20 bildiriş, nisbi vaxt, oxunmamışlar seçilmiş; klik tapşırıq panelini açır (`/w/{ws}/p/{layihə}?item={n}`) və bildirişi oxunmuş edir; "Hamısını oxunmuş et" | Sadə polling hələlik kifayətdir |
| Əlçatanlıq: düymənin adı sayla ("Bildirişlər, 3 oxunmamış"), say dəyişikliyi `aria-live="polite"` ilə elan olunur (ilk yükləmə yox), panel `role="dialog"`, açılanda fokus siyahıya keçir, ↑/↓/Home/End ilə hərəkət, Escape bağlayır və fokusu düyməyə qaytarır, kənara klik bağlayır | Klaviatura və ekran oxuyucusu ilə tam istifadə |

## Nəticələr

- **WebSocket / SSE yoxdur.** Yeni bildiriş ən gec ~30 saniyəyə (və ya tab fokus alanda) görünür. Real vaxt lazım
  olarsa, `LISTEN/NOTIFY` üzərində SSE kanalı əlavə oluna bilər; o zaman polling ehtiyat yol kimi qalır.
- Sayğac sorğusu hər istifadəçi və tab üçün dəqiqədə 2 sorğudur. 1000 aktiv istifadəçidə ~33 sorğu/s, hər biri
  ən çox 100 sətirlik qismən indeks skanıdır; istifadəçi başına rate limit-ə (ADR-0004) təsiri cüzidir.
- Bildiriş yaradılarkən giriş yoxlaması alıcı başına bir sorğudur (bir hadisədə ən çox ~22 alıcı: 20 mention +
  icraçı + müəllif).
- Sayğac giriş yoxlamasından keçdiyi üçün girişi itirilmiş layihənin 100-dən çox oxunmamış bildirişi olan
  istifadəçidə sayğac daha çox sətir gəzə bilər; 180 günlük saxlama bunu məhdudlaşdırır.
- `overdue` bildirişi hadisə anındakı icraçıya gedir; `comment` bildirişi işin icra anındakı icraçı və müəllifə.
- Tapşırıq başlığı və açarı oxuma anında götürülür (bildirişdə kopyası saxlanmır); silinmiş tapşırıqların
  bildirişləri siyahıdan düşür, sətirlər isə saxlama müddəti bitənə qədər qalır.
- Bildiriş tənzimləmələri (növləri söndürmək, email ilə birləşdirmək, gündəlik xülasə) gələcək işdir.
- Miqrasiya nömrəsi 00014-dür; 00013 (workspace dəvətləri) ondan əvvəl tətbiq olunmalıdır, yoxsa goose
  "missing migration" xətası verər.
