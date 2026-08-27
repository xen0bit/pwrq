//go:build !(js && wasm)

package sqlite

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterInvokeSqliteCommand registers invoke_sqlite_command: a statement that
// changes the database rather than reading it.
//
//	invoke_sqlite_command("app.db"; "create table users (id integer primary key, email text)")
//	invoke_sqlite_command("app.db"; "delete from users where id = ?"; [42])
//	invoke_sqlite_command("app.db"; "update users set seen = :now"; {now: now})
//
// It returns a single object - {RowsAffected, LastInsertId} - because that is
// one answer about one statement, not an enumeration; no [...] goes around it.
// invoke_sqlite_query is the other half, and the split is what lets a query
// open the file read-only.
//
// The database is created if it does not exist, as `sqlite3 new.db` would.
//
// Several statements separated by semicolons run in the order written, and
// RowsAffected then describes the last of them. They are not wrapped in a
// transaction: a caller who wants one writes BEGIN and COMMIT, which is the
// only way a script containing its own transaction control can also work.
func RegisterInvokeSqliteCommand() gojq.CompilerOption {
	return common.WithFunction("invoke_sqlite_command", 1, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		path, err := bindDatabase(in, "invoke_sqlite_command")
		if err != nil {
			return err
		}
		statement, err := common.BindString(rest[0], "sql")
		if err != nil {
			return fmt.Errorf("invoke_sqlite_command: %v", err)
		}
		var params []any
		if len(rest) > 1 {
			params, err = bindParams(rest[1], "invoke_sqlite_command")
			if err != nil {
				return err
			}
		}

		db, err := openDB(path, false)
		if err != nil {
			return fmt.Errorf("invoke_sqlite_command: %v", err)
		}
		defer func() { _ = db.Close() }()

		result, err := db.Exec(statement, params...)
		if err != nil {
			return fmt.Errorf("invoke_sqlite_command: %v", err)
		}

		out := map[string]any{
			psobject.PSTypeNameKey: commandType,
			"Database":             path,
			psobject.PSPathKey:     path,
			"RowsAffected":         0,
			"LastInsertId":         nil,
		}
		// Both are advisory: a CREATE TABLE has neither, and a driver is
		// allowed to refuse them. A refusal is not a failed statement, so it
		// leaves the property null rather than turning the write into an error.
		if affected, err := result.RowsAffected(); err == nil {
			out["RowsAffected"] = int(affected)
		}
		if id, err := result.LastInsertId(); err == nil && id != 0 {
			out["LastInsertId"] = int(id)
		}
		return out
	})
}
