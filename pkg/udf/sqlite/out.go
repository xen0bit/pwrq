//go:build !(js && wasm)

package sqlite

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterOutSqlite registers out_sqlite: a pipeline of objects written into a
// table, which is PSSQLite's Invoke-SQLiteBulkCopy.
//
//	[get_childitem("."; {Recurse: true})] | out_sqlite("files.db"; "files")
//	[invoke_web_request($url) | .Content | json_parse] | out_sqlite("api.db"; "hits")
//	[$rows] | out_sqlite("app.db"; "users"; {Truncate: true})
//
// The rows come from the pipeline and only from the pipeline: the two operands
// are the database and the table. An array is a set of rows and a single object
// is one row, matching PowerShell, where every value is a pipeline of length
// one.
//
// It returns a single object describing the write - {RowCount, Created, ...} -
// rather than passing the rows on. A bulk insert's answer is how much was
// written; echoing ten thousand rows back into the terminal is not an answer.
//
// The table is created from the first row's shape unless it already exists, in
// which case the rows are checked against its columns and a key that is not a
// column is an error. Silently dropping it would lose data the caller believes
// they stored.
//
// PSTypeName is not written. It is how pwrq marks the kind of an object, not a
// property of the thing described, and a column of "System.IO.FileInfo"
// repeated a million times is not what anyone means to store.
//
// Values are stored as SQLite stores them: null, integers, reals, TEXT for
// strings, and BLOB for a jq string that is not valid UTF-8 - so bytes read
// with read_bytes survive the round trip. A nested object or array is stored as
// JSON text, which SQLite's own json_extract can read.
func RegisterOutSqlite() gojq.CompilerOption {
	return common.WithFunction("out_sqlite", 2, 3, func(v any, args []any) any {
		path, err := bindDatabase(args[0], "out_sqlite")
		if err != nil {
			return err
		}
		table, err := common.BindString(args[1], "table")
		if err != nil {
			return fmt.Errorf("out_sqlite: %v", err)
		}
		opts, err := outOptions(args[2:])
		if err != nil {
			return fmt.Errorf("out_sqlite: %v", err)
		}
		rows, err := bindRows(v)
		if err != nil {
			return fmt.Errorf("out_sqlite: %v", err)
		}

		db, err := openDB(path, false)
		if err != nil {
			return fmt.Errorf("out_sqlite: %v", err)
		}
		defer func() { _ = db.Close() }()

		written, created, err := writeRows(db, table, rows, opts)
		if err != nil {
			return fmt.Errorf("out_sqlite: %v", err)
		}
		return map[string]any{
			psobject.PSTypeNameKey: writeType,
			"Database":             path,
			psobject.PSPathKey:     path,
			"Table":                table,
			"RowCount":             written,
			"Created":              created,
		}
	})
}

type outOpts struct {
	create   bool
	truncate bool
}

func outOptions(args []any) (outOpts, error) {
	o := outOpts{create: true}
	if len(args) == 0 {
		return o, nil
	}
	m, ok := common.BindValue(args[0]).(map[string]any)
	if !ok {
		return o, fmt.Errorf("options must be an object, got %T", common.BindValue(args[0]))
	}
	for k, val := range m {
		b, isBool := val.(bool)
		switch strings.ToLower(k) {
		case "create":
			if !isBool {
				return o, fmt.Errorf("expected a boolean for Create")
			}
			o.create = b
		case "truncate":
			if !isBool {
				return o, fmt.Errorf("expected a boolean for Truncate")
			}
			o.truncate = b
		default:
			return o, fmt.Errorf("unknown option %q", k)
		}
	}
	return o, nil
}

