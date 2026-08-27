//go:build !(js && wasm)

package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterInvokeSqliteQuery registers invoke_sqlite_query, PSSQLite's
// Invoke-SqliteQuery: the rows a SELECT returns, one object per row.
//
//	invoke_sqlite_query("app.db"; "select * from users")
//	invoke_sqlite_query("app.db"; "select * from users where id = ?"; [42])
//	invoke_sqlite_query("app.db"; "select * from users where email = :e"; {e: $addr})
//	"app.db" | invoke_sqlite_query("select count(*) as n from users") | .n
//
// It emits one object per row, as a stream, which is the rule everywhere in
// pwrq: a cmdlet enumerating something streams, and the caller collects with
// [...] or filters as it goes. The stream is lazy down to the database - rows
// are read as they are asked for - so `first(invoke_sqlite_query(...))` on a
// million-row table reads one row.
//
// The database is opened read-only. A statement that writes belongs to
// invoke_sqlite_command, and keeping the two apart means a typo in a SELECT
// cannot modify the file being read.
//
// Values map the way SQLite stores them: NULL to null, INTEGER to a number,
// REAL to a number, TEXT to a string, and BLOB to a jq byte string - so a blob
// can go straight on to sha256 or out_file, and utf8bytelength measures it.
//
// The options object is not available in the piped-database form: the database
// is one operand, so at two arguments the leading one is always the database.
func RegisterInvokeSqliteQuery() gojq.CompilerOption {
	return common.WithIterFunction("invoke_sqlite_query", 1, 3, func(v any, args []any) gojq.Iter {
		in, rest := common.SplitInput(v, args, 1)
		path, err := bindDatabase(in, "invoke_sqlite_query")
		if err != nil {
			return gojq.NewIter(err)
		}
		query, err := common.BindString(rest[0], "sql")
		if err != nil {
			return gojq.NewIter(fmt.Errorf("invoke_sqlite_query: %v", err))
		}
		var params []any
		if len(rest) > 1 {
			params, err = bindParams(rest[1], "invoke_sqlite_query")
			if err != nil {
				return gojq.NewIter(err)
			}
		}
		iter, err := newRowIter(path, query, params)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("invoke_sqlite_query: %v", err))
		}
		return iter
	})
}

// rowIter yields one object per row, reading them as the caller asks.
//
// The handles it holds are the reason this is not simply a slice: a gojq.Iter
// has no Close, so an abandoned stream - `first(invoke_sqlite_query(...))` -
// would leave an open database behind. Exhausting or failing closes
// immediately; a cleanup closes whatever the caller walked away from, which
// matters in the long-lived processes (the MCP server, pwrq-viz) where a leak
// accumulates instead of being collected by the process exiting.
type rowIter struct {
	handles  *dbHandles
	fn       string // the cmdlet to name in an error raised mid-stream
	columns  []string
	typeName string
	extra    map[string]any       // properties added to every row, such as the database
	fixup    func(map[string]any) // last pass over a row, where a column needs a truer JSON type
	scan     []any
	targets  []any
	done     bool
}

// dbHandles is what has to be closed, held apart from the iterator so that a
// cleanup can reference it without keeping the iterator itself alive.
type dbHandles struct {
	db   *sql.DB
	rows *sql.Rows
	once sync.Once
}

func (h *dbHandles) close() {
	h.once.Do(func() {
		if h.rows != nil {
			_ = h.rows.Close()
		}
		if h.db != nil {
			_ = h.db.Close()
		}
	})
}

func newRowIter(path, query string, params []any) (*rowIter, error) {
	db, err := openDB(path, true)
	if err != nil {
		return nil, err
	}
	it, err := newRowIterOnDB(db, "invoke_sqlite_query", query, params, rowType, nil, nil)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return it, nil
}

// newRowIterOnDB streams the rows of a query against a database that is already
// open, and takes ownership of it: the iterator closes the database when it is
// exhausted, fails, or is abandoned.
func newRowIterOnDB(db *sql.DB, fn, query string, params []any, typeName string, extra map[string]any, fixup func(map[string]any)) (*rowIter, error) {
	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	columns, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		return nil, err
	}

	handles := &dbHandles{db: db, rows: rows}
	it := &rowIter{
		handles:  handles,
		fn:       fn,
		columns:  columns,
		typeName: typeName,
		extra:    extra,
		fixup:    fixup,
		scan:     make([]any, len(columns)),
		targets:  make([]any, len(columns)),
	}
	for i := range it.scan {
		it.targets[i] = &it.scan[i]
	}
	runtime.AddCleanup(it, func(h *dbHandles) { h.close() }, handles)
	return it, nil
}

func (it *rowIter) Next() (any, bool) {
	if it.done {
		return nil, false
	}
	if !it.handles.rows.Next() {
		it.finish()
		if err := it.handles.rows.Err(); err != nil {
			return fmt.Errorf("%s: %v", it.fn, err), true
		}
		return nil, false
	}
	if err := it.handles.rows.Scan(it.targets...); err != nil {
		it.finish()
		return fmt.Errorf("%s: %v", it.fn, err), true
	}

	row := make(map[string]any, len(it.columns)+len(it.extra)+1)
	row[psobject.PSTypeNameKey] = it.typeName
	for k, v := range it.extra {
		row[k] = v
	}
	for i, name := range it.columns {
		// A duplicate column name - `select a.id, b.id from ...` - would
		// otherwise silently drop one of the two. SQLite reports both, so
		// report both, the second under the name the caller can distinguish.
		key := name
		if _, taken := row[key]; taken {
			key = fmt.Sprintf("%s_%d", name, i)
		}
		row[key] = it.scan[i]
	}
	if it.fixup != nil {
		it.fixup(row)
	}
	return row, true
}

func (it *rowIter) finish() {
	it.done = true
	it.handles.close()
}

// marshalJSON encodes a nested value for storage as TEXT.
//
// SQLite has no object or array type, so a nested value is stored as JSON - the
// form SQLite's own json_extract reads, which keeps the column queryable from
// SQL rather than only from jq.
func marshalJSON(v any) (string, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("cannot store %T in SQLite: %w", v, err)
	}
	return string(encoded), nil
}
