package httpbase

import (
	"net/url"
	"testing"
)

func TestInsecureLoopbackOverrideIsOffByDefault(t *testing.T) {
	t.Setenv(insecureOverride, "")
	if err := Validate("Example", mustParse(t, "http://127.0.0.1:8080")); err == nil {
		t.Fatal("Validate() accepted insecure loopback URL without test override")
	}
}

func TestInsecureLoopbackOverrideIsNarrow(t *testing.T) {
	t.Setenv(insecureOverride, "1")
	if err := Validate("Example", mustParse(t, "http://127.0.0.1:8080")); err != nil {
		t.Fatalf("Validate() rejected test loopback URL: %v", err)
	}
	if err := Validate("Example", mustParse(t, "http://example.com")); err == nil {
		t.Fatal("Validate() accepted non-loopback insecure URL")
	}
}

func mustParse(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", value, err)
	}
	return parsed
}
