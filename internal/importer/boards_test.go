package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// boardArchive builds a synthetic .boardarchive (no real data).
func boardArchive(t *testing.T, version string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if version != "" {
		w, _ := zw.Create("version.json")
		_, _ = w.Write([]byte(version))
	}
	for name, body := range files {
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

func jsonl(t *testing.T, lines ...map[string]any) string {
	t.Helper()
	var sb strings.Builder
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func boardLine(title string, template bool) map[string]any {
	return map[string]any{"type": "board", "data": map[string]any{
		"id": "b1", "title": title, "isTemplate": template, "deleteAt": 0,
		"cardProperties": []any{
			map[string]any{"id": "pS", "name": "Status", "type": "select", "options": []any{
				map[string]any{"id": "o1", "value": "İcrada", "color": "propColorBlue"},
				map[string]any{"id": "o2", "value": "Tamamlandı", "color": "propColorGreen"},
			}},
			map[string]any{"id": "pT", "name": "Teqlər", "type": "multiSelect", "options": []any{
				map[string]any{"id": "t1", "value": "Vergi"}, map[string]any{"id": "t2", "value": "Hesabat"},
			}},
			map[string]any{"id": "pA", "name": "İcraçı", "type": "person"},
			map[string]any{"id": "pD", "name": "Son tarix", "type": "date"},
			map[string]any{"id": "pN", "name": "Məbləğ", "type": "number"},
			map[string]any{"id": "pC", "name": "Təcili", "type": "checkbox"},
			map[string]any{"id": "pF", "name": "Fayllar", "type": "file"},
			map[string]any{"id": "pU", "name": "Yeniləyən", "type": "updatedBy"},
		},
	}}
}

func block(id, parent, typ, title string, createAt int64, fields map[string]any) map[string]any {
	return map[string]any{"type": "block", "data": map[string]any{
		"id": id, "parentId": parent, "boardId": "b1", "createdBy": "uAlice", "type": typ, "title": title,
		"fields": fields, "createAt": createAt, "deleteAt": 0,
	}}
}

func sampleBoard(t *testing.T) string {
	t.Helper()
	comment := block("m1", "c1", "comment", "Bank çıxarışı gözlənilir", 1764000003000, map[string]any{})
	comment["data"].(map[string]any)["createdBy"] = "uBob"
	deleted := block("c3", "b1", "card", "Silinmiş", 1764000000500, map[string]any{})
	deleted["data"].(map[string]any)["deleteAt"] = 1
	return jsonl(t,
		boardLine("Maliyyə", false),
		block("c1", "b1", "card", "Aylıq ƏDV bəyannaməsi", 1764000001000, map[string]any{
			"contentOrder": []any{"x1", []any{"x2", "x3"}},
			"properties": map[string]any{
				"pS": "o1", "pT": []any{"t1", "t2"}, "pA": "uBob",
				"pD": `{"from":1766448000000}`, "pN": "1200.5", "pC": "true",
			},
		}),
		block("x1", "c1", "text", "Hesabatı **20-dək** göndər", 1764000002000, map[string]any{}),
		block("x2", "c1", "checkbox", "Qaimələri yoxla", 1764000002100, map[string]any{"value": true}),
		block("x3", "c1", "divider", "", 1764000002200, map[string]any{}),
		comment,
		block("c2", "b1", "card", "Şablon kart", 1764000000100, map[string]any{"isTemplate": true}),
		deleted,
		block("c4", "b1", "card", "Tarixsiz iş", 1764000004000, map[string]any{"properties": map[string]any{"pS": "o2"}}),
		block("v1", "b1", "view", "Board view", 1764000000000, map[string]any{"viewType": "board"}),
		map[string]any{"type": "boardMember", "data": map[string]any{"boardId": "b1", "userId": "uAlice"}},
	)
}

const v2 = `{"version":2,"date":1764000000000}`

func TestBoardsArchive(t *testing.T) {
	data := boardArchive(t, v2, map[string]string{"b1/board.jsonl": sampleBoard(t), "b1/img.png": "\x89PNG"})
	tb, err := Parse(context.Background(), "archive-2026-11-24.boardarchive", data, "", DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if tb.Format != FormatBoards || tb.Sheet != "Maliyyə" {
		t.Fatalf("format/sheet: %v %q", tb.Format, tb.Sheet)
	}
	want := []string{"Başlıq", "Status", "Teqlər", "İcraçı", "Son tarix", "Məbləğ", "Təcili", "Təsvir"}
	if strings.Join(tb.Headers, "|") != strings.Join(want, "|") {
		t.Fatalf("headers: %q", tb.Headers)
	}
	if len(tb.Rows) != 2 { // template and deleted cards are skipped
		t.Fatalf("rows: %d", len(tb.Rows))
	}
	r := tb.Rows[0].Cells
	if r[0] != "Aylıq ƏDV bəyannaməsi" || r[1] != "İcrada" || r[2] != "Vergi, Hesabat" || r[3] != "" ||
		r[4] != "2025-12-23" || r[5] != "1200.5" || r[6] != "true" {
		t.Fatalf("row before mapping: %q", r)
	}
	desc := r[7]
	for _, s := range []string{"Hesabatı **20-dək** göndər", "- [x] Qaimələri yoxla", "---",
		"Mattermost istifadəçisi", "Bank çıxarışı gözlənilir"} {
		if !strings.Contains(desc, s) {
			t.Errorf("description lacks %q:\n%s", s, desc)
		}
	}
	if strings.Index(desc, "Hesabatı") > strings.Index(desc, "Qaimələri") {
		t.Error("content order not kept")
	}
	if got := tb.Rows[1].Cells; got[0] != "Tarixsiz iş" || got[1] != "Tamamlandı" || got[4] != "" {
		t.Fatalf("second row: %q", got)
	}

	people := tb.People()
	if len(people) != 1 || people[0].ID != "uBob" || people[0].Cards != 1 || people[0].Samples[0] != "Aylıq ƏDV bəyannaməsi" {
		t.Fatalf("people: %+v", people)
	}
	if err := tb.ApplyPeople(map[string]string{"uBob": "bob@example.com"}, DefaultLimits); err != nil {
		t.Fatal(err)
	}
	r = tb.Rows[0].Cells
	if r[3] != "bob@example.com" || !strings.Contains(r[7], "**bob@example.com**") {
		t.Fatalf("after mapping: %q", r)
	}
}

func TestBoardsArchiveChoosesBoard(t *testing.T) {
	other := strings.Replace(sampleBoard(t), `"title":"Maliyyə"`, `"title":"HR"`, 1)
	data := boardArchive(t, v2, map[string]string{"b1/board.jsonl": sampleBoard(t), "b2/board.jsonl": other})
	tb, err := Parse(context.Background(), "a.boardarchive", data, "Maliyyə", DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tb.Sheets, ",") != "HR,Maliyyə" || tb.Sheet != "Maliyyə" {
		t.Fatalf("sheets: %v %q", tb.Sheets, tb.Sheet)
	}
	if _, err := Parse(context.Background(), "a.boardarchive", data, "Yoxdur", DefaultLimits); !isCode(err, "sheet_not_found") {
		t.Fatalf("unknown board: %v", err)
	}
}

func isCode(err error, code string) bool {
	var ie *Error
	return errors.As(err, &ie) && ie.Code == code
}

func TestBoardsArchiveRejects(t *testing.T) {
	good := sampleBoard(t)
	long := `{"type":"block","data":{"title":"` + strings.Repeat("a", maxBoardLineBytes) + `"}}`
	cases := map[string]struct {
		data []byte
		code string
	}{
		"no version":     {boardArchive(t, "", map[string]string{"b1/board.jsonl": good}), "malformed_file"},
		"version 1":      {boardArchive(t, `{"version":1}`, map[string]string{"b1/board.jsonl": good}), "unsupported_format"},
		"no boards":      {boardArchive(t, v2, map[string]string{"x.txt": "hi"}), "malformed_file"},
		"traversal only": {boardArchive(t, v2, map[string]string{"../b1/board.jsonl": good, "a/b/board.jsonl": good}), "malformed_file"},
		"template":       {boardArchive(t, v2, map[string]string{"b1/board.jsonl": jsonl(t, boardLine("Şablon", true))}), "unsupported_format"},
		"bad json":       {boardArchive(t, v2, map[string]string{"b1/board.jsonl": "{nope\n"}), "malformed_file"},
		"unknown type":   {boardArchive(t, v2, map[string]string{"b1/board.jsonl": `{"type":"user","data":{}}` + "\n"}), "malformed_file"},
		"no board line":  {boardArchive(t, v2, map[string]string{"b1/board.jsonl": jsonl(t, block("c1", "b1", "card", "x", 1, map[string]any{}))}), "malformed_file"},
		"long line":      {boardArchive(t, v2, map[string]string{"b1/board.jsonl": jsonl(t, boardLine("B", false)) + long}), "malformed_file"},
		"not a zip":      {[]byte("hello"), "malformed_file"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(context.Background(), "x.boardarchive", c.data, "", DefaultLimits); !isCode(err, c.code) {
				t.Fatalf("want %s, got %v", c.code, err)
			}
		})
	}

	// Zip bomb guard: declared uncompressed size of the boards counts.
	lim := DefaultLimits
	lim.MaxUnzippedBytes = 100
	if _, err := Parse(context.Background(), "x.boardarchive", boardArchive(t, v2, map[string]string{"b1/board.jsonl": good}), "", lim); !isCode(err, "file_too_large") {
		t.Fatalf("size limit: %v", err)
	}
	lim = DefaultLimits
	lim.MaxRows = 1
	if _, err := Parse(context.Background(), "x.boardarchive", boardArchive(t, v2, map[string]string{"b1/board.jsonl": good}), "", lim); !isCode(err, "too_many_rows") {
		t.Fatalf("row limit: %v", err)
	}
}

func TestBoardDate(t *testing.T) {
	for in, want := range map[string]string{
		`{"from":1766448000000}`:                    "2025-12-23",
		`{"from":1766491200000,"includeTime":true}`: "2025-12-23T12:00:00Z",
		`1766448000000`:                             "2025-12-23",
		`{"from":1766448000000,"to":1766534400000}`: "2025-12-23",
		`garbage`: "garbage",
	} {
		if got := boardDate(in); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
}

func FuzzBoardsArchive(f *testing.F) {
	f.Add([]byte(`{"type":"board","data":{"id":"b","title":"x"}}` + "\n"))
	f.Add([]byte(`{"type":"block","data":{"type":"card","fields":{"contentOrder":[["a"],"b",1]}}}`))
	f.Fuzz(func(_ *testing.T, body []byte) {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("version.json")
		_, _ = w.Write([]byte(v2))
		w, _ = zw.Create("b/board.jsonl")
		_, _ = w.Write(body)
		_ = zw.Close()
		tb, err := Parse(context.Background(), "f.boardarchive", buf.Bytes(), "", DefaultLimits)
		if err == nil {
			_ = tb.People()
			_ = tb.ApplyPeople(map[string]string{"u": "a@b.c"}, DefaultLimits)
		}
	})
}
