//go:build !(js && wasm)

package sqlite

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xen0bit/pwrq/pkg/core/shape"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// run collects every value a query emits, failing on the first error.
//
// The query cmdlets stream, so a test that read one result would pass while
// reporting one row out of however many there are.
func run(t *testing.T, query string) []any {
	t.Helper()
	values, err := eval(query)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return values
}

// runErr runs a query that is expected to fail, returning the error.
func runErr(t *testing.T, query string) error {
	t.Helper()
	values, err := eval(query)
	if err == nil {
		t.Fatalf("%s: expected an error, got %v", query, values)
	}
	return err
}

func eval(query string) ([]any, error) {
	q, err := gojq.Parse(query)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	var out []any
	iter := code.Run(nil)
	for {
		v, ok := iter.Next()
		if !ok {
			return out, nil
		}
		if e, isErr := v.(error); isErr {
			return nil, e
		}
		out = append(out, v)
	}
}

// fixture builds a database with database/sql directly rather than with the
// cmdlets, so a bug in out_sqlite cannot hide one in the query cmdlets.
func fixture(t *testing.T, statements ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	return path
}

func usersFixture(t *testing.T) string {
	t.Helper()
	return fixture(t,
		`create table users (id integer primary key, email text not null, score real)`,
		`insert into users (id, email, score) values (1, 'a@example.com', 1.5), (2, 'b@example.com', 2.5)`,
	)
}

// row reads one property of one emitted object.
func row(t *testing.T, v any, key string) any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected an object, got %T", v)
	}
	return m[key]
}

func TestInvokeSqliteQueryStreamsOneObjectPerRow(t *testing.T) {
	path := usersFixture(t)

	rows := run(t, fmt.Sprintf(`invoke_sqlite_query(%q; "select * from users")`, path))
	if len(rows) != 2 {
		t.Fatalf("emitted %d values, want 2 - one per row", len(rows))
	}
	if got := row(t, rows[0], "email"); got != "a@example.com" {
		t.Errorf("first row email = %v, want a@example.com", got)
	}
	if got := row(t, rows[0], psobject.PSTypeNameKey); got != rowType {
		t.Errorf("PSTypeName = %v, want %s", got, rowType)
	}

	// Collecting is the caller's decision, and it has to produce one array.
	collected := run(t, fmt.Sprintf(`[invoke_sqlite_query(%q; "select * from users")] | length`, path))
	if len(collected) != 1 || collected[0] != 2 {
		t.Errorf("[...] | length = %v, want [2]", collected)
	}
}

func TestInvokeSqliteQueryBindsDatabaseFromThePipeline(t *testing.T) {
	path := usersFixture(t)

	got := run(t, fmt.Sprintf(`%q | invoke_sqlite_query("select count(*) as n from users") | .n`, path))
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("piped database: got %v, want [2]", got)
	}
}

func TestInvokeSqliteQueryBindsParameters(t *testing.T) {
	path := usersFixture(t)

	positional := run(t, fmt.Sprintf(
		`invoke_sqlite_query(%q; "select email from users where id = ?"; [2]) | .email`, path))
	if len(positional) != 1 || positional[0] != "b@example.com" {
		t.Errorf("positional params: got %v, want [b@example.com]", positional)
	}

	named := run(t, fmt.Sprintf(
		`invoke_sqlite_query(%q; "select id from users where email = :e"; {e: "a@example.com"}) | .id`, path))
	if len(named) != 1 || named[0] != 1 {
		t.Errorf("named params: got %v, want [1]", named)
	}
}

// TestInvokeSqliteQueryDoesNotInterpolate is the reason params exist at all.
//
// A value that reaches SQL as text can end the string literal it is sitting in
// and mean something else. Bound, it can only ever be a value that matches
// nothing.
func TestInvokeSqliteQueryDoesNotInterpolate(t *testing.T) {
	path := usersFixture(t)

	rows := run(t, fmt.Sprintf(
		`[invoke_sqlite_query(%q; "select * from users where email = ?"; ["' or 1=1 --"])]`, path))
	if len(rows) != 1 {
		t.Fatalf("got %d results, want 1 (an empty array)", len(rows))
	}
	if list, ok := rows[0].([]any); !ok || len(list) != 0 {
		t.Errorf("an injected value matched %v rows, want none", rows[0])
	}
}

