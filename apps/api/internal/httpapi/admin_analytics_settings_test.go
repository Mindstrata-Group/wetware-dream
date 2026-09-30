package httpapi

import (
	"strings"
	"testing"
)

func TestValidMetrikaCounterID(t *testing.T) {
	t.Parallel()

	valid := []string{"", "1", "12345678", "999999999999"}
	for _, id := range valid {
		if !validMetrikaCounterID(id) {
			t.Errorf("validMetrikaCounterID(%q)=false, want true", id)
		}
	}

	invalid := []string{
		"abc",
		"123abc",
		"12 34",
		"-1",
		"1.5",
		"1234567890123", // longer than 12 digits
		"<script>",
	}
	for _, id := range invalid {
		if validMetrikaCounterID(id) {
			t.Errorf("validMetrikaCounterID(%q)=true, want false", id)
		}
	}
}

func TestValidMetrikaParams(t *testing.T) {
	t.Parallel()

	valid := []string{
		"",
		"{}",
		defaultMetrikaParams,
		`{"webvisor":true,"ecommerce":"dataLayer"}`,
	}
	for _, p := range valid {
		if !validMetrikaParams(p) {
			t.Errorf("validMetrikaParams(%q)=false, want true", p)
		}
	}

	invalid := []string{
		"true",
		"[1,2]",
		`"строка"`,
		"{не json",
		"{" + strings.Repeat(`"k":1,`, 1000) + `"x":1}`, // more than 4 KB
	}
	for _, p := range invalid {
		if validMetrikaParams(p) {
			t.Errorf("validMetrikaParams(%.40q...)=true, want false", p)
		}
	}
}
