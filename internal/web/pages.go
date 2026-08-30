package web

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (s *Server) seedContentPageTemplates(ctx context.Context) {
	if s.c.MockMode {
		return
	}
	templates := []contentPage{
		{Slug: "server-history", Title: "Our realm's history", Summary: "Tell players who operates the realm, why it exists, and the milestones that shaped it.", Body: "Replace this draft with your server's real story. Include the launch date, major progression eras, important community milestones, ownership changes, and the principles that guide development. Do not publish claims you cannot verify.", Status: "draft", ShowFooter: true, SortOrder: 80, SEOTitle: "Server history"},
		{Slug: "team", Title: "Meet the team", Summary: "Introduce the people responsible for development, operations, moderation, and support.", Body: "Replace this draft with the public names and responsibilities of your team. Explain how staff are selected, how conflicts are handled, and where players can report staff conduct. Never expose private staff details without consent.", Status: "draft", ShowFooter: true, SortOrder: 90, SEOTitle: "Realm team"},
		{Slug: "staff-recruitment", Title: "Join the realm team", Summary: "Publish open volunteer or staff roles and a clear, fair application process.", Body: "Replace this draft with current openings, eligibility requirements, expected time commitment, conduct rules, conflicts-of-interest policy, selection steps, and the official application channel. State clearly whether each role is volunteer or paid.", Status: "draft", ShowFooter: true, SortOrder: 100, SEOTitle: "Staff recruitment"},
		{Slug: "faq", Title: "Frequently asked questions", Summary: "Answer common questions about accounts, connection, rates, support, and realm rules.", Body: "Replace this draft with concise answers based on your actual realm configuration. Link to the Play guide, rules, support, security, and refund policy instead of duplicating information that may drift.", Status: "draft", ShowNavigation: true, ShowFooter: true, SortOrder: 70, SEOTitle: "Frequently asked questions"},
		{Slug: "rules", Title: "Realm rules", Summary: "Publish the conduct, gameplay, naming, trading, multiboxing, and enforcement rules that apply to every player.", Body: "Replace this draft with your actual rules. Define prohibited automation and exploitation, naming standards, chat conduct, trading policy, multiboxing policy, sanction ladder, appeal process, and the date the policy takes effect.", Status: "draft", ShowFooter: true, SortOrder: 110, SEOTitle: "Realm rules"},
		{Slug: "terms", Title: "Terms of service", Summary: "Document the agreement governing portal accounts and access to the realm.", Body: "Replace this draft with operator-reviewed terms. Identify the operator, eligibility rules, acceptable use, account ownership, virtual-item policy, suspension and termination rights, service availability, liability limits, governing law, contact channel, effective date, and revision date. Obtain local legal review before publishing.", Status: "draft", ShowFooter: true, SortOrder: 120, SEOTitle: "Terms of service"},
		{Slug: "privacy", Title: "Privacy policy", Summary: "Explain what account, session, moderation, gameplay, and payment data the portal processes.", Body: "Replace this draft with an operator-reviewed privacy notice. Describe collected data, purposes, lawful basis where applicable, processors, cookies, retention windows, security controls, access and deletion rights, international transfers, contact channel, effective date, and revision date. Never claim practices the deployment does not follow.", Status: "draft", ShowFooter: true, SortOrder: 130, SEOTitle: "Privacy policy"},
		{Slug: "refund-policy", Title: "Shop and refund policy", Summary: "Set clear expectations for credits, digital delivery, failed fulfillment, refunds, chargebacks, and support escalation.", Body: "Replace this draft with the policy that matches your payment provider and local law. Explain prices and currency, virtual-credit ownership, delivery timing, failed or partial fulfillment, refund eligibility, withdrawal rights, gifting, fraud, chargebacks, contact details, and response times. Obtain legal review before accepting money.", Status: "draft", ShowFooter: true, SortOrder: 140, SEOTitle: "Shop and refund policy"},
		{Slug: "client-troubleshooting", Title: "Client troubleshooting", Summary: "Help players verify, install, and repair the supported 3.3.5a client safely.", Body: "Replace this draft with tested platform-specific instructions. Include supported operating systems, archive extraction, realmlist setup, launcher usage, version and SHA-256 verification, common login errors, firewall guidance, clean-cache steps, uninstall instructions, and the official support route. Never advise players to disable platform security controls.", Status: "draft", ShowFooter: true, SortOrder: 60, SEOTitle: "Client troubleshooting"},
	}
	for _, page := range templates {
		_, _ = s.s.Auth.ExecContext(ctx, `INSERT IGNORE INTO portal_pages(realm_key,slug,title,summary,body,status,show_navigation,show_footer,sort_order,seo_title,seo_description,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,0)`, s.c.RealmKey, page.Slug, page.Title, page.Summary, page.Body, page.Status, page.ShowNavigation, page.ShowFooter, page.SortOrder, page.SEOTitle, page.Summary)
	}
}

