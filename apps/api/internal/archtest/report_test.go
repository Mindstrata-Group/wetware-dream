package archtest

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestReport prints module sizes and the cross-module table access list,
// sorted by number of SQL literals. Only runs with ARCHTEST_REPORT=1; used to
// keep docs/architecture/modules.md honest.
func TestReport(t *testing.T) {
	t.Parallel()
	if os.Getenv("ARCHTEST_REPORT") != "1" {
		t.Skip("set ARCHTEST_REPORT=1 to print the report")
	}
	files := scanSources(t)

	type size struct{ files, lines int }
	sizes := map[string]*size{}
	for _, f := range files {
		s := sizes[f.module]
		if s == nil {
			s = &size{}
			sizes[f.module] = s
		}
		s.files++
		s.lines += f.lines
	}
	mods := make([]string, 0, len(sizes))
	for m := range sizes {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	var b strings.Builder
	b.WriteString("module\tfiles\tlines\towned_tables\n")
	owned := map[string]int{}
	for _, m := range tableOwner {
		owned[m]++
	}
	for _, m := range mods {
		fmt.Fprintf(&b, "%s\t%d\t%d\t%d\n", m, sizes[m].files, sizes[m].lines, owned[m])
	}

	counts, where, vs := crossModuleAccess(files)
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	total := 0
	for _, k := range keys {
		total += counts[k]
	}
	fmt.Fprintf(&b, "\ncross-module pairs=%d literals=%d\nmodule\ttable\towner\trefs\tfiles\n", len(keys), total)
	for _, k := range keys {
		v := vs[k]
		fmt.Fprintf(&b, "%s\t%s\t%s\t%d\t%d\n", v.module, v.table, v.owner, counts[k], len(where[k]))
	}

	byOwner := map[string]int{}
	byReader := map[string]int{}
	for _, k := range keys {
		byOwner[vs[k].owner] += counts[k]
		byReader[vs[k].module] += counts[k]
	}
	fmt.Fprintf(&b, "\nby owner: %v\nby reader: %v\n", byOwner, byReader)
	t.Log("\n" + b.String())
}
