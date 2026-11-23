# CSV Schema in Depth (**DEPRECATED**)

> Note: this version of `csv` schema has been deprecated and superseded by
> [`csv2`](./csv2_in_depth.md) schema. Its functionality and support will continue but (incredibly
> easy) [migration to `csv2`](./csv2_in_depth.md#migration-from-csv-schemas) is strongly recommended.
>

CSV (comma separated values) schema covers any delimiter (so not just comma) based flat file format. A
complete `"omni.2.1"` CSV schema has 3 parts: `parser_settings`, `file_declaration`, and
`transform_declarations`. We've covered `parser_settings` in [Getting Started](./gettingstarted.md);
we've covered `transform_declarations` in depth in [All About Transforms](./transforms.md); we've covered
some parts of CSV `file_declarations` in [Getting Started](./gettingstarted.md), we'll go into more details
about it here.

## CSV `file_declaration`

Full `file_declaration` schema looks as follows:
```
"file_declaration": {
    "delimiter": "<delimiter>"                  <= required
    "replace_double_quotes": true/false,        <= optional
    "header_row_index": integer >= 1,           <= optional
    "data_row_index": integer >= 1,             <= required
    "columns": [                                <= required, must not be empty array.
        {
            "name": "<column name>",            <= required
            "alias": "<alias name>"             <= optional
            "header_aliases": [                 <= optional
                "<historical header name>",
                ...
            ]
        },
        ...
    ]
}
```

- `delimiter`: self-explanatory.
    - Note 1: the delimiter must be of a single character.
    - Note 2: the delimiter doesn't have to be limited to ASCII character(s), utf8 is supported.

- `replace_double_quotes`: omniparser will replace any occurrences of double quotes `"` with single
quotes `'`.

    While [CSV RFC](https://tools.ietf.org/html/rfc4180) clearly specifies the rules of using double
    quotes and how to escape them, (un)surprisingly many CSV data providers have bad implementations
    and unescaped double quotes are quite frequently seen and breaking CSV parsing. The worst offender
    is this:

    ```
    COLUMN_1|COLUMN_2|COLUMN_3
    ...
    data 1|"data 2 has a leading double quote" then some|data 3
    ...
    ```

    Unfortunately Golang's [CSV parser](https://golang.org/pkg/encoding/csv/#Reader) cannot (nor should
    it) deal with this situation (even if you set `LazyQuotes` to `true`): it would start gobbling
    everything after the leading double quote in `COLUMN_2` as string literals, even passing the delimiter
    `|`. Usually it would consume **many many** lines until it finally hits another leading quote by luck.

    Given asking data providers/partners to fix double quote escaping properly according to CSV RFC is
    usually mission impossible from our experience, we've added `replace_double_quotes` flag to do a
    (frankly quite harsh) double quote to single quote replacement - yes, the content is altered, but at
    least the parsing and transform will succeed and minor differences result is the least evil we can do.
    **Use it only as a last resort**.

- `header_row_index`: line number (1-based) where the column header declaration sits in the input.

    If specified, omniparser will read the header row from the given line and **bind** every
    declared column to the actual input column(s) whose header text matches the declared
    column's `name` or one of its `header_aliases`, regardless of the order in which the
    input columns appear. This means input files whose columns are reordered (e.g. weekly
    feeds whose column order isn't stable) are read correctly, and each declared column is
    still labeled with its canonical `name`/`alias` in the IDR, so transforms see identical
    keys regardless of the input's column order or historical header naming.

    Binding rules:
    - A declared column that matches nothing in the actual header row fails the ingestion
      (fatal `ErrInvalidHeader`).
    - Actual input columns that match no declared column are ignored.
    - If multiple actual columns match the same declared column (e.g. both the current
      header and a historical alias from `header_aliases` are present), they are all bound
      to that column, and every data record must carry identical values in all of them:
      if a record has conflicting values under two headers that point to the same declared
      column, the entire ingestion fails (fatal `ErrColumnValueConflict`).

    All header verification failures are considered fatal, i.e. the entire ingestion and
    transform operation against the input will fail.

    If not specified, no header verification or binding is done and omniparser will assume
    the column names and order declared in the `columns` section for the input data.

- `data_row_index`: line number (1-based) where the first actual data line starts in the input. Required.

- `columns.name`: the name of a column.
    - Note 1: it must match the corresponding column header value if `header_row_index` is specified
      (unless one of the column's `header_aliases` matches instead).
    - Note 2: if name contains white space, then `alias` use is advised (to make XPath query possible).

- `columns.alias`: the alias of a column. Optional.

    If a column's name contains space, while it's completely legitimate in CSV, it would make the XPath
    based transform hard/impossible later. In situations like this, we strongly advise schema writer to
    use `alias` to assign an alias to the column that is XPath friendly, such as containing no spaces.

- `columns.header_aliases`: additional, usually historical/legacy, header names under which this
  column may appear in the actual input header row. Optional.

    Data providers sometimes rename their CSV column headers over time (e.g. a procurement
    feed that used to call a column `PO Number` now calls it `po_number`). Listing the old
    names in `header_aliases` lets one schema read both the current and the historical
    layouts: when `header_row_index` is specified, the column is bound to whichever actual
    header matches its `name` or one of the `header_aliases`. The column is always labeled
    with its canonical `name`/`alias` in the IDR, so transforms are unaffected by which
    header variant a given input file uses.

    Note a header name (a `name` or a `header_aliases` entry) must not be accepted by two
    different declared columns - such a schema is rejected as ambiguous at validation time.

## CSV Specific IDR Structure

See [here](./idr.md#csv-aka-delimited) for more details.
