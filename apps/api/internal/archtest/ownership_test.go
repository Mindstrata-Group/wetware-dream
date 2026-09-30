package archtest

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const baselineFile = "testdata/cross_module_table_access.txt"

// violation is "module X touches table T owned by module Y".
type violation struct{ module, table, owner string }

func (v violation) key() string { return v.module + " " + v.table }

// crossModuleAccess aggregates violations over all scanned files:
// key -> number of SQL string literals, plus the files involved.
func crossModuleAccess(files []sourceFile) (map[string]int, map[string][]string, map[string]violation) {
	counts := map[string]int{}
	where := map[string][]string{}
	vs := map[string]violation{}
	for _, f := range files {
		if !f.known {
			continue
		}
		for tbl, n := range f.tables {
			owner := tableOwner[tbl]
			if owner == f.module {
				continue
			}
			v := violation{module: f.module, table: tbl, owner: owner}
			counts[v.key()] += n
			where[v.key()] = append(where[v.key()], f.rel)
			vs[v.key()] = v
		}
	}
	return counts, where, vs
}

func readBaseline(t *testing.T) map[string]bool {
	t.Helper()
	fh, err := os.Open(baselineFile)
	if err != nil {
		t.Fatalf("open baseline: %v", err)
	}
	defer fh.Close()
	out := map[string]bool{}
	sc := bufio.NewScanner(fh)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// "module table  # owner=..., N refs" — the key is the first two fields.
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Fatalf("bad baseline line %q", line)
		}
		out[fields[0]+" "+fields[1]] = true
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func writeBaseline(t *testing.T, counts map[string]int, vs map[string]violation) {
	t.Helper()
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("# Frozen cross-module table access (ratchet). See internal/archtest/doc.go.\n")
	b.WriteString("# Format: <module> <table>  # owner, number of SQL literals at generation time.\n")
	b.WriteString("# A new line here needs a reason in the PR; removing lines is always welcome.\n")
	for _, k := range keys {
		v := vs[k]
		fmt.Fprintf(&b, "%s %s  # owner=%s refs=%d\n", v.module, v.table, v.owner, counts[k])
	}
	if err := os.MkdirAll(filepath.Dir(baselineFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(baselineFile, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTableOwnership_NoNewCrossModuleAccess is the ratchet: a module may not
// start querying another module's table, and a fixed violation must be
// removed from the baseline so it cannot silently come back.
func TestTableOwnership_NoNewCrossModuleAccess(t *testing.T) {
	t.Parallel()
	counts, where, vs := crossModuleAccess(scanSources(t))
	if os.Getenv("ARCHTEST_UPDATE_BASELINE") == "1" {
		writeBaseline(t, counts, vs)
		t.Logf("baseline rewritten: %d entries", len(counts))
		return
	}
	base := readBaseline(t)
	var added, fixed []string
	for k := range counts {
		if !base[k] {
			v := vs[k]
			added = append(added, fmt.Sprintf("%s -> %s (owned by %s) in %s", v.module, v.table, v.owner, strings.Join(where[k], ", ")))
		}
	}
	for k := range base {
		if _, still := counts[k]; !still {
			fixed = append(fixed, k)
		}
	}
	sort.Strings(added)
	sort.Strings(fixed)
	if len(added) > 0 {
		t.Errorf("new cross-module table access (call the owning module's API instead, or justify and regenerate the baseline):\n  %s", strings.Join(added, "\n  "))
	}
	if len(fixed) > 0 {
		t.Errorf("baseline entries no longer needed; delete these lines from %s:\n  %s", baselineFile, strings.Join(fixed, "\n  "))
	}
}

// TestTableOwnership_EveryTableHasOwner: a migration that creates a table
// must say which module owns it.
func TestTableOwnership_EveryTableHasOwner(t *testing.T) {
	t.Parallel()
	var missing []string
	for _, name := range createdTables(t) {
		if _, ok := tableOwner[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("tables without an owner module; add them to tableOwner in ownership_map_test.go: %v", missing)
	}
}

// TestFileModule_EveryLegacyFileAssigned: every source file belongs to a
// module, so the ownership check never skips code.
func TestFileModule_EveryLegacyFileAssigned(t *testing.T) {
	t.Parallel()
	var missing []string
	for _, f := range scanSources(t) {
		if !f.known {
			missing = append(missing, f.rel)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("files without a module; add a rule to httpapiFileModule or packageModule in ownership_map_test.go: %v", missing)
	}
}

// TestCrossModuleAccess_DetectsViolation proves the ratchet can fail.
func TestCrossModuleAccess_DetectsViolation(t *testing.T) {
	t.Parallel()
	files := []sourceFile{
		{rel: "internal/modules/games/a.go", module: "games", known: true, tables: map[string]int{"game_results": 2, "users": 1}},
		{rel: "internal/modules/auth/b.go", module: "auth", known: true, tables: map[string]int{"users": 3}},
	}
	counts, where, vs := crossModuleAccess(files)
	if len(counts) != 1 || counts["games users"] != 1 {
		t.Fatalf("counts = %v, want only games->users", counts)
	}
	if vs["games users"].owner != "auth" || where["games users"][0] != "internal/modules/games/a.go" {
		t.Fatalf("unexpected violation detail: %+v %v", vs["games users"], where)
	}
}
