package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type portalDownload struct {
	ID            uint32           `json:"id"`
	Name          string           `json:"name"`
	Platform      string           `json:"platform"`
	URL           string           `json:"url"`
	Version       string           `json:"version"`
	FileSize      string           `json:"fileSize"`
	SHA256        string           `json:"sha256"`
	SignatureURL  string           `json:"signatureUrl"`
	VirusTotalURL string           `json:"virusTotalUrl"`
	ChangelogURL  string           `json:"changelogUrl"`
	ReleasedAt    string           `json:"releasedAt"`
	Requirements  string           `json:"requirements"`
	Notes         string           `json:"notes"`
	Mirrors       []downloadMirror `json:"mirrors"`
	MirrorsJSON   string           `json:"-"`
	Active        bool             `json:"active"`
	SortOrder     int              `json:"sortOrder"`
}

type downloadMirror struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type launcherPatch struct {
	ID           uint64           `json:"id"`
	Platform     string           `json:"platform"`
	FromVersion  string           `json:"fromVersion"`
	ToVersion    string           `json:"toVersion"`
	URL          string           `json:"url"`
	Mirrors      []downloadMirror `json:"mirrors"`
	MirrorsJSON  string           `json:"-"`
	FileSize     string           `json:"fileSize"`
	SHA256       string           `json:"sha256"`
	SignatureURL string           `json:"signatureUrl"`
	Notes        string           `json:"notes"`
	ReleasedAt   string           `json:"releasedAt"`
	Active       bool             `json:"active"`
	SortOrder    int              `json:"sortOrder"`
}

var sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func validateMirrors(items []downloadMirror) error {
	if len(items) > 10 {
		return &validationError{"A package can have at most 10 mirrors"}
	}
	seen := map[string]bool{}
	for index := range items {
		items[index].Label = strings.TrimSpace(items[index].Label)
		items[index].URL = strings.TrimSpace(items[index].URL)
		parsed, err := url.ParseRequestURI(items[index].URL)
		if items[index].Label == "" || len(items[index].Label) > 60 || err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return &validationError{"Every mirror needs a short label and an absolute HTTP(S) URL"}
		}
		if seen[items[index].URL] {
			return &validationError{"Mirror URLs must be unique"}
		}
		seen[items[index].URL] = true
	}
	return nil
}

func decodeMirrors(raw string) []downloadMirror {
	items := []downloadMirror{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &items)
	}
	return items
}

func encodeMirrors(items []downloadMirror) string {
	if len(items) == 0 {
		return "[]"
	}
	encoded, _ := json.Marshal(items)
	return string(encoded)
}

func validateDownload(item portalDownload) error {
	item.Name, item.Platform = strings.TrimSpace(item.Name), strings.TrimSpace(item.Platform)
	if item.Name == "" || len(item.Name) > 100 || item.Platform == "" || len(item.Platform) > 30 || len(item.Version) > 40 || len(item.FileSize) > 40 || len(item.Notes) > 500 || len(item.Requirements) > 1000 {
		return &validationError{"Check the download name, platform, version, size, and notes"}
	}
	for _, raw := range []string{item.URL, item.SignatureURL, item.VirusTotalURL, item.ChangelogURL} {
		if raw == "" {
			continue
		}
		parsed, err := url.ParseRequestURI(raw)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return &validationError{"Download and signature URLs must be absolute HTTP(S) URLs"}
		}
	}
	if item.URL == "" || item.SHA256 != "" && !sha256Pattern.MatchString(item.SHA256) {
		return &validationError{"A download URL and valid optional SHA-256 checksum are required"}
	}
	if err := validateMirrors(item.Mirrors); err != nil {
		return err
	}
	if item.ReleasedAt != "" {
		if _, err := time.Parse("2006-01-02", item.ReleasedAt); err != nil {
			return &validationError{"Release date must use YYYY-MM-DD"}
		}
	}
	return nil
}

