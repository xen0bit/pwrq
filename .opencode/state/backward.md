# Backward Agent State

## Session Info
- Mode: Bootstrap
- Bootstrap Completed: Wed Apr 29 2026
- Revision Count: 0

## Persona Chosen
- Name: Rob Pike
- Rationale: Go systems programming task requiring Unix tool design principles for a PowerShell-compatible CLI processor - direct domain match.

## Revision History
- Phase 1.3: Create Pipeline Infrastructure | pkg/core/pipeline/parameter.go,context.go,output.go | BindParameters ignores positionMap (positional params broken), output writers don't check cancellation, TableFormatter has non-deterministic column order | 2026-04-29
- Phase 1.2: Integrate Session State into CLI | cli/cli.go | Session state created but never passed to UDFs, no preference variables, missing aliases, no tests | 2026-04-29
- Phase 1.1: Register PowerShell UDFs in CLI | pkg/udf/registry.go | Encoder panics on time.Time, select_object positional param parsing broken, no integration tests | 2026-04-29
- Phase 2.1: Variable Management Cmdlets | pkg/udf/powershell/variables/*.go | Critical Name->Scope bug, incomplete scope chain traversal, naive wildcard matching, missing protection tests | 2026-04-29
- Phase 2.1: Variable Management Cmdlets | pkg/udf/powershell/variables/utils.go, get_variable.go, remove_variable.go, variables_test.go | variableOptionsToString didn't handle bitmask combinations, deprecated strings.Title in scope filtering | 2026-04-29
- Phase 2.2: Location Management Cmdlets | pkg/udf/powershell/location/*.go | Global mutable state breaks test isolation, StackName options defined but never used, internal functions lack sessionState parameter, tests manipulate global state directly | 2026-04-29
- Phase 2.3: Enhanced File System Cmdlets | pkg/udf/powershell/filesystem/copy_item.go | Filter/Include/Exclude not applied to single files, dead code in matchPattern, no case-insensitive wildcard matching, Force doesn't handle read-only files, no timestamp preservation | 2026-04-29
- Phase 2.4: Process & Service Cmdlets | pkg/udf/powershell/process/get_process.go, service/service.go | ps field mapping wrong (hardcoded values), custom wildcard reinvents filepath.Match, systemctl O(N) calls, Windows parsers incomplete, Force param unused | 2026-04-29
- Phase 2.5: Web & Network Cmdlets | pkg/udf/powershell/web/test_connection.go | defer cancel() inside loops causing resource leaks, dialer recreated per iteration | 2026-04-29
- Phase 2.6: Date & Time Cmdlets | pkg/udf/powershell/datetime/*.go | Date component overrides can't set zero values, TimeSpan days calculation loses precision, formatDuration uses wrong precision, set_date stub has no tests | 2026-04-29
