package csv

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/antchfx/xpath"
	"github.com/jf-tech/go-corelib/caches"
	"github.com/jf-tech/go-corelib/ios"

	"github.com/jf-tech/omniparser/idr"
)

// ErrInvalidHeader indicates the header of the CSV input is corrupted, mismatched, or simply
// unreadable. This is a fatal, non-continuable error.
type ErrInvalidHeader string

func (e ErrInvalidHeader) Error() string { return string(e) }

// IsErrInvalidHeader checks if the `err` is of ErrInvalidHeader type.
func IsErrInvalidHeader(err error) bool {
	switch err.(type) {
	case ErrInvalidHeader:
		return true
	default:
		return false
	}
}

// ErrColumnValueConflict indicates a data record carries conflicting values in multiple actual
// input columns that are all bound (via header row matching) to the same declared column. This
// is a fatal, non-continuable error: the entire ingestion batch against the input fails.
type ErrColumnValueConflict string

func (e ErrColumnValueConflict) Error() string { return string(e) }

// IsErrColumnValueConflict checks if the `err` is of ErrColumnValueConflict type.
func IsErrColumnValueConflict(err error) bool {
	switch err.(type) {
	case ErrColumnValueConflict:
		return true
	default:
		return false
	}
}

type reader struct {
	inputName     string
	decl          *FileDecl
	xpath         *xpath.Expr
	r             *ios.LineNumReportingCsvReader
	headerChecked bool
	// header is the actual input header row, nil if 'header_row_index' isn't declared.
	header []string
	// colBindings[i] contains the actual input column index/indices (0-based) the i-th
	// declared column is bound to. Usually a single index; multiple indices when several
	// actual columns (e.g. a current and a historical header) map to the same declared
	// column, in which case their per-record values must agree.
	colBindings [][]int
}

func (r *reader) Read() (*idr.Node, error) {
	if !r.headerChecked {
		err := r.checkHeader()
		r.headerChecked = true
		if err != nil {
			return nil, err
		}
	}
read:
	record, err := r.r.Read()
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, r.FmtErr("failed to fetch record: %s", err.Error())
	}
	n, err := r.recordToNode(record)
	if err != nil {
		return nil, err
	}
	if r.xpath != nil && !idr.MatchAny(n, r.xpath) {
		goto read
	}
	return n, nil
}

func (r *reader) checkHeader() error {
	if r.decl.HeaderRowIndex == nil {
		// Without an actual header row to read and bind against, declared columns are
		// bound positionally, in their declared order.
		r.colBindings = make([][]int, len(r.decl.Columns))
		for i := range r.colBindings {
			r.colBindings[i] = []int{i}
		}
	} else {
		if err := r.jumpTo(*r.decl.HeaderRowIndex - 1); err != nil {
			return ErrInvalidHeader(r.fmtErrStr("unable to read header: %s", err.Error()))
		}
		header, err := r.r.Read()
		if err != nil {
			return ErrInvalidHeader(r.fmtErrStr("unable to read header: %s", err.Error()))
		}
		// The csv reader reuses its record slice across reads, so the header row must be
		// copied for it to survive subsequent data record reads.
		r.header = append([]string(nil), header...)
		if err := r.bindColumns(); err != nil {
			return err
		}
	}
	return r.jumpTo(r.decl.DataRowIndex - 1)
}

// bindColumns binds every declared column to the actual input column position(s) whose header
// text matches the declared column's canonical name or one of its historical header aliases,
// regardless of the order in which the input columns appear. A declared column found nowhere
// in the header row is a fatal error. Multiple actual columns matching the same declared
// column (e.g. a current and a historical header both present) are allowed: their per-record
// values must agree, which is verified in recordToNode.
func (r *reader) bindColumns() error {
	r.colBindings = make([][]int, len(r.decl.Columns))
	for i, column := range r.decl.Columns {
		accepted := map[string]bool{}
		for _, h := range column.acceptedHeaders() {
			accepted[h] = true
		}
		for index, h := range r.header {
			if accepted[strings.TrimSpace(h)] {
				r.colBindings[i] = append(r.colBindings[i], index)
			}
		}
		if len(r.colBindings[i]) == 0 {
			return ErrInvalidHeader(r.fmtErrStr(
				"header row does not contain declared column '%s' (accepted headers: '%s')",
				column.Name, strings.Join(column.acceptedHeaders(), "', '")))
		}
	}
	return nil
}

