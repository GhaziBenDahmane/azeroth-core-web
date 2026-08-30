package web

import (
	"crypto/elliptic"
	"encoding/binary"
	"testing"

	"github.com/example/azeroth-portal/internal/config"
)

func TestDecodeCBORRejectsUnhashableMapKeys(t *testing.T) {
	// A one-entry map whose key is an array must return an error, not panic.
	if _, _, err := decodeCBOR([]byte{0xa1, 0x80, 0x01}); err == nil {
		t.Fatal("decodeCBOR accepted an unsupported array map key")
	}
}

func TestParseNoneAttestationES256(t *testing.T) {
	s := &Server{c: config.Config{PublicURL: "https://portal.example.com"}}
	authData := append([]byte(nil), s.passkeyRPHash()...)
	authData = append(authData, 0x45, 0, 0, 0, 1)
	authData = append(authData, make([]byte, 16)...)
	credentialID := []byte("credential-id-01")
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(credentialID)))
	authData = append(authData, length...)
	authData = append(authData, credentialID...)
	x, y := elliptic.P256().Params().Gx.FillBytes(make([]byte, 32)), elliptic.P256().Params().Gy.FillBytes(make([]byte, 32))
	authData = append(authData, 0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20)
	authData = append(authData, x...)
	authData = append(authData, 0x22, 0x58, 0x20)
	authData = append(authData, y...)
	attestation := []byte{0xa3, 0x63, 'f', 'm', 't', 0x64, 'n', 'o', 'n', 'e', 0x68, 'a', 'u', 't', 'h', 'D', 'a', 't', 'a', 0x58, byte(len(authData))}
	attestation = append(attestation, authData...)
	attestation = append(attestation, 0x67, 'a', 't', 't', 'S', 't', 'm', 't', 0xa0)
	gotID, gotX, gotY, count, err := s.parseNoneAttestation(attestation)
	if err != nil || string(gotID) != string(credentialID) || string(gotX) != string(x) || string(gotY) != string(y) || count != 1 {
		t.Fatalf("parsed passkey = id %q count %d err %v", gotID, count, err)
	}
}
