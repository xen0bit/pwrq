package sqlite

import "github.com/xen0bit/pwrq/pkg/core/shape"

// PowerShell type names for what these cmdlets emit. A row is a DataRow because
// that is what PSSQLite's Invoke-SqliteQuery produces and what a PowerShell
// user will expect; the rest describe SQLite itself, which .NET has no name for.
//
// They sit beside the shapes that stamp them: a name and the property list it
// stands for are one fact, and splitting them across files is how the two come
// to disagree.
const (
	rowType     = "System.Data.DataRow"
	tableType   = "Pwrq.Sqlite.Table"
	columnType  = "Pwrq.Sqlite.Column"
	commandType = "Pwrq.Sqlite.CommandResult"
	writeType   = "Pwrq.Sqlite.WriteResult"
)

// A query's rows are the one shape in pwrq that nothing can declare in advance:
// the keys are the columns the SELECT asked for, so they change with the query
// rather than with the cmdlet. That is what the Dynamic kind is for, and saying
// where the keys come from is the whole of what can honestly be said.
var (
	// QueryRow is one row of a result set.
	QueryRow = shape.Dynamic(
		"one key per column the query selected, holding that column's value; " +
			"a duplicate column name gets a _N suffix so neither is lost").
		Named(rowType)

	// TableShape is one table in a database.
	TableShape = shape.Fixed(tableType,
		shape.Prop("Name", shape.String, "table name"),
		shape.Prop("Type", shape.String, "table or view"),
		shape.Prop("Sql", shape.String, "the CREATE statement, as SQLite stored it"),
		shape.Prop("Database", shape.String, "path to the database file"),
		shape.OptProp("PSPath", shape.String, "the database path, as the bindable value"),
	)

	// ColumnShape is one column of one table.
	ColumnShape = shape.Fixed(columnType,
		shape.Prop("Name", shape.String, "column name"),
		shape.Prop("Type", shape.String, "declared type, as written in the CREATE statement"),
		shape.Prop("Position", shape.Number, "zero-based position in the table"),
		shape.Prop("NotNull", shape.Boolean, "whether the column is declared NOT NULL"),
		shape.Prop("IsPrimaryKey", shape.Boolean, "whether the column is part of the primary key"),
		shape.Prop("DefaultValue", shape.Any, "the declared default, or null"),
		shape.Prop("Table", shape.String, "the table this column belongs to"),
		shape.Prop("Database", shape.String, "path to the database file"),
		shape.OptProp("PSPath", shape.String, "the database path, as the bindable value"),
	)

	// CommandResult is what a statement that is not a query reports.
	CommandResult = shape.Fixed(commandType,
		shape.Prop("RowsAffected", shape.Number, "rows the statement changed"),
		shape.Prop("LastInsertId", shape.Any, "rowid of the last insert, or null when the statement inserted nothing"),
		shape.Prop("Database", shape.String, "path to the database file"),
		shape.OptProp("PSPath", shape.String, "the database path, as the bindable value"),
	)

	// WriteResult is what writing a table of rows reports.
	WriteResult = shape.Fixed(writeType,
		shape.Prop("Table", shape.String, "table that was written"),
		shape.Prop("RowCount", shape.Number, "rows written"),
		shape.Prop("Created", shape.Boolean, "whether the table had to be created"),
		shape.Prop("Database", shape.String, "path to the database file"),
		shape.OptProp("PSPath", shape.String, "the database path, as the bindable value"),
	)
)
