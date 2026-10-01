package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDocumentedMakeTargetsFake(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		makefile   string
		docs       map[string]string
		wantErrors int
	}{
		{
			name:     "all targets exist",
			makefile: "check: guard\nguard:\nfmt:\n",
			docs: map[string]string{
				"README.md": "Run `make check` and `make fmt` before committing.",
			},
			wantErrors: 0,
		},
		{
			name:     "missing target fails",
			makefile: "check:\nfmt:\n",
			docs: map[string]string{
				"README.md": "Run `make check` and `make non-existent-target` to build.",
			},
			wantErrors: 1,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			targets := ParseMakefileTargets(c.makefile)
			errs := CheckDocumentedMakeTargets(targets, c.docs)
			if len(errs) != c.wantErrors {
				t.Fatalf("got %d errors (%v), want %d", len(errs), errs, c.wantErrors)
			}
		})
	}
}

func TestCheckDocumentedMakeTargetsRealRepo(t *testing.T) {
	t.Parallel()

	rootDir := filepath.Join("..", "..")
	makefileBytes, err := os.ReadFile(filepath.Join(rootDir, "Makefile"))
	if err != nil {
		t.Fatalf("failed to read Makefile: %v", err)
	}

	targets := ParseMakefileTargets(string(makefileBytes))
	docs := make(map[string]string)

	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(rootDir, path)
		base := filepath.Base(rel)
		if strings.HasSuffix(rel, ".md") {
			if strings.HasPrefix(base, "README") || strings.HasPrefix(base, "CONTRIBUTING") || strings.HasPrefix(base, "AGENTS") || strings.HasPrefix(rel, "docs/") {
				content, err := os.ReadFile(path)
				if err == nil {
					docs[rel] = string(content)
				}
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk repository docs: %v", err)
	}

	missing := CheckDocumentedMakeTargets(targets, docs)
	if len(missing) > 0 {
		t.Errorf("documented make targets check failed:\n  %s", strings.Join(missing, "\n  "))
	}
}
