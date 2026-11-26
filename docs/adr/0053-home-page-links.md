# ADR-0053: Ana səhifədə layihə keçidləri

- Status: Qəbul edilib
- Tarix: 2026-11-26
- Əsas: [ADR-0037](0037-home-page.md) (ana səhifə), [ADR-0013](0013-project-privacy.md) (gizli layihələr)

## Kontekst

Komanda hər gün eyni bir neçə səhifəni açır (məs. "Müştəri Aylıq Hesabat" və "Lizinq Reyestri"). Ana səhifədə
yalnız "son layihələr" var idi, o da fəaliyyətə görə dəyişir. Hər kəs üçün sabit, admin tərəfindən seçilən
keçidlər istənildi.

## Qərarlar

| Qərar | Səbəb | Alternativ və niyə yox |
|---|---|---|
| Admin workspace ayarlarında (Ümumi → "Ana səhifə keçidləri") **≤ 8 layihə** seçir, sıranı ↑/↓ ilə təyin edir; `workspaces.home_projects uuid[]` | Bir yer, hamı üçün eyni; sıra adminin istədiyi kimi | Hər istifadəçi öz ulduzu: komandaya "bu səhifələrdən başla" demək olmur; lazım olsa sonra əlavə oluna bilər |
| Layihə **id** ilə saxlanılır, açar ilə yox | Layihənin adı/açarı dəyişəndə keçid qalır | Açar: dəyişəndə keçid qırılır |
| Yalnız bu workspace-in **aktiv** layihələri qəbul olunur (başqa tenant-ın id-si 422) | Tenant izolyasiyası; arxivlənmiş layihəyə keçid mənasızdır | — |
| Keçidlər `/me/home` cavabında (`pinned`) **görünmə filtri ilə** gəlir (`visibleProjectFor`) | Gizli layihəni yalnız onun üzvləri görür; adı başqalarına sızmır | Bütün id-ləri klientə vermək və orada süzmək: gizli layihənin adı API-də görünərdi |
| Arxivlənmiş/silinmiş layihə ana səhifədən **özü düşür**, növbəti saxlamada siyahıdan da çıxır | Əlavə təmizlik işi lazım deyil | Trigger ilə silmək: mürəkkəblik, faydası az |
| Dəyişiklik `workspace.updated` audit hadisəsinə yazılır (`home_projects` from/to) | Digər workspace ayarları kimi | — |
| Xarici URL **yoxdur**, yalnız layihə | Phishing / `javascript:` riski yoxdur; daxili naviqasiya | Sərbəst link: təhlükəsizlik yoxlaması və ayrıca UI tələb edir |

## Nəticələr

- Miqrasiya 00039 (`workspaces.home_projects`, CHECK ≤ 8). Down yalnız seçimi itirir.
- API: `PATCH /workspaces/{ws}` `home_projects: [uuid] | null` (admin+); `GET /workspaces/{ws}` və `/me/home`
  (`pinned`) qaytarır.
- Ana səhifədə sayğaclardan yuxarıda kartlar; bir neçə workspace varsa workspace adı da göstərilir.
