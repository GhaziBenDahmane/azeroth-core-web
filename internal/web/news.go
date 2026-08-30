package web

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) newsList(w http.ResponseWriter, r *http.Request) {
	items := s.publicNews(r)
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	if kind != "" {
		filtered := make([]newsEntry, 0, len(items))
		for _, item := range items {
			if item.Kind == kind {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	jsonOut(w, http.StatusOK, map[string]any{"news": items})
}

func (s *Server) newsDetail(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.PathValue("slug")))
	if slug == "" {
		problem(w, http.StatusNotFound, "Article not found")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for _, item := range s.mock.news {
			if item.Slug == slug && item.Status == "published" && item.Active {
				item.CreatedBy = 0
				jsonOut(w, http.StatusOK, item)
				return
			}
		}
		problem(w, http.StatusNotFound, "Article not found")
		return
	}
	var item newsEntry
	err := scanNews(s.s.Auth.QueryRowContext(r.Context(), `SELECT `+newsSelect+` FROM portal_news WHERE realm_key=? AND slug=? AND status='published' AND active=1 AND (publish_at IS NULL OR publish_at<=NOW()) AND (expires_at IS NULL OR expires_at>NOW()) LIMIT 1`, s.c.RealmKey, slug), &item)
	if err != nil {
		problem(w, http.StatusNotFound, "Article not found")
		return
	}
	item.CreatedBy = 0
	jsonOut(w, http.StatusOK, item)
}

func (s *Server) adminNewsRevisions(w http.ResponseWriter, r *http.Request) {
	_, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid article")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for _, item := range s.mock.news {
			if item.ID == id {
				jsonOut(w, http.StatusOK, map[string]any{"revisions": []newsRevision{{ID: 1, NewsID: id, EditorID: 1, Snapshot: item}}})
				return
			}
		}
		problem(w, http.StatusNotFound, "Article not found")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT id,news_id,editor_account_id,title,COALESCE(slug,''),summary,body,url,cover_url,tags,author_name,kind,status,publish_at,expires_at,created_at FROM portal_news_revisions WHERE news_id=? AND realm_key=? ORDER BY id DESC LIMIT 100`, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load article revisions")
		return
	}
	defer rows.Close()
	revisions := []newsRevision{}
	for rows.Next() {
		var revision newsRevision
		n := &revision.Snapshot
		if rows.Scan(&revision.ID, &revision.NewsID, &revision.EditorID, &n.Title, &n.Slug, &n.Summary, &n.Body, &n.URL, &n.CoverURL, &n.Tags, &n.AuthorName, &n.Kind, &n.Status, &n.PublishAt, &n.ExpiresAt, &revision.CreatedAt) == nil {
			n.ID = revision.NewsID
			n.Active = n.Status == "published"
			revisions = append(revisions, revision)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"revisions": revisions})
}
