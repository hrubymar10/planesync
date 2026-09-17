package jira

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestHTMLToADFSupportedBlocks(t *testing.T) {
	backLink := "https://plane.example.com/item"
	tests := []struct {
		name     string
		html     string
		wantType string
		check    func(*testing.T, adfNode)
	}{
		{
			name: "paragraph and break", html: `<p>First<br>Second</p>`, wantType: "paragraph",
			check: func(t *testing.T, node adfNode) {
				if len(node.Content) != 3 || node.Content[1].Type != "hardBreak" {
					t.Errorf("paragraph content = %#v", node.Content)
				}
			},
		},
		{
			name: "heading", html: `<h3>Heading</h3>`, wantType: "heading",
			check: func(t *testing.T, node adfNode) {
				if node.Attrs["level"] != float64(3) {
					t.Errorf("heading attrs = %#v", node.Attrs)
				}
			},
		},
		{
			name: "bullet list", html: `<ul><li>One</li><li>Two</li></ul>`, wantType: "bulletList",
			check: func(t *testing.T, node adfNode) {
				if len(node.Content) != 2 || node.Content[0].Type != "listItem" {
					t.Errorf("bullet list = %#v", node)
				}
			},
		},
		{
			name: "ordered list", html: `<ol><li>One</li></ol>`, wantType: "orderedList",
			check: func(t *testing.T, node adfNode) {
				if len(node.Content) != 1 || node.Content[0].Type != "listItem" {
					t.Errorf("ordered list = %#v", node)
				}
			},
		},
		{
			name: "code block", html: `<pre><code>line one
line two</code></pre>`, wantType: "codeBlock",
			check: func(t *testing.T, node adfNode) {
				if len(node.Content) != 1 || node.Content[0].Text != "line one\nline two" {
					t.Errorf("code block = %#v", node)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := decodeADF(t, HTMLToADF(test.html, backLink))
			if len(document.Content) != 2 || document.Content[0].Type != test.wantType {
				t.Fatalf("document content = %#v", document.Content)
			}
			test.check(t, document.Content[0])
			assertBackLink(t, document.Content[1], backLink)
		})
	}
}

func TestHTMLToADFInlineMarks(t *testing.T) {
	document := decodeADF(t, HTMLToADF(
		`<p><a href="https://example.com/?a=1&amp;b=2">Link</a> <strong>Strong</strong> <b>Bold</b> <em>Emphasis</em> <i>Italic</i> <code>Code</code></p>`,
		"https://plane.example.com/item",
	))
	paragraph := document.Content[0]
	wantMarks := []string{"link", "strong", "strong", "em", "em", "code"}
	var gotMarks []string
	for _, node := range paragraph.Content {
		if len(node.Marks) > 0 {
			gotMarks = append(gotMarks, node.Marks[0].Type)
		}
	}
	if len(gotMarks) != len(wantMarks) {
		t.Fatalf("marks = %#v, want %#v", gotMarks, wantMarks)
	}
	for index := range wantMarks {
		if gotMarks[index] != wantMarks[index] {
			t.Errorf("mark %d = %q, want %q", index, gotMarks[index], wantMarks[index])
		}
	}
	if got := paragraph.Content[0].Marks[0].Attrs["href"]; got != "https://example.com/?a=1&b=2" {
		t.Errorf("link href = %q", got)
	}
}

func TestHTMLToADFHeadingLevels(t *testing.T) {
	for level := 1; level <= 6; level++ {
		t.Run(fmt.Sprintf("h%d", level), func(t *testing.T) {
			source := fmt.Sprintf("<h%d>Heading</h%d>", level, level)
			document := decodeADF(t, HTMLToADF(source, "https://plane.example.com/item"))
			heading := document.Content[0]
			if heading.Type != "heading" || heading.Attrs["level"] != float64(level) {
				t.Errorf("heading = %#v", heading)
			}
		})
	}
}

func TestHTMLToADFUnknownTagDegradesToParagraph(t *testing.T) {
	document := decodeADF(t, HTMLToADF(`<aside>Fallback <u>content</u></aside>`, "https://plane.example.com/item"))
	if len(document.Content) != 2 || document.Content[0].Type != "paragraph" {
		t.Fatalf("document content = %#v", document.Content)
	}
	if got := inlineText(document.Content[0]); got != "Fallback content" {
		t.Errorf("fallback text = %q", got)
	}
}

func TestHTMLToADFAlwaysReturnsValidDocument(t *testing.T) {
	for _, source := range []string{"", `<p>unclosed`, `<div title=">">Text &amp; more</div>`, `<p><!-- ignored -->Visible</p>`} {
		encoded := HTMLToADF(source, "https://plane.example.com/item")
		if !json.Valid(encoded) {
			t.Errorf("HTMLToADF(%q) returned invalid JSON: %s", source, encoded)
		}
		document := decodeADF(t, encoded)
		if document.Version != 1 || document.Type != "doc" || len(document.Content) == 0 {
			t.Errorf("invalid ADF envelope: %#v", document)
		}
	}
}

func decodeADF(t *testing.T, encoded json.RawMessage) struct {
	Version int       `json:"version"`
	Type    string    `json:"type"`
	Content []adfNode `json:"content"`
} {
	t.Helper()
	var document struct {
		Version int       `json:"version"`
		Type    string    `json:"type"`
		Content []adfNode `json:"content"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("unmarshal ADF: %v", err)
	}
	return document
}

func assertBackLink(t *testing.T, paragraph adfNode, backLink string) {
	t.Helper()
	if paragraph.Type != "paragraph" || len(paragraph.Content) != 2 {
		t.Fatalf("back-link paragraph = %#v", paragraph)
	}
	link := paragraph.Content[1]
	if link.Text != backLink || len(link.Marks) != 1 || link.Marks[0].Type != "link" || link.Marks[0].Attrs["href"] != backLink {
		t.Errorf("back-link node = %#v", link)
	}
}

func inlineText(node adfNode) string {
	var text string
	for _, child := range node.Content {
		text += child.Text
	}
	return text
}
