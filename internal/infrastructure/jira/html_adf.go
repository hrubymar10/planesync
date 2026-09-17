package jira

import (
	"encoding/json"
	"html"
	"strconv"
	"strings"
	"unicode"
)

type adfNode struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []adfNode      `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []adfMark      `json:"marks,omitempty"`
}

type adfMark struct {
	Type  string            `json:"type"`
	Attrs map[string]string `json:"attrs,omitempty"`
}

type htmlNode struct {
	tag      string
	text     string
	attrs    map[string]string
	children []*htmlNode
}

// HTMLToADF converts a deterministic HTML subset to an ADF document.
func HTMLToADF(bodyHTML, reference string) json.RawMessage {
	root := parseHTML(bodyHTML)
	content := blocksFrom(root.children)
	content = append(content, referenceParagraph(strings.TrimSpace(reference)))
	document := struct {
		Version int       `json:"version"`
		Type    string    `json:"type"`
		Content []adfNode `json:"content"`
	}{Version: 1, Type: "doc", Content: content}
	encoded, _ := json.Marshal(document)
	return encoded
}

func parseHTML(source string) *htmlNode {
	root := &htmlNode{tag: "root"}
	stack := []*htmlNode{root}
	appendText := func(value string) {
		if value != "" {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, &htmlNode{text: html.UnescapeString(value)})
		}
	}

	for index := 0; index < len(source); {
		next := strings.IndexByte(source[index:], '<')
		if next < 0 {
			appendText(source[index:])
			break
		}
		if next > 0 {
			appendText(source[index : index+next])
			index += next
		}
		if strings.HasPrefix(source[index:], "<!--") {
			end := strings.Index(source[index+4:], "-->")
			if end < 0 {
				break
			}
			index += 4 + end + 3
			continue
		}
		end := htmlTagEnd(source, index+1)
		if end < 0 {
			appendText(source[index:])
			break
		}
		closing, selfClosing, name, attrs := parseHTMLTag(source[index+1 : end])
		index = end + 1
		if name == "" || strings.HasPrefix(name, "!") || strings.HasPrefix(name, "?") {
			continue
		}
		if closing {
			for stackIndex := len(stack) - 1; stackIndex > 0; stackIndex-- {
				if stack[stackIndex].tag == name {
					stack = stack[:stackIndex]
					break
				}
			}
			continue
		}

		node := &htmlNode{tag: name, attrs: attrs}
		parent := stack[len(stack)-1]
		parent.children = append(parent.children, node)
		if !selfClosing && !isVoidHTMLTag(name) {
			stack = append(stack, node)
		}
	}
	return root
}

func htmlTagEnd(source string, start int) int {
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

func parseHTMLTag(raw string) (closing, selfClosing bool, name string, attrs map[string]string) {
	raw = strings.TrimSpace(raw)
	closing = strings.HasPrefix(raw, "/")
	if closing {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "/"))
	}
	selfClosing = strings.HasSuffix(raw, "/")
	if selfClosing {
		raw = strings.TrimSpace(strings.TrimSuffix(raw, "/"))
	}

	nameEnd := strings.IndexFunc(raw, unicode.IsSpace)
	if nameEnd < 0 {
		return closing, selfClosing, strings.ToLower(raw), nil
	}
	name = strings.ToLower(raw[:nameEnd])
	attrs = parseHTMLAttrs(raw[nameEnd:])
	return closing, selfClosing, name, attrs
}

func parseHTMLAttrs(raw string) map[string]string {
	attrs := make(map[string]string)
	for index := 0; index < len(raw); {
		for index < len(raw) && unicode.IsSpace(rune(raw[index])) {
			index++
		}
		start := index
		for index < len(raw) && !unicode.IsSpace(rune(raw[index])) && raw[index] != '=' {
			index++
		}
		if start == index {
			index++
			continue
		}
		key := strings.ToLower(raw[start:index])
		for index < len(raw) && unicode.IsSpace(rune(raw[index])) {
			index++
		}
		if index >= len(raw) || raw[index] != '=' {
			attrs[key] = ""
			continue
		}
		index++
		for index < len(raw) && unicode.IsSpace(rune(raw[index])) {
			index++
		}
		if index >= len(raw) {
			attrs[key] = ""
			break
		}
		var value string
		if raw[index] == '\'' || raw[index] == '"' {
			quote := raw[index]
			index++
			start = index
			for index < len(raw) && raw[index] != quote {
				index++
			}
			value = raw[start:index]
			if index < len(raw) {
				index++
			}
		} else {
			start = index
			for index < len(raw) && !unicode.IsSpace(rune(raw[index])) {
				index++
			}
			value = raw[start:index]
		}
		attrs[key] = html.UnescapeString(value)
	}
	return attrs
}

func isVoidHTMLTag(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

func blocksFrom(nodes []*htmlNode) []adfNode {
	var result []adfNode
	var inline []*htmlNode
	flushInline := func() {
		content := inlineFrom(inline, nil)
		if hasVisibleInline(content) {
			result = append(result, adfNode{Type: "paragraph", Content: content})
		}
		inline = nil
	}

	for _, node := range nodes {
		if node.tag == "" || isInlineHTMLTag(node.tag) {
			inline = append(inline, node)
			continue
		}
		flushInline()
		result = append(result, blockFrom(node)...)
	}
	flushInline()
	return result
}

func blockFrom(node *htmlNode) []adfNode {
	switch node.tag {
	case "p":
		return []adfNode{{Type: "paragraph", Content: inlineFrom(node.children, nil)}}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level, _ := strconv.Atoi(strings.TrimPrefix(node.tag, "h"))
		return []adfNode{{Type: "heading", Attrs: map[string]any{"level": level}, Content: inlineFrom(node.children, nil)}}
	case "ul":
		return []adfNode{listFrom(node, "bulletList")}
	case "ol":
		return []adfNode{listFrom(node, "orderedList")}
	case "pre":
		text := rawHTMLText(node.children)
		content := []adfNode(nil)
		if text != "" {
			content = []adfNode{{Type: "text", Text: text}}
		}
		return []adfNode{{Type: "codeBlock", Content: content}}
	default:
		return []adfNode{{Type: "paragraph", Content: inlineFrom(node.children, nil)}}
	}
}

func listFrom(node *htmlNode, kind string) adfNode {
	list := adfNode{Type: kind}
	for _, child := range node.children {
		if child.tag == "li" {
			list.Content = append(list.Content, listItemFrom(child))
		}
	}
	return list
}

func listItemFrom(node *htmlNode) adfNode {
	item := adfNode{Type: "listItem", Content: blocksFrom(node.children)}
	if len(item.Content) == 0 {
		item.Content = []adfNode{{Type: "paragraph"}}
	}
	return item
}

func inlineFrom(nodes []*htmlNode, marks []adfMark) []adfNode {
	var result []adfNode
	for _, node := range nodes {
		if node.tag == "" {
			text := normalizeInlineText(node.text)
			if text != "" {
				result = append(result, adfNode{Type: "text", Text: text, Marks: cloneMarks(marks)})
			}
			continue
		}
		if node.tag == "br" {
			result = append(result, adfNode{Type: "hardBreak"})
			continue
		}

		nextMarks := marks
		switch node.tag {
		case "strong", "b":
			nextMarks = appendMark(marks, adfMark{Type: "strong"})
		case "em", "i":
			nextMarks = appendMark(marks, adfMark{Type: "em"})
		case "code":
			nextMarks = appendMark(marks, adfMark{Type: "code"})
		case "a":
			if href := strings.TrimSpace(node.attrs["href"]); href != "" {
				nextMarks = appendMark(marks, adfMark{Type: "link", Attrs: map[string]string{"href": href}})
			}
		}
		result = append(result, inlineFrom(node.children, nextMarks)...)
	}
	return result
}

func isInlineHTMLTag(name string) bool {
	switch name {
	case "a", "strong", "b", "em", "i", "code", "br":
		return true
	default:
		return false
	}
}

func normalizeInlineText(value string) string {
	var result strings.Builder
	inSpace := false
	for _, character := range value {
		if unicode.IsSpace(character) {
			inSpace = true
			continue
		}
		if inSpace {
			result.WriteByte(' ')
			inSpace = false
		}
		result.WriteRune(character)
	}
	if inSpace {
		result.WriteByte(' ')
	}
	return result.String()
}

func rawHTMLText(nodes []*htmlNode) string {
	var result strings.Builder
	for _, node := range nodes {
		if node.tag == "" {
			result.WriteString(node.text)
			continue
		}
		if node.tag == "br" {
			result.WriteByte('\n')
			continue
		}
		result.WriteString(rawHTMLText(node.children))
	}
	return result.String()
}

func hasVisibleInline(nodes []adfNode) bool {
	for _, node := range nodes {
		if node.Type == "hardBreak" || strings.TrimSpace(node.Text) != "" {
			return true
		}
	}
	return false
}

func appendMark(marks []adfMark, mark adfMark) []adfMark {
	result := cloneMarks(marks)
	return append(result, mark)
}

func cloneMarks(marks []adfMark) []adfMark {
	if len(marks) == 0 {
		return nil
	}
	return append([]adfMark(nil), marks...)
}

func referenceParagraph(reference string) adfNode {
	return adfNode{
		Type:    "paragraph",
		Content: []adfNode{{Type: "text", Text: "Mirrored from Plane: " + reference}},
	}
}
