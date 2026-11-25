# ADR-0041: Workspace hesabatları "Hesabatlar"

- Status: Qəbul edilib
- Tarix: 2026-11-25
- Əsas: [ADR-0013](0013-project-privacy.md) (gizli layihələr), [ADR-0026](0026-four-eyes-approvals.md) və
  [ADR-0034](0034-approval-inbox-reapproval-deadlines.md) (təsdiqlər, son müddət), [ADR-0037](0037-home-page.md) (bir endpoint, pgx batch),
  [ADR-0040](0040-item-table-export.md) (CSV yazıcısı), [ADR-0019](0019-mobile-web-ui.md) (telefon), [ADR-0025](0025-ui-languages.md) (4 dil)

## Kontekst

Maliyyə/bek-ofis rəhbəri (FinTez pilotu) həftəlik iclasa gələndə eyni sualları verir: **nə qədər iş açıqdır, nə qədəri gecikib,
bu dövrdə nə qədər iş gəldi və bağlandı, təsdiqlər nə qədər gözləyir, kimdə nə qədər iş var?** Bu rəqəmlər indi yalnız
layihə-layihə filtr sayğaclarından, əl ilə toplanırdı. Lazım olan: workspace üzrə bir səhifə, hər rəqəmin **təkrarlana bilən**
tərifi (auditor "bu 12 haradan gəldi?" soruşa bilər) və rəqəmdən tapşırıq siyahısına keçid.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| **Bir endpoint**: `GET /workspaces/{ws}/reports?from&to&projects&assignee&type` bütün vidjetlərin məlumatını bir cavabda qaytarır (ADR-0037 nümunəsi) | Səhifə bir sorğu ilə açılır; bütün vidjetlər eyni anın rəqəmləridir | Vidjet başına endpoint: 6 sorğu, 6 dəfə filtr/məxfilik kodu, fərqli anların rəqəmləri |
| Server tərəfdə **bir pgx batch** (6 SQL, bir şəbəkə gedişi), **`REPEATABLE READ READ ONLY` tranzaksiyada** | Bütün vidjetlər eyni snapshot-u sayır (KPI-da 12 → cədvəldə də 12); yazma mümkün deyil | Adi `READ COMMITTED`: iki sorğu arasında dəyişiklik rəqəmləri uyğunsuz edə bilərdi |
| Batch-in ilk əmri **`SET LOCAL plan_cache_mode = force_custom_plan`** | Filtrlər `$5 IS NULL OR …` açarlarıdır; pgx statement keşi 5 icradan sonra PostgreSQL-ə *generic* plan seçdirdi və o plan hadisə jurnalını hər tapşırıq üçün skan etdi: ölçmədə **60 ms → 13 s**. Xüsusi plan hər dəfə parametrlərə uyğundur (planlama ~1 ms) | Dinamik SQL qurmaq (filtrə görə şərt əlavə etmək): daha çox kod və test; `QueryExecModeExec`: pgx batch üçün sorğu başına rejim dəstəkləmir |
| **Məxfilik**: hər SQL eyni `vis` CTE-dən başlayır — `visibleProjectFor` (ADR-0013), arxivlənmiş layihələr çıxılır. Layihə filtri yalnız `vis`-i daraldır; görünməyən açar boş nəticə verir (400 yox, sızma da yox). Rol: workspace-in **hər üzvü, guest daxil** | Guest yalnız görə bildiyi layihələri sayır; layihəsi yoxdursa sıfırlar (UI: "Görə bildiyiniz layihə yoxdur", naviqasiya linki gizlənir) | Yalnız admin/member: guest auditor üçün faydalı imkanı bağlayardı; məxfilik onsuz da SQL-dədir |
| **Saat qurşağı = workspace-in** (`workspaces.timezone`): `from`/`to` yerli tarixlərdir, dövr `[from 00:00, to+1 00:00)` yerli vaxtdır; "bu gün" `now() AT TIME ZONE tz` | Bakıdakı komanda üçün 25 yanvar 23:30-da yaradılan iş 25-inə düşür (test: UTC-də 26-sı olan tapşırıq) | Brauzerin qurşağı: fərqli şəhərlərdəki iki rəhbər fərqli rəqəm görərdi |
| İş günü təqvimi (ADR-0027) **istifadə olunmur** | Bu metrikalar təqvim günü ilə sayılır; iş günü ilə yaş/müddət əlavə mürəkkəblikdir, pilot tələb etmir | — |
| **Dövr limiti 731 gün** (2 il, uzun il daxil); həftəlik sütunlar **186 günə qədər**, daha uzun dövrdə **aylıq** | Sorğu xərci məhduddur; qrafikdə ən çox ~27 həftə və ya 25 ay sütunu olur | Limitsiz: 10 illik sorğu bütün jurnalı skan edərdi |
| **"Bitib" (completed) = status dəyişikliyi hadisəsi**: dövrdə `item.updated` hadisəsi, `changes`-də `status_id` var və `snapshot.status_category = 'done'`; tapşırıq başına dövrdəki **sonuncu** belə hadisə (bir dəfə sayılır). Birbaşa "Hazır" statusunda yaradılan (import daxil) tapşırıq **sayılmır** | Hadisə jurnalı tarixi saxlayır, yeni sütun/trigger lazım deyil; snapshot keçid anındakı kateqoriyanı saxlayır (sonra kateqoriya dəyişsə də tarix düz qalır). Import edilmiş köhnə tapşırıqlar import günü "bitmiş" görünməsin | `items.completed_at` sütunu + trigger + backfill: miqrasiya, ikinci həqiqət mənbəyi; hazırkı status: "dövrdə bitən" sualına cavab vermir |
| Filtrlər (layihə, icraçı, tip) tapşırığın **hazırkı** dəyərlərinə tətbiq olunur; iş yükündə "bitən" də hazırkı icraçıya yazılır | Siyahıya keçid eyni filtrlə eyni tapşırıqları göstərir; sadə və proqnozlaşdırılan | Bitmə anındakı icraçı (snapshot): siyahı ilə uyğunsuz rəqəm |
| **Vidjetlər** (yalnız bunlar): KPI-lar; yaradılan və bitən (həftə/ay); açıq tapşırıqların yaşı və gecikməsi (0–7, 8–30, 31–90, 90+ gün); təsdiq müddəti (median, p90, keçid üzrə, təsdiq/rədd/ləğv/gec); icraçılar üzrə yük; layihələr üzrə status bölgüsü. "İndi" və "dövrdə" rəqəmləri UI-da ayrıca işarələnir | Rəhbərin həftəlik sualları; "indi" rəqəmləri dövrdən asılı deyil (dövr dəyişəndə dəyişməməsi testdədir) | Daha çox vidjet: hər biri izah və saxlanma tələb edir |
| **Keçidlər** layihənin tapşırıq siyahısına, siyahının öz URL parametrləri ilə: açıq → həmin layihənin açıq statusları (`status=…`), gecikən → `due=overdue`, təsdiq gözləyən → `approval=pending`, status seqmenti → `status=<id>`, hamısında hesabatın icraçısı. KPI linkləri **bir layihə** əhatə olunanda; iş yükündə sətir bir layihədədirsə rəqəm link, çox layihədə sətri açıb layihə üzrə linklər. **Tip filtri varsa keçid yoxdur** (siyahıda tip filtri yoxdur, rəqəm uyğun gəlməzdi) | Hər keçid **tam həmin rəqəmi** göstərir; workspace üzrə layihələrarası siyahı yoxdur (ADR-0037 açıq sualı) | Layihələrarası yeni siyahı səhifəsi: bu ADR-in həcmindən böyükdür |
| **Qrafiklər**: `web/package.json`-da qrafik kitabxanası yoxdur → **əl ilə CSS/HTML zolaqları**, yeni asılılıq yoxdur. Rənglər dataviz yoxlayıcısı ilə yoxlanıb (sky-700/amber-600 açıq, sky-600/amber-600 tünd — CVD ΔE ≥ 23), kateqoriyalar: boz (etiketli neytral), sky, emerald, ləğv — ştrixli. Hər qrafikin **legendi**, `sr-only` xülasəsi və **cədvəl alternativi** ("Cədvəl kimi göstər"), yaş/gecikmə zolaqları etiketli rəqəm siyahısıdır (`dl`), statuslar mətn kimi də yazılır — rəng heç vaxt tək məna daşımır. Tünd rejim Tailwind token-ləri ilə | Sabit sayda sadə formalar üçün kitabxana (recharts ~100 KB+) əsassızdır; HTML zolaqları istənilən enlikdə kəskindir (SVG `preserveAspectRatio="none"` mətni əyirdi) | Kitabxana: təchizat zənciri və bundle artımı |
| **Vəziyyətlər**: hər vidjet eyni sorğudan yüklənmə (spinner, `motion-reduce`), xəta (`ErrorState` + "Yenidən cəhd et") və boş vəziyyət (nə üçün boş olduğunu deyir); filtr dəyişəndə köhnə rəqəmlər solğun qalır (`aria-busy`, `aria-live` "Yenilənir…") | Bir sorğu — bir səbəb; ekran tullanmır | — |
| **Filtrlər URL-dədir** (`range`, `from`, `to`, `projects`, `assignee`, `type`), hazır dövrlər workspace-in bugünkü tarixindən hesablanır (`lib/reports.ts`, vitest) | Hesabat paylaşılan linkdir; iclasda "bu linki açın" kifayətdir | Lokal state: link paylaşılmır |
| **Keş**: `Cache-Control: private, max-age=60` + `Vary: Authorization` (qlobal `no-store`-un yerinə); TanStack Query 30 s təzə sayır | Geri düyməsi və yeniləmə batch-i təkrarlamır; `Vary` eyni brauzerdə başqa istifadəçiyə köhnə cavabın verilməsinin qarşısını alır | `no-store`: hər klik ağır sorğu; ictimai keş: şəxsi məlumat |
| **Rate limit**: istifadəçi başına dəqiqədə 60 (ardıcıl 30), API-nin ümumi limitinə əlavə | Hesabat batch-i adi siyahıdan ağırdır; insan üçün bol, skreyper üçün yox | — |
| **CSV (iş yükü cədvəli)**: `GET /workspaces/{ws}/reports/workload.csv` + eyni filtrlər, `lang`, `delimiter=comma\|semicolon`; ADR-0040 yazıcısı (`tableexport`: BOM, CRLF, düstur inyeksiyası qoruması, `;` ilə onluq vergül), ixrac limiti (saatda 30) və **`reports.exported`** workspace audit hadisəsi (filtr xülasəsi və sətir sayı, rəqəmlər yox) | Ucuzdur: mövcud yazıcı; rəhbər cədvəli Excel-də iclasa aparır | xlsx: CSV kifayətdir, sonra əlavə oluna bilər |
| **Miqrasiya yoxdur** — ölçmə aşağıda; mövcud indekslər kifayətdir | Yeni indeks yazma xərcidir; ölçmədə darboğaz CPU-dur (100k sətirin aqreqasiyası), indeks yox | Hadisələrdə qismən indeks (`status_id` dəyişiklikləri): 90 günlük `done` CTE indi ~5 ms bitmap skanıdır, qazanc yoxdur |

