//go:build !(js && wasm)

// Package sqlite implements the SQLite cmdlets: querying a database file,
// running statements against it, reading its schema, and writing a pipeline of
// objects back into a table.
//
// A row is an ordinary JSON object, so a query is just another source of
// objects for jq to work on:
//
//	pwrq -nc '[invoke_sqlite_query("app.db"; "select * from users")] | map(.email)'
//
// The driver is modernc.org/sqlite, which is SQLite compiled to Go rather than
// linked through cgo. That matters for a tool people `go install`: a cgo
// dependency would need a C toolchain on every machine that builds pwrq and
// would break cross-compilation. It does not build for js/wasm, which is why
// this package is split across a build tag - see unsupported.go.
package sqlite

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// RegisterAll returns every SQLite cmdlet.
//
// One cmdlet per CompilerOption, deliberately: udf.VocabularyFor includes an
// option only when every name it registers is allowed, so an option that
// registered two cmdlets would make both unreachable from a restricted agent
// vocabulary that asked for one.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterInvokeSqliteQuery(),
		RegisterInvokeSqliteCommand(),
		RegisterGetSqliteTable(),
		RegisterGetSqliteSchema(),
		RegisterOutSqlite(),
	}
}

// PowerShell type names for what these cmdlets emit. A row is a DataRow because
// that is what PSSQLite's Invoke-SqliteQuery produces and what a PowerShell
// user will expect; the rest describe SQLite itself, which .NET has no name for.
const (
	rowType     = "System.Data.DataRow"
	tableType   = "Pwrq.Sqlite.Table"
	columnType  = "Pwrq.Sqlite.Column"
	commandType = "Pwrq.Sqlite.CommandResult"
	writeType   = "Pwrq.Sqlite.WriteResult"
)

// openDB opens a database file.
//
// Read-only is a URI flag rather than a promise: the query cmdlets pass
// readOnly, so a SELECT that was actually an UPDATE fails at the database
// instead of quietly rewriting the file the user meant to read.
//
// A missing file is reported here rather than by SQLite, which answers "unable
// to open database file" whether the path is wrong, the directory is
// unwritable, or the disk is full.
func openDB(path string, readOnly bool) (*sql.DB, error) {
	dsn, err := dsn(path, readOnly)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite takes a write lock on the file, not on a connection, so a pool
	// that opened a second connection would deadlock against its own
	// transaction rather than wait for someone else's.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return db, nil
}

// dsn builds the driver's connection string.
//
// A path that already spells a URI is passed through untouched: someone who
// wrote file:app.db?mode=memory&cache=shared means it, and second-guessing it
// would take away the only way to reach SQLite's own options.
func dsn(path string, readOnly bool) (string, error) {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return path, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path %q: %w", path, err)
	}
	if readOnly {
		if _, err := os.Stat(abs); err != nil {
			if os.IsNotExist(err) {
				return "", fmt.Errorf("no such database: %q", abs)
			}
			return "", fmt.Errorf("cannot read database %q: %w", abs, err)
		}
	}
	// The path is escaped as a URI rather than concatenated: a database called
	// "my db#1.sqlite" would otherwise be read as a fragment and truncated.
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	q := url.Values{}
	if readOnly {
		q.Set("mode", "ro")
	}
	// Without a busy timeout a database another process is writing fails
	// immediately with SQLITE_BUSY, which for a five-millisecond write is a
	// failure the caller can do nothing useful about.
	q.Set("_pragma", "busy_timeout(5000)")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// bindDatabase resolves the database operand.
//
// Binding follows the same ByValue-then-ByPropertyName rule as every other
// path-taking cmdlet, so the objects get_sqlite_table emits - which carry the
// database in PSPath - can be piped straight into the next cmdlet.
func bindDatabase(v any, fn string) (string, error) {
	path, ok := common.BindPath(v)
	if !ok || path == "" {
		return "", fmt.Errorf("%s: expected a database path, got %T", fn, common.BindValue(v))
	}
	return path, nil
}

// bindParams turns the params operand into driver arguments.
//
// An array binds positionally to ?, an object binds by name to :name. Both
// exist so that a value never has to be pasted into the SQL text, which is the
// whole of the injection story: a query built by string interpolation is one
// apostrophe away from doing something else.
func bindParams(v any, fn string) ([]any, error) {
	switch val := common.BindValue(v).(type) {
	case nil:
		return nil, nil
	case []any:
		args := make([]any, 0, len(val))
		for _, item := range val {
			arg, err := toDriverValue(item)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fn, err)
			}
			args = append(args, arg)
		}
		return args, nil
	case map[string]any:
		args := make([]any, 0, len(val))
		for name, item := range val {
			arg, err := toDriverValue(item)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", fn, err)
			}
			args = append(args, sql.Named(name, arg))
		}
		return args, nil
	default:
		return nil, fmt.Errorf("%s: params must be an array or an object, got %T", fn, val)
	}
}

// toDriverValue converts one JSON value into something SQLite can store.
//
// A jq string is a byte string, so one that is not valid UTF-8 is binary and
// is stored as a BLOB. Storing it as TEXT would either corrupt it or make the
// database itself invalid, and this is the same contract the rest of pwrq
// keeps: bytes survive the pipeline.
func toDriverValue(v any) (any, error) {
	switch val := common.BindValue(v).(type) {
	case nil:
		return nil, nil
	case bool:
		return val, nil
	case string:
		if !utf8.ValidString(val) {
			return []byte(val), nil
		}
		return val, nil
	case []any, map[string]any:
		encoded, err := marshalJSON(val)
		if err != nil {
			return nil, err
		}
		return encoded, nil
	default:
		if f, ok := common.ToFloat64(val); ok {
			if i, ok := common.ToInt(val); ok && float64(i) == f {
				return int64(i), nil
			}
			return f, nil
		}
		return nil, fmt.Errorf("cannot store %T in SQLite", val)
	}
}
