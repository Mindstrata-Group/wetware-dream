package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The hygiene gate exempts only what must hold real-looking personal data:
// the PII detector's own test corpus and the published maintainer contacts.
// Everything else, including the detector's source code, is still checked.
func TestSkipFileExemptions(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"apps/anonymizer/tests/gold/gold_ru.jsonl":              true,
		"apps/anonymizer/tests/test_rules.py":                   true,
		"apps/api/internal/pseudonym/service_test.go":           true,
		"apps/api/internal/pseudonym/vault_integration_test.go": true,
		"COMMERCIAL.md":                          true,
		"GOVERNANCE.md":                          true,
		"CLA.md":                                 true,
		"apps/anonymizer/anonymizer/rules.py":    false,
		"apps/api/internal/pseudonym/service.go": false,
		"apps/api/internal/httpapi/auth_test.go": false,
		"docs/COMMERCIAL.md":                     false,
		"README.md":                              false,
	}
	for rel, want := range cases {
		if got := skipFile(rel); got != want {
			t.Errorf("skipFile(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestDecideTestsTouched(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		changed []string
		labels  []string
		ok      bool
	}{
		{"docs only", []string{"README.md", "docs/TESTING.md"}, nil, true},
		{"go code without test", []string{"apps/api/internal/httpapi/chat.go"}, nil, false},
		{"go code with test", []string{"apps/api/internal/httpapi/chat.go", "apps/api/internal/httpapi/chat_test.go"}, nil, true},
		{"test in another package counts", []string{"apps/api/internal/config/config.go", "apps/api/internal/httpapi/x_integration_test.go"}, nil, true},
		{"frontend without test", []string{"apps/web/src/app/page.tsx"}, nil, false},
		{"frontend with vitest", []string{"apps/web/src/app/page.tsx", "apps/web/src/app/page.test.tsx"}, nil, true},
		{"frontend with e2e", []string{"apps/web/src/app/page.tsx", "apps/web/e2e/login.spec.ts"}, nil, true},
		{"css is not logic", []string{"apps/web/src/app/globals.css"}, nil, true},
		{"tools without test", []string{"tools/prcheck/workflows.go"}, nil, false},
		{"maintainer label", []string{"apps/api/main.go"}, []string{"docs", "no-tests-needed"}, true},
		{"label case and spaces", []string{"apps/api/main.go"}, []string{" No-Tests-Needed "}, true},
		{"other label does not waive", []string{"apps/api/main.go"}, []string{"bug"}, false},
		{"windows separators", []string{`apps\api\main.go`}, nil, false},
		{"empty change set", nil, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			v := DecideTestsTouched(c.changed, c.labels)
			if v.OK != c.ok {
				t.Fatalf("OK=%v, want %v (%s)", v.OK, c.ok, v.Reason)
			}
			if !v.OK && !strings.Contains(v.Reason, NoTestsLabel) {
				t.Fatalf("refusal does not mention the label: %s", v.Reason)
			}
		})
	}
}

