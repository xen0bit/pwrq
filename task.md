# pwrq PowerShell Port - Production Readiness Plan

## Executive Summary

**Current State:**
- PSObject type system implemented (`pkg/core/psobject/`) - 576 LOC
- Session state implemented (`pkg/core/sessionstate/`) - 601 LOC  
- PowerShell cmdlets partially ported (filesystem, objects, formatting)
- **CRITICAL:** PowerShell UDFs NOT registered in CLI - cannot be used
- **CRITICAL:** Session state NOT integrated into CLI pipeline
- **CRITICAL:** No integration tests for end-to-end user flows

**Goal:** Transform pwrq from prototype to production-ready PowerShell-style JSON processor

---

## Phase 1: Critical Integration (Week 1-2)

### 1.1 Register PowerShell UDFs in CLI

**Problem:** PowerShell cmdlets exist but are not accessible from CLI

**Files to modify:**
- `pkg/udf/registry.go` - Add PowerShell UDF registration

**Implementation:**
```go
// Add imports
import (
    "github.com/xen0bit/pwrq/pkg/udf/powershell/filesystem"
    "github.com/xen0bit/pwrq/pkg/udf/powershell/objects"
    "github.com/xen0bit/pwrq/pkg/udf/powershell/formatting"
)

// In DefaultRegistry():
func DefaultRegistry() *Registry {
    reg := NewRegistry()
    
    // ... existing registrations ...
    
    // Register PowerShell cmdlets
    for _, opt := range filesystem.RegisterAll() {
        reg.Register(opt)
    }
    for _, opt := range objects.RegisterAll() {
        reg.Register(opt)
    }
    for _, opt := range formatting.RegisterAll() {
        reg.Register(opt)
    }
    
    return reg
}
```

**Acceptance Criteria:**
- [ ] `./pwrq 'get_childitem(".")'` returns directory listing
- [ ] `./pwrq 'select_object("Name")'` works in pipeline
- [ ] `./pwrq --udf-list` shows all PowerShell cmdlets
- [ ] All existing tests pass

### 1.2 Integrate Session State into CLI

**Problem:** Session state exists but is never initialized or used

**Files to modify:**
- `cli/cli.go` - Initialize and pass session state to UDFs
- `pkg/udf/common/util.go` - Add session state helpers

**Implementation:**
```go
// In cli/cli.go
type cli struct {
    // Existing fields...
    sessionState *sessionstate.SessionState
}

func (cli *cli) runInternal(args []string) (err error) {
    // Initialize session state after flag parsing
    cli.sessionState = sessionstate.NewSessionState()
    
    // Load standard modules (aliases, etc.)
    cli.loadStandardAliases()
    
    // ... rest of existing code ...
}

func (cli *cli) loadStandardAliases() {
    // PowerShell standard aliases
    cli.sessionState.SetAlias("gci", "get_childitem")
    cli.sessionState.SetAlias("ls", "get_childitem")
    cli.sessionState.SetAlias("dir", "get_childitem")
    cli.sessionState.SetAlias("select", "select_object")
    cli.sessionState.SetAlias("where", "where_object")
    cli.sessionState.SetAlias("sort", "sort_object")
    cli.sessionState.SetAlias("group", "group_object")
    cli.sessionState.SetAlias("measure", "measure_object")
    cli.sessionState.SetAlias("fl", "format_list")
    cli.sessionState.SetAlias("ft", "format_table")
}
```

**Acceptance Criteria:**
- [ ] Session state persists across pipeline stages
- [ ] Aliases resolve correctly
- [ ] Variable drive accessible via `$VARIABLE:varname`
- [ ] Environment drive accessible via `$ENV:PATH`

### 1.3 Create Pipeline Infrastructure

**Problem:** No cmdlet base class or parameter binding system

**New files to create:**
- `pkg/core/pipeline/cmdlet.go` - Base cmdlet class
- `pkg/core/pipeline/parameter.go` - Parameter binding
- `pkg/core/pipeline/output.go` - Write-Object methods
- `pkg/core/pipeline/context.go` - Pipeline context