// bindRows resolves the pipeline input to the rows to write.
func bindRows(v any) ([]map[string]any, error) {
	values := common.NormalizeToSlice(common.BindValue(v))
	rows := make([]map[string]any, 0, len(values))
	for i, value := range values {
		row, ok := common.BindValue(value).(map[string]any)
		if !ok {
			return nil, fmt.Errorf("row %d is %T, but a table row is an object", i, common.BindValue(value))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// writeRows creates the table if it is needed and inserts every row inside one
// transaction, so a failure half way through leaves the table as it was rather
// than half written.
func writeRows(db *sql.DB, table string, rows []map[string]any, opts outOpts) (int, bool, error) {
	existing, err := tableColumns(db, table)
	if err != nil {
		return 0, false, err
	}

	// Nothing to write is not a reason to create a table: an empty pipeline
	// says nothing about what the columns would have been.
	if len(rows) == 0 && existing == nil {
		return 0, false, nil
	}

	columns := existing
	created := false
	if existing == nil {
		if !opts.create {
			return 0, false, fmt.Errorf("no such table: %q (pass {Create: true} to create it)", table)
		}
		columns = columnNames(rows)
		if len(columns) == 0 {
			return 0, false, fmt.Errorf("the rows have no properties, so there are no columns to create")
		}
		if err := createTable(db, table, columns, rows); err != nil {
			return 0, false, err
		}
		created = true
	} else if err := checkColumns(rows, existing, table); err != nil {
		return 0, false, err
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, created, err
	}
	defer func() { _ = tx.Rollback() }()

	if opts.truncate {
		if _, err := tx.Exec("delete from " + quoteIdent(table)); err != nil {
			return 0, created, err
		}
	}

	stmt, err := tx.Prepare(insertStatement(table, columns))
	if err != nil {
		return 0, created, err
	}
	defer func() { _ = stmt.Close() }()

	written := 0
	for i, row := range rows {
		args := make([]any, len(columns))
		for j, name := range columns {
			value, err := toDriverValue(row[name])
			if err != nil {
				return 0, created, fmt.Errorf("row %d, column %q: %v", i, name, err)
			}
			args[j] = value
		}
		if _, err := stmt.Exec(args...); err != nil {
			return 0, created, fmt.Errorf("row %d: %v", i, err)
		}
		written++
	}
	if err := tx.Commit(); err != nil {
		return 0, created, err
	}
	return written, created, nil
}

// tableColumns reports an existing table's columns, or nil if there is no such
// table.
func tableColumns(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(`select name from pragma_table_info(?) order by cid`, table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var columns []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

// columnNames is every property the rows carry, in a stable order.
//
// It is the union rather than the first row's keys: objects reaching a pipeline
// stage do not all have to have the same shape, and a column that exists only
// in later rows would otherwise be dropped without anyone being told.
func columnNames(rows []map[string]any) []string {
	seen := make(map[string]bool)
	var names []string
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for k := range row {
			if k == psobject.PSTypeNameKey || seen[k] {
				continue
			}
			keys = append(keys, k)
		}
		// A Go map has no order, so the first row would otherwise decide the
		// column order differently on every run.
		sort.Strings(keys)
		for _, k := range keys {
			seen[k] = true
			names = append(names, k)
		}
	}
	return names
}

// checkColumns refuses a row carrying a property the table has no column for.
func checkColumns(rows []map[string]any, columns []string, table string) error {
	known := make(map[string]bool, len(columns))
	for _, name := range columns {
		known[name] = true
	}
	unknown := make(map[string]bool)
	for _, row := range rows {
		for k := range row {
			if k == psobject.PSTypeNameKey || known[k] {
				continue
			}
			unknown[k] = true
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	names := make([]string, 0, len(unknown))
	for name := range unknown {
		names = append(names, name)
	}
	sort.Strings(names)
	return fmt.Errorf("table %q has no column %s (its columns are %s)",
		table, strings.Join(quoteAll(names), ", "), strings.Join(quoteAll(columns), ", "))
}

// createTable declares the table, giving each column the type of the first
// value seen for it. SQLite types are advisory, but a column declared INTEGER
// sorts and compares as a number in SQL, which a TEXT column does not.
func createTable(db *sql.DB, table string, columns []string, rows []map[string]any) error {
	declarations := make([]string, len(columns))
	for i, name := range columns {
		declarations[i] = quoteIdent(name) + " " + inferColumnType(name, rows)
	}
	_, err := db.Exec(fmt.Sprintf("create table %s (%s)",
		quoteIdent(table), strings.Join(declarations, ", ")))
	return err
}

// inferColumnType infers one column's declared type from the first row that has a
// value for it.
func inferColumnType(name string, rows []map[string]any) string {
	for _, row := range rows {
		switch val := common.BindValue(row[name]).(type) {
		case nil:
			continue
		case bool:
			return "INTEGER"
		case string:
			if !utf8.ValidString(val) {
				return "BLOB"
			}
			return "TEXT"
		case []any, map[string]any:
			return "TEXT"
		default:
			if f, ok := common.ToFloat64(val); ok {
				if i, ok := common.ToInt(val); ok && float64(i) == f {
					return "INTEGER"
				}
				return "REAL"
			}
		}
	}
	// A column that was null in every row says nothing about its type. BLOB
	// affinity is the one that converts nothing on the way in, which is the
	// right thing to declare when there is nothing to go on.
	return "BLOB"
}

func insertStatement(table string, columns []string) string {
	names := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, name := range columns {
		names[i] = quoteIdent(name)
		placeholders[i] = "?"
	}
	return fmt.Sprintf("insert into %s (%s) values (%s)",
		quoteIdent(table), strings.Join(names, ", "), strings.Join(placeholders, ", "))
}

// quoteIdent quotes a table or column name.
//
// Identifiers cannot be bound as parameters, so they are quoted instead, with
// any embedded quote doubled as SQL requires. A column called `x" from y; --`
// is a table this cmdlet may well have to create, because the objects it writes
// came from somewhere else.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = fmt.Sprintf("%q", name)
	}
	return out
}
