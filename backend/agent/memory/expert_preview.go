package memory

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/schema"
)

const (
	DOC_PREVIEW_START_MARKER = "<!--DOC_PREVIEW_START-->"
	DOC_PREVIEW_END_MARKER   = "<!--DOC_PREVIEW_END-->"

	docPreviewStartPrefix = "<!--DOC_PREVIEW_START"
	docPreviewPrefix      = "<!--DOC_PREVIEW"
)

var reThinkNeverUsed = regexp.MustCompile(`</think_never_used_[^>]*>`)

func SanitizeModelLeakage(s string) string {
	if s == "" {
		return ""
	}
	return reThinkNeverUsed.ReplaceAllString(s, "")
}

func ExtractExpertPreviewMarkdown(full string) string {
	full = strings.TrimSpace(SanitizeModelLeakage(full))
	if full == "" {
		return ""
	}
	if body, ok := extractBetweenPreviewMarkers(full, DOC_PREVIEW_START_MARKER); ok {
		return body
	}
	if body, ok := extractBetweenPreviewMarkers(full, docPreviewStartPrefix); ok {
		return body
	}
	if body, ok := extractBetweenPreviewMarkers(full, docPreviewPrefix); ok {
		return body
	}

	const fence = "```markdown"
	if i := strings.Index(full, fence); i >= 0 {
		rest := full[i+len(fence):]
		rest = strings.TrimLeft(rest, "\r\n")
		if j := strings.Index(rest, "```"); j >= 0 {
			return strings.TrimSpace(rest[:j])
		}
	}
	return ""
}

func extractBetweenPreviewMarkers(full, start string) (string, bool) {
	i := strings.Index(full, start)
	if i < 0 {
		return "", false
	}
	rest := full[i+len(start):]
	rest = strings.TrimPrefix(rest, "-->")
	rest = strings.TrimLeftFunc(rest, unicode.IsSpace)
	if end := strings.Index(rest, DOC_PREVIEW_END_MARKER); end >= 0 {
		return strings.TrimSpace(rest[:end]), true
	}
	return strings.TrimSpace(rest), true
}

func StripExpertPreviewRegions(full string) string {
	s := SanitizeModelLeakage(full)
	for {
		i, startLen := findPreviewStart(s)
		if i < 0 {
			break
		}
		restStart := i + startLen
		if strings.HasPrefix(s[restStart:], "-->") {
			restStart += 3
		}
		if j := strings.Index(s[restStart:], DOC_PREVIEW_END_MARKER); j >= 0 {
			endCut := restStart + j + len(DOC_PREVIEW_END_MARKER)
			s = strings.TrimSpace(s[:i] + s[endCut:])
			continue
		}
		s = strings.TrimSpace(s[:i])
		break
	}
	s = strings.ReplaceAll(s, DOC_PREVIEW_START_MARKER, "")
	s = strings.ReplaceAll(s, DOC_PREVIEW_END_MARKER, "")
	s = strings.ReplaceAll(s, docPreviewStartPrefix, "")
	s = strings.ReplaceAll(s, docPreviewPrefix, "")
	return strings.TrimSpace(s)
}

func StripExpertPreviewRegionsForStream(full string) string {
	s := StripExpertPreviewRegions(full)
	for i := 1; i < len(docPreviewPrefix); i++ {
		if strings.HasSuffix(s, docPreviewPrefix[:i]) {
			return strings.TrimSpace(s[:len(s)-i])
		}
	}
	return s
}

func findPreviewStart(s string) (int, int) {
	candidates := []string{
		DOC_PREVIEW_START_MARKER,
		docPreviewStartPrefix,
		docPreviewPrefix,
	}
	best := -1
	bestLen := 0
	for _, marker := range candidates {
		i := strings.Index(s, marker)
		if i < 0 {
			continue
		}
		if best < 0 || i < best || (i == best && len(marker) > bestLen) {
			best = i
			bestLen = len(marker)
		}
	}
	return best, bestLen
}

func ExpertPreviewMarkdownFromHistory(msgs []*schema.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if m != nil && m.Role == schema.Assistant && strings.TrimSpace(m.Content) != "" {
			b.WriteString(m.Content)
		}
	}
	return ExtractExpertPreviewMarkdown(b.String())
}