**Implementation:**
```go
// pkg/core/pipeline/cmdlet.go
package pipeline

import (
    "github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

// CmdletBase provides common cmdlet functionality
type CmdletBase struct {
    SessionState *sessionstate.SessionState
    PipelineInput any
    OutputWriter func(any)
    ErrorWriter func(error)
}

// WriteObject writes an object to the pipeline output
func (c *CmdletBase) WriteObject(obj any, enumerate bool) {
    if c.OutputWriter != nil {
        c.OutputWriter(obj)
    }
}

// WriteError writes an error to the error stream
func (c *CmdletBase) WriteError(err error) {
    if c.ErrorWriter != nil {
        c.ErrorWriter(err)
    }
}

// Cmdlet interface for all cmdlets
type Cmdlet interface {
    BeginProcessing()
    ProcessRecord(input any) any
    EndProcessing()
}
```

**Acceptance Criteria:**
- [ ] CmdletBase compiles and passes tests
- [ ] Parameter binding works with struct tags
- [ ] WriteObject/WriteError methods functional

---

## Phase 2: Complete Core Cmdlets (Week 3-4)

### 2.1 Variable Management Cmdlets

**New files:**
- `pkg/udf/powershell/variables/set_variable.go`
- `pkg/udf/powershell/variables/get_variable.go`
- `pkg/udf/powershell/variables/remove_variable.go`
- `pkg/udf/powershell/variables/registry.go`

**Cmdlets:**
| Cmdlet | Function | Status |
|--------|----------|--------|
| `Set-Variable` | `set_variable()` | TODO |
| `Get-Variable` | `get_variable()` | TODO |
| `Remove-Variable` | `remove_variable()` | TODO |
| `New-Variable` | `new_variable()` | TODO |
| `Clear-Variable` | `clear_variable()` | TODO |

**Example usage:**
```bash
# Set variable
./pwrq 'set_variable("count"; 42)'

# Get variable (in same session)
./pwrq 'set_variable("name"; "test") | get_variable("name")'

# List all variables
./pwrq 'get_variable("*")'
```

### 2.2 Location Management Cmdlets

**New files:**
- `pkg/udf/powershell/location/set_location.go`
- `pkg/udf/powershell/location/get_location.go`
- `pkg/udf/powershell/location/push_location.go`
- `pkg/udf/powershell/location/pop_location.go`

**Cmdlets:**
| Cmdlet | Function | Status |
|--------|----------|--------|
| `Set-Location` | `set_location()` / `cd()` | TODO |
| `Get-Location` | `get_location()` / `pwd()` | TODO |
| `Push-Location` | `push_location()` / `pushd()` | TODO |
| `Pop-Location` | `pop_location()` / `popd()` | TODO |

### 2.3 Enhanced File System Cmdlets

**Enhance existing:**
- `pkg/udf/powershell/filesystem/get_childitem.go` - Add -Recurse, -Filter, -Include, -Exclude
- `pkg/udf/file/cat.go` - Add -Tail, -TotalCount, -Encoding
- `pkg/udp/file/rm.go` - Add -Recurse, -Force

**New files:**
- `pkg/udf/powershell/filesystem/copy_item.go`
- `pkg/udf/powershell/filesystem/move_item.go`
- `pkg/udf/powershell/filesystem/new_item.go`
- `pkg/udf/powershell/filesystem/resolve_path.go`
- `pkg/udf/powershell/filesystem/split_path.go`

### 2.4 Process & Service Cmdlets

**New files:**
- `pkg/udf/powershell/process/get_process.go`
- `pkg/udf/powershell/process/stop_process.go`
- `pkg/udf/powershell/process/start_process.go`
- `pkg/udf/powershell/service/get_service.go`
- `pkg/udf/powershell/service/start_service.go`
- `pkg/udf/powershell/service/stop_service.go`

**Linux implementation notes:**
- `Get-Process` → `/proc` filesystem or `ps` command
- `Get-Service` → systemd DBus or `systemctl` command

### 2.5 Web & Network Cmdlets

**Enhance existing:**
- `pkg/udf/http/http.go` - Add full parameter support (Method, Headers, Body, Timeout)

