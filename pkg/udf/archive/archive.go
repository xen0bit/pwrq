// Package archive provides the archive cmdlets: listing, extracting and
// creating .zip and .tar files, including the gzip- and bzip2-compressed tar
// variants.
//
// The compression cmdlets elsewhere in pwrq work on a byte stream — gzip a
// value, zlib a value — which is a different job from handling an archive
// file, where the point is the set of entries inside it. read_archive is the
// one that fits the object model: it emits one object per entry, so the
// entries flow into select, group_by and format_table like any other cmdlet
// output.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every archive cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterReadArchive(),
		RegisterExpandArchive(),
		RegisterCompressArchive(),
	}
}

// entry is one member of an archive, shaped like the FileInfo objects
// get_childitem emits so the two can be treated alike.
type entry struct {
	Name             string
	Length           int64
	CompressedLength int64
	IsDirectory      bool
	Mode             string
	LastWriteTime    any
}

func (e entry) object() map[string]any {
	return ArchiveEntry.Build(map[string]any{
		"Name":             e.Name,
		"Length":           e.Length,
		"CompressedLength": e.CompressedLength,
		"IsDirectory":      e.IsDirectory,
		"Mode":             e.Mode,
		"LastWriteTime":    e.LastWriteTime,
	})
}

// kind identifies an archive by extension. Content sniffing would be friendlier
// but the extension is what the caller asked for, and guessing differently from
// the name is how you overwrite the wrong thing.
type kind int

const (
	kindZip kind = iota
	kindTar
	kindTarGz
	kindTarBz2
)

func kindOf(path string) (kind, error) {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return kindZip, nil
	case strings.HasSuffix(lower, ".tar"):
		return kindTar, nil
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return kindTarGz, nil
	case strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		return kindTarBz2, nil
	}
	return 0, fmt.Errorf("unsupported archive %q: expected .zip, .tar, .tar.gz/.tgz or .tar.bz2/.tbz2", filepath.Base(path))
}

// tarReader opens the decompression layer a tar variant needs.
func tarReader(f *os.File, k kind) (*tar.Reader, error) {
	switch k {
	case kindTarGz:
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		return tar.NewReader(gz), nil
	case kindTarBz2:
		return tar.NewReader(bzip2.NewReader(f)), nil
	default:
		return tar.NewReader(f), nil
	}
}

// RegisterReadArchive registers read_archive, one object per entry in an
// archive without extracting anything.
func RegisterReadArchive() gojq.CompilerOption {
	return common.WithFunctionOf("read_archive", 0, 1, ArchiveEntry.Each(), func(v any, args []any) any {
		path, err := archivePath(v, args, "read_archive")
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		entries, err := listArchive(path)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("read_archive: %v", err), nil)
		}
		out := make([]any, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.object())
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func listArchive(path string) ([]entry, error) {
	k, err := kindOf(path)
	if err != nil {
		return nil, err
	}
	if k == kindZip {
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		out := make([]entry, 0, len(zr.File))
		for _, f := range zr.File {
			out = append(out, entry{
				Name:             f.Name,
				Length:           int64(f.UncompressedSize64),
				CompressedLength: int64(f.CompressedSize64),
				IsDirectory:      f.FileInfo().IsDir(),
				Mode:             f.Mode().String(),
				LastWriteTime:    typed.NormalizeJSON(f.Modified),
			})
		}
		return out, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	tr, err := tarReader(f, k)
	if err != nil {
		return nil, err
	}
	var out []entry
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, entry{
			Name:             h.Name,
			Length:           h.Size,
			CompressedLength: h.Size,
			IsDirectory:      h.FileInfo().IsDir(),
			Mode:             h.FileInfo().Mode().String(),
			LastWriteTime:    typed.NormalizeJSON(h.ModTime),
		})
	}
	return out, nil
}

