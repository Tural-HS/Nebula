package tableexport

import (
	"io"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// XLSXOptions configure NewXLSX.
type XLSXOptions struct {
	// Sheet is the worksheet name (Excel: at most 31 characters, none of
	// []:*?/\); invalid characters are replaced.
	Sheet  string
	Labels Labels
	// MaxBytes bounds the sheet XML (an estimate). The stream writer keeps
	// 16 MiB in memory and spills the rest to a temporary file, so this is
	// what an export may cost in temporary disk (tmpfs in production).
	MaxBytes int64
}

type xlsxWriter struct {
	dst   io.Writer
	f     *excelize.File
	sw    *excelize.StreamWriter
	opts  XLSXOptions
	cols  []Column
	row   int
	bytes int64
	vals  []any
	// styles
	header, date, dateTime int
	number                 map[int]int // scale -> style; -1: no fixed scale
}

// NewXLSX streams one worksheet; Close writes the workbook to dst.
func NewXLSX(dst io.Writer, opts XLSXOptions) (Writer, error) {
	f := excelize.NewFile()
	name := SheetName(opts.Sheet)
	if err := f.SetSheetName("Sheet1", name); err != nil {
		_ = f.Close()
		return nil, err
	}
	x := &xlsxWriter{dst: dst, f: f, opts: opts, number: map[int]int{}}
	var err error
	if x.header, err = f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EDEDED"}}}); err != nil {
		return nil, x.fail(err)
	}
	// Built-in formats 14 (short date) and 22 (date + time) follow the
	// reader's regional settings in Excel.
	if x.date, err = f.NewStyle(&excelize.Style{NumFmt: 14}); err != nil {
		return nil, x.fail(err)
	}
	if x.dateTime, err = f.NewStyle(&excelize.Style{NumFmt: 22}); err != nil {
		return nil, x.fail(err)
	}
	if x.sw, err = f.NewStreamWriter(name); err != nil {
		return nil, x.fail(err)
	}
	return x, nil
}

func (x *xlsxWriter) fail(err error) error {
	_ = x.f.Close()
	return err
}

// SheetName makes s a valid worksheet name.
func SheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) || r < ' ' {
			return '_'
		}
		return r
	}, strings.Trim(s, "' "))
	for utf8.RuneCountInString(s) > 31 {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	if s == "" {
		return "Sheet1"
	}
	return s
}

// numberStyle: "#,##0.00" for a known scale (digit grouping and decimal
// mark follow the reader's Excel settings), General otherwise.
func (x *xlsxWriter) numberStyle(scale *int) (int, error) {
	if scale == nil {
		return 0, nil
	}
	if id, ok := x.number[*scale]; ok {
		return id, nil
	}
	f := "#,##0"
	if *scale > 0 {
		f += "." + strings.Repeat("0", min(*scale, 10))
	}
	id, err := x.f.NewStyle(&excelize.Style{CustomNumFmt: &f})
	if err != nil {
		return 0, err
	}
	x.number[*scale] = id
	return id, nil
}

func (x *xlsxWriter) Header(cols []Column) error {
	x.cols = cols
	x.vals = make([]any, len(cols))
	for i, c := range cols {
		w := float64(min(max(utf8.RuneCountInString(c.Header)+4, 12), 50))
		switch c.Kind {
		case DateTime:
			w = max(w, 18)
		case Text:
			if i == 1 { // the title
				w = 45
			}
		}
		if err := x.sw.SetColWidth(i+1, i+1, w); err != nil {
			return err
		}
	}
	// The header row stays in view while scrolling.
	if err := x.sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	for i, c := range cols {
		x.vals[i] = excelize.Cell{StyleID: x.header, Value: c.Header} // an inline string, like text cells
	}
	return x.write()
}

func (x *xlsxWriter) Row(cells []Cell) error {
	for i := range x.vals {
		x.vals[i] = nil
		if i >= len(cells) || !cells[i].Valid {
			continue
		}
		v := cells[i]
		x.bytes += 32 // <c r="AB12345" s="12" t="..."><v></v></c>
		switch v.Kind {
		case Number:
			s, err := x.numberStyle(x.cols[i].Scale)
			if err != nil {
				return err
			}
			x.vals[i] = excelize.Cell{StyleID: s, Value: v.Num}
			x.bytes += 16
		case Date:
			x.vals[i] = excelize.Cell{StyleID: x.date, Value: v.Time}
			x.bytes += 8
		case DateTime:
			x.vals[i] = excelize.Cell{StyleID: x.dateTime, Value: v.Time}
			x.bytes += 16
		case Bool:
			// Words, as in the table: a TRUE/FALSE cell would show in
			// Excel's language, not the export's.
			if v.Bool {
				x.vals[i] = x.opts.Labels.Yes
			} else {
				x.vals[i] = x.opts.Labels.No
			}
			x.bytes += 8
		default:
			// Written as-is: an inline string cell (t="inlineStr", no <f>)
			// is never evaluated, so "=…" or "-foo" stays the exact text
			// (ADR-0040). Neutralize is for CSV only, where every value is
			// re-parsed by the spreadsheet; here it would alter the data.
			x.vals[i] = v.Text
			x.bytes += int64(len(v.Text)) + 24 // inline string markup, escaping
		}
	}
	if x.opts.MaxBytes > 0 && x.bytes > x.opts.MaxBytes {
		return ErrTooLarge
	}
	return x.write()
}

func (x *xlsxWriter) write() error {
	x.row++
	cell, err := excelize.CoordinatesToCellName(1, x.row)
	if err != nil {
		return err
	}
	return x.sw.SetRow(cell, x.vals)
}

func (x *xlsxWriter) Close() error {
	defer func() { _ = x.f.Close() }() // removes the temporary files
	if err := x.sw.Flush(); err != nil {
		return err
	}
	return x.f.Write(x.dst)
}

// Abort releases a writer that will not be closed (an error mid-export).
func Abort(w Writer) {
	if x, ok := w.(*xlsxWriter); ok {
		_ = x.f.Close()
	}
}