**New files:**
- `pkg/udf/powershell/web/invoke_rest_method.go`
- `pkg/udf/powershell/web/invoke_web_request.go`
- `pkg/udf/powershell/web/test_connection.go`

### 2.6 Date & Time Cmdlets

**New files:**
- `pkg/udf/powershell/datetime/get_date.go`
- `pkg/udf/powershell/datetime/set_date.go`
- `pkg/udf/powershell/datetime/new_timespan.go`

---

## Phase 3: Data Manipulation & Formatting (Week 5-6)

### 3.1 Import/Export Cmdlets

**New files:**
- `pkg/udf/powershell/data/export_csv.go`
- `pkg/udf/powershell/data/import_csv.go`
- `pkg/udf/powershell/data/export_json.go`
- `pkg/udf/powershell/data/import_json.go`
- `pkg/udf/powershell/data/convertto_json.go`
- `pkg/udf/powershell/data/convertfrom_json.go`
- `pkg/udf/powershell/data/convertto_csv.go`
- `pkg/udf/powershell/data/convertfrom_csv.go`

### 3.2 Enhanced Formatting

**Enhance existing:**
- `pkg/udf/powershell/formatting/format_list.go` - Add property filtering
- `pkg/udf/powershell/formatting/format_table.go` - Add auto-column sizing

**New files:**
- `pkg/udf/powershell/formatting/format_wide.go`
- `pkg/udf/powershell/formatting/out_file.go`
- `pkg/udf/powershell/formatting/out_string.go`
- `pkg/udf/powershell/formatting/format_hex.go`

### 3.3 Output Cmdlets

**New files:**
- `pkg/udf/powershell/output/write_host.go` - Colored console output
- `pkg/udf/powershell/output/write_verbose.go` - Verbose output
- `pkg/udf/powershell/output/write_warning.go` - Warning output
- `pkg/udf/powershell/output/write_error.go` - Error output
- `pkg/udf/powershell/output/write_debug.go` - Debug output
- `pkg/udf/powershell/output/write_information.go` - Information output
- `pkg/udf/powershell/output/write_progress.go` - Progress bars

---

## Phase 4: Module System & Discovery (Week 7-8)

### 4.1 Module Loader

**New files:**
- `pkg/modules/loader.go` - Module loading infrastructure
- `pkg/modules/manifest.go` - Module manifest parsing
- `pkg/modules/standard/management.go` - Microsoft.PowerShell.Management
- `pkg/modules/standard/utility.go` - Microsoft.PowerShell.Utility
- `pkg/modules/standard/security.go` - Microsoft.PowerShell.Security

**Cmdlets:**
| Cmdlet | Function | Status |
|--------|----------|--------|
| `Import-Module` | `import_module()` | TODO |
| `Get-Module` | `get_module()` | TODO |
| `Remove-Module` | `remove_module()` | TODO |
| `New-Module` | `new_module()` | TODO |
| `Test-ModuleManifest` | `test_module_manifest()` | TODO |

### 4.2 Command Discovery

**New files:**
- `pkg/core/discovery/command.go` - Command metadata
- `pkg/core/discovery/loader.go` - Command loading
- `pkg/core/discovery/alias.go` - Alias management

**Cmdlets:**
| Cmdlet | Function | Status |
|--------|----------|--------|
| `Get-Command` | `get_command()` | TODO |
| `Get-Help` | `get_help()` | TODO |

---

## Phase 5: Error Handling & Advanced Features (Week 9-10)

### 5.1 Error Handling

**New files:**
- `pkg/udf/powershell/error/get_error.go`
- `pkg/udf/powershell/error/clear_error.go`
- `pkg/udf/powershell/error/resolve_error.go`

**Variables:**
- `$ERROR` - Error collection array
- `$?` - Last command success status
- `$LASTEXITCODE` - Last native command exit code

### 5.2 History

**New files:**
- `pkg/udf/powershell/history/get_history.go`
- `pkg/udf/powershell/history/invoke_history.go`

### 5.3 Security (Optional)

**New files:**
- `pkg/udf/powershell/security/get_execution_policy.go`
- `pkg/udf/powershell/security/set_execution_policy.go`
- `pkg/udf/powershell/security/get_credential.go`
- `pkg/udf/powershell/security/protect_cms_message.go`
- `pkg/udf/powershell/security/unprotect_cms_message.go`

