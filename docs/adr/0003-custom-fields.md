# ADR-0003: Fərdi sahələr (custom fields)

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

ADR-0001 fərdi sahələri `field_definitions` cədvəli + `items.fields JSONB` (GIN indeks) kimi seçib.
Bu ADR həmin modelin dəqiq qaydalarını müəyyən edir: sahələr harada tətbiq olunur, dəyərlər necə saxlanılır,
nə dəyişdirilə bilər və nə dəyişdirilə bilməz.

## Qərarlar

| Mövzu | Qərar | Səbəb |
|---|---|---|
| Əhatə | Sahə workspace səviyyəsində bir dəfə təyin olunur, `item_type_fields` ilə item type-a bağlanır | "Bir mühərrik, çox şablon": bug-da `severity`, HR-da `start_date`; lazımsız sahələr görünmür |
| Məcburilik | `is_required` bağlamadadır (təkdə deyil) | Eyni sahə bug üçün məcburi, task üçün opsional ola bilər |
| Məcburilik qaydası | **Yazılarkən** tətbiq olunur: yeni item-da tələb olunur, mövcud dəyər `null` edilə bilməz; sahə sonradan məcburi edilərsə köhnə item-lar bloklanmır | Backfill və uzun kilidlər lazım deyil (Jira davranışı) |
| `key`, `field_type` | Yaradılandan sonra **dəyişməzdir** (API strict decoder + DB trigger) | Dəyərlər `key` altında, tipin formatında saxlanılır; dəyişiklik məlumatı sahibsiz qoyar və ya pozar |
| Select seçimləri | `options.choices = [{id, label, color, archived}]`; item-da **choice id** saxlanılır | Label dəyişəndə milyonlarla sətri yenidən yazmaq lazım deyil |
| Seçimin silinməsi | Qadağandır; yalnız arxivləmək olar | Saxlanmış dəyərlər həmişə etibarlı qalır |
| Arxivlənmiş sahə | Dəyər saxlanılır və oxunur; yeni dəyər qəbul edilmir, məcburilik yoxlanmır; açar rezervdə qalır | Hard delete məlumat itirir |
| Bağlamanın silinməsi | Mövcud dəyərlər qalır (silinə bilər), yeni dəyər qəbul edilmir | Məlumat itkisi olmadan konfiqurasiyanı dəyişmək |
| Limitlər | Workspace üzrə 200 aktiv sahə, 500 seçim, `text` ≤ 1000, `long_text` ≤ 10 000 simvol, `multi_select` ≤ 100 dəyər, sorğuda ≤ 200 sahə, ≤ 10 filtr | DoS və JSONB şişməsinə qarşı |

### Kanonik dəyərlər

Hər tipin `items.fields`-də tək bir JSON forması var. Buna görə `fields @> '{"k": v}'` filtrləri dəqiqdir
və `items_fields_idx` (GIN, `jsonb_path_ops`) indeksindən istifadə edir.

| Tip | Saxlanılan forma |
|---|---|
| `text`, `long_text` | kənar boşluqlar silinmiş string; boş string = sil |
| `number` | JSON number, \|v\| ≤ 1e15 (float64-də dəqiq) |
| `boolean` | `true` / `false` |
| `date` | `YYYY-MM-DD` |
| `datetime` | `YYYY-MM-DDTHH:MM:SSZ` (UTC, saniyə dəqiqliyi; giriş RFC 3339, saat qurşağı ilə) |
| `select` | aktiv choice id |
| `multi_select` | sıralanmış, təkrarsız choice id massivi; `[]` = sil |
| `user` | workspace üzvünün kiçik hərfli UUID-i |
| `url` | yalnız `http`/`https` və host ilə (`javascript:`, `data:` XSS-in qarşısını almaq üçün rədd edilir) |
| `email` | display name olmadan ünvan |

## API

| Metod və yol | Rol | Təsvir |
|---|---|---|
| `GET /workspaces/{ws}/fields` | guest+ | Bütün təriflər (arxivlənmişlər daxil) |
| `POST /workspaces/{ws}/fields` | admin+ | `{key, name, type, options?: {choices: [{label, color?}]}}` |
| `GET /workspaces/{ws}/fields/{key}` | guest+ | Tək tərif |
| `PATCH /workspaces/{ws}/fields/{key}` | admin+ | `name`, `options.choices` (tam siyahı; mövcud `id` saxlanılır), `archived` |
| `PUT /workspaces/{ws}/item-types/{type}/fields/{key}` | admin+ | Bağla və ya yenilə: `{required, position}` |
| `DELETE /workspaces/{ws}/item-types/{type}/fields/{key}` | admin+ | Bağlamanı sil |
| `POST/PATCH .../items` → `fields` | member+ | PATCH birləşdirir: dəyər = təyin et, `null` = sil, göndərilməyən açara toxunulmur |
| `GET .../items?field.<key>=<value>` | guest+ | Bərabərlik filtri; `multi_select` üçün "ehtiva edir" |

Xətalar ADR-0002 formatındadır: `422` + `field: "fields.<key>"` (yazma) və ya `"field.<key>"` (filtr).
Hər sahə dəyişikliyi `item.updated` hadisəsində `changes["fields.<key>"] = {from, to}` kimi qeyd olunur.

## Nəticələr

- Müsbət: sahələr item type-a görə fərqlənir, dəyərlər həmişə etibarlı və indekslənə bilən formadadır,
  label/arxiv dəyişiklikləri item-lara toxunmur.
- Mənfi: aralıq filtrləri (`estimate > 3`, tarix aralığı) hələ yoxdur. Bunlar ölçüldükdən sonra isti sahələr üçün
  expression indeks ilə əlavə olunacaq. `text` üçün yalnız dəqiq uyğunluq var, tam mətn axtarışı `title`/`description`
  ilə məhdudlaşır.
- Riskli nöqtə: tip dəyişdirmək mümkün olmadığı üçün səhv tip seçilərsə, yeni sahə yaradıb məlumatı köçürmək lazımdır
  (gələcəkdə miqrasiya alətı).
