package csv

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradleyjkemp/cupaloy"
	"github.com/jf-tech/go-corelib/jsons"
	"github.com/stretchr/testify/assert"

	"github.com/jf-tech/omniparser"
	csvfileformat "github.com/jf-tech/omniparser/extensions/omniv21/fileformat/csv"
	"github.com/jf-tech/omniparser/extensions/omniv21/samples"
	"github.com/jf-tech/omniparser/transformctx"
)

func Test1_Weather_Data_CSV(t *testing.T) {
	cupaloy.SnapshotT(t, jsons.BPJ(samples.SampleTestCommon(
		t, "./1_weather_data_csv.schema.json", "./1_weather_data_csv.input.csv")))
}

// transformAll runs one input file through the schema and returns all transformed records.
func transformAll(t *testing.T, schema omniparser.Schema, inputFile string) ([]interface{}, error) {
	inputReader, err := os.Open(inputFile)
	assert.NoError(t, err)
	defer inputReader.Close()
	tr, err := schema.NewTransform(filepath.Base(inputFile), inputReader, &transformctx.Ctx{})
	assert.NoError(t, err)
	var records []interface{}
	for {
		recordBytes, err := tr.Read()
		if err == io.EOF {
			return records, nil
		}
		if err != nil {
			return records, err
		}
		var transformed interface{}

		assert.NoError(t, json.Unmarshal(recordBytes, &transformed))
		records = append(records, transformed)
	}
}

// Test2_Procurement_CSV proves the header-row binding behavior required for weekly
// procurement CSV feeds: the same schema keeps producing identical output keys and types
// even though weekly inputs reorder columns and use historical header aliases; and a
// batch containing two aliased columns with conflicting values fails clearly.
func Test2_Procurement_CSV(t *testing.T) {
	schemaReader, err := os.Open("./2_procurement_csv.schema.json")
	assert.NoError(t, err)
	defer schemaReader.Close()
	schema, err := omniparser.NewSchema("2_procurement_csv.schema.json", schemaReader)
	assert.NoError(t, err)

	// Week 1: canonical headers in declared order.
	week1, err := transformAll(t, schema, "./2_procurement_csv.week1.input.csv")
	assert.NoError(t, err)
	// Week 2: historical header aliases, shuffled column order, plus an extra
	// unmodeled column. Transformed records must be identical to week 1's.
	week2, err := transformAll(t, schema, "./2_procurement_csv.week2.input.csv")
	assert.NoError(t, err)
	assert.Equal(t, week1, week2)
	cupaloy.SnapshotT(t, jsons.BPJ(jsons.BMM(week1)))

	// Week 3: both the canonical 'po_number' header and its historical alias 'PO#'
	// are present, and one row carries conflicting values under them. The whole batch
	// must fail with a clear, non-continuable error.
	_, err = transformAll(t, schema, "./2_procurement_csv.week3_conflicting.input.csv")
	assert.Error(t, err)
	assert.True(t, csvfileformat.IsErrColumnValueConflict(err))
	assert.Equal(t,
		"input '2_procurement_csv.week3_conflicting.input.csv' line 3: column[1] 'po_number' "+
			"and column[5] 'PO#' both map to declared column 'po_number' but have conflicting "+
			"values 'A-102' and 'A-103'",
		err.Error())
}

var benchSchemaFile = "./1_weather_data_csv.schema.json"
var benchInputFile = "./1_weather_data_csv.input.csv"
var benchSchema omniparser.Schema
var benchInput []byte

func init() {
	schema, err := ioutil.ReadFile(benchSchemaFile)
	if err != nil {
		panic(err)
	}
	benchSchema, err = omniparser.NewSchema("bench", bytes.NewReader(schema))
	if err != nil {
		panic(err)
	}
	benchInput, err = ioutil.ReadFile(benchInputFile)
	if err != nil {
		panic(err)
	}
}

// Benchmark1_Weather_Data_CSV-8   	  167850	    216683 ns/op	   75836 B/op	    1267 allocs/op
func Benchmark1_Weather_Data_CSV(b *testing.B) {
	for i := 0; i < b.N; i++ {
		transform, err := benchSchema.NewTransform(
			"bench", bytes.NewReader(benchInput), &transformctx.Ctx{})
		if err != nil {
			b.FailNow()
		}
		for {
			_, err = transform.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				b.FailNow()
			}
		}
	}
}
