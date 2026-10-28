# MatrixOrigin Linter

This repo contains custom linters developed by MatrixOrigin. Building requires
Go 1.24 or later. CI consumers should pin a reviewed commit.

## Installation and usage

```sh
go install github.com/matrixorigin/linter/cmd/molint@latest
cd matrixone
molint check -mod=readonly ./...
```

`molint check` runs all four checks, including internal and external test source.
Go build flags precede package arguments. `GOFLAGS` is honored; command-line
values override it. Build tags, modules, vendor mode, replacements, workspaces,
CGo settings, and overlays use the same Go configuration in both phases. Use
`molint check -C directory ...` to change directory before resolving inputs.

Exit status is **0** for clean, **1** for findings, and **2** for usage or execution
errors. `-json` writes one object with `diagnostics` (package → analyzer → array
of `posn`/`message`) and an optional `error`. Findings still exit 1 in JSON mode.

Analyzer selection follows `go vet`: `-checkrecover` enables only that analyzer;
`-checkrecover=false` disables it while retaining the other defaults. Compilation
and type errors remain fatal even with only import checks enabled.

### Migration from the vettool entry

Use `molint check` instead of `go vet -vettool=molint`. The old invocation is
rejected before Go can reuse a cached partial result. The exported
`pkgblocklist.Analyzer` and `checkunsafe.Analyzer` entry points are removed;
these policies require the command's selected source and module graph.

The command asks `go list` once for the selected packages and their dependencies.
Import checks run freshly against that graph and the original source. Typed
checks (`checkrecover`, `checklog`) continue to use Go's vet cache. A module
mapping change can therefore neither retain a clean result nor retain an obsolete
finding. There is no nested `go/packages` loader or separate metadata cache.

`GOPACKAGESDRIVER` is intentionally not used: this command uses the same Go
driver as `go vet`, rather than combining two potentially different build graphs.
Selected SWIG inputs are rejected; their generated Go source is not supported.
`-n`, `-fix`, `-diff`, and flags not listed in `molint check -h` are rejected.
Build inputs, including overlay backing files, must remain stable during a run,
as with an ordinary Go build; the command does not create an atomic snapshot.

## Checks

### checkrecover

As required by the [error handling rules](https://github.com/matrixorigin/matrixone/blob/main/pkg/common/moerr/error_handling.md),
recover is allowed only at explicitly approved function boundaries. Tests are
exempt. Recognition uses the built-in function's type binding, including
parentheses; an unrelated function named `recover` is not a violation.

### pkgblocklist

Reject disallowed package paths and the actual logical module owning an import.
A replacement changes its code location, not its module identity. Unused required
modules and imports of unselected dependency packages are not checked.

### checkunsafe

Check authored `unsafe` imports against the exact file allowlist. CGo's generated
support imports are not authored imports; handwritten imports in a CGo file are
still checked, including raw and escaped string literals. Existing CGo file
approvals retain their historical `<source>.cgo1.go` keys. Generated source files
on disk are checked like other selected Go files.

### checklog

Check `log` imports and `fmt.Print*` calls against the existing approvals. Unit
test source is exempt under this analyzer's existing policy.
