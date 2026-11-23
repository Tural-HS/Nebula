// Package importer reads CSV and Excel (.xlsx) sheets into plain string rows
// and converts cell text into custom field values. It never touches the
// database and never writes the uploaded bytes anywhere: everything happens
// in memory, under hard limits (ADR-0011).
package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// Format is the detected file format.
type Format string

// Supported formats.
const (
	FormatCSV  Format = "csv"
	FormatXLSX Format = "xlsx"
)

// Limits bound the work one file can cause.
type Limits struct {
	MaxRows          int   // data rows (blank rows are not counted)
	MaxColumns       int   // columns after trimming trailing blank cells
	MaxCellRunes     int   // characters per cell
	MaxUnzippedBytes int64 // xlsx: total uncompressed size of all zip entries
	MaxZipEntries    int   // xlsx: number of zip entries
}

// DefaultLimits are the production limits (see ADR-0011).
var DefaultLimits = Limits{
	MaxRows:          20_000,
	MaxColumns:       200,
	MaxCellRunes:     32_767, // Excel's own per-cell limit
	MaxUnzippedBytes: 64 << 20,
	MaxZipEntries:    2_000,
}

// Row is one non-blank data row.
type Row struct {
	Line  int      // 1-based line (CSV) or row number (xlsx) in the file, header included
	Cells []string // exactly Table.Width cells
}

// Table is a parsed sheet: a header row plus data rows.
type Table struct {
	Format    Format
	Sheets    []string // xlsx: every sheet name in workbook order; CSV: nil
	Sheet     string   // xlsx: the sheet that was read
	Delimiter string   // CSV: ",", ";" or "\t"
	Date1904  bool     // xlsx: serial dates count from 1904 (old Mac workbooks)
	Headers   []string // Width entries; blank header cells are ""
	Rows      []Row
}

// Width is the number of columns.
func (t *Table) Width() int { return len(t.Headers) }

// Error is a problem with the uploaded file that the client can fix.
type Error struct {
	Code     string
	Message  string
	TooLarge bool // a size limit was exceeded (HTTP 413)
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func fail(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func tooLarge(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), TooLarge: true}
}

var (
	zipMagic = []byte("PK\x03\x04")
	oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1} // .xls, encrypted .xlsx
	utf8BOM  = []byte{0xEF, 0xBB, 0xBF}
)

// Parse reads a whole file. The format is decided by content (zip magic means
// xlsx), with the extension only used to reject formats we do not read.
// sheet selects an xlsx sheet by name; empty means the first visible sheet.
// The context bounds parse time; cancellation is checked between rows.
func Parse(ctx context.Context, fileName string, data []byte, sheet string, lim Limits) (*Table, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch {
	case len(data) == 0:
		return nil, fail("empty_file", "the file is empty")
	case bytes.HasPrefix(data, oleMagic):
		return nil, fail("unsupported_format", "legacy .xls and password-protected workbooks are not supported; save the sheet as .xlsx or CSV (UTF-8)")
	case bytes.HasPrefix(data, zipMagic):
		if ext != ".xlsx" && ext != ".xlsm" {
			return nil, fail("unsupported_format", "only .xlsx workbooks and CSV files are supported")
		}
		return parseXLSX(ctx, data, sheet, lim)
	}
	switch ext {
	case ".csv", ".tsv", ".txt", "":
		if sheet != "" {
			return nil, fail("sheet_not_found", "CSV files have no sheets")
		}
		return parseCSV(ctx, data, ext == ".tsv", lim)
	case ".xlsx", ".xlsm":
		return nil, fail("malformed_file", "the file is not a valid .xlsx workbook")
	}
	return nil, fail("unsupported_format", "only .xlsx workbooks and CSV files are supported")
}

// builder collects rows and enforces the row, column and cell limits.
type builder struct {
	ctx     context.Context
	lim     Limits
	table   *Table
	header  bool
	seen    int
	maxCols int
}

func (b *builder) add(line int, cells []string) error {
	b.seen++
	if b.seen%256 == 0 {
		if err := b.ctx.Err(); err != nil {
			return fail("timeout", "the file took too long to read")
		}
	}
	// Trailing blank cells (Excel formatting, trailing delimiters) are not columns.
	n := len(cells)
	for n > 0 && strings.TrimSpace(cells[n-1]) == "" {
		n--
	}
	if n == 0 {
		return nil // blank row
	}
	cells = cells[:n]
	if n > b.lim.MaxColumns {
		return tooLarge("too_many_columns", "row %d has %d columns; at most %d are supported", line, n, b.lim.MaxColumns)
	}
	for i, c := range cells {
		if utf8.RuneCountInString(c) > b.lim.MaxCellRunes {
			return tooLarge("cell_too_long", "row %d, column %s is longer than %d characters", line, ColumnName(i), b.lim.MaxCellRunes)
		}
	}
	if !b.header {
		b.header = true
		b.table.Headers = trimAll(cells)
		b.maxCols = n
		return nil
	}
	if len(b.table.Rows) >= b.lim.MaxRows {
		return tooLarge("too_many_rows", "the sheet has more than %d data rows; split it into smaller files", b.lim.MaxRows)
	}
	b.maxCols = max(b.maxCols, n)
	b.table.Rows = append(b.table.Rows, Row{Line: line, Cells: cells})
	return nil
}