// TestInvokeSqliteQueryIsLazy pins that the stream reads only as far as the
// caller asks.
//
// The second row is the one that cannot be evaluated - json_extract raises on
// malformed JSON - so a first() that succeeds proves row two was never read.
// Collecting everything must still fail, or the probe would prove nothing.
func TestInvokeSqliteQueryIsLazy(t *testing.T) {
	path := fixture(t,
		`create table docs (id integer, doc text)`,
		`insert into docs values (1, '{"a": 1}'), (2, 'not json at all')`,
	)
	query := fmt.Sprintf(`invoke_sqlite_query(%q; "select id, json_extract(doc, '$.a') as a from docs")`, path)

	first := run(t, "first("+query+") | .id")
	if len(first) != 1 || first[0] != 1 {
		t.Fatalf("first(...) = %v, want [1]", first)
	}
	if err := runErr(t, "["+query+"]"); !strings.Contains(err.Error(), "malformed JSON") {
		t.Errorf("collecting every row = %v, want the malformed JSON failure", err)
	}
}

// TestInvokeSqliteQueryIsReadOnly checks the split between the two cmdlets is
// enforced by the database rather than by intent.
func TestInvokeSqliteQueryIsReadOnly(t *testing.T) {
	path := usersFixture(t)

	err := runErr(t, fmt.Sprintf(`invoke_sqlite_query(%q; "delete from users")`, path))
	if !strings.Contains(err.Error(), "readonly") {
		t.Errorf("error = %v, want a read-only database failure", err)
	}

	remaining := run(t, fmt.Sprintf(`%q | invoke_sqlite_query("select count(*) as n from users") | .n`, path))
	if len(remaining) != 1 || remaining[0] != 2 {
		t.Errorf("rows after the refused delete = %v, want [2]", remaining)
	}
}

// TestInvokeSqliteQueryMissingDatabase checks a wrong path is reported as one,
// rather than silently creating an empty database and reporting no rows.
func TestInvokeSqliteQueryMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")

	err := runErr(t, fmt.Sprintf(`invoke_sqlite_query(%q; "select 1")`, path))
	if !strings.Contains(err.Error(), "no such database") {
		t.Errorf("error = %v, want it to name the missing database", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("reading a database that does not exist created one")
	}
}

// TestInvokeSqliteQueryValueTypes pins the mapping between SQLite's storage
// classes and jq's values, including the one that carries bytes.
func TestInvokeSqliteQueryValueTypes(t *testing.T) {
	path := fixture(t,
		`create table v (n integer, r real, t text, b blob, absent text)`,
		`insert into v values (42, 1.5, 'hello', x'00ff10', null)`,
	)

	rows := run(t, fmt.Sprintf(`invoke_sqlite_query(%q; "select * from v")`, path))
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	for _, tc := range []struct {
		column string
		want   any
	}{
		{"n", 42},
		{"r", 1.5},
		{"t", "hello"},
		{"b", "\x00\xff\x10"}, // a jq string is a byte string, so a BLOB is one
		{"absent", nil},
	} {
		if got := row(t, rows[0], tc.column); got != tc.want {
			t.Errorf("column %s = %#v (%T), want %#v", tc.column, got, got, tc.want)
		}
	}
}

// TestInvokeSqliteQueryKeepsDuplicateColumns checks a join that selects the
// same column name twice reports both values rather than one of them.
func TestInvokeSqliteQueryKeepsDuplicateColumns(t *testing.T) {
	path := usersFixture(t)

	rows := run(t, fmt.Sprintf(
		`invoke_sqlite_query(%q; "select id, id from users where id = 1")`, path))
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	m := rows[0].(map[string]any)
	if m["id"] != 1 || m["id_1"] != 1 {
		t.Errorf("duplicate columns = %v, want both under distinct keys", m)
	}
}

