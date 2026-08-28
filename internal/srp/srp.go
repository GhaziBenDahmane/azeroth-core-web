package srp

import (
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"math/big"
	"strings"
)

// Parameters and byte order match AzerothCore's Acore::Crypto::SRP6.
var modulus = func() *big.Int {
	n := new(big.Int)
	if _, ok := n.SetString("894B645E89E1535BBDAD5B8B290650530801B18EBFBF5E8FAB3C82872A3E9BB7", 16); !ok {
		panic("bad SRP modulus")
	}
	return n
}()

func Registration(username, password string) (salt, verifier []byte, err error) {
	username, password = strings.ToUpper(username), strings.ToUpper(password)
	salt = make([]byte, 32)
	if _, err = rand.Read(salt); err != nil {
		return nil, nil, err
	}
	verifier = calculate(username, password, salt)
	return salt, verifier, nil
}

func Verify(username, password string, salt, verifier []byte) bool {
	got := calculate(strings.ToUpper(username), strings.ToUpper(password), salt)
	if len(got) != len(verifier) {
		return false
	}
	var diff byte
	for i := range got {
		diff |= got[i] ^ verifier[i]
	}
	return diff == 0
}

func calculate(username, password string, salt []byte) []byte {
	inner := sha1.Sum([]byte(username + ":" + password))
	h := sha1.New()
	_, _ = h.Write(salt)
	_, _ = h.Write(inner[:])
	x := littleInt(h.Sum(nil))
	v := new(big.Int).Exp(big.NewInt(7), x, modulus)
	return littleBytes(v, 32)
}
func littleInt(b []byte) *big.Int {
	c := append([]byte(nil), b...)
	reverse(c)
	return new(big.Int).SetBytes(c)
}
func littleBytes(n *big.Int, size int) []byte {
	b := n.Bytes()
	out := make([]byte, size)
	for i := range b {
		if i < size {
			out[i] = b[len(b)-1-i]
		}
	}
	return out
}
func reverse(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}

func Validate(username, password string) error {
	if len(username) < 3 || len(username) > 32 {
		return fmt.Errorf("username must be 3–32 characters")
	}
	if len(password) < 8 || len(password) > 32 {
		return fmt.Errorf("password must be 8–32 characters")
	}
	for _, r := range username {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return fmt.Errorf("username may only contain Latin letters and numbers")
		}
	}
	for _, r := range password {
		if r < 32 || r > 126 {
			return fmt.Errorf("password must use printable ASCII characters")
		}
	}
	return nil
}