---

## Phase 6: Testing & Quality (Week 11-12)

### 6.1 Unit Test Coverage

**Goal:** 80%+ code coverage

**Actions:**
- [ ] Add tests for all new cmdlets
- [ ] Test edge cases (nil input, empty arrays, errors)
- [ ] Test parameter binding scenarios
- [ ] Test type conversion edge cases

### 6.2 Integration Tests

**New directory:** `test/integration/`

**Test files:**
- `test/integration/pipeline_test.go` - Pipeline chaining
- `test/integration/variables_test.go` - Variable persistence
- `test/integration/aliases_test.go` - Alias resolution
- `test/integration/filesystem_test.go` - File operations
- `test/integration/formatting_test.go` - Output formatting

**Example integration test:**
```go
// test/integration/pipeline_test.go
func TestPipelineChaining(t *testing.T) {
    result := RunQuery(`
        get_childitem(".") | 
        select_object("Name", "Length") | 
        sort_object("Length" | desc) |
        .[0:5]
    `)
    
    if len(result) == 0 {
        t.Fatal("Expected results from pipeline")
    }
    
    // Verify each result has expected properties
    for _, item := range result {
        psobj, err := psobject.FromMap(item.(map[string]any))
        if err != nil {
            t.Fatalf("Failed to parse PSObject: %v", err)
        }
        if psobj.TypeName != "System.Management.Automation.PSObject" {
            t.Errorf("Expected PSObject type, got %s", psobj.TypeName)
        }
    }
}
```

### 6.3 Pester-Style Tests

**New directory:** `test/pester/`

**Framework:** Go implementation of Pester BDD syntax

**Example:**
```go
// test/pester/filesystem_test.go
func TestGetChildItem(t *testing.T) {
    Describe("Get-ChildItem", func() {
        It("returns files in directory", func() {
            result := RunQuery(`get_childitem("pkg/udf")`)
            Expect(result).ToNot(BeEmpty())
        })
        
        It("filters by extension", func() {
            result := RunQuery(`
                get_childitem("pkg/udf") | 
                where_object { ._val | endswith(".go") }
            `)
            for _, item := range result {
                Expect(item.(map[string]any)["_val"]).To(HaveSuffix(".go"))
            }
        })
        
        It("supports -Recurse flag", func() {
            result := RunQuery(`get_childitem("pkg"; {"Recurse": true})`)
            Expect(len(result)).To(BeNumerically(">", 10))
        })
    })
}
```

### 6.4 CLI End-to-End Tests

**New directory:** `test/e2e/`

**Test files:**
- `test/e2e/basic_test.go` - Basic query execution
- `test/e2e/variables_test.go` - Variable assignment/retrieval
- `test/e2e/pipeline_test.go` - Complex pipelines
- `test/e2e/error_handling_test.go` - Error scenarios

**Example:**
```go
// test/e2e/basic_test.go
func TestBasicQuery(t *testing.T) {
    cmd := exec.Command("pwrq", ".foo")
    cmd.Stdin = strings.NewReader(`{"foo": "bar"}`)
    output, err := cmd.Output()
    
    if err != nil {
        t.Fatalf("Command failed: %v", err)
    }
    
    expected := "\"bar\"\n"
    if string(output) != expected {
        t.Errorf("Expected %q, got %q", expected, string(output))
    }
}
```

### 6.5 Performance Benchmarks

**New directory:** `test/benchmark/`

**Benchmarks:**
- Pipeline throughput (objects/second)
- Memory usage for large inputs
- Startup time
- Comparison with jq/gojq

---

## Phase 7: Documentation (Week 13)

### 7.1 User Documentation

**New directory:** `docs/`

**Files:**
- `docs/getting-started.md` - Quick start guide
- `docs/powershell-compatibility.md` - Compatibility matrix
- `docs/examples.md` - Usage examples
- `docs/faq.md` - Frequently asked questions

### 7.2 Cmdlet Reference

**New directory:** `docs/cmdlets/`

