package ws

// HashTokenForTest exposes hashToken for use in tests. This function is
// intentionally in a non-_test.go file so that external test packages
// (ws_test) can compute expected hashes.
func HashTokenForTest(plaintext string) string {
	return hashToken(plaintext)
}