## Rəqəmlərin SQL tərifləri

Hamısı `internal/store/reports.go`-dadır; burada qısaldılmış formada. Parametrlər: `$1` workspace, `$2` baxan, `$3` baxan admin/sahibdir,
`$4` layihə açarları (NULL = hamısı), `$5` icraçı (NULL = hər kəs), `$6` yalnız təyin olunmamış, `$7` tip açarı ('' = hamısı), `$8`/`$9` from/to.

```sql
-- Ortaq əhatə (hər sorğunun başı)
ws   AS (SELECT timezone, (now() AT TIME ZONE timezone)::date AS today,
                $8::date::timestamp AT TIME ZONE timezone AS lo,
                ($9::date + 1)::timestamp AT TIME ZONE timezone AS hi
         FROM workspaces WHERE id = $1),
vis  AS (SELECT p.id, p.key, p.name FROM projects p
         WHERE p.workspace_id = $1 AND p.archived_at IS NULL
           AND ($4 IS NULL OR p.key = ANY ($4))
           AND (p.visibility = 'workspace' OR $3
                OR EXISTS (SELECT 1 FROM project_members pm WHERE pm.project_id = p.id AND pm.user_id = $2))),
it   AS (SELECT i.*, (i.created_at AT TIME ZONE ws.timezone)::date AS created_on,
                s.category IN ('todo', 'in_progress') AS open
         FROM items i JOIN statuses s ON s.id = i.status_id CROSS JOIN ws
         WHERE i.workspace_id = $1 AND i.deleted_at IS NULL AND i.project_id IN (SELECT id FROM vis)
           AND ($5 IS NULL OR i.assignee_id = $5) AND (NOT $6 OR i.assignee_id IS NULL)
           AND ($7 = '' OR i.item_type_id = (SELECT id FROM item_types WHERE workspace_id = $1 AND key = $7))),
done AS (SELECT DISTINCT ON (e.entity_id) e.entity_id AS item_id, e.occurred_at AS at
         FROM events e CROSS JOIN ws JOIN items i ON i.id = e.entity_id      -- + it-in filtri i üzərində
         WHERE e.workspace_id = $1 AND e.entity_type = 'item' AND e.action = 'item.updated'
           AND e.occurred_at >= ws.lo AND e.occurred_at < ws.hi
           AND e.payload->'changes' ? 'status_id'
           AND e.payload->'snapshot'->>'status_category' = 'done'
         ORDER BY e.entity_id, e.occurred_at DESC, e.id DESC)
```

