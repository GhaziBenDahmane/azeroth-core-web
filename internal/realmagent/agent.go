package realmagent

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	Addr, ConfigPath, BackupDir, Token, RealmKey string
}

type mapping struct {
	Keys            []string
	Boolean         bool
	RestartRequired bool
}

var allowed = map[string]mapping{
	"rate.xp.quest":          {Keys: []string{"Rate.XP.Quest"}},
	"rate.xp.kill":           {Keys: []string{"Rate.XP.Kill"}},
	"rate.xp.explore":        {Keys: []string{"Rate.XP.Explore"}},
	"rate.drop.item":         {Keys: []string{"Rate.Drop.Item.Poor", "Rate.Drop.Item.Normal", "Rate.Drop.Item.Uncommon", "Rate.Drop.Item.Rare", "Rate.Drop.Item.Epic", "Rate.Drop.Item.Legendary", "Rate.Drop.Item.Artifact", "Rate.Drop.Item.Referenced"}},
	"rate.reputation":        {Keys: []string{"Rate.Reputation.Gain"}},
	"rate.honor":             {Keys: []string{"Rate.Honor"}},
	"rate.profession":        {Keys: []string{"Rate.Skill.Discovery"}},
	"cross_faction.accounts": {Keys: []string{"AllowTwoSide.Accounts"}, Boolean: true, RestartRequired: true},
	"cross_faction.calendar": {Keys: []string{"AllowTwoSide.Interaction.Calendar"}, Boolean: true, RestartRequired: true},
	"cross_faction.channels": {Keys: []string{"AllowTwoSide.Interaction.Channel"}, Boolean: true, RestartRequired: true},
	"cross_faction.groups":   {Keys: []string{"AllowTwoSide.Interaction.Group"}, Boolean: true, RestartRequired: true},
	"cross_faction.guilds":   {Keys: []string{"AllowTwoSide.Interaction.Guild"}, Boolean: true, RestartRequired: true},
	"cross_faction.auctions": {Keys: []string{"AllowTwoSide.Interaction.Auction"}, Boolean: true, RestartRequired: true},
	"cross_faction.mail":     {Keys: []string{"AllowTwoSide.Interaction.Mail"}, Boolean: true, RestartRequired: true},
	"cross_faction.who":      {Keys: []string{"AllowTwoSide.Who.List"}, Boolean: true, RestartRequired: true},
	"cross_faction.friends":  {Keys: []string{"AllowTwoSide.Add.Friend"}, Boolean: true, RestartRequired: true},
	"cross_faction.trade":    {Keys: []string{"AllowTwoSide.Trade"}, Boolean: true, RestartRequired: true},
}

type Server struct {
	c  Config
	mu sync.Mutex
}

type snapshot struct {
	Version         string         `json:"version"`
	Values          map[string]any `json:"values"`
	RestartRequired []string       `json:"restartRequired,omitempty"`
	BackupID        string         `json:"backupId,omitempty"`
	ObservedAt      time.Time      `json:"observedAt"`
}