type contentPage struct {
	ID             uint64     `json:"id"`
	Slug           string     `json:"slug"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	Body           string     `json:"body"`
	Status         string     `json:"status"`
	ShowNavigation bool       `json:"showNavigation"`
	ShowFooter     bool       `json:"showFooter"`
	SortOrder      int        `json:"sortOrder"`
	SEOTitle       string     `json:"seoTitle"`
	SEODescription string     `json:"seoDescription"`
	UpdatedBy      uint32     `json:"updatedBy,omitempty"`
	CreatedAt      *time.Time `json:"createdAt,omitempty"`
	UpdatedAt      *time.Time `json:"updatedAt,omitempty"`
}

const pageSelect = `id,slug,title,summary,body,status,show_navigation,show_footer,sort_order,seo_title,seo_description,updated_by,created_at,updated_at`

func scanContentPage(row rowScanner, page *contentPage) error {
	return row.Scan(&page.ID, &page.Slug, &page.Title, &page.Summary, &page.Body, &page.Status, &page.ShowNavigation, &page.ShowFooter, &page.SortOrder, &page.SEOTitle, &page.SEODescription, &page.UpdatedBy, &page.CreatedAt, &page.UpdatedAt)
}

func (s *Server) publicPages(w http.ResponseWriter, r *http.Request) {
	if s.c.MockMode {
		s.mock.mu.Lock()
		pages := append([]contentPage(nil), s.mock.pages...)
		s.mock.mu.Unlock()
		out := []contentPage{}
		for _, page := range pages {
			if page.Status == "published" {
				page.UpdatedBy = 0
				out = append(out, page)
			}
		}
		jsonOut(w, 200, map[string]any{"pages": out})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+pageSelect+" FROM portal_pages WHERE realm_key=? AND status='published' ORDER BY sort_order,title", s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not load pages")
		return
	}
	defer rows.Close()
	out := []contentPage{}
	for rows.Next() {
		var page contentPage
		if scanContentPage(rows, &page) == nil {
			page.UpdatedBy = 0
			out = append(out, page)
		}
	}
	jsonOut(w, 200, map[string]any{"pages": out})
}

func (s *Server) publicPage(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.PathValue("slug")))
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for _, page := range s.mock.pages {
			if page.Slug == slug && page.Status == "published" {
				page.UpdatedBy = 0
				jsonOut(w, 200, page)
				return
			}
		}
		problem(w, 404, "Page not found")
		return
	}
	var page contentPage
	if scanContentPage(s.s.Auth.QueryRowContext(r.Context(), "SELECT "+pageSelect+" FROM portal_pages WHERE realm_key=? AND slug=? AND status='published'", s.c.RealmKey, slug), &page) != nil {
		problem(w, 404, "Page not found")
		return
	}
	page.UpdatedBy = 0
	jsonOut(w, 200, page)
}

func validateContentPage(page *contentPage) error {
	page.Title = strings.TrimSpace(page.Title)
	page.Slug = strings.Trim(strings.ToLower(strings.TrimSpace(page.Slug)), "-")
	if page.Slug == "" {
		page.Slug = articleSlug(page.Title)
	}
	page.Status = strings.ToLower(strings.TrimSpace(page.Status))
	if page.Status == "" {
		page.Status = "draft"
	}
	if len(page.Title) < 2 || len(page.Title) > 160 || len(page.Slug) > 120 || len(page.Summary) > 1000 || len(page.Body) > 100000 || len(page.SEOTitle) > 160 || len(page.SEODescription) > 300 {
		return fmt.Errorf("page fields exceed their limits")
	}
	if !regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`).MatchString(page.Slug) {
		return fmt.Errorf("invalid page slug")
	}
	if !map[string]bool{"draft": true, "published": true, "archived": true}[page.Status] {
		return fmt.Errorf("invalid page status")
	}
	return nil
}

