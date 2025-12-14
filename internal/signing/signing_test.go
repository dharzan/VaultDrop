package signing

import "testing"

func TestSigner(t *testing.T) {
	// testing.T is provided by Go's stdlib test framework; helper methods like
	// Fatalf fail the test immediately.
	secret := []byte("topsecret")
	s := NewSigner(secret)
	sig := s.Sign("file123", 1700000000)
	if len(sig) == 0 {
		t.Fatalf("expected signature")
	}
	// Positive case: Validate should succeed with matching inputs.
	if !s.Validate("file123", "1700000000", sig) {
		t.Fatalf("expected signature to validate")
	}
	// Negative cases ensure Validate is strict about every parameter.
	if s.Validate("wrong", "1700000000", sig) {
		t.Fatalf("expected validation to fail for wrong file id")
	}
	if s.Validate("file123", "42", sig) {
		t.Fatalf("expected validation to fail for wrong expiry")
	}
}
