package jwt

import (
	"testing"
	"time"
)

func TestSignerIssueAndVerify(t *testing.T) {
	signer, err := NewSigner("test-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := signer.Sign("user-1")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := signer.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "user-1" || !claims.ExpiresAt.After(claims.IssuedAt) {
		t.Fatalf("claims=%+v", claims)
	}
}

func TestSignerRejectsInvalidSignature(t *testing.T) {
	issuer, err := NewSigner("issuer-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, err := issuer.Sign("user-1")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewSigner("verifier-secret", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(token); err == nil {
		t.Fatal("Verify() error = nil")
	}
}
