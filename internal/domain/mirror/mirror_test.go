package mirror

import "testing"

func TestSummary(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		title  string
		want   string
	}{
		{name: "prefix", prefix: " [team] ", title: " Example title ", want: "[team] Example title"},
		{name: "empty prefix", title: " Example title ", want: "Example title"},
		{name: "empty title", prefix: "[team]", want: "[team]"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Summary(test.prefix, test.title); got != test.want {
				t.Errorf("Summary() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLabel(t *testing.T) {
	if got := Label(" source-item "); got != "plane-source-item" {
		t.Errorf("Label() = %q", got)
	}
}

func TestBackLink(t *testing.T) {
	got := BackLink("https://app.plane.so/", "example workspace", "project/value", "item value")
	want := "https://app.plane.so/example%20workspace/projects/project%2Fvalue/issues/item%20value"
	if got != want {
		t.Errorf("BackLink() = %q, want %q", got, want)
	}
}

func TestPlainTextBody(t *testing.T) {
	got := PlainTextBody(`<h2>Heading &amp; more</h2><p>Hello <strong>world</strong>.<br>Next line.</p>`, "https://plane.example.com/item")
	want := "Heading & more\n\nHello world.\nNext line.\n\nMirrored from Plane: https://plane.example.com/item"
	if got != want {
		t.Errorf("PlainTextBody() = %q, want %q", got, want)
	}
}

func TestPlainTextBodyWithoutHTML(t *testing.T) {
	got := PlainTextBody("", "https://plane.example.com/item")
	want := "Mirrored from Plane: https://plane.example.com/item"
	if got != want {
		t.Errorf("PlainTextBody() = %q, want %q", got, want)
	}
}
