// Package archtest guards the module boundaries of the API (see
// docs/architecture/adr-0001-modular-monolith.md).
//
// It has no production code: everything lives in _test.go files and runs as
// part of `go test ./...` (CI job api-unit). Two kinds of checks:
//
//   - import rules between internal/kernel, internal/modules/<name> and the
//     legacy internal/httpapi package;
//   - table ownership: every table belongs to exactly one module, and a SQL
//     string in module A that touches a table of module B is a boundary
//     violation. Existing violations are frozen in
//     testdata/cross_module_table_access.txt (a ratchet): new ones fail the
//     build, fixed ones must be removed from the file.
//
// Regenerate the baseline after an intentional change:
//
//	ARCHTEST_UPDATE_BASELINE=1 go test ./internal/archtest/
//
// Print the full report (module sizes, violations by count):
//
//	ARCHTEST_REPORT=1 go test ./internal/archtest/ -run TestReport -v
package archtest
