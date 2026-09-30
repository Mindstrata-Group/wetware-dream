package main

import (
	"fmt"
	"go/scanner"
	"go/token"
	"strings"
	"unicode"
)

// Code comments are written in English so that anyone can review them.
// Product UI copy and test data stay as they are: string literals are not
// comments and are never flagged.

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

// GoComments returns the comments of a Go source file with their lines.
func GoComments(src []byte) []Comment {
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))
	var s scanner.Scanner
	s.Init(file, src, nil, scanner.ScanComments)
	var out []Comment
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			return out
		}
		if tok == token.COMMENT {
			out = append(out, Comment{Line: fset.Position(pos).Line, Text: lit})
		}
	}
}

// Comment is a single comment with the line where it starts.
type Comment struct {
	Line int
	Text string
}

// TSComments extracts comments from TypeScript/TSX with a small lexer that
// understands string, template and comment states. JSX text is not a comment;
// to avoid reading "https://" in JSX text as a comment, "//" only starts a
// comment at the start of a line or after whitespace or punctuation.
func TSComments(src []byte) []Comment {
	s := string(src)
	var out []Comment
	line := 1
	i := 0
	n := len(s)
	for i < n {
		c := s[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == '\'' || c == '"' || c == '`':
			q := c
			i++
			for i < n && s[i] != q {
				if s[i] == '\\' {
					i++
				} else if s[i] == '\n' {
					line++
					if q != '`' {
						break // unterminated plain string: stop at line end
					}
				}
				i++
			}
			i++
		case c == '/' && i+1 < n && s[i+1] == '*':
			start := line
			j := strings.Index(s[i+2:], "*/")
			end := n
			if j >= 0 {
				end = i + 2 + j + 2
			}
			text := s[i:end]
			line += strings.Count(text, "\n")
			out = append(out, Comment{Line: start, Text: text})
			i = end
		case c == '/' && i+1 < n && s[i+1] == '/' && lineCommentStart(s, i):
			j := strings.IndexByte(s[i:], '\n')
			end := n
			if j >= 0 {
				end = i + j
			}
			out = append(out, Comment{Line: line, Text: s[i:end]})
			i = end
		default:
			i++
		}
	}
	return out
}

func lineCommentStart(s string, i int) bool {
	if i == 0 {
		return true
	}
	return strings.ContainsRune(" \t\n;{}(),=", rune(s[i-1]))
}

// CheckComments flags comments with Cyrillic text.
func CheckComments(name string, src []byte) []string {
	var comments []Comment
	switch {
	case strings.HasSuffix(name, ".go"):
		comments = GoComments(src)
	case strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".tsx"):
		comments = TSComments(src)
	default:
		return nil
	}
	var problems []string
	for _, c := range comments {
		if hasCyrillic(c.Text) {
			problems = append(problems, fmt.Sprintf("%s:%d: comment is not in English", name, c.Line))
		}
	}
	return problems
}
