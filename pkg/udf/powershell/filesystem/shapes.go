package filesystem

import "github.com/xen0bit/pwrq/pkg/core/shape"

// The shapes this package emits.
//
// They are declared once here and used both to build the objects and to
// describe them, so the property list a caller reads is the property list the
// cmdlet writes. Before this, the type name was a string literal at five
// separate construction sites and the property list existed nowhere at all.

// fileItemProps are the properties of a filesystem item. A file and a directory
// carry the same set - a directory reports Length 0 rather than omitting it -
// so the two shapes differ only in the type name they stamp.
func fileItemProps() []shape.Property {
	return []shape.Property{
		shape.Prop("Name", shape.String, "base name, without the directory"),
		shape.Prop("FullName", shape.String, "absolute path, so it survives a change of directory"),
		shape.Prop("Extension", shape.String, "final extension including the dot, or \"\""),
		shape.Prop("Length", shape.Number, "size in bytes; 0 for a directory"),
		shape.Prop("Mode", shape.String, "permission bits, ls-style, with a leading d for a directory"),
		shape.Prop("CreationTime", shape.String, "RFC 3339 timestamp"),
		shape.Prop("LastWriteTime", shape.String, "RFC 3339 timestamp"),
		shape.Prop("LastAccessTime", shape.String, "RFC 3339 timestamp"),
		shape.Prop("IsReadOnly", shape.Boolean, "true when no write bit is set"),
		shape.Prop("IsHidden", shape.Boolean, "true when the name starts with a dot"),
		shape.OptProp("PSPath", shape.String, "the path as given, which downstream cmdlets bind to"),
	}
}

var (
	// FileInfo is one file.
	FileInfo = shape.Fixed("System.IO.FileInfo", fileItemProps()...)
	// DirectoryInfo is one directory. PowerShell draws the same distinction,
	// and format_table uses it to decide what to show.
	DirectoryInfo = shape.Fixed("System.IO.DirectoryInfo", fileItemProps()...)

	// PathInfo is a resolved path.
	PathInfo = shape.Fixed("System.IO.PathInfo",
		shape.Prop("Path", shape.String, "the resolved path"),
		shape.OptProp("ProviderPath", shape.String, "the same path, absolute"),
		shape.OptProp("Drive", shape.String, "the drive or root the path sits under"),
		shape.OptProp("Provider", shape.String, "the PowerShell provider name, always FileSystem here"),
		shape.OptProp("PSPath", shape.String, "the path as given"),
	)
)

var (
	// JoinedPath is what join_path returns: the joined path, plus the parts it
	// decomposes into.
	//
	// It used to call itself a System.String, which was wrong in a way that
	// only mattered once the type name became a key into a catalogue: a caller
	// looking System.String up would find an eight-property path object rather
	// than a string. A name that means two things cannot be looked up.
	JoinedPath = shape.Fixed("Pwrq.Path.Joined",
		shape.Prop("Path", shape.String, "the joined path"),
		shape.Prop("DirectoryName", shape.String, "everything up to the final separator"),
		shape.Prop("FileName", shape.String, "the final component"),
		shape.Prop("Extension", shape.String, "final extension including the dot, or \"\""),
		shape.Prop("Drive", shape.String, "drive prefix such as C:, or \"\" on Unix"),
		shape.Prop("IsAbsolute", shape.Boolean, "whether the path is rooted"),
		shape.Prop("IsUnc", shape.Boolean, "whether the path is a Windows UNC share"),
		shape.Prop("PSPath", shape.String, "the joined path, as the bindable value"),
	)

	// SplitPath is what split_path returns. Name and BaseName are the pair
	// callers most often mix up, so both are spelled out.
	SplitPath = shape.Fixed("Pwrq.Path.Split",
		shape.Prop("Path", shape.String, "the path that was split"),
		shape.Prop("DirectoryName", shape.String, "everything up to the final separator"),
		shape.Prop("BaseName", shape.String, "the final component, with its extension"),
		shape.Prop("Name", shape.String, "the final component, without its extension"),
		shape.Prop("Extension", shape.String, "final extension including the dot, or \"\""),
		shape.Prop("Drive", shape.String, "drive prefix such as C:, or \"\" on Unix"),
		shape.Prop("IsAbsolute", shape.Boolean, "whether the path is rooted"),
		shape.Prop("PSPath", shape.String, "the path that was split, as the bindable value"),
	)
)

// itemShape picks the shape for a filesystem item.
func itemShape(isDir bool) *shape.Shape {
	if isDir {
		return DirectoryInfo
	}
	return FileInfo
}
