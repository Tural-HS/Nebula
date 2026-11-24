// Package tableexport writes a table (header + typed rows) as CSV or Excel
// (.xlsx) for spreadsheet users (ADR-0040).
//
// Both writers stream: rows go out (CSV) or to excelize's stream writer
// (xlsx, which spills to a temporary file past 16 MiB) one at a time, so an
// export never holds its rows in memory.
//
// Text cells are neutralized against formula injection (OWASP "CSV
// Injection"): a text value that a spreadsheet could read as a formula gets
// a leading apostrophe. Number, date and boolean cells are typed, so a real
// negative amount stays a number.
package tableexport

import (
	"encoding/csv"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Kind is the type of a column's cells.
type Kind int

// Cell kinds.
const (
	Text     Kind = iota
	Number        // float64
	Date          // calendar date, no time of day
	DateTime      // wall-clock time (already in the reader's time zone)
	Bool
)

// Column is one column of the export.
type Column struct {
	Header string
	Kind   Kind
	// Scale is the number of decimals of a Number column, when known
	// (formula fields, ADR-0030); nil writes the value as is.
	Scale *int
}

// Cell is one value. A zero Cell is empty. Its Kind may differ from the
// column's (a formula error is Text in a Number column).
type Cell struct {
	Kind  Kind
	Valid bool
	Text  string
	Num   float64
	Time  time.Time
	Bool  bool
}

// TextCell is a text cell; an empty string is an empty cell.
func TextCell(s string) Cell {
	if s == "" {
		return Cell{}
	}
	return Cell{Kind: Text, Valid: true, Text: s}
}

// NumberCell is a numeric cell.
func NumberCell(v float64) Cell { return Cell{Kind: Number, Valid: true, Num: v} }

// DateCell is a date cell; only the calendar date of t is used.
func DateCell(t time.Time) Cell { return Cell{Kind: Date, Valid: true, Time: t} }

// DateTimeCell is a date-and-time cell, written as t's wall clock.
func DateTimeCell(t time.Time) Cell { return Cell{Kind: DateTime, Valid: true, Time: t} }

// BoolCell is a yes/no cell, written with the writer's Labels.
func BoolCell(b bool) Cell { return Cell{Kind: Bool, Valid: true, Bool: b} }

// Writer writes one table: Header once, then rows, then Close.
type Writer interface {
	Header(cols []Column) error
	Row(cells []Cell) error
	// Close finishes the file. For xlsx it writes the whole file to the
	// destination; for CSV everything has been written already.
	Close() error
}

// ErrTooLarge means the export passed the writer's byte budget.
var ErrTooLarge = errors.New("export is too large for this format")

// Labels are the words a writer needs in the reader's language.
type Labels struct{ Yes, No string }

// Neutralize makes a text value safe to open in a spreadsheet. A value
// starting with = + - @ (or their full-width forms), a tab or a carriage
// return, also after leading spaces or line breaks, gets a leading
// apostrophe so it is read as text, never as a formula or DDE call. Plain
// numbers ("-12.50") are left alone: they evaluate to themselves.
func Neutralize(s string) string {
	if s == "" {
		return s
	}
	if s[0] == '\t' || s[0] == '\r' {
		return "'" + s
	}
	t := strings.TrimLeft(s, " \n")
	r, _ := utf8.DecodeRuneInString(t)
	switch r {
	case '=', '+', '-', '@', '＝', '＋', '－', '＠', '\t', '\r':
		if plainNumber(t) {
			return s
		}
		return "'" + s
	}
	return s
}

// plainNumber: an optional sign, digits, and at most one decimal mark.
func plainNumber(s string) bool {
	if len(s) < 2 || (s[0] != '-' && s[0] != '+') {
		return false
	}
	digits, mark := 0, false
	for _, c := range s[1:] {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case (c == '.' || c == ',') && !mark && digits > 0:
			mark = true
		default:
			return false
		}
	}
	return digits > 0 && s[len(s)-1] != '.' && s[len(s)-1] != ','
}

// ---------------------------------------------------------------------------
// CSV
// ---------------------------------------------------------------------------

// CSVOptions configure NewCSV. The delimiter and the decimal mark go
// together, as in Excel's regional settings: ',' with '.', ';' with ','.
type CSVOptions struct {
	Semicolon bool
	Labels    Labels
}

type csvWriter struct {
	w    *csv.Writer
	opts CSVOptions
	cols []Column
	rec  []string
}

// NewCSV writes UTF-8 with a byte order mark (so Excel reads non-ASCII
// text correctly) and CRLF line ends (RFC 4180).
func NewCSV(dst io.Writer, opts CSVOptions) (Writer, error) {
	if _, err := io.WriteString(dst, "\xef\xbb\xbf"); err != nil {
		return nil, err
	}
	w := csv.NewWriter(dst)
	w.UseCRLF = true
	if opts.Semicolon {
		w.Comma = ';'
	}
	return &csvWriter{w: w, opts: opts}, nil
}

func (c *csvWriter) Header(cols []Column) error {
	c.cols = cols
	c.rec = make([]string, len(cols))
	for i, col := range cols {
		c.rec[i] = Neutralize(col.Header)
	}
	return c.w.Write(c.rec)
}

func (c *csvWriter) Row(cells []Cell) error {
	for i := range c.rec {
		c.rec[i] = ""
		if i < len(cells) {
			c.rec[i] = c.format(cells[i], c.cols[i])
		}
	}
	if err := c.w.Write(c.rec); err != nil {
		return err
	}
	// Flush keeps memory flat: csv.Writer buffers 4 KiB at most.
	c.w.Flush()
	return c.w.Error()
}

func (c *csvWriter) format(v Cell, col Column) string {
	if !v.Valid {
		return ""
	}
	switch v.Kind {
	case Number:
		return FormatNumber(v.Num, col.Scale, c.opts.Semicolon)
	case Date:
		return v.Time.Format(time.DateOnly)
	case DateTime:
		return v.Time.Format(time.DateTime)
	case Bool:
		if v.Bool {
			return c.opts.Labels.Yes
		}
		return c.opts.Labels.No
	default:
		return Neutralize(v.Text)
	}
}

func (c *csvWriter) Close() error {
	c.w.Flush()
	return c.w.Error()
}

// FormatNumber writes a number without digit grouping (spreadsheets parse
// it back as a number) and with ',' as the decimal mark when commaDecimal.
func FormatNumber(v float64, scale *int, commaDecimal bool) string {
	prec := -1
	if scale != nil {
		prec = *scale
	}
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if s == "-0" || strings.Trim(s, "-0.") == "" && strings.HasPrefix(s, "-") {
		s = s[1:] // no "-0.00"
	}
	if commaDecimal {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}
