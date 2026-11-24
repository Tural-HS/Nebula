package evidence

// labels are the report's texts. Azerbaijani is the default (ADR-0025); the
// exporter's UI language is used when it is one of these.
type labels struct {
	Title, Workspace, Kind, KindItem, KindScope, Projects, AllProjects, Range, WholeHistory        string
	ApprovalsOnly, Yes, No, ExportedBy, ExportedAt, SnapshotAt, Timezone, ServerVersion, ExportID  string
	KeyFingerprint, VerifyNote, Summary                                                            string
	Items, Events, Comments, Approvals, Links, Attachments                                         string
	Project, Type, Status, Assignee, Reporter, Created, Updated, Due, Deleted, Fields, Description string
	History, ProjectEvents, Requested, By, Transition, State, Required, Decisions, Approve, Reject string
	CommentDeleted, File, Size, SHA256, Uploader, Time, Who, Action, Details, System, None, Edited string
	Invalidated                                                                                    string
	States                                                                                         map[string]string
	LinkTypes                                                                                      map[string]string
}

var allLabels = map[string]labels{
	"az": {
		Title: "Audit sübut paketi", Workspace: "Workspace", Kind: "Növ", KindItem: "Tapşırıq", KindScope: "Əhatə",
		Projects: "Layihələr", AllProjects: "ixracçının gördüyü bütün layihələr", Range: "Tarix aralığı", WholeHistory: "bütün tarixçə",
		ApprovalsOnly: "Yalnız təsdiqdən keçən tapşırıqlar", Yes: "bəli", No: "xeyr", ExportedBy: "İxrac edən",
		ExportedAt: "İxrac vaxtı", SnapshotAt: "Məlumatların vəziyyəti", Timezone: "Saat qurşağı", ServerVersion: "Server versiyası",
		ExportID: "İxrac ID", KeyFingerprint: "İmza açarının barmaq izi",
		VerifyNote: "Bu hesabat və data/ qovluğundakı fayllar manifest.json-da SHA-256 ilə qeyd olunub; manifest serverin Ed25519 açarı ilə imzalanıb (manifest.sig). Yoxlama: nebula evidence verify <zip>. Barmaq izini Nebula-da Parametrlər → Audit log səhifəsindəki ilə müqayisə edin.",
		Summary:    "Xülasə", Items: "Tapşırıqlar", Events: "Hadisələr", Comments: "Şərhlər", Approvals: "Təsdiqlər", Links: "Əlaqələr",
		Attachments: "Fayllar", Project: "Layihə", Type: "Növ", Status: "Status", Assignee: "İcraçı", Reporter: "Yaradan",
		Created: "Yaradılıb", Updated: "Yenilənib", Due: "Son tarix", Deleted: "Silinib", Fields: "Sahələr", Description: "Təsvir",
		History: "Tarixçə", ProjectEvents: "Layihə və təsdiq qaydası hadisələri", Requested: "Sorğu", By: "Kim",
		Transition: "Keçid", State: "Vəziyyət", Required: "Tələb olunan", Decisions: "Qərarlar", Approve: "təsdiqlədi",
		Reject: "rədd etdi", CommentDeleted: "(şərh silinib)", File: "Fayl", Size: "Ölçü", SHA256: "SHA-256",
		Uploader: "Yükləyən", Time: "Vaxt", Who: "Kim", Action: "Hadisə", Details: "Təfərrüat", System: "sistem / avtomatlaşdırma",
		None: "—", Edited: "redaktə olunub", Invalidated: "məzmun dəyişdi, təsdiq etibarsızdır",
		States:    map[string]string{"pending": "gözləyir", "approved": "təsdiqlənib", "rejected": "rədd edilib", "canceled": "ləğv edilib"},
		LinkTypes: map[string]string{"blocks": "bloklayır", "relates": "əlaqəlidir", "duplicates": "təkrarlayır"},
	},
	"en": {
		Title: "Audit evidence package", Workspace: "Workspace", Kind: "Kind", KindItem: "Item", KindScope: "Scope",
		Projects: "Projects", AllProjects: "every project the exporter can see", Range: "Date range", WholeHistory: "whole history",
		ApprovalsOnly: "Only items that went through approval", Yes: "yes", No: "no", ExportedBy: "Exported by",
		ExportedAt: "Exported at", SnapshotAt: "Data as of", Timezone: "Time zone", ServerVersion: "Server version",
		ExportID: "Export ID", KeyFingerprint: "Signing key fingerprint",
		VerifyNote: "This report and the files in data/ are listed with their SHA-256 in manifest.json; the manifest is signed with the server's Ed25519 key (manifest.sig). Verify with: nebula evidence verify <zip>. Compare the fingerprint with the one shown in Nebula under Settings → Audit log.",
		Summary:    "Summary", Items: "Items", Events: "Events", Comments: "Comments", Approvals: "Approvals", Links: "Links",
		Attachments: "Attachments", Project: "Project", Type: "Type", Status: "Status", Assignee: "Assignee", Reporter: "Reporter",
		Created: "Created", Updated: "Updated", Due: "Due date", Deleted: "Deleted", Fields: "Fields", Description: "Description",
		History: "History", ProjectEvents: "Project and approval policy events", Requested: "Requested", By: "By",
		Transition: "Transition", State: "State", Required: "Required", Decisions: "Decisions", Approve: "approved",
		Reject: "rejected", CommentDeleted: "(comment deleted)", File: "File", Size: "Size", SHA256: "SHA-256",
		Uploader: "Uploaded by", Time: "Time", Who: "Who", Action: "Event", Details: "Details", System: "system / automation",
		None: "—", Edited: "edited", Invalidated: "content changed, approval invalidated",
		States:    map[string]string{"pending": "pending", "approved": "approved", "rejected": "rejected", "canceled": "canceled"},
		LinkTypes: map[string]string{"blocks": "blocks", "relates": "relates to", "duplicates": "duplicates"},
	},
	"tr": {
		Title: "Denetim kanıt paketi", Workspace: "Çalışma alanı", Kind: "Tür", KindItem: "Görev", KindScope: "Kapsam",
		Projects: "Projeler", AllProjects: "dışa aktaranın görebildiği tüm projeler", Range: "Tarih aralığı", WholeHistory: "tüm geçmiş",
		ApprovalsOnly: "Yalnızca onaydan geçen görevler", Yes: "evet", No: "hayır", ExportedBy: "Dışa aktaran",
		ExportedAt: "Dışa aktarma zamanı", SnapshotAt: "Verilerin durumu", Timezone: "Saat dilimi", ServerVersion: "Sunucu sürümü",
		ExportID: "Dışa aktarma ID", KeyFingerprint: "İmza anahtarı parmak izi",
		VerifyNote: "Bu rapor ve data/ klasöründeki dosyalar SHA-256 ile manifest.json'da listelenir; manifest sunucunun Ed25519 anahtarıyla imzalanmıştır (manifest.sig). Doğrulama: nebula evidence verify <zip>. Parmak izini Nebula'da Ayarlar → Denetim günlüğü sayfasındakiyle karşılaştırın.",
		Summary:    "Özet", Items: "Görevler", Events: "Olaylar", Comments: "Yorumlar", Approvals: "Onaylar", Links: "Bağlantılar",
		Attachments: "Dosyalar", Project: "Proje", Type: "Tür", Status: "Durum", Assignee: "Atanan", Reporter: "Oluşturan",
		Created: "Oluşturuldu", Updated: "Güncellendi", Due: "Bitiş tarihi", Deleted: "Silindi", Fields: "Alanlar", Description: "Açıklama",
		History: "Geçmiş", ProjectEvents: "Proje ve onay kuralı olayları", Requested: "Talep", By: "Kim",
		Transition: "Geçiş", State: "Durum", Required: "Gerekli", Decisions: "Kararlar", Approve: "onayladı",
		Reject: "reddetti", CommentDeleted: "(yorum silindi)", File: "Dosya", Size: "Boyut", SHA256: "SHA-256",
		Uploader: "Yükleyen", Time: "Zaman", Who: "Kim", Action: "Olay", Details: "Ayrıntı", System: "sistem / otomasyon",
		None: "—", Edited: "düzenlendi", Invalidated: "içerik değişti, onay geçersiz",
		States:    map[string]string{"pending": "bekliyor", "approved": "onaylandı", "rejected": "reddedildi", "canceled": "iptal edildi"},
		LinkTypes: map[string]string{"blocks": "engelliyor", "relates": "ilişkili", "duplicates": "yineliyor"},
	},
	"ru": {
		Title: "Пакет аудиторских доказательств", Workspace: "Рабочее пространство", Kind: "Тип", KindItem: "Задача", KindScope: "Выборка",
		Projects: "Проекты", AllProjects: "все проекты, видимые экспортёру", Range: "Период", WholeHistory: "вся история",
		ApprovalsOnly: "Только задачи, прошедшие согласование", Yes: "да", No: "нет", ExportedBy: "Экспортировал",
		ExportedAt: "Время экспорта", SnapshotAt: "Данные на момент", Timezone: "Часовой пояс", ServerVersion: "Версия сервера",
		ExportID: "ID экспорта", KeyFingerprint: "Отпечаток ключа подписи",
		VerifyNote: "Этот отчёт и файлы в папке data/ перечислены с SHA-256 в manifest.json; манифест подписан ключом Ed25519 сервера (manifest.sig). Проверка: nebula evidence verify <zip>. Сравните отпечаток с указанным в Nebula: Настройки → Журнал аудита.",
		Summary:    "Сводка", Items: "Задачи", Events: "События", Comments: "Комментарии", Approvals: "Согласования", Links: "Связи",
		Attachments: "Файлы", Project: "Проект", Type: "Тип", Status: "Статус", Assignee: "Исполнитель", Reporter: "Автор",
		Created: "Создана", Updated: "Обновлена", Due: "Срок", Deleted: "Удалена", Fields: "Поля", Description: "Описание",
		History: "История", ProjectEvents: "События проекта и правил согласования", Requested: "Запрос", By: "Кто",
		Transition: "Переход", State: "Состояние", Required: "Требуется", Decisions: "Решения", Approve: "согласовал(а)",
		Reject: "отклонил(а)", CommentDeleted: "(комментарий удалён)", File: "Файл", Size: "Размер", SHA256: "SHA-256",
		Uploader: "Загрузил(а)", Time: "Время", Who: "Кто", Action: "Событие", Details: "Подробности", System: "система / автоматизация",
		None: "—", Edited: "изменён", Invalidated: "содержание изменено, согласование аннулировано",
		States:    map[string]string{"pending": "ожидает", "approved": "согласовано", "rejected": "отклонено", "canceled": "отменено"},
		LinkTypes: map[string]string{"blocks": "блокирует", "relates": "связана с", "duplicates": "дублирует"},
	},
}

// Languages are the report languages; the first is the default.
var Languages = []string{"az", "en", "tr", "ru"}

// NormalizeLanguage returns lang if the report has it, else "az".
func NormalizeLanguage(lang string) string {
	if _, ok := allLabels[lang]; ok {
		return lang
	}
	return "az"
}