func TestInvokeSqliteCommandReportsWhatItDid(t *testing.T) {
	path := usersFixture(t)

	inserted := run(t, fmt.Sprintf(
		`invoke_sqlite_command(%q; "insert into users (email) values (?)"; ["c@example.com"])`, path))
	if len(inserted) != 1 {
		t.Fatalf("emitted %d values, want a single object", len(inserted))
	}
	if got := row(t, inserted[0], "RowsAffected"); got != 1 {
		t.Errorf("RowsAffected = %v, want 1", got)
	}
	if got := row(t, inserted[0], "LastInsertId"); got != 3 {
		t.Errorf("LastInsertId = %v, want 3", got)
	}

	deleted := run(t, fmt.Sprintf(`invoke_sqlite_command(%q; "delete from users") | .RowsAffected`, path))
	if len(deleted) != 1 || deleted[0] != 3 {
		t.Errorf("RowsAffected for the delete = %v, want [3]", deleted)
	}
}

// TestInvokeSqliteCommandCreatesTheDatabase checks the write cmdlet does what
// `sqlite3 new.db` does, since a database has to start somewhere.
func TestInvokeSqliteCommandCreatesTheDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")

	run(t, fmt.Sprintf(`invoke_sqlite_command(%q; "create table t (a integer)")`, path))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the database was not created: %v", err)
	}
	tables := run(t, fmt.Sprintf(`[get_sqlite_table(%q)] | map(.Name)`, path))
	if len(tables) != 1 || fmt.Sprint(tables[0]) != "[t]" {
		t.Errorf("tables = %v, want [[t]]", tables)
	}
}

func TestGetSqliteTable(t *testing.T) {
	path := fixture(t,
		`create table users (id integer primary key autoincrement)`,
		`insert into users default values`,
		`create view recent as select * from users`,
	)

	names := run(t, fmt.Sprintf(`get_sqlite_table(%q) | .Name`, path))
	if fmt.Sprint(names) != "[recent users]" {
		t.Errorf("names = %v, want [recent users]", names)
	}
	// AUTOINCREMENT creates sqlite_sequence, which is SQLite's bookkeeping
	// rather than anything the caller put there.
	for _, name := range names {
		if strings.HasPrefix(fmt.Sprint(name), "sqlite_") {
			t.Errorf("%v is SQLite's own table and should not be listed", name)
		}
	}

	rows := run(t, fmt.Sprintf(`get_sqlite_table(%q) | select(.Type == "view")`, path))
	if len(rows) != 1 {
		t.Fatalf("got %d views, want 1", len(rows))
	}
	if got := row(t, rows[0], "Sql"); !strings.Contains(strings.ToLower(fmt.Sprint(got)), "select * from users") {
		t.Errorf("Sql = %v, want the view's definition", got)
	}
}

// TestGetSqliteTableBindsToTheNextCmdlet checks the database travels on the
// objects, which is what makes the two schema cmdlets compose.
func TestGetSqliteTableBindsToTheNextCmdlet(t *testing.T) {
	path := fixture(t, `create table users (id integer primary key, email text)`)

	columns := run(t, fmt.Sprintf(`get_sqlite_table(%q) | get_sqlite_schema(.Name) | .Name`, path))
	if fmt.Sprint(columns) != "[id email]" {
		t.Errorf("columns = %v, want [id email]", columns)
	}
}

