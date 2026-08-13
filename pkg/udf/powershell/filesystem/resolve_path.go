package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// ResolvePathOptions represents options for Resolve-Path
type ResolvePathOptions struct {
	Path       string
	Credential any
	Literal    bool
}

// parseResolvePathArgs parses arguments for resolve_path
func parseResolvePathArgs(args []any) (ResolvePathOptions, error) {
	opts := ResolvePathOptions{
		Path: ".",
	}

	if len(args) == 0 {
		return opts, nil
	}

	for i, arg := range args {
		argVal := common.BindValue(arg)

		switch v := argVal.(type) {
		case string:
			if i == 0 {
				opts.Path = v
			}
		case map[string]any:
			if path, ok := v["Path"].(string); ok {
				opts.Path = path
			}
			if literal, ok := v["Literal"].(bool); ok {
				opts.Literal = literal
			}
			if credential, ok := v["Credential"]; ok {
				opts.Credential = credential
			}
		}
	}

	return opts, nil
}

// resolvePath resolves a path to its absolute form
func resolvePath(opts ResolvePathOptions) ([]any, error) {
	var results []any

	path := opts.Path

	// Expand tilde to home directory
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %v", err)
		}
		if len(path) == 1 {
			path = homeDir
		} else if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
			path = filepath.Join(homeDir, path[2:])
		}
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve path: %v", err)
	}

	// Check if path exists
	_, err = os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path not found: %s", path)
		}
		return nil, fmt.Errorf("cannot access path: %v", err)
	}

	// Create PSObject for the resolved path
	psobj := psobject.NewPSObject(absPath)
	psobj.TypeName = "System.IO.PathInfo"
	psobj.AddNoteProperty("Path", absPath)
	psobj.AddNoteProperty("Provider", "FileSystem")
	psobj.AddNoteProperty("ProviderPath", absPath)

	results = append(results, psobj.ToMap())

	return results, nil
}

// RegisterResolvePath registers the resolve_path function with gojq
func RegisterResolvePath() gojq.CompilerOption {
	return common.WithIterFunction("resolve_path", 0, 2, func(v any, args []any) gojq.Iter {
		opts, err := parseResolvePathArgs(args)
		if err != nil {
			return gojq.NewIter(err)
		}

		results, err := resolvePath(opts)
		if err != nil {
			return gojq.NewIter(fmt.Errorf("resolve_path: %v", err))
		}

		return common.SliceIter(results)
	})
}