// The gate must also work on a real diff: build a throwaway repository,
// commit code without a test and check the command exits non-zero.
func TestTestsTouchedOnRealDiff(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("git", "init", "-q", "-b", "main")
	write("README.md", "x\n")
	run("git", "add", ".")
	run("git", "commit", "-qm", "base")
	run("git", "checkout", "-qb", "feature")
	write("apps/api/internal/x/x.go", "package x\n")
	run("git", "add", ".")
	run("git", "commit", "-qm", "code without test")

	out, err := exec.Command("git", "-C", dir, "diff", "--name-only", "main...HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	changed, _ := readLines(strings.NewReader(string(out)))
	if v := DecideTestsTouched(changed, nil); v.OK {
		t.Fatalf("code without a test passed: %+v", v)
	}
	if v := DecideTestsTouched(changed, []string{NoTestsLabel}); !v.OK {
		t.Fatalf("label did not waive: %+v", v)
	}
}

const testRepo = "someone/mindstrata"

func TestCheckWorkflow(t *testing.T) {
	t.Parallel()
	canon := SelfHostedRunsOn(testRepo)
	cases := []struct {
		name  string
		yaml  string
		wants []string // substrings of expected problems; empty = clean
	}{
		{"canonical and ubuntu", "jobs:\n  a:\n    runs-on: " + canon + "\n  b:\n    runs-on: ubuntu-latest\n", nil},
		{"secrets on github-hosted job are fine", "jobs:\n  a:\n    runs-on: ubuntu-latest\n    env:\n      K: ${{ secrets.GITHUB_TOKEN }}\n", nil},
		{"reusable workflow call", "jobs:\n  a:\n    uses: ./.github/workflows/x.yml\n", nil},
		{"bare self-hosted", "jobs:\n  a:\n    runs-on: [self-hosted, linux]\n", []string{"job a"}},
		{"self-hosted string", "jobs:\n  a:\n    runs-on: self-hosted\n", []string{"job a"}},
		{"expression without event guard",
			"jobs:\n  a:\n    runs-on: ${{ github.repository != '" + testRepo + "' && 'self-hosted' || 'ubuntu-latest' }}\n",
			[]string{"job a"}},
		{"canonical for another repo", "jobs:\n  a:\n    runs-on: " + SelfHostedRunsOn("other/repo") + "\n", []string{"job a"}},
		{"secrets on contributor runner", "jobs:\n  a:\n    runs-on: " + canon + "\n    env:\n      K: ${{ secrets.X }}\n", []string{"must not use secrets"}},
		{"missing runs-on", "jobs:\n  a:\n    steps: []\n", []string{"no runs-on"}},
		{"broken yaml", "jobs: [\n", []string{"YAML"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CheckWorkflow("ci.yml", []byte(c.yaml), testRepo)
			if len(c.wants) == 0 && len(got) > 0 {
				t.Fatalf("false positive: %v", got)
			}
			for _, w := range c.wants {
				if !strings.Contains(strings.Join(got, "\n"), w) {
					t.Fatalf("want a problem with %q, got %v", w, got)
				}
			}
		})
	}
}

// The expression itself must carry all three guards.
func TestSelfHostedRunsOnGuards(t *testing.T) {
	t.Parallel()
	expr := SelfHostedRunsOn(testRepo)
	for _, must := range []string{
		"github.repository != '" + testRepo + "'",
		"github.event_name == 'push'",
		"vars.MINDSTRATA_SELF_HOSTED == 'true'",
		"'ubuntu-latest'",
	} {
		if !strings.Contains(expr, must) {
			t.Fatalf("expression lacks %q: %s", must, expr)
		}
	}
}

func TestCheckPinnedActions(t *testing.T) {
	t.Parallel()
	sha := strings.Repeat("a", 40)
	cases := []struct {
		name string
		yaml string
		bad  int
	}{
		{"pinned", "steps:\n  - uses: actions/checkout@" + sha + " # v5\n", 0},
		{"pinned sub-path", "steps:\n  - uses: github/codeql-action/init@" + sha + "\n", 0},
		{"local and docker", "steps:\n  - uses: ./.github/actions/x\n  - uses: docker://alpine:3\n", 0},
		{"tag", "steps:\n  - uses: actions/checkout@v5\n", 1},
		{"branch", "steps:\n  - uses: \"actions/setup-go@main\"\n", 1},
		{"short sha", "steps:\n  - uses: actions/checkout@abc1234\n", 1},
		{"job-level reusable workflow by tag", "jobs:\n  a:\n    uses: org/repo/.github/workflows/x.yml@v1\n", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckPinnedActions("w.yml", []byte(c.yaml)); len(got) != c.bad {
				t.Fatalf("got %d problem(s), want %d: %v", len(got), c.bad, got)
			}
		})
	}
}

