package web

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type passkeyCredential struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Transports []string   `json:"transports,omitempty"`
	Created    time.Time  `json:"created"`
	LastUsed   *time.Time `json:"lastUsed,omitempty"`
}

type passkeyResponse struct {
	ID       string `json:"id"`
	RawID    string `json:"rawId"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Response struct {
		ClientDataJSON    string   `json:"clientDataJSON"`
		AttestationObject string   `json:"attestationObject"`
		AuthenticatorData string   `json:"authenticatorData"`
		Signature         string   `json:"signature"`
		UserHandle        string   `json:"userHandle"`
		Transports        []string `json:"transports"`
	} `json:"response"`
}

type parsedClientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

func (s *Server) passkeyRegistrationOptions(w http.ResponseWriter, r *http.Request) {
	active, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		problem(w, http.StatusNotImplemented, "Passkeys require a database-backed portal")
		return
	}
	identityID, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load master account")
		return
	}
	challenge, err := s.createPasskeyChallenge(r, "register", identityID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not start passkey registration")
		return
	}
	exclude := []map[string]any{}
	rows, _ := s.s.Auth.QueryContext(r.Context(), `SELECT credential_id,transports FROM portal_passkey_credentials WHERE identity_id=?`, identityID)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var id []byte
			var transports string
			if rows.Scan(&id, &transports) == nil {
				exclude = append(exclude, map[string]any{"type": "public-key", "id": base64.RawURLEncoding.EncodeToString(id), "transports": splitTransports(transports)})
			}
		}
	}
	userID := make([]byte, 8)
	binary.BigEndian.PutUint64(userID, identityID)
	jsonOut(w, http.StatusOK, map[string]any{
		"challenge":        challenge,
		"rp":               map[string]string{"name": s.c.PortalName, "id": s.passkeyRPID()},
		"user":             map[string]string{"id": base64.RawURLEncoding.EncodeToString(userID), "name": active.Username, "displayName": active.Username},
		"pubKeyCredParams": []map[string]any{{"type": "public-key", "alg": -7}},
		"timeout":          60000, "attestation": "none", "excludeCredentials": exclude,
		"authenticatorSelection": map[string]any{"residentKey": "required", "requireResidentKey": true, "userVerification": "required"},
	})
}

func (s *Server) passkeyRegistrationFinish(w http.ResponseWriter, r *http.Request) {
	active, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	var input passkeyResponse
	if !decode(w, r, &input) {
		return
	}
	clientRaw, client, err := s.verifyPasskeyClientData(input.Response.ClientDataJSON, "webauthn.create")
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "Invalid passkey registration response")
		return
	}
	identityID, err := s.consumePasskeyChallenge(r, client.Challenge, "register")
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "Passkey challenge expired or was already used")
		return
	}
	currentIdentity, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil || currentIdentity != identityID {
		problem(w, http.StatusForbidden, "Passkey challenge does not belong to this account")
		return
	}
	attestation, err := decodeURLBase64(input.Response.AttestationObject, 1<<20)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "Invalid passkey attestation")
		return
	}
	credentialID, x, y, signCount, err := s.parseNoneAttestation(attestation)
	if err != nil || !bytes.Equal(credentialID, mustDecodeURLBase64(input.RawID)) {
		problem(w, http.StatusUnprocessableEntity, "Unsupported or invalid passkey attestation")
		return
	}
	_ = clientRaw
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Passkey"
	}
	if len(name) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Passkey name is too long")
		return
	}
	transports := cleanTransports(input.Response.Transports)
	_, err = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_passkey_credentials(credential_id,identity_id,name,public_key_x,public_key_y,sign_count,transports) VALUES(?,?,?,?,?,?,?)`, credentialID, identityID, name, x, y, signCount, strings.Join(transports, ","))
	if err != nil {
		problem(w, http.StatusConflict, "That passkey is already registered")
		return
	}
	s.auditIdentity(r, active.ID, "identity.passkey.register", identityID, name)
	jsonOut(w, http.StatusCreated, map[string]bool{"registered": true})
}

func (s *Server) passkeyAuthenticationOptions(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		problem(w, http.StatusNotImplemented, "Passkeys require a database-backed portal")
		return
	}
	challenge, err := s.createPasskeyChallenge(r, "login", 0)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not start passkey sign-in")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"challenge": challenge, "rpId": s.passkeyRPID(), "timeout": 60000, "userVerification": "required"})
}

