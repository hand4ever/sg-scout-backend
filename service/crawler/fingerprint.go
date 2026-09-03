package crawler

import (
	"crypto/sha256"
	"encoding/hex"
)

// Fingerprint computes the content fingerprint of the body markdown text
// (front-matter stripped by caller). Research rev2: sha256 of engine markdown.
func Fingerprint(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
