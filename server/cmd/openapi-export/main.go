package main

import (
	"fmt"
	"os"

	cthttp "github.com/aleksclark/crosstalk/server/http"
)

func main() {
	_, err := os.Stdout.Write(cthttp.OpenAPISpecJSON())
	if err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}
}