func (s *Server) passkeyAuthenticationFinish(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		problem(w, http.StatusNotImplemented, "Passkeys require a database-backed portal")
		return
	}
	var input passkeyResponse
	if !decode(w, r, &input) {
		return
	}
	clientRaw, client, err := s.verifyPasskeyClientData(input.Response.ClientDataJSON, "webauthn.get")
	if err != nil {
		problem(w, http.StatusUnauthorized, "Invalid passkey response")
		return
	}
	if _, err = s.consumePasskeyChallenge(r, client.Challenge, "login"); err != nil {
		problem(w, http.StatusUnauthorized, "Passkey challenge expired or was already used")
		return
	}
	credentialID, err := decodeURLBase64(input.RawID, 2048)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Invalid passkey credential")
		return
	}
	var identityID, oldCount uint64
	var xBytes, yBytes []byte
	err = s.s.Auth.QueryRowContext(r.Context(), `SELECT identity_id,public_key_x,public_key_y,sign_count FROM portal_passkey_credentials WHERE credential_id=?`, credentialID).Scan(&identityID, &xBytes, &yBytes, &oldCount)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Passkey credential is not registered")
		return
	}
	authData, err := decodeURLBase64(input.Response.AuthenticatorData, 4096)
	if err != nil || len(authData) < 37 || !bytes.Equal(authData[:32], s.passkeyRPHash()) || authData[32]&0x05 != 0x05 {
		problem(w, http.StatusUnauthorized, "Invalid authenticator data")
		return
	}
	newCount := uint64(binary.BigEndian.Uint32(authData[33:37]))
	if oldCount != 0 && newCount <= oldCount {
		problem(w, http.StatusUnauthorized, "Passkey counter did not advance")
		return
	}
	signature, err := decodeURLBase64(input.Response.Signature, 1024)
	clientHash := sha256.Sum256(clientRaw)
	signed := append(append([]byte(nil), authData...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	key := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(xBytes), Y: new(big.Int).SetBytes(yBytes)}
	if err != nil || !key.Curve.IsOnCurve(key.X, key.Y) || !ecdsa.VerifyASN1(key, digest[:], signature) {
		problem(w, http.StatusUnauthorized, "Passkey signature is invalid")
		return
	}
	if input.Response.UserHandle != "" {
		handle, handleErr := decodeURLBase64(input.Response.UserHandle, 64)
		if handleErr != nil || len(handle) != 8 || binary.BigEndian.Uint64(handle) != identityID {
			problem(w, http.StatusUnauthorized, "Passkey user handle is invalid")
			return
		}
	}
	var active account
	query := fmt.Sprintf(`SELECT a.id,a.username,a.email FROM portal_identity_accounts ia JOIN %s.account a ON a.id=ia.account_id WHERE ia.identity_id=? AND ia.is_primary=1 AND a.locked=0 AND NOT EXISTS (SELECT 1 FROM %s.account_banned b WHERE b.id=a.id AND b.active=1) LIMIT 1`, s.c.AuthDB, s.c.AuthDB)
	if err = s.s.Auth.QueryRowContext(r.Context(), query, identityID).Scan(&active.ID, &active.Username, &active.Email); err != nil {
		problem(w, http.StatusUnauthorized, "No available game account is linked to this passkey")
		return
	}
	if _, err = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_passkey_credentials SET sign_count=?,last_used_at=NOW() WHERE credential_id=? AND sign_count=?`, newCount, credentialID, oldCount); err != nil {
		problem(w, http.StatusInternalServerError, "Could not update passkey")
		return
	}
	if err = s.issuePortalSession(w, r, active, identityID); err != nil {
		problem(w, http.StatusInternalServerError, "Could not create session")
		return
	}
	s.auditIdentity(r, active.ID, "identity.passkey.login", identityID, "Signed in with passkey")
	jsonOut(w, http.StatusOK, map[string]any{"account": active})
}

func (s *Server) passkeyList(w http.ResponseWriter, r *http.Request) {
	active, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"credentials": []passkeyCredential{}})
		return
	}
	identityID, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load master account")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT credential_id,name,transports,created_at,last_used_at FROM portal_passkey_credentials WHERE identity_id=? ORDER BY created_at DESC`, identityID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load passkeys")
		return
	}
	defer rows.Close()
	out := []passkeyCredential{}
	for rows.Next() {
		var item passkeyCredential
		var id []byte
		var transports string
		var last sql.NullTime
		if rows.Scan(&id, &item.Name, &transports, &item.Created, &last) == nil {
			item.ID = base64.RawURLEncoding.EncodeToString(id)
			item.Transports = splitTransports(transports)
			if last.Valid {
				item.LastUsed = &last.Time
			}
			out = append(out, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"credentials": out})
}

