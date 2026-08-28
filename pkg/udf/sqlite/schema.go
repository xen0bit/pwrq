//go:build !(js && wasm)

package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterGetSqliteTable registers get_sqlite_table: what is in a database.
//
//	get_sqlite_table("app.db")
//	[get_sqlite_table("app.db")] | map(.Name)
//	get_sqlite_table("app.db") | select(.Type == "view") | .Sql
//
// One object per table and view, as a stream. SQLite's own bookkeeping tables
// are left out: sqlite_sequence is an implementation detail of AUTOINCREMENT,
// not something the caller put in the database.
//
// Each object carries the database in PSPath as well as in Database, so it
// binds as the database operand of the next cmdlet:
//
//	get_sqlite_table("app.db") | get_sqlite_schema(.Name)
//
// A row count is deliberately absent. It would mean a full scan of every table
// to answer a question most callers did not ask; the one who did asks for it
// with `invoke_sqlite_query($db; "select count(*) from t")`.
func RegisterGetSqliteTable() gojq.CompilerOption {
	common.DeclareInput("get_sqlite_table", common.InputPipeline)
	return common.WithIterFunction("get_sqlite_table", 0, 1, func(v any, args []any) gojq.Iter {
		in, _ := common.SplitInput(v, args, 0)
		path, err := bindDatabase(in, "get_sqlite_table")
		if err != nil {
			return gojq.NewIter(err)
		}

		db, err := openDB(path, true)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("get_sqlite_table: %v", err))
		}
		const query = `select name as "Name", type as "Type", sql as "Sql"
			from sqlite_schema
			where type in ('table', 'view') and name not like 'sqlite_%'
			order by name`
		extra := map[string]any{"Database": path, psobject.PSPathKey: path}
		iter, err := newRowIterOnDB(db, "get_sqlite_table", query, nil, tableType, extra, nil)
		if err != nil {
			_ = db.Close()
			return gojq.NewIter(fmt.Errorf("get_sqlite_table: %v", err))
		}
		return iter
	})
}

// RegisterGetSqliteSchema registers get_sqlite_schema: the columns of a table.
//
//	get_sqlite_schema("app.db"; "users")
//	[get_sqlite_schema("app.db"; "users")] | map(select(.IsPrimaryKey) | .Name)
//	get_sqlite_table("app.db") | get_sqlite_schema(.Name)
//
// One object per column, as a stream, in the order the table declares them.
// NotNull and IsPrimaryKey are booleans rather than the 0 and 1 SQLite stores,
// because `select(.IsPrimaryKey)` is what a caller will write and 0 is truthy
// in jq.
//
// A table that does not exist is an error rather than an empty stream: no table
// has zero columns, so an empty answer could only be read as a lie.
func RegisterGetSqliteSchema() gojq.CompilerOption {
	common.DeclareInput("get_sqlite_schema", common.InputPipeline)
	return common.WithIterFunction("get_sqlite_schema", 1, 2, func(v any, args []any) gojq.Iter {
		in, rest := common.SplitInput(v, args, 1)
		path, err := bindDatabase(in, "get_sqlite_schema")
		if err != nil {
			return gojq.NewIter(err)
		}
		table, err := common.BindString(rest[0], "table")
		if err != nil {
			return gojq.NewIter(fmt.Errorf("get_sqlite_schema: %v", err))
		}

		db, err := openDB(path, true)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("get_sqlite_schema: %v", err))
		}
		if err := requireTable(db, table); err != nil {
			_ = db.Close()
			return gojq.NewIter(fmt.Errorf("get_sqlite_schema: %v", err))
		}

		// pragma_table_info is the table-valued form of PRAGMA table_info. The
		// PRAGMA statement itself takes no bound parameters, so reaching it any
		// other way would mean pasting the table name into the SQL.
		// Every alias is quoted because one of them has to be: NOTNULL is a
		// SQLite keyword, so `as NotNull` is a syntax error rather than a
		// column name.
		const query = `select cid as "Position", name as "Name", type as "Type",
				"notnull" as "NotNull", dflt_value as "DefaultValue", pk as "IsPrimaryKey"
			from pragma_table_info(?)
			order by cid`
		extra := map[string]any{"Database": path, "Table": table, psobject.PSPathKey: path}
		iter, err := newRowIterOnDB(db, "get_sqlite_schema", query, []any{table}, columnType, extra, asBooleans("NotNull", "IsPrimaryKey"))
		if err != nil {
			_ = db.Close()
			return gojq.NewIter(fmt.Errorf("get_sqlite_schema: %v", err))
		}
		return iter
	})
}

// requireTable reports a missing table by name, rather than leaving SQLite to
// answer with an empty result the caller would have to interpret.
func requireTable(db *sql.DB, table string) error {
	var found string
	err := db.QueryRow(
		`select name from sqlite_schema where type in ('table', 'view') and name = ?`,
		table,
	).Scan(&found)
	if err == sql.ErrNoRows {
		return fmt.Errorf("no such table: %q", table)
	}
	return err
}

// asBooleans rewrites SQLite's 0 and 1 into JSON booleans for the named columns.
func asBooleans(names ...string) func(map[string]any) {
	return func(row map[string]any) {
		for _, name := range names {
			n, ok := common.ToInt(row[name])
			if !ok {
				continue
			}
			row[name] = n != 0
		}
	}
}
