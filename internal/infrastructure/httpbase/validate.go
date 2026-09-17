package httpbase

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

const insecureOverride = "PLANESYNC_ALLOW_INSECURE_BASE_URLS"

// Validate requires HTTPS, except for an explicitly enabled test-only loopback seam.
func Validate(service string, baseURL *url.URL) error {
	if baseURL.Host == "" {
		return fmt.Errorf("%s base URL must use HTTPS and include a host", service)
	}
	if baseURL.Scheme == "https" {
		return nil
	}
	if baseURL.Scheme == "http" && os.Getenv(insecureOverride) == "1" && isLoopback(baseURL.Hostname()) {
		return nil
	}
	return fmt.Errorf("%s base URL must use HTTPS and include a host", service)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