func (s *Server) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	active, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	credentialID, err := decodeURLBase64(r.PathValue("id"), 2048)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid passkey")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"deleted": true})
		return
	}
	identityID, err := s.ensureIdentity(r.Context(), active.ID, active.Username, active.Email)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load master account")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_passkey_credentials WHERE credential_id=? AND identity_id=?`, credentialID, identityID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not remove passkey")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		problem(w, http.StatusNotFound, "Passkey not found")
		return
	}
	s.auditIdentity(r, active.ID, "identity.passkey.delete", identityID, "Passkey removed")
	jsonOut(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) createPasskeyChallenge(r *http.Request, mode string, identityID uint64) (string, error) {
	challenge, err := oauthSecret(32)
	if err != nil {
		return "", err
	}
	raw, _ := base64.RawURLEncoding.DecodeString(challenge)
	hash := sha256.Sum256(raw)
	_, _ = s.s.Auth.ExecContext(r.Context(), `DELETE FROM portal_passkey_challenges WHERE expires_at<NOW()`)
	_, err = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_passkey_challenges(challenge_hash,flow_mode,identity_id,expires_at) VALUES(?,?,?,DATE_ADD(NOW(),INTERVAL 5 MINUTE))`, hash[:], mode, identityID)
	return challenge, err
}

func (s *Server) consumePasskeyChallenge(r *http.Request, challenge, mode string) (uint64, error) {
	raw, err := decodeURLBase64(challenge, 128)
	if err != nil {
		return 0, err
	}
	hash := sha256.Sum256(raw)
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var identityID uint64
	if err = tx.QueryRowContext(r.Context(), `SELECT identity_id FROM portal_passkey_challenges WHERE challenge_hash=? AND flow_mode=? AND expires_at>NOW() FOR UPDATE`, hash[:], mode).Scan(&identityID); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM portal_passkey_challenges WHERE challenge_hash=?`, hash[:]); err != nil {
		return 0, err
	}
	return identityID, tx.Commit()
}

func (s *Server) verifyPasskeyClientData(encoded, expectedType string) ([]byte, parsedClientData, error) {
	raw, err := decodeURLBase64(encoded, 1<<20)
	if err != nil {
		return nil, parsedClientData{}, err
	}
	var data parsedClientData
	if json.Unmarshal(raw, &data) != nil || data.Type != expectedType || data.CrossOrigin || data.Origin != s.passkeyOrigin() {
		return nil, data, fmt.Errorf("invalid client data")
	}
	return raw, data, nil
}

func (s *Server) parseNoneAttestation(data []byte) ([]byte, []byte, []byte, uint64, error) {
	decoded, _, err := decodeCBOR(data)
	object, ok := decoded.(map[any]any)
	if err != nil || !ok || object["fmt"] != "none" {
		return nil, nil, nil, 0, fmt.Errorf("unsupported attestation")
	}
	authData, ok := object["authData"].([]byte)
	if !ok || len(authData) < 55 || !bytes.Equal(authData[:32], s.passkeyRPHash()) || authData[32]&0x45 != 0x45 {
		return nil, nil, nil, 0, fmt.Errorf("invalid authenticator data")
	}
	signCount := uint64(binary.BigEndian.Uint32(authData[33:37]))
	credentialLength := int(binary.BigEndian.Uint16(authData[53:55]))
	if credentialLength < 16 || credentialLength > 1024 || 55+credentialLength >= len(authData) {
		return nil, nil, nil, 0, fmt.Errorf("invalid credential id")
	}
	credentialID := append([]byte(nil), authData[55:55+credentialLength]...)
	keyValue, _, err := decodeCBOR(authData[55+credentialLength:])
	key, ok := keyValue.(map[any]any)
	if err != nil || !ok || key[int64(1)] != int64(2) || key[int64(3)] != int64(-7) || key[int64(-1)] != int64(1) {
		return nil, nil, nil, 0, fmt.Errorf("only ES256 passkeys are supported")
	}
	x, xOK := key[int64(-2)].([]byte)
	y, yOK := key[int64(-3)].([]byte)
	if !xOK || !yOK || len(x) != 32 || len(y) != 32 || !elliptic.P256().IsOnCurve(new(big.Int).SetBytes(x), new(big.Int).SetBytes(y)) {
		return nil, nil, nil, 0, fmt.Errorf("invalid ES256 public key")
	}
	return credentialID, x, y, signCount, nil
}

func (s *Server) passkeyOrigin() string {
	u, _ := url.Parse(s.c.PublicURL)
	return u.Scheme + "://" + u.Host
}

func (s *Server) passkeyRPID() string {
	u, _ := url.Parse(s.c.PublicURL)
	return u.Hostname()
}

func (s *Server) passkeyRPHash() []byte {
	hash := sha256.Sum256([]byte(s.passkeyRPID()))
	return hash[:]
}

func decodeURLBase64(value string, max int) ([]byte, error) {
	if value == "" || len(value) > max*2 {
		return nil, fmt.Errorf("invalid base64url value")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) > max {
		return nil, fmt.Errorf("invalid base64url value")
	}
	return decoded, nil
}

func mustDecodeURLBase64(value string) []byte {
	decoded, _ := base64.RawURLEncoding.DecodeString(value)
	return decoded
}

func cleanTransports(values []string) []string {
	allowed := map[string]bool{"usb": true, "nfc": true, "ble": true, "internal": true, "hybrid": true}
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if allowed[value] && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func splitTransports(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	return cleanTransports(strings.Split(value, ","))
}