// RegisterExpandArchive registers expand_archive, extracting an archive into a
// destination directory and returning the paths written.
func RegisterExpandArchive() gojq.CompilerOption {
	common.DeclareInput("expand_archive", common.InputPipeline)
	return common.WithFunction("expand_archive", 1, 2, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		path, ok := common.BindPath(in)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("expand_archive: expected an archive path, got %T", common.BindValue(in)), nil)
		}
		dest, err := common.BindString(rest[0], "destination")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("expand_archive: %v", err), nil)
		}
		written, err := expand(path, dest)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("expand_archive: %v", err), nil)
		}
		out := make([]any, 0, len(written))
		for _, w := range written {
			out = append(out, w)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// safeJoin resolves an entry name under dest, refusing anything that would
// escape it. Archive entries are attacker-controlled data: "../../etc/cron.d/x"
// is a valid tar member name, and extracting it where it asks is the Zip Slip
// vulnerability. Callers get an error rather than a surprise write.
func safeJoin(dest, name string) (string, error) {
	clean := filepath.Clean(filepath.Join(dest, name))
	root := filepath.Clean(dest)
	if clean != root && !strings.HasPrefix(clean, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("entry %q would escape the destination directory", name)
	}
	return clean, nil
}

func expand(path, dest string) ([]string, error) {
	k, err := kindOf(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}
	var written []string

	writeFile := func(target string, mode os.FileMode, r io.Reader) (err error) {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
		if err != nil {
			return err
		}
		// Close is where a short write finally surfaces. Dropping its error
		// lets extraction report success over a truncated file.
		defer func() {
			if cerr := out.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()
		if _, err := io.Copy(out, r); err != nil {
			return err
		}
		written = append(written, target)
		return nil
	}

	if k == kindZip {
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		for _, f := range zr.File {
			target, err := safeJoin(dest, f.Name)
			if err != nil {
				return nil, err
			}
			if f.FileInfo().IsDir() {
				if err := os.MkdirAll(target, 0o755); err != nil {
					return nil, err
				}
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = writeFile(target, f.Mode(), rc)
			_ = rc.Close()
			if err != nil {
				return nil, err
			}
		}
		return written, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	tr, err := tarReader(f, k)
	if err != nil {
		return nil, err
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		target, err := safeJoin(dest, h.Name)
		if err != nil {
			return nil, err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return nil, err
			}
		case tar.TypeReg:
			if err := writeFile(target, h.FileInfo().Mode(), tr); err != nil {
				return nil, err
			}
		}
		// Links and devices are skipped: a symlink in an archive is the other
		// half of the escape safeJoin closes.
	}
	return written, nil
}

// RegisterCompressArchive registers compress_archive, building an archive from
// a path or a list of paths and returning the archive's own FileInfo.
func RegisterCompressArchive() gojq.CompilerOption {
	common.DeclareInput("compress_archive", common.InputPipeline)
	return common.WithFunctionOf("compress_archive", 1, 2, ArchiveWritten, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 1)
		sources, err := sourcePaths(in)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("compress_archive: %v", err), nil)
		}
		dest, err := common.BindString(rest[0], "destination")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("compress_archive: %v", err), nil)
		}
		if err := compressArchive(sources, dest); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("compress_archive: %v", err), nil)
		}
		info, err := os.Stat(dest)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("compress_archive: %v", err), nil)
		}
		abs, _ := filepath.Abs(dest)
		return common.MakeUDFSuccessResult(ArchiveWritten.Build(map[string]any{
			"Name":          info.Name(),
			"FullName":      abs,
			"Length":        info.Size(),
			"LastWriteTime": typed.NormalizeJSON(info.ModTime()),
			typed.ValueKey:  dest,
		}), nil)
	})
}

// sourcePaths accepts one path or an array of them, so both
// `"src" | compress_archive("out.zip")` and a piped stream collected with
// [ ... ] work.
func sourcePaths(in any) ([]string, error) {
	if arr, ok := common.BindValue(in).([]any); ok {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			p, ok := common.BindPath(item)
			if !ok {
				return nil, fmt.Errorf("expected a path, got %T", common.BindValue(item))
			}
			out = append(out, p)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("no source paths given")
		}
		return out, nil
	}
	p, ok := common.BindPath(in)
	if !ok {
		return nil, fmt.Errorf("expected a path, got %T", common.BindValue(in))
	}
	return []string{p}, nil
}

func compressArchive(sources []string, dest string) (err error) {
	k, err := kindOf(dest)
	if err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	// The archive writers buffer, so Close is what flushes the central
	// directory or the tar trailer. An unchecked Close here hands back a
	// truncated archive that reports success, so close them innermost-first
	// and keep the first failure.
	var closers []io.Closer
	defer func() {
		for i := len(closers) - 1; i >= 0; i-- {
			if cerr := closers[i].Close(); cerr != nil && err == nil {
				err = cerr
			}
		}
	}()
	closers = append(closers, out)

	// walk yields each file to add, with the name it should carry inside the
	// archive: relative to the source's parent, so archiving "src" produces
	// entries under "src/".
	walk := func(add func(path, name string, info os.FileInfo) error) error {
		for _, src := range sources {
			info, err := os.Stat(src)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				if err := add(src, filepath.Base(src), info); err != nil {
					return err
				}
				continue
			}
			base := filepath.Dir(filepath.Clean(src))
			if err := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(base, p)
				if err != nil {
					return err
				}
				return add(p, filepath.ToSlash(rel), fi)
			}); err != nil {
				return err
			}
		}
		return nil
	}

	switch k {
	case kindZip:
		zw := zip.NewWriter(out)
		closers = append(closers, zw)
		return walk(func(path, name string, info os.FileInfo) error {
			if info.IsDir() {
				_, err := zw.Create(name + "/")
				return err
			}
			h, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			h.Name, h.Method = name, zip.Deflate
			w, err := zw.CreateHeader(h)
			if err != nil {
				return err
			}
			return copyInto(w, path)
		})
	case kindTarBz2:
		return fmt.Errorf("bzip2 archives can be read but not written")
	default:
		var tw *tar.Writer
		if k == kindTarGz {
			gz := gzip.NewWriter(out)
			closers = append(closers, gz)
			tw = tar.NewWriter(gz)
		} else {
			tw = tar.NewWriter(out)
		}
		closers = append(closers, tw)
		return walk(func(path, name string, info os.FileInfo) error {
			h, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			h.Name = name
			if info.IsDir() {
				h.Name += "/"
			}
			if err := tw.WriteHeader(h); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			return copyInto(tw, path)
		})
	}
}

func copyInto(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(w, f)
	return err
}

// archivePath binds the archive's own path from the pipeline or an argument,
// the way cat does, so `get_childitem(".") | select(...) | read_archive` works.
func archivePath(v any, args []any, fn string) (string, error) {
	in, _ := common.SplitInput(v, args, 0)
	p, ok := common.BindPath(in)
	if !ok {
		return "", fmt.Errorf("%s: expected an archive path, got %T", fn, common.BindValue(in))
	}
	return p, nil
}
