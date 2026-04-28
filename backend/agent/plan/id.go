package plan

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

func NewID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return sanitizePrefix(prefix) + "_unknown"
	}
	return sanitizePrefix(prefix) + "_" + hex.EncodeToString(b[:])
}

func sanitizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "id"
	}
	return strings.ReplaceAll(prefix, " ", "_")
}