func TestGetSqliteSchema(t *testing.T) {
	path := fixture(t, `create table users (
		id integer primary key,
		email text not null,
		score real default 1.5
	)`)

	rows := run(t, fmt.Sprintf(`[get_sqlite_schema(%q; "users")]`, path))
	columns := rows[0].([]any)
	if len(columns) != 3 {
		t.Fatalf("got %d columns, want 3", len(columns))
	}

	id := columns[0].(map[string]any)
	if id["Name"] != "id" || id["Position"] != 0 {
		t.Errorf("first column = %v, want id at position 0", id)
	}
	// 0 and 1 are both truthy in jq, so `select(.IsPrimaryKey)` would keep
	// every column if these stayed as SQLite stores them.
	if id["IsPrimaryKey"] != true {
		t.Errorf("IsPrimaryKey = %#v, want true", id["IsPrimaryKey"])
	}
	email := columns[1].(map[string]any)
	if email["NotNull"] != true {
		t.Errorf("NotNull = %#v, want true", email["NotNull"])
	}
	if email["IsPrimaryKey"] != false {
		t.Errorf("IsPrimaryKey = %#v, want false", email["IsPrimaryKey"])
	}
	score := columns[2].(map[string]any)
	if fmt.Sprint(score["DefaultValue"]) != "1.5" {
		t.Errorf("DefaultValue = %v, want 1.5", score["DefaultValue"])
	}
}

// TestGetSqliteSchemaMissingTable checks a typo is an error rather than an
// empty stream, which would read as "this table has no columns".
func TestGetSqliteSchemaMissingTable(t *testing.T) {
	path := usersFixture(t)

	err := runErr(t, fmt.Sprintf(`get_sqlite_schema(%q; "userz")`, path))
	if !strings.Contains(err.Error(), "no such table") {
		t.Errorf("error = %v, want it to name the missing table", err)
	}
}

func TestOutSqliteCreatesTheTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")

	written := run(t, fmt.Sprintf(
		`[{Name: "a", Length: 10, Ratio: 0.5, Hidden: false}] | out_sqlite(%q; "files")`, path))
	if len(written) != 1 {
		t.Fatalf("emitted %d values, want a single object", len(written))
	}
	if got := row(t, written[0], "RowCount"); got != 1 {
		t.Errorf("RowCount = %v, want 1", got)
	}
	if got := row(t, written[0], "Created"); got != true {
		t.Errorf("Created = %v, want true", got)
	}

	// The declared types are advisory in SQLite, but a column declared INTEGER
	// still sorts and compares as a number in SQL.
	types := map[string]string{}
	for _, column := range run(t, fmt.Sprintf(`get_sqlite_schema(%q; "files")`, path)) {
		m := column.(map[string]any)
		types[fmt.Sprint(m["Name"])] = fmt.Sprint(m["Type"])
	}
	want := map[string]string{"Name": "TEXT", "Length": "INTEGER", "Ratio": "REAL", "Hidden": "INTEGER"}
	for name, wantType := range want {
		if types[name] != wantType {
			t.Errorf("column %s declared %s, want %s", name, types[name], wantType)
		}
	}
}

// TestOutSqliteDoesNotStoreTheTypeName checks the marker that says what kind of
// object this is does not become a column repeating itself a million times.
func TestOutSqliteDoesNotStoreTheTypeName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")

	run(t, fmt.Sprintf(
		`[{PSTypeName: "System.IO.FileInfo", Name: "a"}] | out_sqlite(%q; "files")`, path))
	for _, column := range run(t, fmt.Sprintf(`get_sqlite_schema(%q; "files") | .Name`, path)) {
		if column == psobject.PSTypeNameKey {
			t.Errorf("%s was written as a column", psobject.PSTypeNameKey)
		}
	}
}

// TestOutSqliteTakesEveryProperty checks the columns are the union of what the
// rows carry: objects in one pipeline do not have to have the same shape, and a
// property only later rows have would otherwise vanish.
func TestOutSqliteTakesEveryProperty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")

	run(t, fmt.Sprintf(`[{a: 1}, {a: 2, b: "late"}] | out_sqlite(%q; "t")`, path))

	rows := run(t, fmt.Sprintf(`[invoke_sqlite_query(%q; "select a, b from t")]`, path))
	values := rows[0].([]any)
	if len(values) != 2 {
		t.Fatalf("got %d rows, want 2", len(values))
	}
	if got := values[0].(map[string]any)["b"]; got != nil {
		t.Errorf("the row without b stored %#v, want null", got)
	}
	if got := values[1].(map[string]any)["b"]; got != "late" {
		t.Errorf("the row with b stored %#v, want late", got)
	}
}