**Files (one per cmdlet):**
- `docs/cmdlets/get-childitem.md`
- `docs/cmdlets/select-object.md`
- `docs/cmdlets/where-object.md`
- ... (all cmdlets)

**Template:**
```markdown
# Get-ChildItem

## Synopsis
Gets the items at a specified location.

## Syntax
```
get_childitem(<path>; <options>)
```

## Parameters
- `path` (string, required): Path to enumerate
- `options` (object, optional):
  - `Recurse` (bool): Include subdirectories
  - `Filter` (string): Wildcard filter
  - `Include` (array): Include patterns
  - `Exclude` (array): Exclude patterns

## Examples
```bash
# List directory contents
./pwrq 'get_childitem(".")'

# Recursive listing
./pwrq 'get_childitem("src"; {"Recurse": true})'

# Filter by extension
./pwrq 'get_childitem("src"; {"Filter": "*.go"})'
```

## See Also
- `Test-Path`
- `Get-Content`
- `Set-Location`
```

### 7.3 Developer Documentation

**Files:**
- `docs/developer/adding-udfs.md` - How to add new UDFs
- `docs/developer/cmdlet-pattern.md` - Cmdlet implementation pattern
- `docs/developer/testing.md` - Testing guidelines
- `docs/developer/architecture.md` - Architecture overview

---

## Phase 8: Polish & Release (Week 14-16)

### 8.1 Performance Optimization

**Actions:**
- [ ] Profile CPU usage with `go tool pprof`
- [ ] Profile memory allocation
- [ ] Optimize hot paths (object creation, type conversion)
- [ ] Add caching for expensive operations

### 8.2 Error Message Improvement

**Actions:**
- [ ] User-friendly error messages
- [ ] Suggest fixes for common errors
- [ ] Add error codes for documentation lookup

### 8.3 Release Preparation

**Checklist:**
- [ ] All tests passing
- [ ] Coverage >= 80%
- [ ] Documentation complete
- [ ] Changelog updated
- [ ] Version bumped
- [ ] Release notes written
- [ ] Binary builds for Linux/macOS/Windows

---

## Appendix A: Complete Cmdlet Inventory

### P0 - Foundation (Must Have)

