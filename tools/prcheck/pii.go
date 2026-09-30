package main

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Fixtures are where real personal data leaks most often: a real request "to
// make the test realistic" carries a real phone number or a real SNILS. Test
// data must be obviously synthetic; these rules catch values that look real.

var (
	ruPhone  = regexp.MustCompile(`(?:\+7|\b8)[\s(-]*9\d{2}[\s)-]*\d{3}[\s-]*\d{2}[\s-]*\d{2}\b`)
	snils    = regexp.MustCompile(`\b\d{3}-\d{3}-\d{3}[ -]\d{2}\b`)
	passport = regexp.MustCompile(`\b\d{2}\s?\d{2}\s?\d{6}\b`)
	passWord = regexp.MustCompile(`(?i)паспорт|passport`)
	nonDigit = regexp.MustCompile(`\D`)
)

// isFixtureFile reports whether a file holds test data.
func isFixtureFile(p string) bool {
	return isTestFile(p) || strings.Contains(p, "/testdata/") || strings.HasPrefix(p, "testdata/") ||
		strings.Contains(path.Base(p), "fixture")
}

// syntheticDigits reports whether a number is obviously made up: a long run
// of one digit, an ascending run, or a block of zeros.
func syntheticDigits(d string) bool {
	if strings.Contains(d, "0000") || strings.Contains(d, "1234567") || strings.Contains(d, "123456789") {
		return true
	}
	run := 1
	for i := 1; i < len(d); i++ {
		if d[i] == d[i-1] {
			run++
			if run >= 5 {
				return true
			}
		} else {
			run = 1
		}
	}
	return false
}

// CheckPII scans a fixture file for values that look like real personal data.
func CheckPII(name, text string) []string {
	if !isFixtureFile(name) {
		return nil
	}
	var problems []string
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "hygiene:allow") {
			continue
		}
		for _, m := range ruPhone.FindAllString(line, -1) {
			if !syntheticDigits(nonDigit.ReplaceAllString(m, "")) {
				problems = append(problems, fmt.Sprintf("%s:%d: looks like a real phone number (use +7 900 000-00-00)", name, i+1))
			}
		}
		for _, m := range snils.FindAllString(line, -1) {
			if !syntheticDigits(nonDigit.ReplaceAllString(m, "")) {
				problems = append(problems, fmt.Sprintf("%s:%d: looks like a real SNILS (use 000-000-000 00)", name, i+1))
			}
		}
		if passWord.MatchString(line) {
			for _, m := range passport.FindAllString(line, -1) {
				if !syntheticDigits(nonDigit.ReplaceAllString(m, "")) {
					problems = append(problems, fmt.Sprintf("%s:%d: looks like a real passport number (use 0000 000000)", name, i+1))
				}
			}
		}
	}
	return problems
}
