package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultMainRepo is the main public repository. Until the name is chosen it
// is a placeholder; override with -main-repo or MINDSTRATA_MAIN_REPO.
const DefaultMainRepo = "Mindstrata-Group/wetware-dream"

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  go run ./tools/prcheck tests-touched  [-base origin/main] [-files list.txt|-] [-labels a,b]
  go run ./tools/prcheck runner-safety  [-dir .github/workflows] [-main-repo Mindstrata-Group/wetware-dream]
  go run ./tools/prcheck runner-compose [-file runner/docker-compose.yml]
  go run ./tools/prcheck pinned-actions [-dir .github/workflows]
  go run ./tools/prcheck hygiene        [-root .]
  go run ./tools/prcheck comments       [-root .]
  go run ./tools/prcheck deploy         [-dir deploy]
  go run ./tools/prcheck fresh-clone    [-file .github/workflows/ci.yml]`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	commands := map[string]func([]string) error{
		"tests-touched":  runTestsTouched,
		"runner-safety":  runRunnerSafety,
		"runner-compose": runRunnerCompose,
		"pinned-actions": runPinnedActions,
		"hygiene":        runHygiene,
		"comments":       runComments,
		"deploy":         runDeploy,
		"fresh-clone":    runFreshClone,
	}
	cmd, ok := commands[os.Args[1]]
	if !ok {
		usage()
	}
	if err := cmd(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "prcheck:", err)
		os.Exit(1)
	}
}

func fail(title string, problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	const max = 200
	shown := problems
	if len(shown) > max {
		shown = shown[:max]
	}
	msg := fmt.Sprintf("%s (%d):\n  %s", title, len(problems), strings.Join(shown, "\n  "))
	if len(problems) > max {
		msg += fmt.Sprintf("\n  ... and %d more", len(problems)-max)
	}
	return fmt.Errorf("%s", msg)
}

func runTestsTouched(args []string) error {
	fl := flag.NewFlagSet("tests-touched", flag.ExitOnError)
	base := fl.String("base", "origin/main", "compare against the merge base with this ref")
	files := fl.String("files", "", "file with changed paths (- for stdin); empty = ask git")
	labels := fl.String("labels", os.Getenv("PR_LABELS"), "pull request labels, comma separated")
	_ = fl.Parse(args)

	var changed []string
	var err error
	switch *files {
	case "":
		var out []byte
		out, err = exec.Command("git", "diff", "--name-only", *base+"...HEAD").Output()
		if err == nil {
			changed, err = readLines(bytes.NewReader(out))
		}
	case "-":
		changed, err = readLines(os.Stdin)
	default:
		var f *os.File
		if f, err = os.Open(*files); err == nil {
			defer f.Close()
			changed, err = readLines(f)
		}
	}
	if err != nil {
		return err
	}
	v := DecideTestsTouched(changed, strings.Split(*labels, ","))
	for _, c := range v.Code {
		fmt.Println("  logic:", c)
	}
	if !v.OK {
		return fmt.Errorf("%s", v.Reason)
	}
	fmt.Println("tests-touched: ok -", v.Reason)
	return nil
}

func readLines(r io.Reader) ([]string, error) {
	var lines []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if s := strings.TrimSpace(sc.Text()); s != "" {
			lines = append(lines, s)
		}
	}
	return lines, sc.Err()
}

func workflowFiles(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no workflows in %s: nothing to check means the layout is wrong", dir)
	}
	sort.Strings(paths)
	return paths, nil
}

func runRunnerSafety(args []string) error {
	fl := flag.NewFlagSet("runner-safety", flag.ExitOnError)
	dir := fl.String("dir", ".github/workflows", "workflow directory")
	mainRepo := fl.String("main-repo", envOr("MINDSTRATA_MAIN_REPO", DefaultMainRepo), "main repository")
	_ = fl.Parse(args)
	paths, err := workflowFiles(*dir)
	if err != nil {
		return err
	}
	var problems []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		problems = append(problems, CheckWorkflow(filepath.ToSlash(p), data, *mainRepo)...)
	}
	if err := fail("unsafe workflows", problems); err != nil {
		return err
	}
	fmt.Printf("runner-safety: ok - %d workflow(s); self-hosted only in forks, on push, by opt-in\n", len(paths))
	return nil
}

func runPinnedActions(args []string) error {
	fl := flag.NewFlagSet("pinned-actions", flag.ExitOnError)
	dir := fl.String("dir", ".github/workflows", "workflow directory")
	_ = fl.Parse(args)
	paths, err := workflowFiles(*dir)
	if err != nil {
		return err
	}
	var problems []string
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		problems = append(problems, CheckPinnedActions(filepath.ToSlash(p), data)...)
	}
	if err := fail("actions not pinned to a commit SHA", problems); err != nil {
		return err
	}
	fmt.Printf("pinned-actions: ok - %d workflow(s)\n", len(paths))
	return nil
}

func runRunnerCompose(args []string) error {
	fl := flag.NewFlagSet("runner-compose", flag.ExitOnError)
	file := fl.String("file", "runner/docker-compose.yml", "runner compose file")
	_ = fl.Parse(args)
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	if err := fail("unsafe contributor runner", CheckRunnerCompose(data)); err != nil {
		return err
	}
	fmt.Println("runner-compose: ok")
	return nil
}

// Directories and files that are generated, vendored or binary.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, ".next": true, "coverage": true, "test-results": true,
	"playwright-report": true, "dist": true, "bin": true, ".stryker-tmp": true, "reports": true,
}

func skipFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	switch base {
	case "package-lock.json", "go.sum", "LICENSE", "CODE_OF_CONDUCT.md":
		return true
	}
	// The gates' own tests must contain real-looking samples to prove the
	// rules fire; they are exempt by name, visibly.
	if rel == "tools/prcheck/prcheck_test.go" {
		return true
	}
	// Published maintainer contacts (root documents only).
	switch rel {
	case "CLA.md", "COMMERCIAL.md", "GOVERNANCE.md":
		return true
	}
	// The personal-data detector is tested on synthetic data that has to look
	// real: phone numbers, e-mails on common mail domains. Only its test
	// corpus is exempt, not its code.
	if strings.HasPrefix(rel, "apps/anonymizer/tests/") ||
		(strings.HasPrefix(rel, "apps/api/internal/pseudonym/") && strings.HasSuffix(rel, "_test.go")) {
		return true
	}
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".woff", ".woff2", ".ttf", ".otf", ".pdf", ".zip", ".gz", ".br":
		return true
	}
	// Generated API types mirror docs/openapi/openapi.json, which is checked itself.
	return strings.Contains(filepath.ToSlash(rel), "/generated/")
}

func walkText(root string, visit func(rel string, data []byte)) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if skipFile(rel) || !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil // binary
		}
		visit(rel, data)
		return nil
	})
}

func runHygiene(args []string) error {
	fl := flag.NewFlagSet("hygiene", flag.ExitOnError)
	root := fl.String("root", ".", "repository root")
	_ = fl.Parse(args)
	var problems []string
	files := 0
	err := walkText(*root, func(rel string, data []byte) {
		files++
		problems = append(problems, CheckHygiene(rel, string(data))...)
		problems = append(problems, CheckPII(rel, string(data))...)
	})
	if err != nil {
		return err
	}
	if err := fail("installation-specific data in the repository", problems); err != nil {
		return err
	}
	fmt.Printf("hygiene: ok - %d file(s)\n", files)
	return nil
}

func runComments(args []string) error {
	fl := flag.NewFlagSet("comments", flag.ExitOnError)
	root := fl.String("root", ".", "repository root")
	_ = fl.Parse(args)
	var problems []string
	files := 0
	err := walkText(*root, func(rel string, data []byte) {
		if c := CheckComments(rel, data); c != nil || strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, ".ts") || strings.HasSuffix(rel, ".tsx") {
			files++
			problems = append(problems, c...)
		}
	})
	if err != nil {
		return err
	}
	if err := fail("comments must be in English", problems); err != nil {
		return err
	}
	fmt.Printf("comments: ok - %d source file(s)\n", files)
	return nil
}

func runDeploy(args []string) error {
	fl := flag.NewFlagSet("deploy", flag.ExitOnError)
	dir := fl.String("dir", "deploy", "deploy template directory")
	_ = fl.Parse(args)
	var problems []string
	err := walkText(*dir, func(rel string, data []byte) {
		problems = append(problems, CheckDeployTemplate(filepath.ToSlash(filepath.Join(*dir, rel)), string(data))...)
	})
	if err != nil {
		return err
	}
	if err := fail("deploy template is not generic", problems); err != nil {
		return err
	}
	fmt.Println("deploy: ok")
	return nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