func LoadConfig() (Config, error) {
	c := Config{Addr: strings.TrimSpace(os.Getenv("REALM_AGENT_ADDR")), ConfigPath: strings.TrimSpace(os.Getenv("REALM_AGENT_CONFIG_PATH")), BackupDir: strings.TrimSpace(os.Getenv("REALM_AGENT_BACKUP_DIR")), Token: os.Getenv("REALM_AGENT_TOKEN"), RealmKey: strings.TrimSpace(os.Getenv("REALM_AGENT_REALM_KEY"))}
	if c.Addr == "" {
		c.Addr = "127.0.0.1:9090"
	}
	if !filepath.IsAbs(c.ConfigPath) {
		return c, fmt.Errorf("REALM_AGENT_CONFIG_PATH must be an absolute path")
	}
	if c.BackupDir == "" {
		c.BackupDir = filepath.Join(filepath.Dir(c.ConfigPath), "portal-backups")
	}
	if !filepath.IsAbs(c.BackupDir) {
		return c, fmt.Errorf("REALM_AGENT_BACKUP_DIR must be an absolute path")
	}
	if len(c.Token) < 32 || len(c.Token) > 512 {
		return c, fmt.Errorf("REALM_AGENT_TOKEN must contain 32–512 characters")
	}
	if c.RealmKey == "" || !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`).MatchString(c.RealmKey) {
		return c, fmt.Errorf("REALM_AGENT_REALM_KEY is required and invalid")
	}
	return c, nil
}

func Run(c Config) error {
	agent := &Server{c: c}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /v1/config", agent.authorize(agent.get))
	mux.HandleFunc("POST /v1/config/apply", agent.authorize(agent.apply))
	server := &http.Server{Addr: c.Addr, Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 8 * time.Second, WriteTimeout: 8 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	slog.Info("realm configuration agent ready", "address", c.Addr, "realm", c.RealmKey, "config", c.ConfigPath)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	want := sha256.Sum256([]byte("Bearer " + s.c.Token))
	return func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(want[:], got[:]) != 1 || r.Header.Get("X-Portal-Realm") != s.c.RealmKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) get(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := readConfig(s.c.ConfigPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "configuration could not be read"})
		return
	}
	writeJSON(w, http.StatusOK, inspect(data, ""))
}

func (s *Server) apply(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RealmKey string         `json:"realmKey"`
		Values   map[string]any `json:"values"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.RealmKey != s.c.RealmKey || len(input.Values) == 0 || len(input.Values) > len(allowed) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid configuration request"})
		return
	}
	validated := map[string]string{}
	for key, raw := range input.Values {
		spec, ok := allowed[key]
		if !ok {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "unsupported configuration key"})
			return
		}
		value, err := configValue(raw, spec.Boolean)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid value for " + key})
			return
		}
		for _, actual := range spec.Keys {
			validated[actual] = value
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := readConfig(s.c.ConfigPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "configuration could not be read"})
		return
	}
	backupID, err := backup(s.c.BackupDir, data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "configuration backup failed"})
		return
	}
	updated := replaceValues(data, validated)
	if err := atomicWrite(s.c.ConfigPath, updated); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "configuration update failed"})
		return
	}
	writeJSON(w, http.StatusOK, inspect(updated, backupID))
}

func readConfig(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 {
		return nil, fmt.Errorf("configuration exceeds 4 MiB")
	}
	return data, nil
}

func configValue(raw any, boolean bool) (string, error) {
	if boolean {
		value, ok := raw.(bool)
		if !ok {
			return "", fmt.Errorf("boolean required")
		}
		if value {
			return "1", nil
		}
		return "0", nil
	}
	value, ok := raw.(float64)
	if !ok || value <= 0 || value > 1000 {
		return "", fmt.Errorf("rate out of range")
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
}

func replaceValues(data []byte, replacements map[string]string) []byte {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	found := map[string]bool{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		key = strings.TrimSpace(key)
		if !ok {
			continue
		}
		if value, replace := replacements[key]; replace {
			lines[i] = key + " = " + value
			found[key] = true
		}
	}
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		if !found[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		lines = append(lines, "", "# Managed by Azeroth Portal realm agent")
		for _, key := range keys {
			lines = append(lines, key+" = "+replacements[key])
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func inspect(data []byte, backupID string) snapshot {
	actual := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if ok {
			actual[strings.TrimSpace(key)] = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		}
	}
	values := map[string]any{}
	restart := []string{}
	for stable, spec := range allowed {
		var parsed any
		consistent := true
		for index, key := range spec.Keys {
			raw, exists := actual[key]
			if !exists {
				consistent = false
				break
			}
			var value any
			if spec.Boolean {
				if raw != "0" && raw != "1" {
					consistent = false
					break
				}
				value = raw == "1"
			} else {
				number, err := strconv.ParseFloat(raw, 64)
				if err != nil {
					consistent = false
					break
				}
				value = number
			}
			if index > 0 && fmt.Sprint(parsed) != fmt.Sprint(value) {
				consistent = false
				break
			}
			parsed = value
		}
		if consistent {
			values[stable] = parsed
		}
		if spec.RestartRequired {
			restart = append(restart, stable)
		}
	}
	sort.Strings(restart)
	return snapshot{Version: "azeroth-portal-agent/1", Values: values, RestartRequired: restart, BackupID: backupID, ObservedAt: time.Now().UTC()}
}

func backup(directory string, data []byte) (string, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	id := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(sum[:6])
	path := filepath.Join(directory, "worldserver.conf."+id+".bak")
	return id, os.WriteFile(path, data, 0600)
}

func atomicWrite(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".worldserver.conf.portal-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err = temporary.Chmod(info.Mode().Perm()); err == nil {
		_, err = temporary.Write(data)
	}
	if err == nil {
		err = temporary.Sync()
	}
	closeErr := temporary.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
