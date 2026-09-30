package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SelfHostedRunsOn is the only runs-on expression allowed to mention a
// self-hosted runner. A job lands on a contributor's runner only when ALL of
// these hold:
//
//   - the repository is NOT the main one: pull requests from strangers arrive
//     there, and their code must never run on somebody's computer;
//   - the event is a push: pull requests opened against the fork do not run
//     on the fork owner's machine either;
//   - the fork owner opted in with the MINDSTRATA_SELF_HOSTED=true variable;
//     without it the job would wait forever for a runner that does not exist.
//
// Everything else runs on ubuntu-latest (free for public repositories).
func SelfHostedRunsOn(mainRepo string) string {
	return "${{ (github.repository != '" + mainRepo + "' && github.event_name == 'push' && vars.MINDSTRATA_SELF_HOSTED == 'true') && fromJSON('[\"self-hosted\",\"mindstrata-runner\"]') || 'ubuntu-latest' }}"
}

type workflowFile struct {
	Jobs map[string]yaml.Node `yaml:"jobs"`
}

// CheckWorkflow validates runs-on and secret usage of a single workflow.
func CheckWorkflow(name string, data []byte, mainRepo string) []string {
	var wf workflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return []string{fmt.Sprintf("%s: not valid YAML: %v", name, err)}
	}
	allowed := SelfHostedRunsOn(mainRepo)
	var problems []string
	for _, id := range sortedKeys(wf.Jobs) {
		job := wf.Jobs[id]
		var spec struct {
			RunsOn yaml.Node `yaml:"runs-on"`
		}
		if err := job.Decode(&spec); err != nil {
			problems = append(problems, fmt.Sprintf("%s: job %s: %v", name, id, err))
			continue
		}
		var values []string
		switch spec.RunsOn.Kind {
		case yaml.ScalarNode:
			values = []string{spec.RunsOn.Value}
		case yaml.SequenceNode:
			for _, n := range spec.RunsOn.Content {
				values = append(values, n.Value)
			}
		case 0:
			// Reusable-workflow callers have no runs-on; everything else must.
			var uses struct {
				Uses string `yaml:"uses"`
			}
			_ = job.Decode(&uses)
			if uses.Uses == "" {
				problems = append(problems, fmt.Sprintf("%s: job %s has no runs-on", name, id))
			}
			continue
		default:
			problems = append(problems, fmt.Sprintf("%s: job %s: unexpected runs-on shape", name, id))
			continue
		}
		if !strings.Contains(strings.Join(values, ","), "self-hosted") {
			continue
		}
		if len(values) != 1 || values[0] != allowed {
			problems = append(problems, fmt.Sprintf(
				"%s: job %s may run on a self-hosted runner in the main repository or without the fork owner's consent; "+
					"use the expression from tools/prcheck SelfHostedRunsOn", name, id))
			continue
		}
		// A contributor runner never receives secrets: code in a fork may come from anyone.
		raw, _ := yaml.Marshal(&job)
		if strings.Contains(string(raw), "secrets.") {
			problems = append(problems, fmt.Sprintf("%s: job %s can run on a contributor runner and must not use secrets", name, id))
		}
	}
	return problems
}

var usesLine = regexp.MustCompile(`(?m)^\s*-?\s*uses:\s*['"]?([^\s'"#]+)`)
var pinnedRef = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}$`)

// CheckPinnedActions requires every action reference to be pinned to a full
// commit SHA: a moved tag must not change what runs in CI.
func CheckPinnedActions(name string, data []byte) []string {
	var problems []string
	for _, m := range usesLine.FindAllStringSubmatch(string(data), -1) {
		ref := m[1]
		if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "docker://") {
			continue
		}
		if !pinnedRef.MatchString(ref) {
			problems = append(problems, fmt.Sprintf("%s: %s is not pinned to a commit SHA", name, ref))
		}
	}
	return problems
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
