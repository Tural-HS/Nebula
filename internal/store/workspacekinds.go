package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Tural-HS/Nebula/internal/opt"
)

// Workspace kinds (ADR-0033): a second template axis next to the language.
// The language decides the names; the kind decides what a new workspace is
// pre-configured with (workflow, item types, fields, a four-eyes policy, a
// project with shared views and intake forms, the working-day calendar).
// Content is code, like the language templates: there is no template
// editor. Everything it creates is ordinary configuration, editable in
// Settings afterwards, and is created through the same functions (and
// validators and events) the API uses.
const (
	KindGeneral = "general" // the language template alone (the historical behaviour)
	KindFinance = "finance"
	KindIT      = "it"
	KindHR      = "hr"
)

// WorkspaceKinds lists the kinds in display order.
func WorkspaceKinds() []string { return []string{KindGeneral, KindFinance, KindIT, KindHR} }

// loc is one name in every template language: az, en, tr, ru.
type loc [4]string

func (l loc) in(lang string) string {
	switch lang {
	case TemplateAzerbaijani:
		return l[0]
	case TemplateTurkish:
		return l[2]
	case TemplateRussian:
		return l[3]
	}
	return l[1]
}

type kindStatus struct {
	key      string // referenced by policy and views; never stored
	name     loc
	category string
}

type kindType struct {
	key  string
	name loc
}

type kindField struct {
	key     string
	name    loc
	typ     string
	choices []loc
	colors  []string
	// formula gives the expression per language ("" = the same as en).
	formula func(lang string) string
	types   []string // item type keys the field is bound to (never required)
}

type kindView struct {
	name    loc
	columns []string
	sort    string
	// filters builds the view's filters from the created statuses and
	// choice IDs (field key -> choice label index -> id).
	filters func(statuses map[string]uuid.UUID, choices map[string][]string) ViewFilters
}

type kindFormField struct {
	key      string
	label    loc
	help     loc
	required bool
}

type kindForm struct {
	slug          string // suffix after the workspace slug
	title         loc
	intro         loc
	itemType      string
	titleTemplate loc
	fields        []kindFormField
	maxFiles      int
}

type kindSpec struct {
	key       string
	workflow  loc
	statuses  []kindStatus
	itemTypes []kindType
	fields    []kindField
	// policyOn guards entering this status: admins or owners, one approval.
	policyOn string
	// azCalendar adds the official (or fixed-date) Azerbaijani calendar of
	// the current year to az workspaces.
	azCalendar bool
	project    struct {
		key, visibility string
		name, desc      loc
	}
	views []kindView
	forms []kindForm
}

// localize replaces the language template's workflow, statuses and item
// types with the kind's, in lang. Rules and the time zone stay.
func (k kindSpec) localize(tpl workspaceTemplate, lang string) workspaceTemplate {
	tpl.workflow = k.workflow.in(lang)
	tpl.statuses = make([]templateStatus, len(k.statuses))
	for i, st := range k.statuses {
		tpl.statuses[i] = templateStatus{name: st.name.in(lang), category: st.category, initial: i == 0}
	}
	tpl.itemTypes = make([][2]string, len(k.itemTypes))
	for i, it := range k.itemTypes {
		tpl.itemTypes[i] = [2]string{it.key, it.name.in(lang)}
	}
	return tpl
}

type kindTarget struct {
	wsID     uuid.UUID
	wsSlug   string
	owner    uuid.UUID
	lang     string
	workflow installedWorkflow
}

