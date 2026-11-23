package csv

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jf-tech/go-corelib/jsons"
	"github.com/jf-tech/go-corelib/strs"
	"github.com/jf-tech/go-corelib/testlib"
	"github.com/stretchr/testify/assert"

	"github.com/jf-tech/omniparser/idr"
)

func TestIsErrInvalidHeader(t *testing.T) {
	assert.True(t, IsErrInvalidHeader(ErrInvalidHeader("test")))
	assert.Equal(t, "test", ErrInvalidHeader("test").Error())
	assert.False(t, IsErrInvalidHeader(errors.New("test")))
}

func TestIsErrColumnValueConflict(t *testing.T) {
	assert.True(t, IsErrColumnValueConflict(ErrColumnValueConflict("test")))
	assert.Equal(t, "test", ErrColumnValueConflict("test").Error())
	assert.False(t, IsErrColumnValueConflict(errors.New("test")))
}

func TestNewReader_InvalidXPath(t *testing.T) {
	r, err := NewReader("test", nil, nil, "[invalid")
	assert.Error(t, err)
	assert.Equal(t, `invalid xpath '[invalid', err: expression must evaluate to a node-set`, err.Error())
	assert.Nil(t, r)
}

func lf(s string) string {
	return s + "\n"
}

func TestReader(t *testing.T) {
	for _, test := range []struct {
		name     string
		decl     *FileDecl
		xpath    string
		input    io.Reader
		expected []interface{}
	}{
		{
			name: "header row; alias used; with xpath; variable data row column size; replace double quote",
			decl: &FileDecl{
				Delimiter:           "|",
				ReplaceDoubleQuotes: true,
				HeaderRowIndex:      testlib.IntPtr(2),
				DataRowIndex:        4,
				Columns: []Column{
					{Name: "a"},
					{Name: "b with space", Alias: strs.StrPtr("b_with_space")},
					{Name: "c"},
				},
			},
			xpath: ".[a != 'skip']",
			input: strings.NewReader(lf("line1") +
				lf("a|b with space|c") +
				lf("line3") +
				lf(`1|"2|3`) + // put an unescaped double quote here to see if our replacement works or not.
				lf("") + // auto skipped by csv reader.
				lf("skip") + // skipped by the xpath.
				lf("one|two")),
			expected: []interface{}{
				`{ "a": "1", "b_with_space": "'2", "c": "3" }`,
				`{ "a": "one", "b_with_space": "two" }`,
			},
		},
		{
			name: "columns reordered and undeclared columns ignored",
			decl: &FileDecl{
				Delimiter:      "|",
				HeaderRowIndex: testlib.IntPtr(1),
				DataRowIndex:   2,
				Columns: []Column{
					{Name: "a"},
					{Name: "b with space", Alias: strs.StrPtr("b_with_space")},
					{Name: "c"},
				},
			},
			input: strings.NewReader(lf("c|note|a|b with space") +
				lf("3|x|1|2") +
				lf("C|y|A|B")),
			expected: []interface{}{
				`{ "a": "1", "b_with_space": "2", "c": "3" }`,
				`{ "a": "A", "b_with_space": "B", "c": "C" }`,
			},
		},
		{
			name: "historical header aliases bound to declared columns",
			decl: &FileDecl{
				Delimiter:      ",",
				HeaderRowIndex: testlib.IntPtr(1),
				DataRowIndex:   2,
				Columns: []Column{
					{Name: "po_number", HeaderAliases: []string{"PO Number", "PO#"}},
					{Name: "quantity", HeaderAliases: []string{"Qty"}},
					{Name: "unit_price", HeaderAliases: []string{"Unit Price", "Price"}},
				},
			},
			input: strings.NewReader(lf("Qty,PO Number,Price") +
				lf("10,A-100,9.99") +
				lf("20,A-101,5.50")),
			expected: []interface{}{
				`{ "po_number": "A-100", "quantity": "10", "unit_price": "9.99" }`,
				`{ "po_number": "A-101", "quantity": "20", "unit_price": "5.50" }`,
			},
		},
		{
			name: "duplicate headers for same column with identical values",
			decl: &FileDecl{
				Delimiter:      ",",
				HeaderRowIndex: testlib.IntPtr(1),
				DataRowIndex:   2,
				Columns: []Column{
					{Name: "po_number", HeaderAliases: []string{"PO#"}},
					{Name: "quantity"},
				},
			},
			input: strings.NewReader(lf("quantity,PO#,po_number") +
				lf("10,A-100,A-100")),
			expected: []interface{}{
				`{ "po_number": "A-100", "quantity": "10" }`,
			},
		},
		{
			name: "duplicate headers for same column with conflicting values",
			decl: &FileDecl{
				Delimiter:      ",",
				HeaderRowIndex: testlib.IntPtr(1),
				DataRowIndex:   2,
				Columns: []Column{
					{Name: "po_number", HeaderAliases: []string{"PO#"}},
					{Name: "quantity"},
				},
			},
			input: strings.NewReader(lf("quantity,PO#,po_number") +
				lf("10,A-100,A-100") +
				lf("20,A-101,A-102")),
			expected: []interface{}{
				`{ "po_number": "A-100", "quantity": "10" }`,
				ErrColumnValueConflict("input 'test-input' line 3: column[2] 'PO#' and column[3] 'po_number' both map to declared column 'po_number' but have conflicting values 'A-101' and 'A-102'"),
			},
		},
		{
			name: "short record skips bound columns beyond record end",
			decl: &FileDecl{
				Delimiter:      ",",
				HeaderRowIndex: testlib.IntPtr(1),
				DataRowIndex:   2,
				Columns: []Column{
					{Name: "a"},
					{Name: "b"},
					{Name: "c"},
				},
			},
			input: strings.NewReader(lf("c,b,a") +
				lf("C1,B1,A1") +
				lf("C2,B2")),
			expected: []interface{}{
				`{ "a": "A1", "b": "B1", "c": "C1" }`,
				`{ "b": "B2", "c": "C2" }`,
			},
		},
		{
			name: "cannot jump to header row",
			decl: &FileDecl{
				Delimiter:      "|",
				HeaderRowIndex: testlib.IntPtr(3),
			},
			input: strings.NewReader(lf("line1")),
			expected: []interface{}{
				ErrInvalidHeader("input 'test-input' line 2: unable to read header: EOF"),
			},
		},
		{
			name: "cannot read to header row",
			decl: &FileDecl{
				Delimiter:      "|",
				HeaderRowIndex: testlib.IntPtr(2),
			},
			input: strings.NewReader(lf("line1")),
			expected: []interface{}{
				ErrInvalidHeader("input 'test-input' line 2: unable to read header: EOF"),
			},
		},
		{
			name: "declared column missing from header row",
			decl: &FileDecl{
				Delimiter:      "|",
				HeaderRowIndex: testlib.IntPtr(2),
				Columns: []Column{
					{Name: "a"},
					{Name: "b"},
				},
			},
			input: strings.NewReader(lf("line1") + lf("a")),
			expected: []interface{}{
				ErrInvalidHeader("input 'test-input' line 2: header row does not contain declared column 'b' (accepted headers: 'b')"),
			},
		},
		{
			name: "header column not matching the declared",
			decl: &FileDecl{
				Delimiter:      "|",
				HeaderRowIndex: testlib.IntPtr(2),
				Columns: []Column{
					{Name: "a"},
					{Name: "b"},
				},
			},
			input: strings.NewReader(lf("line1") + lf("a|B")),
			expected: []interface{}{
				ErrInvalidHeader("input 'test-input' line 2: header row does not contain declared column 'b' (accepted headers: 'b')"),
			},
		},
		{
			name: "header row but no data row",
			decl: &FileDecl{
				Delimiter:      ",",
				HeaderRowIndex: testlib.IntPtr(2),
				DataRowIndex:   5,
				Columns: []Column{
					{Name: "a"},
					{Name: "b"},
				},
			},
			input:    strings.NewReader(lf("line1") + lf("a,b") + lf("line3")),
			expected: nil,
		},
		{
			name: "unable to read data row",
			decl: &FileDecl{
				Delimiter:    ",",
				DataRowIndex: 1,
				Columns:      []Column{{Name: "a"}},
			},
			input: testlib.NewMockReadCloser("read failure", nil),
			expected: []interface{}{
				errors.New("input 'test-input' line 1: failed to fetch record: read failure"),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, err := NewReader("test-input", test.input, test.decl, test.xpath)
			assert.NoError(t, err)
			for {
				n, err := r.Read()
				if err == io.EOF {
					assert.Equal(t, 0, len(test.expected))
					assert.Nil(t, n)
					break
				}
				assert.True(t, len(test.expected) > 0)
				if expectedErr, ok := test.expected[0].(error); ok {
					assert.Error(t, err)
					assert.Equal(t, expectedErr, err)
					assert.Nil(t, n)
					assert.Equal(t, 1, len(test.expected)) // if there is an error, it will be the last one.
					break
				}
				expectedJSON, ok := test.expected[0].(string)
				assert.True(t, ok)
				assert.Equal(t, jsons.BPJ(expectedJSON), jsons.BPJ(idr.JSONify2(n)))
				r.Release(n)
				test.expected = test.expected[1:]
			}
		})
	}
}

func TestIsContinuableError(t *testing.T) {
	r := &reader{}
	assert.True(t, r.IsContinuableError(errors.New("some error")))
	assert.False(t, r.IsContinuableError(ErrInvalidHeader("invalid header")))
	assert.False(t, r.IsContinuableError(ErrColumnValueConflict("conflicting values")))
	assert.False(t, r.IsContinuableError(io.EOF))
}
