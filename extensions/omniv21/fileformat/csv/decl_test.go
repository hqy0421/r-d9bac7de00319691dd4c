package csv

import (
	"testing"

	"github.com/jf-tech/go-corelib/strs"
	"github.com/stretchr/testify/assert"
)

func TestColumnName(t *testing.T) {
	assert.Equal(t, "name", Column{Name: "name"}.name())
	assert.Equal(t, "alias", Column{Name: "name", Alias: strs.StrPtr("alias")}.name())
}

func TestColumnAcceptedHeaders(t *testing.T) {
	assert.Equal(t,
		[]string{"name"},
		Column{Name: "name"}.acceptedHeaders())
	assert.Equal(t,
		[]string{"name", "old name", "older name"},
		Column{Name: "name", HeaderAliases: []string{"old name", "older name"}}.acceptedHeaders())
	// duplicates (after trimming) and empty aliases are removed; canonical name always first.
	assert.Equal(t,
		[]string{"name", "old name"},
		Column{Name: " name ", HeaderAliases: []string{"old name", "name", " old name ", ""}}.acceptedHeaders())
}
