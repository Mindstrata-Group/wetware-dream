package main

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var makeTargetRegex = regexp.MustCompile("`make\\s+([a-z0-9-]+)`")

// ParseMakefileTargets extracts all defined targets from Makefile content.
func ParseMakefileTargets(content string) map[string]bool {
	targets := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		// Match target definitions like "target:" or "target1 target2:"
		if idx := strings.Index(line, ":"); idx > 0 {
			targetPart := line[:idx]
			// Ensure it's not a variable assignment like VAR := or VAR ?=
			if strings.Contains(targetPart, "=") || strings.Contains(targetPart, "$") {
				continue
			}
			fields := strings.Fields(targetPart)
			for _, f := range fields {
				if f != ".PHONY" && f != ".DEFAULT_GOAL" && !strings.Contains(f, "/") {
					targets[f] = true
				}
			}
		}
	}
	return targets
}

// FindDocumentedMakeTargets scans markdown content for `make <target>` usages.
func FindDocumentedMakeTargets(content string) []string {
	matches := makeTargetRegex.FindAllStringSubmatch(content, -1)
	var found []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			target := m[1]
			if !seen[target] {
				seen[target] = true
				found = append(found, target)
			}
		}
	}
	sort.Strings(found)
	return found
}

// CheckDocumentedMakeTargets checks whether all documented make targets exist in targets map.
func CheckDocumentedMakeTargets(targets map[string]bool, docs map[string]string) []string {
	var missing []string
	seenMissing := make(map[string]bool)
	for docFile, content := range docs {
		docTargets := FindDocumentedMakeTargets(content)
		for _, t := range docTargets {
			if !targets[t] && !seenMissing[t+" in "+docFile] {
				seenMissing[t+" in "+docFile] = true
				missing = append(missing, fmt.Sprintf("target '%s' documented in %s does not exist in Makefile", t, docFile))
			}
		}
	}
	sort.Strings(missing)
	return missing
}
