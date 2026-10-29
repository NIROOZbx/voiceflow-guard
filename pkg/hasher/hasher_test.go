package hasher_test

import (
	"testing"

	"github.com/NIROOZbx/voiceflow-guard/pkg/hasher"
)

func TestString(t *testing.T) {
	input := "hello world"
	// known sha256 for "hello world"
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	actual := hasher.String(input)
	if actual != expected {
		t.Fatalf("expected %s, got %s", expected, actual)
	}
}

func TestBytes(t *testing.T) {
	input := []byte("hello world")
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	actual := hasher.Bytes(input)
	if actual != expected {
		t.Fatalf("expected %s, got %s", expected, actual)
	}
}

func TestJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	p := payload{Name: "Alice", Age: 30}
	hash, err := hasher.JSON(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hash) != 64 {
		t.Fatalf("expected 64-character sha256 hex string, got len %d", len(hash))
	}
}