func validateLauncherPatch(item launcherPatch) error {
	item.Platform, item.FromVersion, item.ToVersion = strings.TrimSpace(item.Platform), strings.TrimSpace(item.FromVersion), strings.TrimSpace(item.ToVersion)
	if item.Platform == "" || len(item.Platform) > 30 || item.FromVersion == "" || len(item.FromVersion) > 40 || item.ToVersion == "" || len(item.ToVersion) > 40 || item.FromVersion == item.ToVersion || len(item.FileSize) > 40 || len(item.Notes) > 500 {
		return &validationError{"Platform and distinct source/target versions are required"}
	}
	if !sha256Pattern.MatchString(item.SHA256) {
		return &validationError{"A 64-character SHA-256 checksum is required for every patch"}
	}
	for index, raw := range []string{item.URL, item.SignatureURL} {
		if index == 1 && raw == "" {
			continue
		}
		parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return &validationError{"Patch and signature URLs must be absolute HTTP(S) URLs"}
		}
	}
	if err := validateMirrors(item.Mirrors); err != nil {
		return err
	}
	if item.ReleasedAt != "" {
		if _, err := time.Parse("2006-01-02", item.ReleasedAt); err != nil {
			return &validationError{"Release date must use YYYY-MM-DD"}
		}
	}
	return nil
}

const downloadColumns = "id,name,platform,url,version,file_size,sha256,signature_url,virus_total_url,changelog_url,COALESCE(DATE_FORMAT(released_at,'%Y-%m-%d'),''),requirements,notes,COALESCE(mirrors_json,'[]'),active,sort_order"

func scanDownload(scanner interface{ Scan(...any) error }, item *portalDownload) error {
	err := scanner.Scan(&item.ID, &item.Name, &item.Platform, &item.URL, &item.Version, &item.FileSize, &item.SHA256, &item.SignatureURL, &item.VirusTotalURL, &item.ChangelogURL, &item.ReleasedAt, &item.Requirements, &item.Notes, &item.MirrorsJSON, &item.Active, &item.SortOrder)
	item.Mirrors = decodeMirrors(item.MirrorsJSON)
	return err
}

const launcherPatchColumns = "id,platform,from_version,to_version,url,COALESCE(mirrors_json,'[]'),file_size,sha256,signature_url,notes,COALESCE(DATE_FORMAT(released_at,'%Y-%m-%d'),''),active,sort_order"

func scanLauncherPatch(scanner interface{ Scan(...any) error }, item *launcherPatch) error {
	err := scanner.Scan(&item.ID, &item.Platform, &item.FromVersion, &item.ToVersion, &item.URL, &item.MirrorsJSON, &item.FileSize, &item.SHA256, &item.SignatureURL, &item.Notes, &item.ReleasedAt, &item.Active, &item.SortOrder)
	item.Mirrors = decodeMirrors(item.MirrorsJSON)
	return err
}

type validationError struct{ message string }

func (e *validationError) Error() string { return e.message }

func (s *Server) downloads(w http.ResponseWriter, r *http.Request) {
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+downloadColumns+" FROM portal_downloads WHERE realm_key=? AND active=1 ORDER BY sort_order,name", s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load downloads")
		return
	}
	defer rows.Close()
	items := []portalDownload{}
	for rows.Next() {
		var item portalDownload
		if scanDownload(rows, &item) == nil {
			items = append(items, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"downloads": items})
}

// launcherManifest is a stable, read-only contract for optional desktop
// launchers. It deliberately contains no updater executable or bypass advice:
// launchers still have to verify the published checksum/signature before
// replacing a client archive.
func (s *Server) launcherManifest(w http.ResponseWriter, r *http.Request) {
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+downloadColumns+" FROM portal_downloads WHERE realm_key=? AND active=1 ORDER BY sort_order,name", s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load launcher manifest")
		return
	}
	defer rows.Close()
	packages := []portalDownload{}
	for rows.Next() {
		var item portalDownload
		if err := scanDownload(rows, &item); err != nil {
			problem(w, http.StatusInternalServerError, "Could not read launcher manifest")
			return
		}
		packages = append(packages, item)
	}
	patchRows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+launcherPatchColumns+" FROM portal_launcher_patches WHERE realm_key=? AND active=1 ORDER BY platform,sort_order,id", s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load launcher patches")
		return
	}
	patches := []launcherPatch{}
	for patchRows.Next() {
		var item launcherPatch
		if err := scanLauncherPatch(patchRows, &item); err != nil {
			patchRows.Close()
			problem(w, http.StatusInternalServerError, "Could not read launcher patches")
			return
		}
		patches = append(patches, item)
	}
	patchRows.Close()
	settings := s.runtimeSettings(r)
	jsonOut(w, http.StatusOK, map[string]any{
		"schemaVersion": 2,
		"generatedAt":   time.Now().UTC(),
		"realm": map[string]any{
			"key": s.c.RealmKey, "name": settings.RealmName,
			"address": settings.RealmAddress,
		},
		"client": map[string]any{
			"expansion": s.c.ExpansionName, "version": s.c.ClientVersion,
			"build": s.c.ClientBuild,
		},
		"packages": packages,
		"patches":  patches,
	})
}

