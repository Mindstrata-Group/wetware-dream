package main

import (
	"strings"
	"testing"
)

func TestCheckFreshClone(t *testing.T) {
	t.Parallel()
	shims := `      - run: |
          shims="$RUNNER_TEMP/newcomer-bin"
          echo "$shims" >> "$GITHUB_PATH"
`
	good := `
jobs:
  fresh-clone:
    name: "Fresh clone: make dev"
    runs-on: ubuntu-latest
    steps:
` + shims + `      - run: make dev
      - run: |
          curl -fsS "$api/health"
          curl -d '{"email":"admin@example.com"}' "$api/api/auth/login"
      - run: make dev-down
`
	cases := []struct {
		name, yaml string
		want       string // substring of the only problem; "" means no problems
	}{
		{"good", good, ""},
		{"missing job", "jobs:\n  web:\n    name: Web\n    runs-on: ubuntu-latest\n", "no job named"},
		{"self-hosted", strings.Replace(good, "runs-on: ubuntu-latest", "runs-on: [self-hosted]", 1), "ubuntu-latest"},
		{"no shims", strings.Replace(good, shims, "", 1), "shims"},
		{"shims after make dev", strings.Replace(strings.Replace(good, shims, "", 1), "      - run: make dev-down\n", "      - run: make dev-down\n"+shims, 1), "shims"},
		{"only dev-down", strings.Replace(good, "      - run: make dev\n", "", 1), "must run `make dev`"},
		{"no site check", strings.Replace(good, `"$api/health"`, `"$api/"`, 1), "health"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := CheckFreshClone("ci.yml", []byte(c.yaml))
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no problems, got %v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], c.want) {
				t.Fatalf("want one problem containing %q, got %v", c.want, got)
			}
		})
	}
}