// finish pads the header and every row to the same width.
func (b *builder) finish() (*Table, error) {
	if !b.header {
		return nil, fail("empty_file", "the sheet has no header row")
	}
	t := b.table
	for len(t.Headers) < b.maxCols {
		t.Headers = append(t.Headers, "")
	}
	for i := range t.Rows {
		cells := t.Rows[i].Cells
		if len(cells) < b.maxCols {
			cells = append(cells, make([]string, b.maxCols-len(cells))...)
		}
		t.Rows[i].Cells = cells
	}
	return t, nil
}

func trimAll(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

// ---------------------------------------------------------------------------
// CSV
// ---------------------------------------------------------------------------

func parseCSV(ctx context.Context, data []byte, tsv bool, lim Limits) (*Table, error) {
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		return nil, fail("invalid_encoding", "UTF-16 files are not supported; save the file as CSV UTF-8")
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	if !utf8.Valid(data) {
		return nil, fail("invalid_encoding", "the file is not UTF-8 encoded; save it as CSV UTF-8 (Excel: File > Save As > CSV UTF-8)")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, fail("malformed_file", "the file contains binary data")
	}
	delim := detectDelimiter(data, tsv)
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = delim
	r.FieldsPerRecord = -1 // ragged rows are normal in exported sheets
	b := &builder{ctx: ctx, lim: lim, table: &Table{Format: FormatCSV, Delimiter: string(delim)}}
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, fail("malformed_file", "CSV syntax error on line %d: %v", pe.Line, pe.Err)
			}
			return nil, fail("malformed_file", "the CSV file could not be read")
		}
		line, _ := r.FieldPos(0)
		if err := b.add(line, rec); err != nil {
			return nil, err
		}
	}
	return b.finish()
}

// detectDelimiter picks the most frequent of tab, semicolon and comma on the
// first line, ignoring quoted text. Excel writes ";" in locales (az, ru, tr,
// de) whose decimal separator is ",".
func detectDelimiter(data []byte, tsv bool) rune {
	if tsv {
		return '\t'
	}
	counts := map[rune]int{}
	inQuote, seen := false, false
	for _, c := range string(data) {
		if c == '"' {
			inQuote = !inQuote
			continue
		}
		if inQuote {
			continue
		}
		if c == '\n' || c == '\r' {
			if len(counts) > 0 || seen {
				break
			}
			continue // leading blank lines
		}
		if !unicode.IsSpace(c) {
			seen = true
		}
		if c == ',' || c == ';' || c == '\t' {
			counts[c]++
		}
	}
	best, n := ',', 0
	for _, c := range []rune{',', ';', '\t'} { // fixed order: ties go to ","
		if counts[c] > n {
			best, n = c, counts[c]
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// XLSX
// ---------------------------------------------------------------------------

func parseXLSX(ctx context.Context, data []byte, sheet string, lim Limits) (*Table, error) {
	// Check the archive before excelize inflates it: archive/zip refuses
	// entries that inflate beyond their declared size, so summing the declared
	// sizes bounds the memory excelize can use (zip bombs).
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fail("malformed_file", "the file is not a valid .xlsx workbook")
	}
	if len(zr.File) > lim.MaxZipEntries {
		return nil, tooLarge("file_too_large", "the workbook has too many parts")
	}
	maxBytes := uint64(max(lim.MaxUnzippedBytes, 0))
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
		if total > maxBytes || total < f.UncompressedSize64 { // second check: overflow
			return nil, tooLarge("file_too_large", "the workbook is larger than %d MiB uncompressed", lim.MaxUnzippedBytes>>20)
		}
	}

	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		RawCellValue:   true, // stored values, not display formatting; formulas are never evaluated
		UnzipSizeLimit: lim.MaxUnzippedBytes,
		// Equal to the total limit: everything stays in memory, nothing is
		// spilled to temporary files.
		UnzipXMLSizeLimit: lim.MaxUnzippedBytes,
	})
	if err != nil {
		return nil, fail("malformed_file", "the file is not a valid .xlsx workbook")
	}
	defer func() { _ = f.Close() }()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fail("empty_file", "the workbook has no sheets")
	}
	if sheet == "" {
		sheet = sheets[0]
		for _, s := range sheets {
			if visible, err := f.GetSheetVisible(s); err == nil && visible {
				sheet = s
				break
			}
		}
	} else if !slices.Contains(sheets, sheet) {
		return nil, fail("sheet_not_found", "the workbook has no sheet named %q", sheet)
	}
	props, err := f.GetWorkbookProps()
	if err != nil {
		return nil, fail("malformed_file", "the workbook properties could not be read")
	}

	t := &Table{Format: FormatXLSX, Sheets: sheets, Sheet: sheet, Date1904: props.Date1904 != nil && *props.Date1904}
	rows, err := f.Rows(sheet)
	if err != nil {
		return nil, fail("malformed_file", "the sheet could not be read")
	}
	defer func() { _ = rows.Close() }()
	b := &builder{ctx: ctx, lim: lim, table: t}
	line := 0
	for rows.Next() {
		line++
		cells, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fail("malformed_file", "row %d could not be read", line)
		}
		if err := b.add(line, cells); err != nil {
			return nil, err
		}
	}
	if err := rows.Error(); err != nil {
		return nil, fail("malformed_file", "the sheet could not be read")
	}
	return b.finish()
}

// ColumnName returns the spreadsheet letter of a 0-based column index (0 → A, 26 → AA).
func ColumnName(i int) string {
	name := ""
	for i++; i > 0; i = (i - 1) / 26 {
		name = string(rune('A'+(i-1)%26)) + name
	}
	return name
}
