package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// CheckRunnerCompose validates the contributor runner compose file.
func CheckRunnerCompose(data []byte) []string {
	var doc struct {
		Services map[string]struct {
			Image       string            `yaml:"image"`
			Restart     string            `yaml:"restart"`
			CPUs        any               `yaml:"cpus"`
			MemLimit    any               `yaml:"mem_limit"`
			Privileged  bool              `yaml:"privileged"`
			Volumes     []any             `yaml:"volumes"`
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []string{"runner compose is not valid YAML: " + err.Error()}
	}
	if _, ok := doc.Services["runner"]; !ok {
		return []string{"runner compose has no runner service"}
	}
	var problems []string
	for _, name := range sortedKeys(doc.Services) {
		svc := doc.Services[name]
		if svc.Restart != "unless-stopped" {
			problems = append(problems, name+": must come back after a reboot: restart: unless-stopped")
		}
		if svc.CPUs == nil || svc.MemLimit == nil {
			problems = append(problems, name+": needs cpus and mem_limit so a test cannot take the whole computer")
		}
		if svc.Privileged {
			problems = append(problems, name+": must not be privileged")
		}
		for _, v := range svc.Volumes {
			if strings.Contains(fmt.Sprint(v), "docker.sock") {
				problems = append(problems, name+": must not mount the host docker.sock (that is root on the contributor's machine)")
			}
		}
	}
	runner := doc.Services["runner"]
	if strings.ToLower(runner.Environment["EPHEMERAL"]) != "true" {
		problems = append(problems, "runner: must be ephemeral (EPHEMERAL=true): every job starts from a clean state")
	}
	if runner.Image == "" {
		problems = append(problems, "runner: image name is missing")
	}
	return problems
}
