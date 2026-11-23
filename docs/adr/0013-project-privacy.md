# ADR-0013: Layihə səviyyəsində məxfilik

- Status: Qəbul edilib
- Tarix: 2026-11-23

## Kontekst

İndiyə qədər workspace üzvü olan hər kəs workspace-in bütün layihələrini görürdü. Şöbələr (HR, hüquq, kredit,
risk…) isə öz işlərini yalnız öz komandası ilə bölüşmək istəyir: FinTez-in Mattermost Boards qurulumunda hər şöbənin
gizli lövhəsi var idi.

## Qərarlar

| Qərar | Səbəb |
|---|---|
| Layihənin `visibility` sahəsi: `workspace` (default, indiki davranış) və ya `private` | Mövcud layihələr dəyişmir; məxfilik açıq seçimdir |
| Gizli layihəni görənlər: `project_members`-dəki istifadəçilər + workspace **admin və sahibləri** | Adminlər workspace-i idarə edir (sahələr, workflow-lar, webhook-lar); onlardan gizlətmək idarəetməni sındırar. Bu, Jira/YouTrack-dakı "admin hər şeyi görür" modelidir |
| Layihə üzvlüyü **rol vermir**: icazələr workspace rolundan gəlir (qonaq yalnız oxuyur, üzv yazır) | Sadə və proqnozlaşdırıla bilən; layihə səviyyəli rollar (məs. "bu layihədə admin") lazım olsa ayrıca əlavə ediləcək |
| Görə bilməyən üçün gizli layihə **404** qaytarır (403 yox), siyahılarda görünmür | Layihənin varlığı belə sızmır (workspace-lər üçün eyni qayda, ADR-0002) |
| Bütün yoxlamalar bir SQL predikatından keçir (`canSeeProject`): layihə yükləmə middleware-i, layihə siyahısı, icraçı təyini, @mention | Bir yerdə səhv, hər yerdə səhv olmasın: ayrı-ayrı məntiq yoxdur |
| İcraçı yalnız layihəni görə bilən şəxs ola bilər; bu, əl ilə yaratma, redaktə, idxal, təkrarlanan tapşırıqlar və avtomatlaşdırma üçün eynidir (hamısı eyni doğrulamadan keçir) | Təyin email-i layihəni görə bilməyən birinə gizli başlığı göndərməsin |
| @mention yalnız layihəni görə bilənlər üçün işləyir; digərləri səssizcə nəzərə alınmır | Email bildirişi məzmunu sızdırmasın; kimin mövcud olduğunu yoxlamaq da mümkün olmasın |
| Workspace-dən çıxarılan istifadəçi layihə üzvlüyünü də avtomatik itirir (FK cascade) | Unudulmuş giriş qalmır |
| Üzvlük və görünürlük dəyişiklikləri audit olunur (`project.updated`, `project.member_added`, `project.member_removed`) | Kimin nə vaxt giriş aldığı izlənir |
| İdarəetmə: admin+ (`PATCH /projects/{p}`, `PUT/DELETE /projects/{p}/members/{userID}`) | Şöbə rəhbərinə layihə səviyyəli admin rolu sonrakı mərhələdə |

## Məlum məhdudiyyət (növbəti iş)

Webhook-lar workspace səviyyəsindədir: `item.*` abunəsi olan webhook **gizli layihələrin** hadisələrini də alır,
gündəlik gecikmə xülasəsi (`workspace.overdue_digest`) isə bütün layihələri bir mesajda toplayır. Webhook-u ümumi
Mattermost kanalına bağlamaq gizli layihənin başlıqlarını həmin kanala göndərə bilər. Həll: webhook-a layihə filtri
(boş = hamısı) və xülasənin çatdırılma zamanı həmin filtrə görə süzülməsi. O vaxta qədər admin gizli layihələr olan
workspace-də ümumi kanal webhook-larını diqqətlə seçməlidir; UI bunu xəbərdarlıqla göstərir.

Adminin yazdığı avtomatlaşdırma qaydasında `send_email` alıcısı kimi konkret email göstərilibsə, o şəxs layihəni
görə bilməsə də email alır: bu, adminin öz qərarıdır və audit olunur.
