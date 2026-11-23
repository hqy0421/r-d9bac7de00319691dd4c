package csv

import (
	"strings"

	"github.com/jf-tech/go-corelib/strs"
)

// Column is a CSV column.
type Column struct {
	Name string `json:"name"`
	// If the CSV column 'name' contains characters (such as space, or special letters) that are
	// not suitable for *idr.Node construction and xpath query, this gives schema writer an
	// alternate way to name/label the column. Optional.
	Alias *string `json:"alias"`
	// HeaderAliases lists additional, usually historical/legacy, header names under which this
	// column may appear in the actual input header row. When 'header_row_index' is specified,
	// the reader binds this column to the actual input column(s) whose header matches either
	// 'name' or one of the 'header_aliases', regardless of their positions. Optional.
	HeaderAliases []string `json:"header_aliases"`
}

func (c Column) name() string {
	return strs.StrPtrOrElse(c.Alias, c.Name)
}

// acceptedHeaders returns the deduped set of header names (the canonical 'name' plus all
// 'header_aliases') under which this column can appear in an actual input header row.
func (c Column) acceptedHeaders() []string {
	seen := map[string]bool{}
	var headers []string
	for _, h := range append([]string{c.Name}, c.HeaderAliases...) {
		h = strings.TrimSpace(h)
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		headers = append(headers, h)
	}
	return headers
}

// FileDecl describes CSV specific schema settings for omniparser reader.
type FileDecl struct {
	Delimiter           string   `json:"delimiter"`
	ReplaceDoubleQuotes bool     `json:"replace_double_quotes"`
	HeaderRowIndex      *int     `json:"header_row_index"`
	DataRowIndex        int      `json:"data_row_index"`
	Columns             []Column `json:"columns"`
}