| Rəqəm | Tərif |
|---|---|
| Açıq (indi) | `count(*) FROM it WHERE open` |
| Gecikən (indi) | `count(*) FROM it WHERE open AND due_date < ws.today` (siyahının `due=overdue` filtri ilə eyni) |
| Təsdiq gözləyir (indi) / müddəti keçib | `count(*) FROM approval_requests r JOIN it ON it.id = r.item_id WHERE r.state = 'pending'` / `… AND r.due_on < ws.today` |
| Yaradılıb (dövrdə) | `count(*) FROM it WHERE created_at >= ws.lo AND created_at < ws.hi` = həftəlik/aylıq sütunların cəmi |
| Bitib (dövrdə) | `count(*) FROM done` = sütunların cəmi |
| Sütun (həftə/ay) | `date_trunc('week'\|'month', created_on)` və `date_trunc(…, done.at AT TIME ZONE ws.timezone)`; sütunlar `generate_series(date_trunc(b, from), to, '1 b')`; həftə bazar ertəsi başlayır; ilk/son natamam həftə yalnız dövrdəki günləri sayır |
| Yaş (açıq) | `ws.today - created_on`: `≤ 7`, `8–30`, `31–90`, `> 90` |
| Gecikmə (açıq) | `ws.today - due_date`: `1–7`, `8–30`, `31–90`, `> 90` (yalnız `due_date < today`) |
| Təsdiq müddəti | `approval_requests r JOIN it` (canlı, görünən tapşırıqlar), `r.state <> 'pending' AND r.closed_at ∈ [lo, hi)`; `GROUP BY GROUPING SETS ((r.to_status_id, status, workflow), ())`. Median/p90 = `percentile_cont(0.5 / 0.9) WITHIN GROUP (ORDER BY extract(epoch FROM closed_at - created_at))` yalnız `approved`/`rejected` (ləğv olunanın qərarı yoxdur); `late` = qərar `(closed_at AT TIME ZONE tz)::date > due_on` |
| İş yükü | `it LEFT JOIN done` üzrə `GROUP BY assignee_id, project`: açıq, gecikən (yuxarıdakı kimi), bitən = `count(done.item_id)`; sətirlər `open OR done` |
| Status bölgüsü | `count(*) FROM it GROUP BY project, status` — bütün canlı tapşırıqların hazırkı statusu |

