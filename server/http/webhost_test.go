package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	cthttp "github.com/aleksclark/crosstalk/server/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFS returns an fstest.MapFS that mimics a built web/dist/ directory.
func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte(`<!doctype html><html><body><div id="root"></div></body></html>`),
		},
		"favicon.svg": &fstest.MapFile{
			Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		},
		"assets/main.js": &fstest.MapFile{
			Data: []byte(`console.log("hello")`),
		},
	}
}

func TestEmbedHandler_RootReturnsHTML(t *testing.T) {
	handler := cthttp.EmbedHandler(testFS())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `<div id="root">`)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

func TestEmbedHandler_SPAFallback(t *testing.T) {
	handler := cthttp.EmbedHandler(testFS())

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `<div id="root">`)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

func TestEmbedHandler_SPAFallbackDeepPath(t *testing.T) {
	handler := cthttp.EmbedHandler(testFS())

	req := httptest.NewRequest(http.MethodGet, "/sessions/123/details", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `<div id="root">`)
}

func TestEmbedHandler_ServesStaticFile(t *testing.T) {
	handler := cthttp.EmbedHandler(testFS())

	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `<svg`)
}

func TestEmbedHandler_ServesNestedStaticFile(t *testing.T) {
	handler := cthttp.EmbedHandler(testFS())

	req := httptest.NewRequest(http.MethodGet, "/assets/main.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `console.log`)
}

func TestEmbedHandler_APIPathNotIntercepted(t *testing.T) {
	handler := cthttp.EmbedHandler(testFS())

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	// Should NOT contain our index.html content.
	assert.NotContains(t, rec.Body.String(), `<div id="root">`)
}

func TestEmbedHandler_WSPathNotIntercepted(t *testing.T) {
	handler := cthttp.EmbedHandler(testFS())

	req := httptest.NewRequest(http.MethodGet, "/ws/signaling", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Body.String(), `<div id="root">`)
}

func TestDevProxyHandler_APIPathNotIntercepted(t *testing.T) {
	// The dev proxy handler should return 404 for /api/ paths
	// even without a running Vite server.
	handler := cthttp.DevProxyHandler("http://127.0.0.1:0")

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDevProxyHandler_WSPathNotIntercepted(t *testing.T) {
	handler := cthttp.DevProxyHandler("http://127.0.0.1:0")

	req := httptest.NewRequest(http.MethodGet, "/ws/broadcast", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDevProxyHandler_ProxiesNonAPIRequests(t *testing.T) {
	// Start a test server to act as the Vite dev server.
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!doctype html><html><body>vite</body></html>`)) //nolint:errcheck
	}))
	defer vite.Close()

	handler := cthttp.DevProxyHandler(vite.URL)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "vite")
}

func TestRouter_WebHandlerIntegration(t *testing.T) {
	// Build a minimal handler with WebFS set to verify the catch-all
	// works alongside the API routes.
	h := &cthttp.Handler{
		WebFS: testFS(),
	}

	router := h.Router()

	// Root should return the SPA.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `<div id="root">`)

	// SPA fallback for unknown path.
	req = httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `<div id="root">`)

	// API path should NOT be intercepted by the web handler.
	// (Will return 405/401/etc from the API routes, not 200 with HTML.)
	req = httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	// The API route requires auth, so we expect 401, not 200 with HTML.
	assert.NotContains(t, rec.Body.String(), `<div id="root">`)
}