func (s *Server) adminDownloads(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+downloadColumns+" FROM portal_downloads WHERE realm_key=? ORDER BY sort_order,name", s.c.RealmKey)
		if err != nil {
			problem(w, 500, "Could not load downloads")
			return
		}
		defer rows.Close()
		items := []portalDownload{}
		for rows.Next() {
			var item portalDownload
			if scanDownload(rows, &item) == nil {
				items = append(items, item)
			}
		}
		jsonOut(w, 200, map[string]any{"downloads": items})
		return
	}
	var item portalDownload
	if !decode(w, r, &item) {
		return
	}
	if err := validateDownload(item); err != nil {
		problem(w, 422, err.Error())
		return
	}
	res, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_downloads(realm_key,name,platform,url,version,file_size,sha256,signature_url,virus_total_url,changelog_url,released_at,requirements,notes,mirrors_json,active,sort_order) VALUES(?,?,?,?,?,?,?,?,?,?,NULLIF(?,''),?,?,?,?,?)", s.c.RealmKey, item.Name, item.Platform, item.URL, item.Version, item.FileSize, strings.ToLower(item.SHA256), item.SignatureURL, item.VirusTotalURL, item.ChangelogURL, item.ReleasedAt, item.Requirements, item.Notes, encodeMirrors(item.Mirrors), item.Active, item.SortOrder)
	if err != nil {
		problem(w, 500, "Could not create download")
		return
	}
	id, _ := res.LastInsertId()
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'download.create',?,?)", a.ID, strconv.FormatInt(id, 10), item.Name)
	jsonOut(w, 201, map[string]any{"ok": true, "id": id})
}

func (s *Server) adminDownloadDelete(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, 403, "Content access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		problem(w, 400, "Invalid download")
		return
	}
	res, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_downloads SET active=0 WHERE id=? AND realm_key=?", id, s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not disable download")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		problem(w, 404, "Download not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'download.disable',?)", a.ID, strconv.FormatUint(id, 10))
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) adminLauncherPatches(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+launcherPatchColumns+" FROM portal_launcher_patches WHERE realm_key=? ORDER BY platform,sort_order,id", s.c.RealmKey)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not load launcher patches")
			return
		}
		defer rows.Close()
		items := []launcherPatch{}
		for rows.Next() {
			var item launcherPatch
			if scanLauncherPatch(rows, &item) == nil {
				items = append(items, item)
			}
		}
		jsonOut(w, http.StatusOK, map[string]any{"patches": items})
		return
	}
	var item launcherPatch
	if !decode(w, r, &item) {
		return
	}
	if err := validateLauncherPatch(item); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_launcher_patches(realm_key,platform,from_version,to_version,url,mirrors_json,file_size,sha256,signature_url,notes,released_at,active,sort_order)
		VALUES(?,?,?,?,?,?,?,?,?,?,NULLIF(?,''),?,?)`, s.c.RealmKey, strings.TrimSpace(item.Platform), strings.TrimSpace(item.FromVersion), strings.TrimSpace(item.ToVersion), strings.TrimSpace(item.URL), encodeMirrors(item.Mirrors), item.FileSize, strings.ToLower(item.SHA256), strings.TrimSpace(item.SignatureURL), item.Notes, item.ReleasedAt, item.Active, item.SortOrder)
	if err != nil {
		problem(w, http.StatusConflict, "Could not create launcher patch; the same version transition may already exist")
		return
	}
	id, _ := result.LastInsertId()
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'launcher-patch.create',?,?)", a.ID, strconv.FormatInt(id, 10), item.Platform+" "+item.FromVersion+" → "+item.ToVersion)
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (s *Server) adminLauncherPatchDelete(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid launcher patch")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_launcher_patches SET active=0 WHERE id=? AND realm_key=?", id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not disable launcher patch")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusNotFound, "Launcher patch not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'launcher-patch.disable',?)", a.ID, strconv.FormatUint(id, 10))
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}