// TestOutSqliteRejectsAnUnknownColumn checks writing into an existing table
// refuses to drop data on the floor.
func TestOutSqliteRejectsAnUnknownColumn(t *testing.T) {
	path := fixture(t, `create table t (a integer)`)

	err := runErr(t, fmt.Sprintf(`[{a: 1, b: 2}] | out_sqlite(%q; "t")`, path))
	if !strings.Contains(err.Error(), `no column "b"`) {
		t.Errorf("error = %v, want it to name the column the table lacks", err)
	}
}

func TestOutSqliteCreateFalse(t *testing.T) {
	path := fixture(t, `create table other (a integer)`)

	err := runErr(t, fmt.Sprintf(`[{a: 1}] | out_sqlite(%q; "t"; {Create: false})`, path))
	if !strings.Contains(err.Error(), "no such table") {
		t.Errorf("error = %v, want it to say the table does not exist", err)
	}
}

func TestOutSqliteTruncate(t *testing.T) {
	path := fixture(t,
		`create table t (a integer)`,
		`insert into t values (1), (2)`,
	)

	run(t, fmt.Sprintf(`[{a: 3}] | out_sqlite(%q; "t"; {Truncate: true})`, path))

	rows := run(t, fmt.Sprintf(`[invoke_sqlite_query(%q; "select a from t") | .a]`, path))
	if fmt.Sprint(rows[0]) != "[3]" {
		t.Errorf("rows after a truncating write = %v, want [3]", rows[0])
	}
}

// TestOutSqliteRollsBackOnFailure checks a write that fails half way leaves the
// table as it was, rather than partly written.
func TestOutSqliteRollsBackOnFailure(t *testing.T) {
	path := fixture(t, `create table t (a integer primary key)`)

	err := runErr(t, fmt.Sprintf(`[{a: 1}, {a: 2}, {a: 1}] | out_sqlite(%q; "t")`, path))
	if !strings.Contains(err.Error(), "row 2") {
		t.Errorf("error = %v, want it to name the row that failed", err)
	}

	count := run(t, fmt.Sprintf(`%q | invoke_sqlite_query("select count(*) as n from t") | .n`, path))
	if len(count) != 1 || count[0] != 0 {
		t.Errorf("rows after a failed write = %v, want [0]", count)
	}
}

// TestOutSqliteQuotesIdentifiers checks a property name that is also SQL cannot
// change the statement it is written into. The objects being written came from
// somewhere else, so their keys are data, not code.
func TestOutSqliteQuotesIdentifiers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")
	column := `x" from t; drop table t; --`

	run(t, fmt.Sprintf(`[{%q: 1}] | out_sqlite(%q; "select")`, column, path))

	names := run(t, fmt.Sprintf(`get_sqlite_schema(%q; "select") | .Name`, path))
	if len(names) != 1 || names[0] != column {
		t.Errorf("column names = %#v, want the name written verbatim", names)
	}
}

// TestOutSqliteBinaryRoundTrip checks bytes survive the round trip through the
// database, as they do through the rest of pwrq.
func TestOutSqliteBinaryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")
	content := []byte{0x00, 0xff, 0x10, 0x80, 0x41}
	encoded := base64.StdEncoding.EncodeToString(content)

	run(t, fmt.Sprintf(`[{Data: (%q | @base64d)}] | out_sqlite(%q; "blobs")`, encoded, path))

	// A jq string that is not valid UTF-8 is binary, so it is stored as a BLOB
	// rather than as TEXT that would either be corrupted or invalid.
	declared := run(t, fmt.Sprintf(`get_sqlite_schema(%q; "blobs") | .Type`, path))
	if len(declared) != 1 || declared[0] != "BLOB" {
		t.Errorf("declared type = %v, want BLOB", declared)
	}
	got := run(t, fmt.Sprintf(`invoke_sqlite_query(%q; "select Data from blobs") | .Data`, path))
	if len(got) != 1 || got[0] != string(content) {
		t.Errorf("stored bytes = %#v, want %#v", got, string(content))
	}
}

