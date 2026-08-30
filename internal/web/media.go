package web

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type mediaAsset struct {
	ID       uint64    `json:"id"`
	Name     string    `json:"name"`
	MIME     string    `json:"mime"`
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	Alt      string    `json:"alt"`
	URL      string    `json:"url"`
	Active   bool      `json:"active"`
	Uploader string    `json:"uploader,omitempty"`
	Created  time.Time `json:"created"`
}

func (s *Server) adminMedia(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "content"); !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"assets": []mediaAsset{}})
		return
	}
	query := fmt.Sprintf(`SELECT m.id,m.file_name,m.mime_type,m.width,m.height,m.alt_text,m.active,m.created_at,COALESCE(a.username,'') FROM portal_media_assets m LEFT JOIN %s.account a ON a.id=m.uploaded_by WHERE m.realm_key=? ORDER BY m.created_at DESC LIMIT 250`, s.c.AuthDB)
	rows, err := s.s.Auth.QueryContext(r.Context(), query, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load media library")
		return
	}
	defer rows.Close()
	out := []mediaAsset{}
	for rows.Next() {
		var item mediaAsset
		if rows.Scan(&item.ID, &item.Name, &item.MIME, &item.Width, &item.Height, &item.Alt, &item.Active, &item.Created, &item.Uploader) == nil {
			item.URL = mediaURL(item.ID, item.Name)
			out = append(out, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"assets": out})
}

func (s *Server) adminMediaUpload(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	if s.c.MockMode {
		problem(w, http.StatusNotImplemented, "Media upload requires a database-backed portal")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if err := r.ParseMultipartForm(6 << 20); err != nil {
		problem(w, http.StatusRequestEntityTooLarge, "Image must be no larger than 5 MB")
		return
	}
	file, header, err := r.FormFile("image")
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, "Choose an image to upload")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (5<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 5<<20 {
		problem(w, http.StatusRequestEntityTooLarge, "Image must be no larger than 5 MB")
		return
	}
	mime := http.DetectContentType(data)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/gif" {
		problem(w, http.StatusUnprocessableEntity, "Only PNG, JPEG, and GIF images are supported")
		return
	}
	dimensions, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || dimensions.Width < 1 || dimensions.Height < 1 || dimensions.Width > 8192 || dimensions.Height > 8192 || int64(dimensions.Width)*int64(dimensions.Height) > 32_000_000 {
		problem(w, http.StatusUnprocessableEntity, "Image dimensions are invalid or exceed 32 megapixels")
		return
	}
	name := cleanMediaName(header.Filename, mime)
	alt := strings.TrimSpace(r.FormValue("alt"))
	if len(alt) > 300 {
		problem(w, http.StatusUnprocessableEntity, "Alternative text is too long")
		return
	}
	hash := sha256.Sum256(data)
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_media_assets(realm_key,content_hash,file_name,mime_type,width,height,alt_text,data,uploaded_by) VALUES(?,?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id),active=1,alt_text=VALUES(alt_text)`, s.c.RealmKey, hash[:], name, mime, dimensions.Width, dimensions.Height, alt, data, actor.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not store image")
		return
	}
	id, _ := result.LastInsertId()
	asset := mediaAsset{ID: uint64(id), Name: name, MIME: mime, Width: dimensions.Width, Height: dimensions.Height, Alt: alt, URL: mediaURL(uint64(id), name), Active: true, Uploader: actor.Username, Created: time.Now()}
	s.auditIdentity(r, actor.ID, "media.upload", uint64(id), name+" sha256="+hex.EncodeToString(hash[:8]))
	jsonOut(w, http.StatusCreated, map[string]any{"asset": asset})
}

func (s *Server) adminMediaUpdate(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid media asset")
		return
	}
	var input struct {
		Alt string `json:"alt"`
	}
	if !decode(w, r, &input) {
		return
	}
	input.Alt = strings.TrimSpace(input.Alt)
	if len(input.Alt) > 300 {
		problem(w, http.StatusUnprocessableEntity, "Alternative text is too long")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"updated": true})
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_media_assets SET alt_text=? WHERE id=? AND realm_key=? AND active=1`, input.Alt, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not update media asset")
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		var exists int
		_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_media_assets WHERE id=? AND realm_key=? AND active=1`, id, s.c.RealmKey).Scan(&exists)
		if exists == 0 {
			problem(w, http.StatusNotFound, "Media asset not found")
			return
		}
	}
	s.auditIdentity(r, actor.ID, "media.update", id, "Alternative text updated")
	jsonOut(w, http.StatusOK, map[string]bool{"updated": true})
}

func (s *Server) adminMediaDelete(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		problem(w, http.StatusBadRequest, "Invalid media asset")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]bool{"archived": true})
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_media_assets SET active=0 WHERE id=? AND realm_key=? AND active=1`, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not archive media asset")
		return
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		problem(w, http.StatusNotFound, "Media asset not found")
		return
	}
	s.auditIdentity(r, actor.ID, "media.archive", id, "Media asset archived")
	jsonOut(w, http.StatusOK, map[string]bool{"archived": true})
}

func (s *Server) mediaServe(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		http.NotFound(w, r)
		return
	}
	var data []byte
	var mime string
	var created time.Time
	var hash []byte
	err = s.s.Auth.QueryRowContext(r.Context(), `SELECT data,mime_type,created_at,content_hash FROM portal_media_assets WHERE id=? AND realm_key=? AND active=1`, id, s.c.RealmKey).Scan(&data, &mime, &created, &hash)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load image")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+hex.EncodeToString(hash)+`"`)
	http.ServeContent(w, r, r.PathValue("name"), created, bytes.NewReader(data))
}

func cleanMediaName(value, mime string) string {
	base := strings.TrimSuffix(filepath.Base(value), filepath.Ext(value))
	var cleaned strings.Builder
	for _, char := range strings.ToLower(base) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			cleaned.WriteRune(char)
		} else if cleaned.Len() > 0 && !strings.HasSuffix(cleaned.String(), "-") {
			cleaned.WriteByte('-')
		}
		if cleaned.Len() >= 120 {
			break
		}
	}
	name := strings.Trim(cleaned.String(), "-")
	if name == "" {
		name = "image"
	}
	extension := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif"}[mime]
	return name + extension
}

func mediaURL(id uint64, name string) string {
	return "/api/media/" + strconv.FormatUint(id, 10) + "/" + urlPathName(name)
}

func urlPathName(value string) string {
	return strings.ReplaceAll(value, " ", "-")
}