func TestCheckRunnerCompose(t *testing.T) {
	t.Parallel()
	good := `services:
  runner:
    image: mindstrata-runner:local
    restart: unless-stopped
    cpus: 2
    mem_limit: 4g
    environment:
      EPHEMERAL: "true"
  postgres:
    image: postgres:17-alpine
    restart: unless-stopped
    cpus: 1
    mem_limit: 1g
`
	cases := []struct {
		name string
		yaml string
		want string // substring of the expected problem; empty = clean
	}{
		{"good", good, ""},
		{"no autostart", strings.Replace(good, "unless-stopped", "\"no\"", 1), "unless-stopped"},
		{"no limits", strings.Replace(good, "    cpus: 2\n", "", 1), "cpus and mem_limit"},
		{"sidecar without limits", strings.Replace(good, "    cpus: 1\n", "", 1), "postgres: needs cpus"},
		{"docker socket", strings.Replace(good, "    environment:", "    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n    environment:", 1), "docker.sock"},
		{"not ephemeral", strings.Replace(good, `EPHEMERAL: "true"`, `EPHEMERAL: "false"`, 1), "ephemeral"},
		{"privileged", strings.Replace(good, "    cpus: 2\n", "    cpus: 2\n    privileged: true\n", 1), "privileged"},
		{"no runner", "services:\n  other:\n    image: x\n", "no runner service"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CheckRunnerCompose([]byte(c.yaml))
			if c.want == "" {
				if len(got) > 0 {
					t.Fatalf("false positive: %v", got)
				}
				return
			}
			if !strings.Contains(strings.Join(got, "\n"), c.want) {
				t.Fatalf("want %q, got %v", c.want, got)
			}
		})
	}
}

func TestCheckHygiene(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
		bad  int
	}{
		{"loopback, private, docs", "127.0.0.1 10.1.2.3 172.16.5.1 192.168.0.1 192.0.2.10 203.0.113.5 0.0.0.0", 0},
		{"placeholder public", "X-Forwarded-For: 1.2.3.4", 0},
		{"payment provider network", "webhook from 185.71.76.5", 0},
		{"public ip", "server: 93.184.215.14", 1},
		{"version is not an ip", "go 1.26.8 and v2.337.0.1 not", 0},
		{"example emails", "a@example.com b@test.local c@users.noreply.github.com git@github.com", 0},
		{"real email", "contact: john.doe@gmail.com", 1},
		{"allow marker", "server: 93.184.215.14 // hygiene:allow reason", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckHygiene("f", c.text); len(got) != c.bad {
				t.Fatalf("got %d problem(s), want %d: %v", len(got), c.bad, got)
			}
		})
	}
}

func TestCheckPII(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		file string
		text string
		bad  int
	}{
		{"synthetic phone", "a_test.go", `"Иван +7 999 000 00 00"`, 0},
		{"synthetic phone compact", "a_test.go", `"+79001234567"`, 0},
		{"real-looking phone", "a_test.go", `"+7 912 518 43 71"`, 1},
		{"real-looking phone with 8", "x.test.ts", `phone: '8 (922) 518-43-71'`, 1},
		{"not a fixture", "service.go", `"+7 912 518 43 71"`, 0},
		{"testdata dir", "apps/api/testdata/chat.json", `"+7 912 518 43 71"`, 1},
		{"synthetic snils", "a_test.go", `"000-000-000 00"`, 0},
		{"real-looking snils", "a_test.go", `"112-233-445 95"`, 1},
		{"passport needs the word", "a_test.go", `"4510 123987"`, 0},
		{"real-looking passport", "a_test.go", `"паспорт 4510 723981"`, 1},
		{"synthetic passport", "a_test.go", `"passport 0000 000000"`, 0},
		{"allow marker", "a_test.go", `"+7 912 518 43 71" // hygiene:allow format test`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckPII(c.file, c.text); len(got) != c.bad {
				t.Fatalf("got %d problem(s), want %d: %v", len(got), c.bad, got)
			}
		})
	}
}

