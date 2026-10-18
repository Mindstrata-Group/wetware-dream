package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// MarkdownLink represents a link extracted from Markdown source.
type MarkdownLink struct {
	Target string
	Line   int
}

// GitHubSlug converts heading text into a GitHub-compatible anchor slug.
func GitHubSlug(heading string) string {
	heading = strings.TrimSpace(heading)
	// Remove markdown links inside headings: [text](url) -> text
	heading = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`).ReplaceAllString(heading, "$1")
	// Remove backticks, asterisks, underscores, etc.
	heading = strings.ReplaceAll(heading, "`", "")
	heading = strings.ReplaceAll(heading, "*", "")
	heading = strings.ReplaceAll(heading, "_", "")

	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' {
			b.WriteRune('-')
		}
	}
	return b.String()
}

var (
	// Matches [text](target) or ![alt](target)
	linkRegex = regexp.MustCompile(`!?\[(?:[^\]]|\\\])*\]\(([^)\s]+)(?:\s+["'][^"']*["'])?\)`)
	// Matches [text]: target
	refLinkRegex = regexp.MustCompile(`(?m)^\s{0,3}\[[^\]]+\]:\s*([^\s]+)`)
	// Matches HTML anchor tags: <a name="slug"> or <a id="slug">
	htmlAnchorRegex = regexp.MustCompile(`(?i)<a\s+(?:[^>]*\s+)?(?:id|name)=["']([^"']+)["']`)
	// Matches Markdown headings: # Heading
	headingRegex = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+)$`)
)

// ExtractHeadingsAndAnchors extracts all valid anchor slugs from Markdown source.
func ExtractHeadingsAndAnchors(src []byte) map[string]bool {
	anchors := make(map[string]bool)
	slugCounts := make(map[string]int)

	sc := bufio.NewScanner(bytes.NewReader(src))
	inCodeBlock := false

	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		// Check HTML anchors
		for _, m := range htmlAnchorRegex.FindAllStringSubmatch(line, -1) {
			if len(m) > 1 {
				anchors[m[1]] = true
			}
		}

		// Check Markdown headings
		if m := headingRegex.FindStringSubmatch(line); len(m) > 1 {
			rawHeading := strings.TrimSpace(m[1])
			// Strip trailing # chars if present
			rawHeading = strings.TrimRight(rawHeading, "# \t")
			baseSlug := GitHubSlug(rawHeading)
			if baseSlug != "" {
				count := slugCounts[baseSlug]
				slugCounts[baseSlug]++
				finalSlug := baseSlug
				if count > 0 {
					finalSlug = fmt.Sprintf("%s-%d", baseSlug, count)
				}
				anchors[finalSlug] = true
			}
		}
	}

	return anchors
}

// ExtractMarkdownLinks extracts all candidate links from Markdown content,
// ignoring links inside code blocks.
func ExtractMarkdownLinks(src []byte) []MarkdownLink {
	var links []MarkdownLink

	lines := strings.Split(string(src), "\n")
	inFencedCode := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFencedCode = !inFencedCode
			continue
		}
		if inFencedCode {
			continue
		}

		// Find inline links [text](target)
		matches := linkRegex.FindAllStringSubmatchIndex(line, -1)
		for _, idx := range matches {
			if len(idx) >= 4 && idx[2] >= 0 && idx[3] >= 0 {
				target := line[idx[2]:idx[3]]
				target = strings.Trim(target, "<>")
				links = append(links, MarkdownLink{
					Target: target,
					Line:   i + 1,
				})
			}
		}

		// Find reference link definitions [id]: target
		refMatches := refLinkRegex.FindAllStringSubmatchIndex(line, -1)
		for _, idx := range refMatches {
			if len(idx) >= 4 && idx[2] >= 0 && idx[3] >= 0 {
				target := line[idx[2]:idx[3]]
				target = strings.Trim(target, "<>")
				links = append(links, MarkdownLink{
					Target: target,
					Line:   i + 1,
				})
			}
		}
	}

	return links
}

// IsExternalLink checks whether a target URL is an external link.
func IsExternalLink(target string) bool {
	lower := strings.ToLower(target)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:") ||
		strings.HasPrefix(lower, "ftp://") ||
		strings.HasPrefix(lower, "irc://") ||
		strings.HasPrefix(lower, "tel:") ||
		strings.HasPrefix(lower, "//")
}

// CheckMarkdownLinks validates that relative links in the file point to existing files/anchors.
func CheckMarkdownLinks(repoRoot, relPath string, src []byte, headingCache map[string]map[string]bool) []string {
	var problems []string
	links := ExtractMarkdownLinks(src)
	dir := filepath.Dir(relPath)

	if headingCache != nil {
		if _, ok := headingCache[relPath]; !ok {
			headingCache[relPath] = ExtractHeadingsAndAnchors(src)
		}
	}

	for _, l := range links {
		target := l.Target
		if target == "" || IsExternalLink(target) {
			continue
		}

		parts := strings.SplitN(target, "#", 2)
		pathPart := parts[0]
		anchorPart := ""
		if len(parts) == 2 {
			anchorPart = parts[1]
		}

		var targetRelPath string
		if pathPart == "" {
			targetRelPath = relPath
		} else if strings.HasPrefix(pathPart, "/") {
			targetRelPath = filepath.Clean(strings.TrimPrefix(pathPart, "/"))
		} else {
			targetRelPath = filepath.Clean(filepath.Join(dir, pathPart))
		}

		// Prevent escaping repoRoot
		targetRelPath = filepath.ToSlash(targetRelPath)
		if strings.HasPrefix(targetRelPath, "../") || targetRelPath == ".." {
			problems = append(problems, fmt.Sprintf("%s:%d: broken link %q: points outside repository", relPath, l.Line, target))
			continue
		}

		targetFullPath := filepath.Join(repoRoot, targetRelPath)
		var isDir bool
		if targetRelPath != relPath {
			info, err := os.Stat(targetFullPath)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s:%d: broken link %q: file not found", relPath, l.Line, target))
				continue
			}
			isDir = info.IsDir()
		}

		if anchorPart != "" {
			checkFileRel := targetRelPath
			checkFileFull := targetFullPath
			if isDir {
				// Check README.md inside the directory
				readmeRel := filepath.ToSlash(filepath.Join(targetRelPath, "README.md"))
				readmeFull := filepath.Join(repoRoot, readmeRel)
				if _, err := os.Stat(readmeFull); err == nil {
					checkFileRel = readmeRel
					checkFileFull = readmeFull
				} else {
					problems = append(problems, fmt.Sprintf("%s:%d: broken link %q: directory has no README.md for anchor #%s", relPath, l.Line, target, anchorPart))
					continue
				}
			}

			if strings.HasSuffix(strings.ToLower(checkFileRel), ".md") {
				anchors, ok := headingCache[checkFileRel]
				if !ok {
					data, err := os.ReadFile(checkFileFull)
					if err != nil {
						problems = append(problems, fmt.Sprintf("%s:%d: broken link %q: cannot read target file", relPath, l.Line, target))
						continue
					}
					anchors = ExtractHeadingsAndAnchors(data)
					if headingCache != nil {
						headingCache[checkFileRel] = anchors
					}
				}

				if !anchors[anchorPart] {
					problems = append(problems, fmt.Sprintf("%s:%d: broken link %q: anchor #%s not found in %s", relPath, l.Line, target, anchorPart, checkFileRel))
				}
			}
		}
	}

	return problems
}
