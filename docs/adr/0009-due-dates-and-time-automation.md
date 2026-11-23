# ADR-0009: Son tarix, gecikmə və vaxta görə avtomatlaşdırma

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

Komandalar (məs. FinTez-in Mattermost Boards qurulumu) bunları əl ilə yazılmış cron skriptləri ilə edirdi:
tarixi keçmiş kartı "Gecikmə" statusuna keçirmək, hər səhər kanala "gecikmədə olanlar" xülasəsi göndərmək.
Bu, sistemdən kənarda, audit-siz və təkrarlanmaya qarşı zəif idi.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| `items.due_date date` (nullable), fərdi sahə yox, **əsas sütun** | İndeks, filtr, gecikmə məntiqi və zamanlayıcı üçün sürətli və tipli olmalıdır |
| Tarix (vaxtsız) + **workspace saat qurşağı** (`workspaces.timezone`, IANA, default `UTC`) | "Bu gün" hər yerdə eyni gün deyil; Bakıda gecə yarısı UTC-də hələ dünəndir (FinTez-də 12:00 UTC hiyləsi bu səbəbdən lazım idi) |
| Gecikmiş = `due_date < bu gün (workspace qurşağında)` **və** status kateqoriyası `todo`/`in_progress` | Hazır və ləğv edilmiş işlər heç vaxt gecikmir |
| Zamanlayıcı yeni olay yaradır: `item.overdue` (hər tapşırıq + hər son tarix üçün **bir dəfə**) | Mövcud konveyer (outbox → FanOut → webhook-lar və qaydalar) dəyişmədən işləyir: "Gecikəndə → statusu Gecikmə et / email göndər" adi qaydadır |
| Təkrarlanmaya qarşı: `item_overdue_marks (item_id, due_date)` cədvəli, `INSERT … ON CONFLICT DO NOTHING` | Bir neçə replika və ya təkrar işləmə eyni olayı iki dəfə yaratmır. `items` sətrinə toxunulmur: `updated_at`/`version` dəyişmir, insan redaktələri ilə konflikt yaranmır. Son tarix dəyişəndə yeni tarix üçün yenidən xəbərdarlıq olur |
| Gündəlik xülasə: `workspace.overdue_digest` olayı, workspace-in seçdiyi saatda (`overdue_digest_hour`, 0–23, default söndürülüb), gündə **bir dəfə** (`workspace_digest_runs (workspace_id, local_date)`) | Səhər kanala "Gecikmədə olanlar (N)" mesajı webhook ilə (ADR-0010-dakı Mattermost/Slack formatı) çatır. Gecikmiş iş yoxdursa olay yaranmır |
| Xülasədə ən çox 100 tapşırıq (ən köhnə son tarix birinci), `total` və `truncated` ilə | Mesaj ölçüsü məhdud qalır |
| Zamanlayıcı worker prosesində dəqiqədə bir işləyir; hər dövr məhdud partiyalarla (`LIMIT`) | Ayrı cron/servis yoxdur; `nebula worker` replikaları təhlükəsiz paralel işləyir (təkrarlanmaya qarşı cədvəllər + `SKIP LOCKED`) |
| Zamanlayıcı olayları `source = 'system'`, `actor_id = NULL` | Avtomatlaşdırmanın yaratdığı olaylar qaydaları tetikləmir (döngü qoruması); sistem olayları isə tetikləyir |
| Qayda trigger-i `item.overdue` əlavə olundu | Şərtlər digər tapşırıq olayları kimi olay anındakı snapshot üzərində işləyir |
| `GET …/items?due=overdue\|today\|week\|none` filtri | Cədvəl, lövhə və sayğaclar eyni filtrlə işləyir |
| Workspace ayarları: `PATCH /workspaces/{ws}` (admin+): ad, saat qurşağı, xülasə saatı; UI-da "Ümumi" bölməsi | |

## Nəticələr

- Gecikmə dəqiqə dəqiqliyi ilə deyil, gün dəqiqliyi ilədir (son tarix tarixdir, saat deyil).
- Saat qurşağı dəyişəndə növbəti dövrdə yeni "bu gün" tətbiq olunur; artıq yaranmış olaylar geri alınmır.
- Tekrarlanan tapşırıqlar (növbəti addım) eyni zamanlayıcı və eyni `due_date` üzərində qurulacaq.