func (s *Server) adminPages(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, 403, "Content permission required")
		return
	}
	if r.Method == http.MethodGet {
		if s.c.MockMode {
			s.mock.mu.Lock()
			pages := append([]contentPage(nil), s.mock.pages...)
			s.mock.mu.Unlock()
			jsonOut(w, 200, map[string]any{"pages": pages})
			return
		}
		s.seedContentPageTemplates(r.Context())
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+pageSelect+" FROM portal_pages WHERE realm_key=? ORDER BY sort_order,title", s.c.RealmKey)
		if err != nil {
			problem(w, 500, "Could not load pages")
			return
		}
		defer rows.Close()
		pages := []contentPage{}
		for rows.Next() {
			var page contentPage
			if scanContentPage(rows, &page) == nil {
				pages = append(pages, page)
			}
		}
		jsonOut(w, 200, map[string]any{"pages": pages})
		return
	}
	var page contentPage
	if !decode(w, r, &page) {
		return
	}
	if err := validateContentPage(&page); err != nil {
		problem(w, 422, err.Error())
		return
	}
	page.UpdatedBy = a.ID
	if s.c.MockMode {
		s.mock.mu.Lock()
		page.ID = uint64(len(s.mock.pages) + 1)
		now := time.Now()
		page.CreatedAt = &now
		page.UpdatedAt = &now
		s.mock.pages = append(s.mock.pages, page)
		s.mock.mu.Unlock()
	} else {
		res, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_pages(realm_key,slug,title,summary,body,status,show_navigation,show_footer,sort_order,seo_title,seo_description,updated_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, s.c.RealmKey, page.Slug, page.Title, page.Summary, page.Body, page.Status, page.ShowNavigation, page.ShowFooter, page.SortOrder, page.SEOTitle, page.SEODescription, a.ID)
		if err != nil {
			problem(w, 409, "Page slug already exists or could not be saved")
			return
		}
		id, _ := res.LastInsertId()
		page.ID = uint64(id)
		s.savePageRevision(r, page, a.ID)
	}
	jsonOut(w, 201, page)
}

func (s *Server) adminPageItem(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, 403, "Content permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Invalid page")
		return
	}
	if r.Method == http.MethodDelete {
		if s.c.MockMode {
			s.mock.mu.Lock()
			defer s.mock.mu.Unlock()
			for i := range s.mock.pages {
				if s.mock.pages[i].ID == id {
					s.mock.pages[i].Status = "archived"
					jsonOut(w, 200, map[string]bool{"ok": true})
					return
				}
			}
		} else {
			res, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_pages SET status='archived',updated_by=? WHERE id=? AND realm_key=?", a.ID, id, s.c.RealmKey)
			if err == nil {
				if changed, _ := res.RowsAffected(); changed > 0 {
					jsonOut(w, 200, map[string]bool{"ok": true})
					return
				}
			}
		}
		problem(w, 404, "Page not found")
		return
	}
	var page contentPage
	if !decode(w, r, &page) {
		return
	}
	if err := validateContentPage(&page); err != nil {
		problem(w, 422, err.Error())
		return
	}
	page.ID = id
	page.UpdatedBy = a.ID
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for i := range s.mock.pages {
			if s.mock.pages[i].ID == id {
				s.mock.pages[i] = page
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
		problem(w, 404, "Page not found")
		return
	}
	res, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_pages SET slug=?,title=?,summary=?,body=?,status=?,show_navigation=?,show_footer=?,sort_order=?,seo_title=?,seo_description=?,updated_by=? WHERE id=? AND realm_key=?`, page.Slug, page.Title, page.Summary, page.Body, page.Status, page.ShowNavigation, page.ShowFooter, page.SortOrder, page.SEOTitle, page.SEODescription, a.ID, id, s.c.RealmKey)
	if err != nil {
		problem(w, 409, "Page slug already exists or could not be saved")
		return
	}
	if changed, _ := res.RowsAffected(); changed == 0 {
		problem(w, 404, "Page not found")
		return
	}
	s.savePageRevision(r, page, a.ID)
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) savePageRevision(r *http.Request, page contentPage, editor uint32) {
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_page_revisions(page_id,realm_key,editor_account_id,title,slug,summary,body,status,show_navigation,show_footer,sort_order,seo_title,seo_description) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, page.ID, s.c.RealmKey, editor, page.Title, page.Slug, page.Summary, page.Body, page.Status, page.ShowNavigation, page.ShowFooter, page.SortOrder, page.SEOTitle, page.SEODescription)
}
