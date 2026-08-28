package archive

import "github.com/xen0bit/pwrq/pkg/core/shape"

var (
	// ArchiveEntry is one member of an archive, read without extracting it.
	ArchiveEntry = shape.Fixed("Pwrq.Archive.Entry",
		shape.Prop("Name", shape.String, "path of the entry inside the archive"),
		shape.Prop("Length", shape.Number, "uncompressed size in bytes"),
		shape.Prop("CompressedLength", shape.Number, "stored size in bytes"),
		shape.Prop("IsDirectory", shape.Boolean, "whether the entry is a directory"),
		shape.Prop("Mode", shape.String, "permission bits recorded in the archive"),
		shape.Prop("LastWriteTime", shape.Any, "RFC 3339 timestamp, or null when the archive did not record one"),
	)

	// ArchiveWritten is the archive compress_archive just produced.
	//
	// It once wore get_childitem's type name and is not a file listing: there
	// is no Mode, Extension or IsHidden here, so a caller who looked that name
	// up would be told about properties that are not present.
	ArchiveWritten = shape.Fixed("Pwrq.Archive.WriteResult",
		shape.Prop("Name", shape.String, "base name of the archive that was written"),
		shape.Prop("FullName", shape.String, "absolute path to it"),
		shape.Prop("Length", shape.Number, "its size in bytes"),
		shape.Prop("LastWriteTime", shape.Any, "RFC 3339 timestamp"),
		shape.OptProp("PwrqValue", shape.String, "the destination as given, as the bindable value"),
	)
)
