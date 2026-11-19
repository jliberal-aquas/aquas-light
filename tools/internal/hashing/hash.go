// Package hashing computes the fingerprints stored in canon/manifest.json.
package hashing

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
)

// Normalize returns data with CRLF line endings converted to LF. It touches
// nothing else: spaces and lone CR bytes are preserved.
func Normalize(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

// Sum returns the SHA-256 of the normalized data as lowercase hex.
func Sum(data []byte) string {
	h := sha256.Sum256(Normalize(data))
	return hex.EncodeToString(h[:])
}
