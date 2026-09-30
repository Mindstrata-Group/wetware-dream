package config

import (
	"reflect"
	"testing"
)

// TestLoad_IgnoresOpenAIAPIKeyEnv: the vsegpt key from env no longer reaches
// any config field. Regression for the 2026-04 leak: the env key sat in the
// repository and kept working in production because env was a second
// source next to the admin UI. Now the key is set only in the gateway registry.
//
// No t.Parallel(): t.Setenv changes the process environment and is forbidden in
// parallel tests.
func TestLoad_IgnoresOpenAIAPIKeyEnv(t *testing.T) {
	const sentinel = "sk-or-vv-env-must-not-leak"
	t.Setenv("OPENAI_API_KEY", sentinel)

	cfg := Load()

	v := reflect.ValueOf(cfg)
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.String && f.String() == sentinel {
			t.Errorf("OPENAI_API_KEY попал в поле конфига %s", v.Type().Field(i).Name)
		}
	}
}
