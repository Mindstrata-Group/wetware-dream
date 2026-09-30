package main

import (
	"fmt"
	"regexp"
	"strings"
)

// The deploy/ directory is a template for anyone's server, so it must not
// carry a particular installation: no concrete domains, addresses or people.

var urlHost = regexp.MustCompile(`(?i)\bhttps?://([a-z0-9.-]+)`)

// Public third-party APIs are the same for every installation.
var publicServiceHosts = map[string]bool{
	"api.telegram.org":                  true,
	"api.openai.com":                    true,
	"api.anthropic.com":                 true,
	"generativelanguage.googleapis.com": true,
	"github.com":                        true,
	"ghcr.io":                           true,
}

func deployHostAllowed(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	switch {
	case publicServiceHosts[h]:
		return true
	case !strings.Contains(h, "."): // compose service names, localhost
		return true
	case h == "example.com" || strings.HasSuffix(h, ".example.com"):
		return true
	case h == "127.0.0.1":
		return true
	}
	return false
}

// CheckDeployTemplate validates one file of the deploy template.
func CheckDeployTemplate(name, text string) []string {
	problems := CheckHygiene(name, text)
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "hygiene:allow") {
			continue
		}
		for _, m := range urlHost.FindAllStringSubmatch(line, -1) {
			if !deployHostAllowed(m[1]) {
				problems = append(problems, fmt.Sprintf("%s:%d: concrete host %s in the deploy template (use ${SITE_DOMAIN} or example.com)", name, i+1, m[1]))
			}
		}
	}
	return problems
}
