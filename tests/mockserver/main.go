// Command mockserver serves deterministic tracker responses for the end-to-end fixture.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
)

func main() {
	readyPath := flag.String("ready", "", "path to write the server base URL")
	violationPath := flag.String("violations", "", "path to record forbidden writes")
	flag.Parse()
	if *readyPath == "" || *violationPath == "" {
		fmt.Fprintln(os.Stderr, "-ready and -violations are required")
		os.Exit(2)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer listener.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/workspaces/source-workspace/projects/source-project/", jsonResponse(http.MethodGet,
		`{"identifier":"SRC"}`))
	mux.HandleFunc("/api/v1/workspaces/source-workspace/projects/source-project/states/", jsonResponse(http.MethodGet,
		`{"next_page_results":false,"results":[{"id":"state-open","name":"Open","group":"started"}]}`))
	mux.HandleFunc("/api/v1/workspaces/source-workspace/projects/source-project/work-items/", jsonResponse(http.MethodGet,
		`{"next_page_results":false,"results":[{"id":"item-current","sequence_id":16,"name":"Example item","description_html":"<p>Fixture body</p>","state":"state-open","updated_at":"2026-09-17T00:00:00Z"}]}`))
	mux.HandleFunc("/rest/api/3/search/jql", jsonResponse(http.MethodPost, `{"issues":[]}`))
	mux.HandleFunc("/rest/api/3/myself", jsonResponse(http.MethodGet, `{"accountId":"fixture-account"}`))
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		record := []byte(request.Method + " " + request.URL.Path + "\n")
		file, openErr := os.OpenFile(*violationPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if openErr == nil {
			_, _ = file.Write(record)
			_ = file.Close()
		}
		http.Error(writer, "forbidden fixture mutation", http.StatusMethodNotAllowed)
	})

	baseURL := "http://" + listener.Addr().String()
	if err := os.WriteFile(*readyPath, []byte(baseURL), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := http.Serve(listener, mux); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func jsonResponse(method, document string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != method {
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(document))
	}
}
