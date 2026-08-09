package udf

import (
	"strings"
	"testing"
)

// TestWebRegistryIsASubset checks that WebRegistry only ever narrows: a
// function the browser offers has to be one the CLI has, or the page would be
// teaching a vocabulary that does not exist anywhere else.
func TestWebRegistryIsASubset(t *testing.T) {
	full, err := DefaultRegistry().Names()
	if err != nil {
		t.Fatalf("discovering the default registry: %v", err)
	}
	available := make(map[string]bool, len(full))
	for _, name := range full {
		available[name] = true
	}

	web, err := WebRegistry().Names()
	if err != nil {
		t.Fatalf("discovering the web registry: %v", err)
	}
	if len(web) == 0 {
		t.Fatal("the web registry registered nothing")
	}
	for _, name := range web {
		if !available[name] {
			t.Errorf("web registry offers %q, which the CLI does not have", name)
		}
	}
}

// TestWebRegistryExcludesTheUnavailable is the guard that matters: a browser
// tab has no filesystem, process table or service manager, so a cmdlet that
// needs one can only fail. Offering it would advertise a pipeline the page can
// never run.
//
// This is a list rather than a rule because there is no property of a Go
// function that says "reaches the disk" - the decision is editorial, so it is
// written down and checked.
func TestWebRegistryExcludesTheUnavailable(t *testing.T) {
	web, err := WebRegistry().Names()
	if err != nil {
		t.Fatal(err)
	}
	registered := make(map[string]bool, len(web))
	for _, name := range web {
		registered[name] = true
	}

	unavailable := []string{
		// Filesystem
		"find", "cat", "mkdir", "rm", "tee", "tempdir",
		"get_childitem", "new_item", "copy_item", "move_item",
		"set_content", "get_content", "test_path", "resolve_path",
		// Location, which is a filesystem cursor
		"get_location", "set_location", "push_location", "pop_location",
		// Processes and services
		"get_process", "stop_process", "start_process",
		"get_service", "start_service", "stop_service",
		// Shell
		"sh",
		// Network: these would work only where CORS allows, which is a failure
		// mode with nothing to do with the query.
		"http", "http_serve", "invoke_web_request", "invoke_rest_method",
		"test_connection",
		// The system clock
		"set_date",
	}
	for _, name := range unavailable {
		if registered[name] {
			t.Errorf("web registry offers %q, which cannot work in a browser", name)
		}
	}
}

// TestWebRegistryOffersTheTransforms checks the other half: narrowing must not
// have taken out the functions that are the reason to open the page.
func TestWebRegistryOffersTheTransforms(t *testing.T) {
	web, err := WebRegistry().Names()
	if err != nil {
		t.Fatal(err)
	}
	registered := make(map[string]bool, len(web))
	for _, name := range web {
		registered[name] = true
	}

	for _, name := range []string{
		"base64_encode", "base64_decode", "hex_encode", "url_encode",
		"gzip_compress", "gzip_decompress",
		"md5", "sha256", "sha512", "hmac_sha256",
		"aes_encrypt", "xor", "entropy",
		"csv_parse", "xml_parse", "json_parse",
		"select_object", "sort_object", "group_object", "measure_object",
		"where_object", "format_table", "format_list",
		"get_command", "get_help",
	} {
		if !registered[name] {
			t.Errorf("web registry is missing %q", name)
		}
	}
}

// TestWebAliasesResolve checks the alias filtering the page relies on. The
// strict AliasFuncDefs refuses an alias whose target is absent - correct for
// the CLI, fatal for a curated registry - so KnownAliases has to drop exactly
// the ones that no longer name anything.
func TestWebAliasesResolve(t *testing.T) {
	reg := WebRegistry()

	// The unfiltered table must still be rejected, or the filtering is
	// pointless and the CLI's guard is not doing its job either.
	if _, err := reg.AliasFuncDefs(StandardAliases); err == nil {
		t.Error("the full alias table should not resolve against the web registry")
	}

	known, err := reg.KnownAliases(StandardAliases)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.AliasFuncDefs(known); err != nil {
		t.Fatalf("the filtered aliases should compile: %v", err)
	}

	names := make(map[string]bool, len(known))
	for _, alias := range known {
		names[alias.Name] = true
	}
	// ft and fl name formatters the page has; gci and gps name cmdlets it does
	// not.
	for _, want := range []string{"ft", "fl"} {
		if !names[want] {
			t.Errorf("alias %q should survive filtering", want)
		}
	}
	for _, gone := range []string{"gci", "dir", "gps", "gsv", "cd", "iwr"} {
		if names[gone] {
			t.Errorf("alias %q names a cmdlet the web registry excludes", gone)
		}
	}
}

// TestWebRegistryCatalogMatchesItself keeps get_help honest inside the page:
// it must describe the vocabulary the page has, not the CLI's.
func TestWebRegistryCatalogMatchesItself(t *testing.T) {
	reg := WebRegistry()
	names, err := reg.Names()
	if err != nil {
		t.Fatal(err)
	}
	registered := make(map[string]bool, len(names))
	for _, name := range names {
		registered[name] = true
	}

	catalog := webCatalog(reg)
	if len(catalog) == 0 {
		t.Fatal("the web catalog is empty; get_command would report nothing")
	}
	for _, cmd := range catalog {
		if !registered[cmd.Name] {
			t.Errorf("the web catalog lists %q, which is not registered there", cmd.Name)
		}
		for _, alias := range cmd.Aliases {
			if strings.TrimSpace(alias) == "" {
				t.Errorf("%s has a blank alias", cmd.Name)
			}
		}
	}
}
