package mirror

import (
	"html"
	"net/url"
	"strings"
	"unicode"
)

// Summary combines an optional prefix and title with normalized outer spacing.
func Summary(prefix, title string) string {
	return strings.TrimSpace(strings.TrimSpace(prefix) + " " + strings.TrimSpace(title))
}

// Label returns the stable idempotency label for a Plane work item.
func Label(planeID string) string {
	return "plane-" + strings.TrimSpace(planeID)
}

// BackLink builds the Plane browser URL for a work item.
func BackLink(appBaseURL, workspace, projectID, itemID string) string {
	return strings.TrimRight(strings.TrimSpace(appBaseURL), "/") + "/" +
		url.PathEscape(workspace) + "/projects/" + url.PathEscape(projectID) +
		"/issues/" + url.PathEscape(itemID)
}

// PlainTextBody strips HTML and appends the source work item's browser link.
func PlainTextBody(bodyHTML, backLink string) string {
	body := stripHTML(bodyHTML)
	footer := "Mirrored from Plane: " + strings.TrimSpace(backLink)
	if body == "" {
		return footer
	}
	return body + "\n\n" + footer
}

func stripHTML(source string) string {
	var output strings.Builder
	for index := 0; index < len(source); {
		if source[index] != '<' {
			next := strings.IndexByte(source[index:], '<')
			if next < 0 {
				next = len(source) - index
			}
			output.WriteString(html.UnescapeString(source[index : index+next]))
			index += next
			continue
		}
		end := findTagEnd(source, index+1)
		if end < 0 {
			output.WriteString(html.UnescapeString(source[index:]))
			break
		}
		name := tagName(source[index+1 : end])
		if isTextBreak(name) {
			output.WriteByte('\n')
		}
		index = end + 1
	}

	lines := strings.Split(output.String(), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.FieldsFunc(line, unicode.IsSpace), " ")
		if line == "" {
			if len(cleaned) > 0 && cleaned[len(cleaned)-1] != "" {
				cleaned = append(cleaned, "")
			}
			continue
		}
		cleaned = append(cleaned, line)
	}
	for len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return strings.Join(cleaned, "\n")
}

func findTagEnd(source string, start int) int {
	var quote byte
	for index := start; index < len(source); index++ {
		character := source[index]
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == '>' {
			return index
		}
	}
	return -1
}

func tagName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "/")
	end := strings.IndexFunc(raw, func(character rune) bool {
		return unicode.IsSpace(character) || character == '/'
	})
	if end >= 0 {
		raw = raw[:end]
	}
	return strings.ToLower(raw)
}

func isTextBreak(name string) bool {
	switch name {
	case "br", "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "pre", "blockquote":
		return true
	default:
		return false
	}
}
