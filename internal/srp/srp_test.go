package srp

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestVerifierKnownVector(t *testing.T) {
	salt, _ := hex.DecodeString("ad9087af43966aef8d7c218100c277406a1f33cb31f626a0504cf65ce6bf1d9c")
	want, _ := hex.DecodeString("ebe3604ce6b9f84f4308a410e718745231e35d8440533bcce6914da14d797d78")
	got := calculate("TEST", "TEST", salt)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	s, v, err := Registration("Player", "Strongpass1")
	if err != nil || !Verify("player", "strongpass1", s, v) {
		t.Fatal("round trip failed")
	}
	if Verify("player", "wrongpass", s, v) {
		t.Fatal("accepted wrong password")
	}
}
