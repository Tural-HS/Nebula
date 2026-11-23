package server

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// Synthetic Mattermost Boards archive (format v2): invented data only.
func boardsArchive(t *testing.T, boardJSONL string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"version.json":   `{"version":2,"date":1764000000000}`,
		"b1/board.jsonl": boardJSONL,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const fintezBoard = `{"type":"board","data":{"id":"b1","title":"Maliyyə şöbəsi","cardProperties":[` +
	`{"id":"pS","name":"Status","type":"select","options":[{"id":"o1","value":"İcrada"},{"id":"o2","value":"Tamamlandı"}]},` +
	`{"id":"pA","name":"İcraçı","type":"person","options":[]},` +
	`{"id":"pD","name":"Son tarix","type":"date","options":[]}]}}
{"type":"block","data":{"id":"c1","parentId":"b1","createdBy":"mmAlice","type":"card","title":"Aylıq ƏDV bəyannaməsi","fields":{"contentOrder":["x1"],"properties":{"pS":"o1","pA":"mmCarol","pD":"{\"from\":1766448000000}"}},"createAt":1764000001000}}
{"type":"block","data":{"id":"x1","parentId":"c1","createdBy":"mmAlice","type":"text","title":"Hesabatı 20-dək göndər","fields":{},"createAt":1764000002000}}
{"type":"block","data":{"id":"m1","parentId":"c1","createdBy":"mmCarol","type":"comment","title":"Bank çıxarışı gözlənilir","fields":{},"createAt":1764000003000}}
{"type":"block","data":{"id":"c2","parentId":"b1","createdBy":"mmAlice","type":"card","title":"Kassa yoxlaması","fields":{"properties":{"pS":"o2"}},"createAt":1764000004000}}
{"type":"boardMember","data":{"boardId":"b1","userId":"mmAlice"}}
`

func TestImportMattermostBoards(t *testing.T) {
	e := newP1Env(t)
	e.setup() // alice owner, bob guest, carol member of acme
	e.must(201, "alice", "POST", "/api/v1/workspaces", map[string]any{"slug": "fz", "name": "FinTez", "template": "az"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/fz/members", map[string]any{"email": "carol@example.com", "role": "member"})
	e.must(201, "alice", "POST", "/api/v1/workspaces/fz/projects", map[string]any{"key": "MAL", "name": "Maliyyə", "visibility": "private"})
	// Private project: people must be project members before they can be assignees.
	e.must(204, "alice", "PUT", "/api/v1/workspaces/fz/projects/MAL/members/"+e.userID("carol"), nil)
	imports := "/api/v1/workspaces/fz/projects/MAL/imports"
	archive := boardsArchive(t, fintezBoard)

	pv := e.upload("alice", imports+"/preview", "archive-2026-11-24.boardarchive", archive, "", nil)
	if pv.Status != 200 || pv.Body["format"] != "boards" || pv.Body["row_count"].(float64) != 2 {
		t.Fatalf("preview: %d %s", pv.Status, pv.Raw)
	}
	people := pv.Body["people"].([]any)
	if len(people) != 1 || people[0].(map[string]any)["id"] != "mmCarol" {
		t.Fatalf("people: %v", people)
	}
	sug := suggestedTargets(pv)
	if sug[0] != "title" || sug[1] != "status" || sug[2] != "assignee" || sug[3] != "due_date" || sug[4] != "description" {
		t.Fatalf("suggested mapping: %v", sug)
	}
	mapping := fullMapping(pv)

	// Only workspace members can be mapped (the email is written into comments).
	mapping["people"] = map[string]any{"mmCarol": "dave@example.com"}
	if r := e.upload("alice", imports+"?dry_run=true", "a.boardarchive", archive, "", mapping); r.Status != 422 {
		t.Fatalf("non-member in people: %d %s", r.Status, r.Raw)
	}
	// Without a person mapping the assignee stays empty; the dry run is clean.
	delete(mapping, "people")
	r := e.upload("alice", imports+"?dry_run=true", "a.boardarchive", archive, "", mapping)
	if r.Status != 200 || r.Body["valid"].(float64) != 2 || r.Body["error_count"].(float64) != 0 {
		t.Fatalf("dry run: %d %s", r.Status, r.Raw)
	}

	mapping["people"] = map[string]any{"mmCarol": "Carol@Example.com", "mmUnknown": ""}
	r = e.upload("alice", imports, "a.boardarchive", archive, "", mapping)
	if r.Status != 201 || r.Body["created"].(float64) != 2 {
		t.Fatalf("import: %d %s", r.Status, r.Raw)
	}
	it := e.must(200, "alice", "GET", "/api/v1/workspaces/fz/projects/MAL/items/1", nil).Body
	if it["title"] != "Aylıq ƏDV bəyannaməsi" || it["due_date"] != "2025-12-23" || it["assignee_id"] != e.userID("carol") {
		t.Fatalf("item 1: %v", it)
	}
	desc, _ := it["description"].(string)
	if !strings.Contains(desc, "Hesabatı 20-dək göndər") || !strings.Contains(desc, "**Carol@Example.com**") ||
		!strings.Contains(desc, "Bank çıxarışı gözlənilir") {
		t.Fatalf("description: %q", desc)
	}
	st := e.must(200, "alice", "GET", "/api/v1/workspaces/fz/projects/MAL/items/2", nil).Body["status"].(map[string]any)
	if st["name"] != "Tamamlandı" {
		t.Fatalf("status of item 2: %v", st)
	}

	// Guests and members cannot import; Boards archives are bounded like other files.
	if r := e.upload("carol", imports+"/preview", "a.boardarchive", archive, "", nil); r.Status != 403 {
		t.Fatalf("member previews import: %d", r.Status)
	}
	big := bytes.Repeat([]byte("x"), maxBoardsFileBytes+1)
	if r := e.upload("alice", imports+"/preview", "big.boardarchive", big, "", nil); r.Status != 413 {
		t.Fatalf("oversized archive: %d", r.Status)
	}
	if r := e.upload("alice", imports+"/preview", "big.csv", bytes.Repeat([]byte("x"), maxImportFileBytes+1), "", nil); r.Status != 413 {
		t.Fatalf("CSV keeps the 10 MiB limit: %d", r.Status)
	}
}