// installKind creates the kind's configuration inside the workspace-create
// transaction. Every step appends its own event, as if an admin had done it.
func installKind(ctx context.Context, tx pgx.Tx, t kindTarget, k kindSpec) error {
	statuses := make(map[string]uuid.UUID, len(k.statuses))
	for i, st := range k.statuses {
		statuses[st.key] = t.workflow.statuses[i]
	}

	choices := map[string][]string{}
	for i, f := range k.fields {
		p := CreateFieldParams{WorkspaceID: t.wsID, ActorID: t.owner, Key: f.key, Name: f.name.in(t.lang), Type: f.typ}
		for j, c := range f.choices {
			in := ChoiceInput{Label: c.in(t.lang)}
			if j < len(f.colors) {
				in.Color = &f.colors[j]
			}
			p.Choices = append(p.Choices, in)
		}
		if f.formula != nil {
			p.Formula = &FormulaInput{Expression: f.formula(t.lang)}
		}
		def, err := createField(ctx, tx, p)
		if err != nil {
			return fmt.Errorf("kind %s: field %s: %w", k.key, f.key, err)
		}
		for _, c := range def.Options.Choices {
			choices[f.key] = append(choices[f.key], c.ID)
		}
		for _, typ := range f.types {
			if _, err := bindField(ctx, tx, t.wsID, t.owner, typ, f.key, false, i); err != nil {
				return fmt.Errorf("kind %s: bind %s to %s: %w", k.key, f.key, typ, err)
			}
		}
	}

	if k.policyOn != "" {
		if _, err := setApprovalPolicy(ctx, tx, t.wsID, t.owner, t.workflow.id, statuses[k.policyOn], ApprovalPolicyInput{
			MinApprovals: 1, ApproverRoles: []string{RoleAdmin, RoleOwner},
		}); err != nil {
			return fmt.Errorf("kind %s: approval policy: %w", k.key, err)
		}
	}

	if k.azCalendar && t.lang == TemplateAzerbaijani {
		today, err := workspaceToday(ctx, tx, t.wsID)
		if err != nil {
			return err
		}
		if _, err := addAzerbaijanHolidays(ctx, tx, t.wsID, t.owner, today.Year()); err != nil {
			return fmt.Errorf("kind %s: calendar: %w", k.key, err)
		}
	}

	desc := k.project.desc.in(t.lang)
	project, err := createProject(ctx, tx, t.wsID, t.owner, k.project.key, k.project.name.in(t.lang), &desc,
		k.project.visibility, ProjectTemplate{})
	if err != nil {
		return fmt.Errorf("kind %s: project: %w", k.key, err)
	}

	caller := ViewCaller{WorkspaceID: t.wsID, ProjectID: project.ID, UserID: t.owner, Role: RoleOwner}
	for _, v := range k.views {
		cfg := ViewConfig{Columns: slices.Clone(v.columns), Sort: v.sort}
		if v.filters != nil {
			cfg.Filters = v.filters(statuses, choices)
		}
		if _, err := createView(ctx, tx, caller, ViewInput{
			Name: opt.Of(v.name.in(t.lang)), Kind: opt.Of(ViewTable), Shared: opt.Of(true), Config: opt.Of(cfg),
		}); err != nil {
			return fmt.Errorf("kind %s: view %s: %w", k.key, v.name.in(TemplateEnglish), err)
		}
	}

	for _, f := range k.forms {
		slug, err := freeFormSlug(ctx, tx, t.wsSlug, f.slug)
		if err != nil {
			return err
		}
		in := FormInput{
			Slug: slug, Title: f.title.in(t.lang), ItemType: f.itemType, TitleTemplate: f.titleTemplate.in(t.lang),
			Audience: FormAudienceMembers, Enabled: true, MaxFiles: f.maxFiles,
		}
		if intro := f.intro.in(t.lang); intro != "" {
			in.Intro = &intro
		}
		for _, ff := range f.fields {
			in.Fields = append(in.Fields, FormField{Key: ff.key, Label: ff.label.in(t.lang), Help: ff.help.in(t.lang), Required: ff.required})
		}
		if _, err := createForm(ctx, tx, t.wsID, project.ID, t.owner, in); err != nil {
			return fmt.Errorf("kind %s: form %s: %w", k.key, f.slug, err)
		}
	}
	return nil
}

