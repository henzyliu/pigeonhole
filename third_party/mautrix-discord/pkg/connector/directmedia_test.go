package connector

import (
	"testing"
	"time"
)

func TestParseAttachmentExpiryParam(t *testing.T) {
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("failed to load test timezone: %v", err)
	}

	expiry := parseAttachmentExpiryParam("69be6214").In(losAngeles)
	got := expiry.String()
	want := "2026-03-21 02:17:08 -0700 PDT"

	if got != want {
		t.Fatalf("unexpected parsed expiry: got %q, want %q", got, want)
	}
}