// the only possible error this jumpTo returns is io.EOF. if there is any reading error, we'll ignore
// because we really don't care about what's corrupted in a line. Now it's possible, but very very
// rarely, that the input reader's underlying media fails to read due memory/disk/IO issue. Since we
// can't reliably tease apart those failures from a simple line corruption failure, we'll choose to
// ignore them equally. And those underlying media failures most likely will repeat and cause subsequent
// read to fail, and then the reader will fail out entirely.
func (r *reader) jumpTo(rowIndex int) error {
	for r.r.LineNum() < rowIndex {
		_, err := r.r.Read()
		if err == io.EOF {
			return io.EOF
		}
	}
	return nil
}

func (r *reader) recordToNode(record []string) (*idr.Node, error) {
	root := idr.CreateNode(idr.DocumentNode, "")
	// Nodes are always created in declared column order with their declared (canonical)
	// labels, so downstream transforms see identical keys regardless of how the actual
	// input columns are ordered or historically named.
	for i, bound := range r.colBindings {
		// A bound index can point past the end of a short record: such a column has no
		// value in this record and its node is skipped (mirroring the previous behavior
		// of only emitting nodes for columns actually present in the record).
		first := -1
		for _, index := range bound {
			if index >= len(record) {
				continue
			}
			if first == -1 {
				first = index
				continue
			}
			if record[first] != record[index] {
				err := ErrColumnValueConflict(r.fmtErrStr(
					"column[%d] '%s' and column[%d] '%s' both map to declared column '%s' but have conflicting values '%s' and '%s'",
					first+1, strings.TrimSpace(r.header[first]),
					index+1, strings.TrimSpace(r.header[index]),
					r.decl.Columns[i].Name, record[first], record[index]))
				idr.RemoveAndReleaseTree(root)
				return nil, err
			}
		}
		if first == -1 {
			continue
		}
		col := idr.CreateNode(idr.ElementNode, r.decl.Columns[i].name())
		idr.AddChild(root, col)
		data := idr.CreateNode(idr.TextNode, record[first])
		idr.AddChild(col, data)
	}
	return root, nil
}

func (r *reader) Release(n *idr.Node) {
	if n != nil {
		idr.RemoveAndReleaseTree(n)
	}
}

func (r *reader) IsContinuableError(err error) bool {
	return !IsErrInvalidHeader(err) && !IsErrColumnValueConflict(err) && err != io.EOF
}

func (r *reader) FmtErr(format string, args ...interface{}) error {
	return errors.New(r.fmtErrStr(format, args...))
}

func (r *reader) fmtErrStr(format string, args ...interface{}) string {
	return fmt.Sprintf("input '%s' line %d: %s", r.inputName, r.r.LineNum(), fmt.Sprintf(format, args...))
}

// NewReader creates an FormatReader for CSV file format.
func NewReader(inputName string, r io.Reader, decl *FileDecl, xpathStr string) (*reader, error) {
	var expr *xpath.Expr
	var err error
	xpathStr = strings.TrimSpace(xpathStr)
	if xpathStr != "" && xpathStr != "." {
		expr, err = caches.GetXPathExpr(xpathStr)
		if err != nil {
			return nil, fmt.Errorf("invalid xpath '%s', err: %s", xpathStr, err.Error())
		}
	}
	if decl.ReplaceDoubleQuotes {
		r = ios.NewBytesReplacingReader(r, []byte(`"`), []byte(`'`))
	}
	csv := ios.NewLineNumReportingCsvReader(r)
	delim := []rune(decl.Delimiter)
	csv.Comma = delim[0]
	csv.FieldsPerRecord = -1
	csv.ReuseRecord = true
	return &reader{
		inputName:     inputName,
		decl:          decl,
		r:             csv,
		headerChecked: false,
		xpath:         expr,
	}, nil
}