// TestOutSqliteEmptyPipeline checks an empty result set writes nothing and
// invents nothing: there is no shape to infer a table from.
func TestOutSqliteEmptyPipeline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")

	written := run(t, fmt.Sprintf(`[] | out_sqlite(%q; "t")`, path))
	if got := row(t, written[0], "RowCount"); got != 0 {
		t.Errorf("RowCount = %v, want 0", got)
	}
	if got := row(t, written[0], "Created"); got != false {
		t.Errorf("Created = %v, want false", got)
	}
	tables := run(t, fmt.Sprintf(`[get_sqlite_table(%q)] | length`, path))
	if tables[0] != 0 {
		t.Errorf("an empty write created %v table(s)", tables[0])
	}
}

func TestOutSqliteRejectsNonObjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")

	err := runErr(t, fmt.Sprintf(`["just a string"] | out_sqlite(%q; "t")`, path))
	if !strings.Contains(err.Error(), "a table row is an object") {
		t.Errorf("error = %v, want it to say a row must be an object", err)
	}
}

func TestOutSqliteRejectsAnUnknownOption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.db")

	err := runErr(t, fmt.Sprintf(`[{a: 1}] | out_sqlite(%q; "t"; {Trunkate: true})`, path))
	if !strings.Contains(err.Error(), `unknown option "Trunkate"`) {
		t.Errorf("error = %v, want a misspelled option to be refused", err)
	}
}

// TestAbandonedStreamClosesTheDatabase covers the hazard a lazy stream brings
// with it: gojq.Iter has no Close, so `first(invoke_sqlite_query(...))` walks
// away from an open database. Exhausting the stream closes it; this is the
// other path, and it matters in the long-lived processes - the MCP server,
// pwrq-viz - where a leaked handle accumulates instead of being cleaned up by
// the process exiting.
func TestAbandonedStreamClosesTheDatabase(t *testing.T) {
	path := usersFixture(t)

	db, err := openDB(path, true)
	if err != nil {
		t.Fatal(err)
	}
	it, err := newRowIterOnDB(db, "invoke_sqlite_query", "select * from users", nil, QueryRow, nil, nil)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	handles := it.handles
	if _, ok := it.Next(); !ok {
		t.Fatal("the query produced no rows")
	}

	// Abandon the iterator without exhausting it, exactly as first() does.
	it = nil
	_ = it

	deadline := time.Now().Add(10 * time.Second)
	for {
		runtime.GC()
		if err := handles.db.Ping(); err != nil {
			if strings.Contains(err.Error(), "closed") {
				return // the cleanup ran
			}
			t.Fatalf("unexpected error from the abandoned database: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("an abandoned stream left the database open")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestShapeDeclarationsMatchTheRowsBuilt closes the loop for this package.
//
// The cross-cutting check in pkg/udf runs each cmdlet's documented example, and
// these cmdlets' examples name a database file that does not exist there, so
// they are skipped. The tests in this file do exercise them properly, and
// shape.Build records every disagreement it sees, so asserting the table is
// empty here covers what the other test cannot reach.
func TestShapeDeclarationsMatchTheRowsBuilt(t *testing.T) {
	if len(shape.Discrepancies()) == 0 {
		return
	}
	var lines []string
	for _, d := range shape.Discrepancies() {
		lines = append(lines, d.String())
	}
	t.Errorf("a sqlite shape declaration disagrees with the object built through it:\n    %s",
		strings.Join(lines, "\n    "))
}
