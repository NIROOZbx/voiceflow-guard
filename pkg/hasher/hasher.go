package hasher

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
)


func NewIdempotencyKey() string {
	return uuid.NewString()
}
// String returns the hex-encoded SHA-256 hash of a string.
func String(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// Bytes returns the hex-encoded SHA-256 hash of a byte slice.
func Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// JSON serializes an arbitrary value to JSON and computes its hex-encoded SHA-256 hash.
// It is useful for generating request hashes from payload structs.
func JSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return Bytes(data), nil
}