// freeFormSlug returns "<workspace slug>-<suffix>", or with a random tail
// when that is taken: form slugs are unique across all workspaces, and a
// clash must not fail the workspace creation.
func freeFormSlug(ctx context.Context, q querier, wsSlug, suffix string) (string, error) {
	prefix := wsSlug
	if maxPrefix := 63 - len(suffix) - 1 - 5; len(prefix) > maxPrefix {
		prefix = strings.TrimRight(prefix[:maxPrefix], "-")
	}
	slug := prefix + "-" + suffix
	for range 5 {
		var taken bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM forms WHERE slug = $1)`, slug).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return slug, nil
		}
		b := make([]byte, 2)
		_, _ = rand.Read(b) // never fails (crypto/rand, Go 1.24+)
		slug = prefix + "-" + suffix + "-" + hex.EncodeToString(b)
	}
	return "", &ConflictError{Code: "form_slug_taken", Message: "could not find a free form address; try another workspace slug"}
}

// Shared names.
var (
	locInProgress = loc{"İcrada", "In Progress", "Devam ediyor", "В работе"}
	locReview     = loc{"Yoxlamada", "Review", "İncelemede", "На проверке"}
	locDone       = loc{"Tamamlandı", "Done", "Tamamlandı", "Готово"}
	locCanceled   = loc{"Ləğv edildi", "Canceled", "İptal edildi", "Отменено"}
	locTask       = loc{"Tapşırıq", "Task", "Görev", "Задача"}
	locMyWork     = loc{"Mənim işlərim", "My work", "İşlerim", "Мои задачи"}
	locOverdue    = loc{"Gecikənlər", "Overdue", "Gecikenler", "Просроченные"}
	locNote       = loc{"Qeyd", "Note", "Not", "Комментарий"}
	locNeededBy   = loc{"Nə vaxta lazımdır", "Needed by", "Ne zamana kadar", "Нужно к дате"}
)

var workspaceKinds = map[string]kindSpec{
	KindGeneral: {key: KindGeneral},
	KindFinance: kindFinance(),
	KindIT:      kindIT(),
	KindHR:      kindHR(),
}

// vatFormula: the Azerbaijani standard rate is 18% (Tax Code art. 173.1);
// an empty rate means 18 in az workspaces only. Elsewhere the language says
// nothing about the country, so the rate must be entered (else VAT is empty).
func vatFormula(lang string) string {
	if lang == TemplateAzerbaijani {
		return "round(amount * coalesce(vat_rate, 18) / 100, 2)"
	}
	return "round(amount * vat_rate / 100, 2)"
}

func kindFinance() kindSpec {
	money := []string{"payment", "invoice"}
	k := kindSpec{
		key:      KindFinance,
		workflow: loc{"Maliyyə", "Finance", "Finans", "Финансы"},
		statuses: []kindStatus{
			{"preparing", loc{"Hazırlanır", "Preparing", "Hazırlanıyor", "Подготовка"}, "todo"},
			{"review", loc{"Yoxlamada", "In review", "İncelemede", "На проверке"}, "in_progress"},
			{"awaiting", loc{"Təsdiq gözləyir", "Awaiting approval", "Onay bekliyor", "Ожидает утверждения"}, "in_progress"},
			{"paid", loc{"Ödənildi", "Paid", "Ödendi", "Оплачено"}, "done"},
			{"canceled", locCanceled, "canceled"},
		},
		itemTypes: []kindType{
			{"payment", loc{"Ödəniş", "Payment", "Ödeme", "Платёж"}},
			{"invoice", loc{"Hesab-faktura", "Invoice", "Fatura", "Счёт-фактура"}},
			{"report", loc{"Hesabat", "Report", "Rapor", "Отчёт"}},
		},
		fields: []kindField{
			{key: "amount", name: loc{"Məbləğ", "Amount", "Tutar", "Сумма"}, typ: FieldNumber, types: money},
			{key: "vat_rate", name: loc{"ƏDV dərəcəsi, %", "VAT rate, %", "KDV oranı, %", "Ставка НДС, %"}, typ: FieldNumber, types: money},
			{key: "vat", name: loc{"ƏDV", "VAT", "KDV", "НДС"}, typ: FieldFormula, formula: vatFormula, types: money},
			{key: "counterparty", name: loc{"Kontragent", "Counterparty", "Karşı taraf", "Контрагент"}, typ: FieldText, types: money},
			{key: "payment_date", name: loc{"Ödəniş tarixi", "Payment date", "Ödeme tarihi", "Дата оплаты"}, typ: FieldDate, types: money},
		},
		policyOn:   "paid",
		azCalendar: true,
		views: []kindView{
			{
				name:    locOverdue,
				columns: []string{"key", "title", "type", "status", "assignee", "due_date", "field.amount", "field.counterparty"},
				sort:    SortDue,
				filters: func(map[string]uuid.UUID, map[string][]string) ViewFilters { return ViewFilters{Due: "overdue"} },
			},
			{
				name:    loc{"Təsdiq gözləyir", "Awaiting approval", "Onay bekliyor", "Ожидает утверждения"},
				columns: []string{"key", "title", "type", "assignee", "field.amount", "field.vat", "field.counterparty", "field.payment_date"},
				sort:    SortDue,
				filters: func(st map[string]uuid.UUID, _ map[string][]string) ViewFilters {
					return ViewFilters{Status: []uuid.UUID{st["awaiting"]}}
				},
			},
		},
		forms: []kindForm{{
			slug:     "payment-request",
			title:    loc{"Ödəniş tələbi", "Payment request", "Ödeme talebi", "Заявка на оплату"},
			intro:    loc{"Hesab-fakturanı və ya müqaviləni əlavə edin.", "Attach the invoice or the contract.", "Faturayı veya sözleşmeyi ekleyin.", "Приложите счёт или договор."},
			itemType: "payment",
			titleTemplate: loc{"Ödəniş: {{field.counterparty}}", "Payment: {{field.counterparty}}",
				"Ödeme: {{field.counterparty}}", "Платёж: {{field.counterparty}}"},
			fields: []kindFormField{
				{key: "counterparty", label: loc{"Kontragent", "Counterparty", "Karşı taraf", "Контрагент"}, required: true},
				{key: "amount", label: loc{"Məbləğ", "Amount", "Tutar", "Сумма"}, required: true},
				{key: "vat_rate", label: loc{"ƏDV dərəcəsi, %", "VAT rate, %", "KDV oranı, %", "Ставка НДС, %"},
					help: loc{"Boş qalsa 18% (Vergi Məcəlləsi, 173.1). ƏDV-dən azad ödəniş üçün 0 yazın.",
						"0 if the payment is exempt from VAT.", "Ödeme KDV'den muafsa 0 yazın.", "0, если платёж не облагается НДС."}},
				{key: "payment_date", label: loc{"Ödəniş tarixi", "Payment date", "Ödeme tarihi", "Дата оплаты"}, required: true},
				{key: FormFieldDescription, label: loc{"Ödənişin təyinatı", "Purpose of payment", "Ödemenin amacı", "Назначение платежа"}},
			},
			maxFiles: 5,
		}},
	}
	k.project.key, k.project.visibility = "FIN", "workspace"
	k.project.name = loc{"Maliyyə", "Finance", "Finans", "Финансы"}
	k.project.desc = loc{"Ödənişlər, hesab-fakturalar və hesabatlar.", "Payments, invoices and reports.",
		"Ödemeler, faturalar ve raporlar.", "Платежи, счета-фактуры и отчёты."}
	return k
}

func kindIT() kindSpec {
	all := []string{"bug", "task", "request"}
	k := kindSpec{
		key:      KindIT,
		workflow: loc{"İT", "IT", "BT", "ИТ"},
		statuses: []kindStatus{
			{"backlog", loc{"Növbədə", "Backlog", "Bekleyen", "Бэклог"}, "todo"},
			{"in_progress", locInProgress, "in_progress"},
			{"review", locReview, "in_progress"},
			{"done", locDone, "done"},
			{"canceled", locCanceled, "canceled"},
		},
		itemTypes: []kindType{
			{"bug", loc{"Xəta", "Bug", "Hata", "Ошибка"}},
			{"task", locTask},
			{"request", loc{"Sorğu", "Request", "Talep", "Запрос"}},
		},
		fields: []kindField{
			{key: "priority", name: loc{"Prioritet", "Priority", "Öncelik", "Приоритет"}, typ: FieldSelect, types: all,
				choices: []loc{
					{"Aşağı", "Low", "Düşük", "Низкий"},
					{"Orta", "Medium", "Orta", "Средний"},
					{"Yüksək", "High", "Yüksek", "Высокий"},
					{"Kritik", "Critical", "Kritik", "Критический"},
				},
				colors: []string{"#71717a", "#2563eb", "#d97706", "#dc2626"}},
			{key: "component", name: loc{"Komponent", "Component", "Bileşen", "Компонент"}, typ: FieldText, types: all},
		},
		views: []kindView{
			{
				name:    locMyWork,
				columns: []string{"key", "title", "type", "status", "due_date", "field.priority", "field.component"},
				sort:    SortDue,
				filters: func(map[string]uuid.UUID, map[string][]string) ViewFilters { return ViewFilters{Assignee: "me"} },
			},
			{
				name:    loc{"Kritik", "Critical", "Kritik", "Критические"},
				columns: []string{"key", "title", "type", "status", "assignee", "due_date", "field.component"},
				filters: func(_ map[string]uuid.UUID, ch map[string][]string) ViewFilters {
					return ViewFilters{Fields: map[string]string{"priority": ch["priority"][3]}}
				},
			},
		},
		forms: []kindForm{{
			slug:     "access-request",
			title:    loc{"İT giriş sorğusu", "IT access request", "BT erişim talebi", "Запрос доступа к ИТ-системам"},
			intro:    loc{"Hansı sistemə və nə üçün giriş lazım olduğunu yazın.", "Tell us which system you need access to and why.", "Hangi sisteme ve neden erişim gerektiğini yazın.", "Укажите, к какой системе нужен доступ и зачем."},
			itemType: "request",
			titleTemplate: loc{"Giriş: {{field.component}} ({{requester}})", "Access: {{field.component}} ({{requester}})",
				"Erişim: {{field.component}} ({{requester}})", "Доступ: {{field.component}} ({{requester}})"},
			fields: []kindFormField{
				{key: "component", label: loc{"Sistem", "System", "Sistem", "Система"}, required: true},
				{key: FormFieldDescription, label: loc{"Əsaslandırma", "Reason", "Gerekçe", "Обоснование"}, required: true},
				{key: "priority", label: loc{"Prioritet", "Priority", "Öncelik", "Приоритет"}},
				{key: FormFieldDueDate, label: locNeededBy},
			},
		}},
	}
	k.project.key, k.project.visibility = "IT", "workspace"
	k.project.name = loc{"İT", "IT", "BT", "ИТ"}
	k.project.desc = loc{"Xətalar, tapşırıqlar və istifadəçi sorğuları.", "Bugs, tasks and user requests.",
		"Hatalar, görevler ve kullanıcı talepleri.", "Ошибки, задачи и запросы пользователей."}
	return k
}

// kindHR keeps personnel data in a private project (ADR-0013): employees
// file requests through the forms and see only their own submissions.
func kindHR() kindSpec {
	people := []string{"onboarding", "leave"}
	k := kindSpec{
		key:      KindHR,
		workflow: loc{"Kadrlar", "HR", "İK", "Кадры"},
		statuses: []kindStatus{
			{"new", loc{"Yeni", "New", "Yeni", "Новая"}, "todo"},
			{"in_progress", locInProgress, "in_progress"},
			{"done", locDone, "done"},
			{"canceled", locCanceled, "canceled"},
		},
		itemTypes: []kindType{
			{"onboarding", loc{"İşə qəbul", "Onboarding", "İşe alım", "Онбординг"}},
			{"leave", loc{"Məzuniyyət", "Leave", "İzin", "Отпуск"}},
			{"task", locTask},
		},
		fields: []kindField{
			{key: "employee", name: loc{"Əməkdaş", "Employee", "Çalışan", "Сотрудник"}, typ: FieldText, types: people},
			{key: "position", name: loc{"Vəzifə", "Position", "Pozisyon", "Должность"}, typ: FieldText, types: []string{"onboarding"}},
			{key: "leave_type", name: loc{"Məzuniyyət növü", "Leave type", "İzin türü", "Вид отпуска"}, typ: FieldSelect, types: []string{"leave"},
				choices: []loc{
					{"Əmək məzuniyyəti", "Annual leave", "Yıllık izin", "Ежегодный отпуск"},
					{"Ödənişsiz məzuniyyət", "Unpaid leave", "Ücretsiz izin", "Отпуск без сохранения зарплаты"},
					{"Təhsil məzuniyyəti", "Study leave", "Eğitim izni", "Учебный отпуск"},
					{"Digər", "Other", "Diğer", "Другое"},
				}},
			{key: "start_date", name: loc{"Başlama tarixi", "Start date", "Başlangıç tarihi", "Дата начала"}, typ: FieldDate, types: people},
			{key: "end_date", name: loc{"Bitmə tarixi", "End date", "Bitiş tarihi", "Дата окончания"}, typ: FieldDate, types: []string{"leave"}},
		},
		azCalendar: true,
		views: []kindView{
			{
				name:    locMyWork,
				columns: []string{"key", "title", "type", "status", "due_date", "field.employee", "field.start_date"},
				sort:    SortDue,
				filters: func(map[string]uuid.UUID, map[string][]string) ViewFilters { return ViewFilters{Assignee: "me"} },
			},
			{
				name:    locOverdue,
				columns: []string{"key", "title", "type", "status", "assignee", "due_date", "field.employee"},
				sort:    SortDue,
				filters: func(map[string]uuid.UUID, map[string][]string) ViewFilters { return ViewFilters{Due: "overdue"} },
			},
		},
		forms: []kindForm{
			{
				slug:     "leave-request",
				title:    loc{"Məzuniyyət sorğusu", "Leave request", "İzin talebi", "Заявление на отпуск"},
				itemType: "leave",
				titleTemplate: loc{"Məzuniyyət: {{requester}}", "Leave: {{requester}}",
					"İzin: {{requester}}", "Отпуск: {{requester}}"},
				fields: []kindFormField{
					{key: "leave_type", label: loc{"Məzuniyyət növü", "Leave type", "İzin türü", "Вид отпуска"}, required: true},
					{key: "start_date", label: loc{"Başlama tarixi", "First day", "İlk gün", "Первый день"}, required: true},
					{key: "end_date", label: loc{"Bitmə tarixi", "Last day", "Son gün", "Последний день"}, required: true},
					{key: FormFieldDescription, label: locNote},
				},
			},
			{
				slug:     "new-hire",
				title:    loc{"Yeni əməkdaş", "New hire", "Yeni çalışan", "Новый сотрудник"},
				itemType: "onboarding",
				titleTemplate: loc{"İşə qəbul: {{field.employee}}", "Onboarding: {{field.employee}}",
					"İşe alım: {{field.employee}}", "Онбординг: {{field.employee}}"},
				fields: []kindFormField{
					{key: "employee", label: loc{"Əməkdaşın adı", "Employee name", "Çalışanın adı", "Имя сотрудника"}, required: true},
					{key: "position", label: loc{"Vəzifə", "Position", "Pozisyon", "Должность"}, required: true},
					{key: "start_date", label: loc{"İşə başlama tarixi", "Start date", "İşe başlama tarihi", "Дата выхода"}, required: true},
					{key: FormFieldDescription, label: locNote},
				},
			},
		},
	}
	k.project.key, k.project.visibility = "HR", "private"
	k.project.name = loc{"Kadrlar", "HR", "İnsan Kaynakları", "Кадры"}
	k.project.desc = loc{"İşə qəbul və məzuniyyət sorğuları (gizli layihə).", "Onboarding and leave requests (private project).",
		"İşe alım ve izin talepleri (gizli proje).", "Онбординг и заявления на отпуск (закрытый проект)."}
	return k
}