#### File System (13 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Get-ChildItem` | `get_childitem()` | filesystem/get_childitem.go | ✅ |
| `Get-Content` | `cat()` (enhance) | file/cat.go | 🔄 |
| `Set-Content` | `set_content()` | filesystem/set_content.go | ✅ |
| `Add-Content` | `add_content()` | filesystem/add_content.go | TODO |
| `Copy-Item` | `copy_item()` | filesystem/copy_item.go | TODO |
| `Move-Item` | `move_item()` | filesystem/move_item.go | TODO |
| `Remove-Item` | `rm()` (enhance) | file/rm.go | 🔄 |
| `New-Item` | `new_item()` | filesystem/new_item.go | TODO |
| `Test-Path` | `test_path()` | filesystem/test_path.go | ✅ |
| `Resolve-Path` | `resolve_path()` | filesystem/resolve_path.go | TODO |
| `Split-Path` | `split_path()` | filesystem/split_path.go | TODO |
| `Join-Path` | `join_path()` | filesystem/join_path.go | ✅ |
| `Push/Pop-Location` | `pushd/popd()` | location/*.go | TODO |

#### Variables (5 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Set-Variable` | `set_variable()` | variables/set_variable.go | TODO |
| `Get-Variable` | `get_variable()` | variables/get_variable.go | TODO |
| `Remove-Variable` | `remove_variable()` | variables/remove_variable.go | TODO |
| `New-Variable` | `new_variable()` | variables/new_variable.go | TODO |
| `Clear-Variable` | `clear_variable()` | variables/clear_variable.go | TODO |

#### Objects (5 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Select-Object` | `select_object()` | objects/select_object.go | ✅ |
| `Where-Object` | `where_object()` | objects/where_object.go | ✅ |
| `Sort-Object` | `sort_object()` | objects/sort_object.go | ✅ |
| `Group-Object` | `group_object()` | objects/group_object.go | ✅ |
| `Measure-Object` | `measure_object()` | objects/measure_object.go | ✅ |

#### Formatting (3 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Format-List` | `format_list()` | formatting/format_list.go | ✅ |
| `Format-Table` | `format_table()` | formatting/format_table.go | ✅ |
| `Format-Wide` | `format_wide()` | formatting/format_wide.go | TODO |

### P1 - Core Functionality (Should Have)

#### Data (8 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `ConvertTo-Json` | `convertto_json()` | data/convertto_json.go | TODO |
| `ConvertFrom-Json` | `convertfrom_json()` | data/convertfrom_json.go | TODO |
| `ConvertTo-Csv` | `convertto_csv()` | data/convertto_csv.go | TODO |
| `ConvertFrom-Csv` | `convertfrom_csv()` | data/convertfrom_csv.go | TODO |
| `Export-Csv` | `export_csv()` | data/export_csv.go | TODO |
| `Import-Csv` | `import_csv()` | data/import_csv.go | TODO |
| `Export-Json` | `export_json()` | data/export_json.go | TODO |
| `Import-Json` | `import_json()` | data/import_json.go | TODO |

#### Process (3 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Get-Process` | `get_process()` | process/get_process.go | TODO |
| `Stop-Process` | `stop_process()` | process/stop_process.go | TODO |
| `Start-Process` | `start_process()` | process/start_process.go | TODO |

#### Service (3 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Get-Service` | `get_service()` | service/get_service.go | TODO |
| `Start-Service` | `start_service()` | service/start_service.go | TODO |
| `Stop-Service` | `stop_service()` | service/stop_service.go | TODO |

#### Web (3 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Invoke-WebRequest` | `invoke_web_request()` | web/invoke_web_request.go | TODO |
| `Invoke-RestMethod` | `invoke_rest_method()` | web/invoke_rest_method.go | TODO |
| `Test-Connection` | `test_connection()` | web/test_connection.go | TODO |

#### DateTime (3 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Get-Date` | `get_date()` | datetime/get_date.go | TODO |
| `Set-Date` | `set_date()` | datetime/set_date.go | TODO |
| `New-TimeSpan` | `new_timespan()` | datetime/new_timespan.go | TODO |

#### Output (8 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Write-Host` | `write_host()` | output/write_host.go | TODO |
| `Write-Verbose` | `write_verbose()` | output/write_verbose.go | TODO |
| `Write-Warning` | `write_warning()` | output/write_warning.go | TODO |
| `Write-Error` | `write_error()` | output/write_error.go | TODO |
| `Write-Debug` | `write_debug()` | output/write_debug.go | TODO |
| `Write-Information` | `write_info()` | output/write_information.go | TODO |
| `Write-Progress` | `write_progress()` | output/write_progress.go | TODO |
| `Out-File` | `out_file()` | output/out_file.go | TODO |

### P2 - Advanced (Nice to Have)

#### Modules (5 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Import-Module` | `import_module()` | modules/import_module.go | TODO |
| `Get-Module` | `get_module()` | modules/get_module.go | TODO |
| `Remove-Module` | `remove_module()` | modules/remove_module.go | TODO |
| `New-Module` | `new_module()` | modules/new_module.go | TODO |
| `Test-ModuleManifest` | `test_module_manifest()` | modules/test_module_manifest.go | TODO |

#### Discovery (2 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Get-Command` | `get_command()` | discovery/get_command.go | TODO |
| `Get-Help` | `get_help()` | discovery/get_help.go | TODO |

#### Error (3 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Get-Error` | `get_error()` | error/get_error.go | TODO |
| `Clear-Error` | `clear_error()` | error/clear_error.go | TODO |
| `Resolve-Error` | `resolve_error()` | error/resolve_error.go | TODO |

#### History (2 cmdlets)
| Cmdlet | Function | File | Status |
|--------|----------|------|--------|
| `Get-History` | `get_history()` | history/get_history.go | TODO |
| `Invoke-History` | `invoke_history()` | history/invoke_history.go | TODO |

---

## Appendix B: File Structure

### Current Structure
```
pwrq/
├── pkg/
│   ├── core/
│   │   ├── psobject/          # ✅ Complete
│   │   └── sessionstate/      # ✅ Complete
│   ├── udf/
│   │   ├── common/            # ✅ Partial
│   │   ├── powershell/
│   │   │   ├── filesystem/    # 🔄 Partial (5/13 cmdlets)
│   │   │   ├── objects/       # ✅ Complete (5/5 cmdlets)
│   │   │   └── formatting/    # 🔄 Partial (2/3 cmdlets)
│   │   └── registry.go        # ❌ Missing PowerShell registration
│   └── cli/
│       └── cli.go             # ❌ Missing session state init
```

### Target Structure
```
pwrq/
├── cmd/
│   └── pwrq/
├── pkg/
│   ├── cli/
│   ├── core/
│   │   ├── psobject/
│   │   ├── sessionstate/
│   │   ├── pipeline/          # NEW
│   │   └── discovery/         # NEW
│   ├── udf/
│   │   ├── common/
│   │   ├── powershell/
│   │   │   ├── filesystem/    # 13 cmdlets
│   │   │   ├── variables/     # NEW - 5 cmdlets
│   │   │   ├── objects/
│   │   │   ├── formatting/
│   │   │   ├── location/      # NEW - 4 cmdlets
│   │   │   ├── process/       # NEW - 3 cmdlets
│   │   │   ├── service/       # NEW - 3 cmdlets
│   │   │   ├── web/           # NEW - 3 cmdlets
│   │   │   ├── datetime/      # NEW - 3 cmdlets
│   │   │   ├── data/          # NEW - 8 cmdlets
│   │   │   ├── output/        # NEW - 8 cmdlets
│   │   │   ├── error/         # NEW - 3 cmdlets
│   │   │   └── history/       # NEW - 2 cmdlets
│   │   └── registry.go
│   └── modules/               # NEW
├── test/
│   ├── unit/
│   ├── integration/           # NEW
│   ├── pester/                # NEW
│   ├── e2e/                   # NEW
│   └── benchmark/             # NEW
└── docs/                      # NEW
    ├── cmdlets/
    ├── developer/
    └── examples.md
```

---

## Appendix C: Success Metrics

### Code Quality
- [ ] 80%+ test coverage
- [ ] Zero linting errors
- [ ] All benchmarks passing

### Functionality
- [ ] All P0 cmdlets implemented and tested
- [ ] 80% of P1 cmdlets implemented
- [ ] Session state working end-to-end
- [ ] Module system functional

### User Experience
- [ ] CLI help complete for all cmdlets
- [ ] Error messages actionable
- [ ] Documentation comprehensive
- [ ] Examples working

### Performance
- [ ] Startup time < 50ms
- [ ] Pipeline throughput > 10K objects/sec
- [ ] Memory efficient (no leaks)

---

## Appendix D: Risk Mitigation

### Technical Risks

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Session state integration breaks existing UDFs | High | Medium | Comprehensive regression tests |
| Performance degradation with PSObject wrapping | High | Medium | Benchmark early, optimize hot paths |
| Parameter binding complexity | Medium | High | Start simple, iterate |
| Cross-platform file path issues | Medium | Medium | Use `filepath` package, test on all platforms |

### Schedule Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Scope creep (too many cmdlets) | High | Strict P0/P1/P2 prioritization |
| Test coverage lagging | Medium | Write tests alongside features |
| Documentation debt | Medium | Document as you go |

---

## Appendix E: Quick Start Commands

### Verify Phase 1 Complete
```bash
# PowerShell cmdlets accessible
./pwrq 'get_childitem(".")'
./pwrq 'select_object("Name")'
./pwrq 'where_object { . | type == "file" }'

# Aliases working
./pwrq 'ls | select Name'
./pwrq 'gci -Recurse | where Extension -eq ".go"'

# Variables working
./pwrq 'set_variable("x"; 42) | get_variable("x")'
```

### Verify Testing Complete
```bash
# All tests pass
make test

# Coverage report
make test-coverage
open coverage.html

# Integration tests
go test -v ./test/integration/...
go test -v ./test/pester/...
go test -v ./test/e2e/...
```

---

**This plan provides a comprehensive roadmap from current state to production-ready release. Each phase builds on the previous, with clear acceptance criteria and deliverables.**
