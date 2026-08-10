// Command gen-openapi writes the server's generated OpenAPI 3.1 spec to stdout
// (or to the file given as the first argument).
//
// It constructs the real API server with empty service dependencies — no
// database or network is required, because only the registered route and type
// metadata is introspected, never the handler bodies. This makes the spec a
// build-time artifact derived from the actual Go API, so the generated
// TypeScript/Go clients can never drift from the server.
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/aleksclark/crosstalk/server/api"
)

func main() {
	// Silence server logs on stderr noise; only the spec goes to stdout.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	srv := api.NewServer(api.Config{Addr: ":0", JWTSecret: "gen"}, api.Services{}, log)

	spec, err := srv.OpenAPIJSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-openapi: marshal spec:", err)
		os.Exit(1)
	}

	out := os.Stdout
	if len(os.Args) > 1 {
		f, err := os.Create(os.Args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "gen-openapi: create output:", err)
			os.Exit(1)
		}
		defer func() { _ = f.Close() }()
		out = f
	}

	if _, err := out.Write(spec); err != nil {
		fmt.Fprintln(os.Stderr, "gen-openapi: write:", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 {
		_, _ = out.Write([]byte("\n"))
	}
}
