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

## Webhook-lar və gizli layihələr (həll edildi, migrasiya 00012)

| Qərar | Səbəb |
|---|---|
| Webhook-un `project_ids` siyahısı var; **boş = bütün açıq (workspace) layihələr** | Mövcud webhook-lar gizli layihə hadisələrini avtomatik almamağa başlayır: təhlükəsiz default |
| Gizli layihə yalnız onu **açıq seçmiş** webhook-a çatır | Şöbənin kanalı = öz layihəsini seçmiş webhook (FinTez modeli) |
| Hadisənin layihəsi bir yerdə təyin olunur (`EventScopeOf`): tapşırıq və şərh (tapşırıq üzərindən), təkrarlanma, layihə, görünüş. Silmə hadisələri `project_id`-ni payload-da daşıyır | Sətri silinmiş obyektin hadisəsi də düzgün süzülür |
| Layihəsi müəyyən edilə bilməyən layihə hadisəsi **heç bir** webhook-a getmir | Fail closed: şübhədə sızma yoxdur |
| Filtr həm FanOut-da, həm də çatdırılma anında yoxlanır | Hadisədən sonra layihə gizli edilibsə, artıq növbədə olan çatdırılma da dayanır |
| Gündəlik xülasə hər layihədən ən köhnə 50 gecikməni (cəmi ≤1000) və `by_project` saylarını daşıyır; çatdırılmada webhook-un layihələrinə görə süzülür, cəm yenidən hesablanır, boş qalırsa göndərilmir | Hər şöbə kanalı yalnız öz gecikmələrini, düzgün sayla alır |
| Workspace səviyyəli hadisələr (sahələr, workflow-lar, üzvlər…) bütün abunə olan webhook-lara gedir | Onlar layihəyə aid deyil |

Adminin yazdığı avtomatlaşdırma qaydasında `send_email` alıcısı kimi konkret email göstərilibsə, o şəxs layihəni
görə bilməsə də email alır: bu, adminin öz qərarıdır və audit olunur.
