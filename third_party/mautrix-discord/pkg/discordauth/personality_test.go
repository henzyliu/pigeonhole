package discordauth

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestPersonalityHeadersEncodesSuperProperties(t *testing.T) {
	personality := newTestPersonality()

	headers, err := personality.Headers()
	if err != nil {
		t.Fatalf("Headers returned error: %v", err)
	}

	encoded := headers.Get(HeaderSuperProperties)
	if encoded == "" {
		t.Fatal("expected super properties header to be set")
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode super properties header: %v", err)
	}

	var parsed map[string]any
	if err = json.Unmarshal(decoded, &parsed); err != nil {
		t.Fatalf("failed to unmarshal super properties JSON: %v", err)
	}
	if parsed["client_build_number"] != float64(1) {
		t.Fatalf("expected client_build_number to equal 1, got %#v", parsed["client_build_number"])
	}
}
