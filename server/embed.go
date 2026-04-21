package crosstalk

import "embed"

// WebDist holds the embedded web UI assets from web/dist/.
// In production builds this contains the Vite build output;
// the placeholder index.html is included for compile-time validity.
//
//go:embed all:web/dist
var WebDist embed.FS