func TestCheckComments(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		file string
		src  string
		bad  int
	}{
		{"go english", "a.go", "package a\n// Hello.\nvar s = \"Привет\" // ok\n", 0},
		{"go cyrillic line", "a.go", "package a\n// Привет\n", 1},
		{"go cyrillic block", "a.go", "package a\n/* Привет */\n", 1},
		{"go cyrillic in raw string is ui", "a.go", "package a\nvar s = `// Привет`\n", 0},
		{"ts english", "a.ts", "// hi\nconst a = 'Привет' // ok\n", 0},
		{"ts cyrillic", "a.ts", "const a = 1 // привет\n", 1},
		{"tsx jsx text with url", "a.tsx", "const x = <a>Сайт: https://example.com текст</a>\n", 0},
		{"tsx jsx comment", "a.tsx", "const x = <div>{/* привет */}</div>\n", 1},
		{"ts template literal", "a.ts", "const a = `line // не комментарий\nеще`\n", 0},
		{"ts string with slashes", "a.ts", "const u = \"https://x\" // коммент\n", 1},
		{"other files ignored", "a.md", "<!-- привет -->", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckComments(c.file, []byte(c.src)); len(got) != c.bad {
				t.Fatalf("got %d problem(s), want %d: %v", len(got), c.bad, got)
			}
		})
	}
}

func TestCheckDeployTemplate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
		bad  int
	}{
		{"placeholders", "{$SITE_DOMAIN} {\n reverse_proxy api:18080\n}\nurl: https://example.com http://localhost:3000 http://api:18080", 0},
		{"concrete domain", "PUBLIC_WEB_URL=https://shop.ru", 1},
		{"public ip", "upstream 93.184.215.14:80", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := CheckDeployTemplate("deploy/x", c.text); len(got) != c.bad {
				t.Fatalf("got %d problem(s), want %d: %v", len(got), c.bad, got)
			}
		})
	}
}

// repoRoot is where the public layout lives: the repository root in the
// public repository, opensource/overlay in the private monorepo it was cut from.
func repoRoot(t *testing.T) string {
	t.Helper()
	if _, err := os.Stat("../../opensource/overlay"); err == nil {
		return "../../opensource/overlay"
	}
	return "../.."
}

// The real files pass their own rules: a rule never applied to the real
// workflow guards nothing.
func TestRealFilesPass(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	paths, err := workflowFiles(filepath.Join(root, ".github/workflows"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := CheckWorkflow(p, data, envOr("MINDSTRATA_MAIN_REPO", DefaultMainRepo)); len(got) > 0 {
			t.Errorf("%v", got)
		}
		if got := CheckPinnedActions(p, data); len(got) > 0 {
			t.Errorf("%v", got)
		}
	}
	compose, err := os.ReadFile(filepath.Join(root, "runner/docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := CheckRunnerCompose(compose); len(got) > 0 {
		t.Errorf("%v", got)
	}
	err = walkText(filepath.Join(root, "deploy"), func(rel string, data []byte) {
		if got := CheckDeployTemplate(rel, string(data)); len(got) > 0 {
			t.Errorf("%v", got)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Compose files must be accepted by Docker Compose itself, not only by our
// YAML reading. Docker is mandatory in CI (REQUIRE_DOCKER=1); a contributor
// machine without Docker skips this test explicitly.
func TestComposeFilesValid(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("docker"); err != nil {
		if os.Getenv("REQUIRE_DOCKER") == "1" {
			t.Fatal("docker is required here (REQUIRE_DOCKER=1)")
		}
		t.Skip("docker is not installed")
	}
	root := repoRoot(t)
	cases := [][]string{
		{"-f", filepath.Join(root, "runner/docker-compose.yml")},
		{"-f", filepath.Join(root, "docker-compose.yml"), "--env-file", filepath.Join(root, ".env.example")},
		{"-f", filepath.Join(root, "deploy/compose.prod.yml"), "--env-file", filepath.Join(root, "deploy/.env.prod.example")},
	}
	for _, c := range cases {
		args := append([]string{"compose"}, c...)
		args = append(args, "config", "-q")
		if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
			t.Errorf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}
