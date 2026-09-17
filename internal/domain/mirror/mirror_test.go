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

func TestPlainTextBody(t *testing.T) {
	got := PlainTextBody(`<h2>Heading &amp; more</h2><p>Hello <strong>world</strong>.<br>Next line.</p>`, " SRC-16 ")
	want := "Heading & more\n\nHello world.\nNext line.\n\nMirrored from Plane: SRC-16"
	if got != want {
		t.Errorf("PlainTextBody() = %q, want %q", got, want)
	}
}

func TestPlainTextBodyWithoutHTML(t *testing.T) {
	got := PlainTextBody("", "SRC-16")
	want := "Mirrored from Plane: SRC-16"
	if got != want {
		t.Errorf("PlainTextBody() = %q, want %q", got, want)
	}
}
