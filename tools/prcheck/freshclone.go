package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// FreshCloneJobName is the CI job that keeps the README promise: on a clean
// machine with only Docker and make, `make dev` brings up a working site.
const FreshCloneJobName = "Fresh clone: make dev"

// newcomerShims is the directory of failing go/node/npm shims the job puts
// first in PATH, so a preinstalled Go or Node on the CI machine cannot hide a
// dependency the README does not mention.
const newcomerShims = "newcomer-bin"

// CheckFreshClone makes sure the fresh-clone job exists and still tests what
// it claims: a GitHub machine (Docker is needed), `make dev` run with the
// newcomer PATH, and a check of the running site.
func CheckFreshClone(name string, data []byte) []string {
	var wf struct {
		Jobs map[string]struct {
			Name   string `yaml:"name"`
			RunsOn any    `yaml:"runs-on"`
			Steps  []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return []string{fmt.Sprintf("%s: not valid YAML: %v", name, err)}
	}
	for id, job := range wf.Jobs {
		if job.Name != FreshCloneJobName {
			continue
		}
		var problems []string
		if runsOn, ok := job.RunsOn.(string); !ok || runsOn != "ubuntu-latest" {
			problems = append(problems, fmt.Sprintf("%s: job %s must run on ubuntu-latest (it needs Docker)", name, id))
		}
		shims, makeDev, checksSite := false, false, false
		for _, s := range job.Steps {
			// The shims must be in place before make dev runs.
			if strings.Contains(s.Run, newcomerShims) && strings.Contains(s.Run, "GITHUB_PATH") {
				shims = true
			}
			if shims && strings.Contains(s.Run, "make dev") && !strings.Contains(s.Run, "make dev-down") {
				makeDev = true
			}
			if strings.Contains(s.Run, "/health") && strings.Contains(s.Run, "admin@example.com") {
				checksSite = true
			}
		}
		if !makeDev {
			problems = append(problems, fmt.Sprintf("%s: job %s must run `make dev` after putting failing go/node shims (%s) first in PATH", name, id, newcomerShims))
		}
		if !checksSite {
			problems = append(problems, fmt.Sprintf("%s: job %s must check the API health and the dev admin login", name, id))
		}
		return problems
	}
	return []string{fmt.Sprintf("%s: no job named %q; the README's one-command promise is untested", name, FreshCloneJobName)}
}

func runFreshClone(args []string) error {
	fl := flag.NewFlagSet("fresh-clone", flag.ExitOnError)
	file := fl.String("file", ".github/workflows/ci.yml", "CI workflow")
	_ = fl.Parse(args)
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	if err := fail("fresh-clone job", CheckFreshClone(*file, data)); err != nil {
		return err
	}
	fmt.Printf("fresh-clone: ok - %s is checked on a clean machine\n", FreshCloneJobName)
	return nil
}