Hamısı `internal/server/reports_integration_test.go`-da tanınmış tarixlərlə (geriyə çəkilmiş `created_at`, təsdiq və status hadisələri,
Bakı saat qurşağı sərhədi) **dəqiq** yoxlanılır: KPI-lar, 3 həftəlik sütun, yaş/gecikmə qrupları, median 6 saat və p90 73 440 s,
iş yükü sırası, status bölgüsü, gizli layihə (üzv olmayan üçün sayılmır, admin və layihə üzvü üçün sayılır), filtrlər, aylıq bölgü,
validasiya, keş başlıqları, CSV və audit.

## Ölçmə

`TestReportsLargeDataset` (`NEBULA_REPORTS_BENCH=1`): bir workspace, 100 000 tapşırıq (2 il, 5 layihə, biri gizli), 250 000 hadisə,
6 400 bağlı təsdiq sorğusu. Bütün batch (6 sorğu):

| Hal | p50 | p95 |
|---|---|---|
| sahib, son 30 gün | 173 ms | 182 ms |
| sahib, son 90 gün | 193 ms | 200 ms |
| sahib, 2 il | 364 ms | 372 ms |
| üzv (gizlisiz), 2 il | 322 ms | 327 ms |
| sahib, 1 layihə + 1 icraçı, 90 gün | 33 ms | 34 ms |

