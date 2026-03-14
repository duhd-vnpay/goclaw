package googlechat

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	codePlaceholder = "\x00"
	boldMarker      = "\x01" // temp marker for bold * to avoid italic conversion
)

var (
	reCodeBlock     = regexp.MustCompile("(?s)(```[\\s\\S]*?```)")
	reCodeInline    = regexp.MustCompile("(`[^`]+`)")
	reBoldItalic    = regexp.MustCompile(`\*\*\*(.+?)\*\*\*`)
	reBold          = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reStrike        = regexp.MustCompile(`~~(.+?)~~`)
	reLink          = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reTable         = regexp.MustCompile(`(?m)^\|.+\|$\n^\|[-| :]+\|$`)
	reLongCodeBlock = regexp.MustCompile("(?s)```[\\w]*\\n(.{500,}?)\\n```")
)

func markdownToGoogleChat(text string) string {
	if text == "" {
		return ""
	}

	var codeBlocks []string
	protected := reCodeBlock.ReplaceAllStringFunc(text, func(match string) string {
		codeBlocks = append(codeBlocks, match)
		return codePlaceholder
	})
	var inlineCodes []string
	protected = reCodeInline.ReplaceAllStringFunc(protected, func(match string) string {
		inlineCodes = append(inlineCodes, match)
		return codePlaceholder
	})

	// Bold+italic: ***text*** → *_text_* (use boldMarker to protect from italic converter)
	protected = reBoldItalic.ReplaceAllString(protected, boldMarker+"_${1}_"+boldMarker)
	// Bold: **text** → *text* (use boldMarker to protect from italic converter)
	protected = reBold.ReplaceAllString(protected, boldMarker+"${1}"+boldMarker)
	// Italic: *text* → _text_ (only matches unprotected single *)
	protected = convertItalic(protected)
	// Restore boldMarker → *
	protected = strings.ReplaceAll(protected, boldMarker, "*")
	protected = reStrike.ReplaceAllString(protected, "~${1}~")
	protected = reLink.ReplaceAllString(protected, "<${2}|${1}>")

	codeIdx := 0
	inlineIdx := 0
	var result strings.Builder
	for _, r := range protected {
		if string(r) == codePlaceholder {
			if codeIdx < len(codeBlocks) {
				result.WriteString(codeBlocks[codeIdx])
				codeIdx++
			} else if inlineIdx < len(inlineCodes) {
				result.WriteString(inlineCodes[inlineIdx])
				inlineIdx++
			}
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

func convertItalic(s string) string {
	var result strings.Builder
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '*' {
			prevStar := i > 0 && runes[i-1] == '*'
			nextStar := i+1 < len(runes) && runes[i+1] == '*'
			if !prevStar && !nextStar {
				end := -1
				for j := i + 1; j < len(runes); j++ {
					if runes[j] == '*' {
						nextJ := j+1 < len(runes) && runes[j+1] == '*'
						prevJ := j > 0 && runes[j-1] == '*'
						if !nextJ && !prevJ {
							end = j
							break
						}
					}
				}
				if end > 0 {
					result.WriteRune('_')
					result.WriteString(string(runes[i+1 : end]))
					result.WriteRune('_')
					i = end + 1
					continue
				}
			}
		}
		result.WriteRune(runes[i])
		i++
	}
	return result.String()
}

func detectStructuredContent(text string) bool {
	return reTable.MatchString(text) || reLongCodeBlock.MatchString(text)
}

func chunkByBytes(text string, maxBytes int) []string {
	if text == "" {
		return nil
	}
	if len([]byte(text)) <= maxBytes {
		return []string{text}
	}

	var chunks []string
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) > 1 {
		var current strings.Builder
		for i, p := range paragraphs {
			sep := ""
			if i > 0 {
				sep = "\n\n"
			}
			candidate := current.String() + sep + p
			if len([]byte(candidate)) > maxBytes && current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
				current.WriteString(p)
			} else {
				if current.Len() > 0 {
					current.WriteString(sep)
				}
				current.WriteString(p)
			}
		}
		if current.Len() > 0 {
			remaining := current.String()
			if len([]byte(remaining)) > maxBytes {
				chunks = append(chunks, chunkByLines(remaining, maxBytes)...)
			} else {
				chunks = append(chunks, remaining)
			}
		}
		return chunks
	}

	return chunkByLines(text, maxBytes)
}

func chunkByLines(text string, maxBytes int) []string {
	lines := strings.Split(text, "\n")
	if len(lines) <= 1 {
		return chunkByWords(text, maxBytes)
	}

	var chunks []string
	var current strings.Builder
	for i, line := range lines {
		sep := ""
		if i > 0 {
			sep = "\n"
		}
		candidate := current.String() + sep + line
		if len([]byte(candidate)) > maxBytes && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			if len([]byte(line)) > maxBytes {
				chunks = append(chunks, chunkByWords(line, maxBytes)...)
			} else {
				current.WriteString(line)
			}
		} else {
			if current.Len() > 0 {
				current.WriteString(sep)
			}
			current.WriteString(line)
		}
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func chunkByWords(text string, maxBytes int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var chunks []string
	var current strings.Builder
	for _, word := range words {
		sep := ""
		if current.Len() > 0 {
			sep = " "
		}
		candidate := current.String() + sep + word
		if len([]byte(candidate)) > maxBytes && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
			if len([]byte(word)) > maxBytes {
				chunks = append(chunks, splitAtUTF8Boundary(word, maxBytes)...)
			} else {
				current.WriteString(word)
			}
		} else {
			if current.Len() > 0 {
				current.WriteString(sep)
			}
			current.WriteString(word)
		}
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func splitAtUTF8Boundary(word string, maxBytes int) []string {
	var chunks []string
	b := []byte(word)
	for len(b) > 0 {
		end := maxBytes
		if end > len(b) {
			end = len(b)
		}
		for end > 0 && !utf8.Valid(b[:end]) {
			end--
		}
		if end == 0 {
			end = 1
		}
		chunks = append(chunks, string(b[:end]))
		b = b[end:]
	}
	return chunks
}

func extractSummary(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	var heading string
	var bullets []string
	var textLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && heading == "" {
			heading = strings.TrimPrefix(trimmed, "# ")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") && heading != "" {
			break
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if len(bullets) < 3 {
				bullets = append(bullets, trimmed)
			}
			continue
		}
		if trimmed != "" && heading != "" && len(bullets) == 0 {
			textLines = append(textLines, trimmed)
		}
	}

	var parts []string
	if heading != "" {
		parts = append(parts, heading)
	}
	if len(textLines) > 0 && len(bullets) == 0 {
		parts = append(parts, strings.Join(textLines, "\n"))
	}
	if len(bullets) > 0 {
		parts = append(parts, strings.Join(bullets, "\n"))
	}

	result := strings.Join(parts, "\n\n")
	if result == "" {
		return content
	}
	return result
}
