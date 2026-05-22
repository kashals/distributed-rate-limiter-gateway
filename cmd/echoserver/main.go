package main

// dev utility: minimal HTTP echo server for local backend testing
// echoes method, path, and all received headers as plain text
// usage: go run ./cmd/echoserver

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s %s\n\n", r.Method, r.RequestURI)
		for key, vals := range r.Header {
			for _, v := range vals {
				fmt.Fprintf(w, "%s: %s\n", key, v)
			}
		}
	})

	log.Println("echo server listening on :9000")
	log.Fatal(http.ListenAndServe(":9000", nil))
}
