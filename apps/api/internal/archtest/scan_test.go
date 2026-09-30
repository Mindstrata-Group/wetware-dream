package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// apiRoot is apps/api relative to this package directory.
const apiRoot = "../.."

// sqlTableRef matches the identifier that follows a SQL keyword naming a
// table. Deliberately simple: it only has to be right for the tables listed
// in tableOwner, everything else (CTE names, functions) is ignored.
var sqlTableRef = regexp.MustCompile(`(?i)\b(?:from|join|into|update|table(?:\s+if\s+(?:not\s+)?exists)?|references|truncate)\s+(?:only\s+)?(?:"?public"?\.)?"?([a-z_][a-z0-9_]*)"?`)

// tablesInSQL returns the owned tables a SQL string touches, deduplicated.
func tablesInSQL(sql string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range sqlTableRef.FindAllStringSubmatch(sql, -1) {
		name := strings.ToLower(m[1])
		if _, owned := tableOwner[name]; owned && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// sourceFile is one non-test Go file of apps/api.
type sourceFile struct {
	rel     string // relative to apps/api, slash-separated
	module  string
	known   bool
	imports []string
	tables  map[string]int // owned table -> number of string literals touching it
	lines   int
}

var (
	scanOnce  sync.Once
	scanFiles []sourceFile
	scanErr   error
)

// scanSources parses every non-test Go file under apps/api once per test
// binary; the tests below share the result and run in parallel.
func scanSources(t *testing.T) []sourceFile {
	t.Helper()
	scanOnce.Do(func() { scanFiles, scanErr = doScan(apiRoot) })
	if scanErr != nil {
		t.Fatalf("scan sources: %v", scanErr)
	}
	return scanFiles
}

func doScan(root string) ([]sourceFile, error) {
	var out []sourceFile
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "testdata", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, p, src, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		sf := sourceFile{rel: rel, tables: map[string]int{}, lines: strings.Count(string(src), "\n")}
		sf.module, sf.known = moduleForFile(rel)
		for _, imp := range f.Imports {
			v, _ := strconv.Unquote(imp.Path.Value)
			sf.imports = append(sf.imports, v)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, tbl := range tablesInSQL(s) {
				sf.tables[tbl]++
			}
			return true
		})
		out = append(out, sf)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out, err
}

// createdTables lists every table created by the migrations and the test base
// schema, so a new table without an owner is caught at the migration.
func createdTables(t *testing.T) []string {
	t.Helper()
	re := regexp.MustCompile(`(?i)create\s+table\s+(?:if\s+not\s+exists\s+)?(?:"?public"?\.)?"?([a-z_][a-z0-9_]*)"?`)
	files, err := filepath.Glob(filepath.Join(apiRoot, "sql", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, filepath.Join(apiRoot, "internal", "testsupport", "schema_base.sql"))
	seen := map[string]bool{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			seen[strings.ToLower(m[1])] = true
		}
	}
	var out []string
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func TestTablesInSQL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{"select", "select id from users where id = $1", []string{"users"}},
		{"join", "select 1 from dialogs_messages m join users u on u.id = m.user_id", []string{"dialogs_messages", "users"}},
		{"insert", "INSERT INTO game_results (a) values ($1)", []string{"game_results"}},
		{"update", "update  promocodes set used = used + 1", []string{"promocodes"}},
		{"delete", "delete from auth_sessions where token_hash = $1", []string{"auth_sessions"}},
		{"schema qualified", `select * from public."site_media"`, []string{"site_media"}},
		{"ddl", "create table if not exists ai_gateways (id text)", []string{"ai_gateways"}},
		{"references", "user_id bigint references users(id)", []string{"users"}},
		{"unknown table ignored", "select * from generate_series(1, 3) as g", nil},
		{"cte name ignored", "with x as (select 1) select * from x", nil},
		{"word in prose ignored", "reads prompts and modes from the admin page", nil},
		{"dedup", "select 1 from users; select 2 from users", []string{"users"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tablesInSQL(tc.sql)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("tablesInSQL(%q) = %v, want %v", tc.sql, got, tc.want)
			}
		})
	}
}

func TestModuleForFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		rel, want string
		ok        bool
	}{
		{"internal/httpapi/game_session.go", "games", true},
		{"internal/httpapi/admin_ai_gateways.go", "aigateway", true},
		{"internal/httpapi/admin_promocode_update.go", "access", true},
		{"internal/httpapi/admin_dialog_handlers.go", "admin", true},
		{"internal/httpapi/chat_quota_prompt.go", "access", true},
		{"internal/httpapi/chat_send.go", "chat", true},
		{"internal/httpapi/handlers.go", "chat", true},
		{"internal/httpapi/brand_new_feature.go", "", false},
		{"internal/modules/games/store.go", "games", true},
		{"internal/modules/games/internal/scoring/x.go", "games", true},
		{"internal/kernel/httpjson/httpjson.go", "kernel", true},
		{"internal/billing/yookassa.go", "billing", true},
		{"main.go", "kernel", true},
		{"internal/unknownpkg/x.go", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.rel, func(t *testing.T) {
			t.Parallel()
			got, ok := moduleForFile(tc.rel)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("moduleForFile(%q) = %q,%v want %q,%v", tc.rel, got, ok, tc.want, tc.ok)
			}
		})
	}
}