Planlar: `items` workspace üzrə seq scan + statuslarla hash join (hər "indi" sorğusu ~35–60 ms, xərc 100k sətirin aqreqasiyasıdır),
`done` — `events_workspace_time_idx` bitmap skanı + `items_pkey`, təsdiqlər — `approval_requests` + `items` pkey. Pilotun real
həcmi (~2 000 tapşırıq) üçün bütün batch bir neçə millisaniyədir. Generic plan ilə eyni sorğular 13 s çəkirdi (yuxarıda).

## Nəticələr

- Yeni: `internal/store/reports.go`, `internal/server/reports_api.go`, `web/src/pages/ReportsPage.tsx`, `web/src/api/reports.ts`,
  `web/src/lib/reports.ts`, lüğət modulu `reports` (az/en/tr/ru) və fasad `i18n-reports.ts`; marşrut `/w/:ws/reports`,
  layihələr səhifəsində "Hesabatlar" linki (təsdiq qutusunun yanında, yalnız görünən layihə varsa).
- Testlər: `reports_integration_test.go` (rəqəmlər, məxfilik, filtrlər, validasiya, CSV), `web/src/lib/reports.test.ts` (dövrlər, URL,
  keçidlər, sıralama), E2E `web/e2e/reports.spec.ts` (rəqəmlər, gizli layihə, drill-down filtrləri, 320 px).
- Test köməkçisi `newP1EnvOwner`: hadisə jurnalı yalnız əlavə edilir, keçmişi geriyə çəkmək üçün sxem sahibinin pool-u lazımdır.
- Rollback: miqrasiya yoxdur, image-i qaytarmaq kifayətdir.

## Açıq suallar

- Tapşırıq siyahısında **tip filtri** (onda tip filtrli hesabatdan da keçid mümkün olar) və workspace üzrə layihələrarası siyahı
  (KPI-lardan çox layihəli keçid).
- "Dövrdə bitən" və "dövrdə yaradılan" üçün siyahı filtri (`created_from/to`, `completed_from/to`) — indi bu rəqəmlərin keçidi yoxdur.
- 100k+ tapşırıqlı workspace üçün "indi" sorğularını bir skana birləşdirmək və ya gecəlik aqreqat cədvəl (indi 0,2–0,4 s).
- İş günü ilə yaş və təsdiq müddəti (ADR-0027), xlsx ixracı, hesabatın planlı email ilə göndərilməsi.
