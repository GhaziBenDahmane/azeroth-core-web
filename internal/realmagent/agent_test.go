package realmagent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceInspectAndBackup(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "worldserver.conf")
	original := []byte("# example\nRate.XP.Quest = 1\nRate.XP.Quest = 1 # duplicate\nAllowTwoSide.Interaction.Group = 0\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	updated := replaceValues(original, map[string]string{"Rate.XP.Quest": "2", "AllowTwoSide.Interaction.Group": "1", "Rate.Honor": "3"})
	if strings.Count(string(updated), "Rate.XP.Quest = 2") != 2 || !strings.Contains(string(updated), "Rate.Honor = 3") {
		t.Fatalf("unexpected updated configuration:\n%s", updated)
	}
	snapshot := inspect(updated, "")
	if snapshot.Values["rate.xp.quest"] != float64(2) || snapshot.Values["cross_faction.groups"] != true || snapshot.Values["rate.honor"] != float64(3) {
		t.Fatalf("unexpected snapshot: %#v", snapshot.Values)
	}
	id, err := backup(filepath.Join(directory, "backups"), original)
	if err != nil || id == "" {
		t.Fatalf("backup id=%q err=%v", id, err)
	}
	if err = atomicWrite(path, updated); err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(path)
	if !bytes.Equal(stored, updated) {
		t.Fatal("atomic write did not persist updated configuration")
	}
}

func TestAgentRejectsUnknownKeysAndRequiresAuthentication(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "worldserver.conf")
	if err := os.WriteFile(path, []byte("Rate.XP.Quest = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{c: Config{ConfigPath: path, BackupDir: filepath.Join(directory, "backups"), Token: strings.Repeat("a", 32), RealmKey: "frost"}}
	handler := s.authorize(s.apply)
	request := httptest.NewRequest(http.MethodPost, "/v1/config/apply", strings.NewReader(`{"realmKey":"frost","values":{"arbitrary.shell":"rm"}}`))
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated response = %d", response.Code)
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/config/apply", strings.NewReader(`{"realmKey":"frost","values":{"arbitrary.shell":"rm"}}`))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("X-Portal-Realm", "frost")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown key response = %d: %s", response.Code, response.Body.String())
	}

	payload, _ := json.Marshal(map[string]any{"realmKey": "frost", "values": map[string]any{"rate.xp.quest": 2, "cross_faction.groups": true}})
	request = httptest.NewRequest(http.MethodPost, "/v1/config/apply", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("X-Portal-Realm", "frost")
	response = httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("valid apply response = %d: %s", response.Code, response.Body.String())
	}
	stored, _ := os.ReadFile(path)
	if !strings.Contains(string(stored), "Rate.XP.Quest = 2") || !strings.Contains(string(stored), "AllowTwoSide.Interaction.Group = 1") {
		t.Fatalf("allowed values were not written:\n%s", stored)
	}
}
