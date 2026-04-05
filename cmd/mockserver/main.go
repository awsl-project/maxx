// cmd/mockserver is a standalone multi-protocol mock upstream server.
// It supports OpenAI, Claude, Gemini, and Codex protocols.
// Behavior is controlled via the X-Mock-Response request header.
//
// Usage:
//
//	go run ./cmd/mockserver -addr :9999
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/awsl-project/maxx/internal/testutil/mockserver"
)

func main() {
	addr := flag.String("addr", ":9999", "Listen address")
	flag.Parse()

	log.Printf("Mock server listening on %s", *addr)
	log.Printf("Control via header: %s", mockserver.MockHeader)

	if err := http.ListenAndServe(*addr, mockserver.Handler()); err != nil {
		log.Fatal(err)
	}
}
