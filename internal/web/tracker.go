package web

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type communityIssue struct {
	ID            uint64             `json:"id"`
	AccountID     uint32             `json:"accountId,omitempty"`
	Author        string             `json:"author"`
	Kind          string             `json:"kind"`
	Title         string             `json:"title"`
	Body          string             `json:"body,omitempty"`
	Category      string             `json:"category"`
	Status        string             `json:"status"`
	Priority      string             `json:"priority"`
	Labels        []string           `json:"labels"`
	StaffResponse string             `json:"staffResponse,omitempty"`
	VoteCount     uint32             `json:"voteCount"`
	CommentCount  uint32             `json:"commentCount"`
	ViewerVoted   bool               `json:"viewerVoted"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
	Comments      []communityComment `json:"comments,omitempty"`
}

type communityComment struct {
	ID         uint64    `json:"id"`
	IssueID    uint64    `json:"issueId"`
	AccountID  uint32    `json:"accountId,omitempty"`
	Author     string    `json:"author"`
	AuthorRole string    `json:"authorRole"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"createdAt"`
}

var communityStatuses = map[string]bool{"open": true, "under_review": true, "planned": true, "in_progress": true, "resolved": true, "closed": true, "declined": true}
var communityPriorities = map[string]bool{"low": true, "normal": true, "high": true, "critical": true}
var communityKinds = map[string]bool{"suggestion": true, "bug": true}

func (s *Server) trackerAccount(r *http.Request) (account, error) {
	if s.c.MockMode {
		username, ok := s.mockUser(r)
		if !ok {
			return account{}, fmt.Errorf("not authenticated")
		}
		return account{ID: 1, Username: username, Email: "demo@example.com", GMLevel: 3}, nil
	}
	return s.auth(r)
}

func (s *Server) trackerViewerID(r *http.Request) uint32 {
	a, err := s.trackerAccount(r)
	if err != nil {
		return 0
	}
	return a.ID
}

func ensureMockCommunityLocked(state *mockState) {
	if state.communityVotes == nil {
		state.communityVotes = map[uint64]bool{}
	}
	if len(state.communityIssues) != 0 {
		return
	}
	now := time.Now()
	state.communityIssues = []communityIssue{
		{ID: 1, AccountID: 1, Author: "DEMO", Kind: "suggestion", Title: "Add a weekly Trial of the Crusader event", Body: "A scheduled weekly run would help returning players meet active guilds and catch up together.", Category: "events", Status: "planned", Priority: "normal", Labels: []string{"community", "weekly"}, StaffResponse: "Accepted for the next event rotation.", VoteCount: 18, CommentCount: 1, CreatedAt: now.Add(-96 * time.Hour), UpdatedAt: now.Add(-8 * time.Hour), Comments: []communityComment{{ID: 1, IssueID: 1, AccountID: 1, Author: "Realm Team", AuthorRole: "staff", Body: "We are preparing the first run and will publish it in the calendar.", CreatedAt: now.Add(-8 * time.Hour)}}},
		{ID: 2, AccountID: 1, Author: "DEMO", Kind: "bug", Title: "Wintergrasp queue message is unclear", Body: "When the queue is full, the client message does not explain when another slot may become available.", Category: "battlegrounds", Status: "in_progress", Priority: "high", Labels: []string{"worldserver", "ux"}, VoteCount: 7, CreatedAt: now.Add(-48 * time.Hour), UpdatedAt: now.Add(-3 * time.Hour)},
	}
}

func normalizeCommunityLabels(raw string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || len(value) > 32 || seen[value] || len(out) >= 10 {
			continue
		}
		valid := true
		for _, char := range value {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
				valid = false
				break
			}
		}
		if valid {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func validCommunityCategory(value string) bool {
	if value == "" || len(value) > 40 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

func scanCommunityIssue(row rowScanner, issue *communityIssue) error {
	var labels string
	err := row.Scan(&issue.ID, &issue.AccountID, &issue.Author, &issue.Kind, &issue.Title, &issue.Body, &issue.Category, &issue.Status, &issue.Priority, &labels, &issue.StaffResponse, &issue.VoteCount, &issue.CommentCount, &issue.ViewerVoted, &issue.CreatedAt, &issue.UpdatedAt)
	issue.Labels = normalizeCommunityLabels(labels)
	return err
}

func (s *Server) communityIssues(w http.ResponseWriter, r *http.Request) {
	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	queryText := strings.TrimSpace(r.URL.Query().Get("q"))
	sortBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort")))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if kind != "" && !communityKinds[kind] {
		problem(w, http.StatusUnprocessableEntity, "Invalid tracker type")
		return
	}
	if status != "" && !communityStatuses[status] {
		problem(w, http.StatusUnprocessableEntity, "Invalid tracker status")
		return
	}
	if len(queryText) > 100 {
		problem(w, http.StatusUnprocessableEntity, "Search is too long")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		ensureMockCommunityLocked(s.mock)
		viewer := s.trackerViewerID(r)
		items := []communityIssue{}
		for _, source := range s.mock.communityIssues {
			if kind != "" && source.Kind != kind || status != "" && source.Status != status {
				continue
			}
			haystack := strings.ToLower(source.Title + " " + source.Body + " " + strings.Join(source.Labels, " "))
			if queryText != "" && !strings.Contains(haystack, strings.ToLower(queryText)) {
				continue
			}
			source.Comments = nil
			source.ViewerVoted = viewer != 0 && s.mock.communityVotes[source.ID]
			items = append(items, source)
		}
		s.mock.mu.Unlock()
		sort.SliceStable(items, func(i, j int) bool {
			if sortBy == "votes" && items[i].VoteCount != items[j].VoteCount {
				return items[i].VoteCount > items[j].VoteCount
			}
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		})
		total := len(items)
		start := (page - 1) * 20
		if start > total {
			start = total
		}
		end := start + 20
		if end > total {
			end = total
		}
		jsonOut(w, http.StatusOK, map[string]any{"issues": items[start:end], "page": page, "pageSize": 20, "total": total})
		return
	}

	viewer := s.trackerViewerID(r)
	where := []string{"i.realm_key=?"}
	args := []any{viewer, s.c.RealmKey}
	if kind != "" {
		where = append(where, "i.kind=?")
		args = append(args, kind)
	}
	if status != "" {
		where = append(where, "i.status=?")
		args = append(args, status)
	}
	if queryText != "" {
		where = append(where, "(i.title LIKE ? OR i.body LIKE ? OR i.labels LIKE ?)")
		like := "%" + queryText + "%"
		args = append(args, like, like, like)
	}
	whereSQL := strings.Join(where, " AND ")
	var total uint64
	countArgs := append([]any(nil), args[1:]...)
	if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_community_issues i WHERE "+whereSQL, countArgs...).Scan(&total); err != nil {
		problem(w, http.StatusInternalServerError, "Could not load community tracker")
		return
	}
	order := "i.updated_at DESC,i.id DESC"
	if sortBy == "votes" {
		order = "i.vote_count DESC,i.updated_at DESC"
	}
	args = append(args, 20, (page-1)*20)
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT i.id,i.account_id,COALESCE(a.username,'Deleted player'),i.kind,i.title,i.body,i.category,i.status,i.priority,i.labels,i.staff_response,i.vote_count,i.comment_count,EXISTS(SELECT 1 FROM portal_community_issue_votes v WHERE v.issue_id=i.id AND v.account_id=?),i.created_at,i.updated_at FROM portal_community_issues i LEFT JOIN account a ON a.id=i.account_id WHERE `+whereSQL+` ORDER BY `+order+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load community tracker")
		return
	}
	defer rows.Close()
	items := []communityIssue{}
	for rows.Next() {
		var issue communityIssue
		if scanCommunityIssue(rows, &issue) == nil {
			issue.Body = ""
			items = append(items, issue)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"issues": items, "page": page, "pageSize": 20, "total": total})
}

func (s *Server) createCommunityIssue(w http.ResponseWriter, r *http.Request) {
	a, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in to submit feedback")
		return
	}
	var in struct{ Kind, Title, Body, Category string }
	if !decode(w, r, &in) {
		return
	}
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.Category = strings.ToLower(strings.TrimSpace(in.Category))
	if !communityKinds[in.Kind] || len(in.Title) < 5 || len(in.Title) > 160 || len(in.Body) < 20 || len(in.Body) > 10000 || !validCommunityCategory(in.Category) {
		problem(w, http.StatusUnprocessableEntity, "Choose a type and category, use a 5–160 character title, and provide 20–10000 characters of detail")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		ensureMockCommunityLocked(s.mock)
		id := uint64(len(s.mock.communityIssues) + 1)
		now := time.Now()
		s.mock.communityIssues = append([]communityIssue{{ID: id, AccountID: a.ID, Author: a.Username, Kind: in.Kind, Title: in.Title, Body: in.Body, Category: in.Category, Status: "open", Priority: "normal", Labels: []string{}, CreatedAt: now, UpdatedAt: now}}, s.mock.communityIssues...)
		s.mock.mu.Unlock()
		jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_community_issues(realm_key,account_id,kind,title,body,category,status,priority,labels,staff_response) VALUES(?,?,?,?,?,?,'open','normal','','')`, s.c.RealmKey, a.ID, in.Kind, in.Title, in.Body, in.Category)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not submit feedback")
		return
	}
	id, _ := result.LastInsertId()
	s.notifyDiscordAsync("New community "+in.Kind, "**%s** submitted **%s** on **%s**.", a.Username, in.Title, s.c.RealmName)
	jsonOut(w, http.StatusCreated, map[string]any{"ok": true, "id": id})
}

func (s *Server) communityIssueDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid tracker item")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		ensureMockCommunityLocked(s.mock)
		for _, issue := range s.mock.communityIssues {
			if issue.ID == id {
				issue.ViewerVoted = s.trackerViewerID(r) != 0 && s.mock.communityVotes[id]
				jsonOut(w, http.StatusOK, map[string]any{"issue": issue})
				return
			}
		}
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	viewer := s.trackerViewerID(r)
	var issue communityIssue
	row := s.s.Auth.QueryRowContext(r.Context(), `SELECT i.id,i.account_id,COALESCE(a.username,'Deleted player'),i.kind,i.title,i.body,i.category,i.status,i.priority,i.labels,i.staff_response,i.vote_count,i.comment_count,EXISTS(SELECT 1 FROM portal_community_issue_votes v WHERE v.issue_id=i.id AND v.account_id=?),i.created_at,i.updated_at FROM portal_community_issues i LEFT JOIN account a ON a.id=i.account_id WHERE i.id=? AND i.realm_key=?`, viewer, id, s.c.RealmKey)
	if err := scanCommunityIssue(row, &issue); err != nil {
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT c.id,c.issue_id,c.account_id,COALESCE(a.username,'Deleted player'),c.author_role,c.body,c.created_at FROM portal_community_issue_comments c LEFT JOIN account a ON a.id=c.account_id WHERE c.issue_id=? ORDER BY c.id`, id)
	if err == nil {
		defer rows.Close()
		issue.Comments = []communityComment{}
		for rows.Next() {
			var comment communityComment
			if rows.Scan(&comment.ID, &comment.IssueID, &comment.AccountID, &comment.Author, &comment.AuthorRole, &comment.Body, &comment.CreatedAt) == nil {
				issue.Comments = append(issue.Comments, comment)
			}
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"issue": issue})
}

func (s *Server) communityIssueVote(w http.ResponseWriter, r *http.Request) {
	a, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in to vote")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid tracker item")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		ensureMockCommunityLocked(s.mock)
		for index := range s.mock.communityIssues {
			if s.mock.communityIssues[index].ID != id {
				continue
			}
			if s.mock.communityVotes[id] {
				delete(s.mock.communityVotes, id)
				if s.mock.communityIssues[index].VoteCount > 0 {
					s.mock.communityIssues[index].VoteCount--
				}
			} else {
				s.mock.communityVotes[id] = true
				s.mock.communityIssues[index].VoteCount++
			}
			s.mock.communityIssues[index].UpdatedAt = time.Now()
			jsonOut(w, http.StatusOK, map[string]any{"voted": s.mock.communityVotes[id], "voteCount": s.mock.communityIssues[index].VoteCount})
			return
		}
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRowContext(r.Context(), "SELECT status FROM portal_community_issues WHERE id=? AND realm_key=? FOR UPDATE", id, s.c.RealmKey).Scan(&status); err != nil {
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	result, err := tx.ExecContext(r.Context(), "DELETE FROM portal_community_issue_votes WHERE issue_id=? AND account_id=?", id, a.ID)
	voted := false
	if err == nil {
		removed, _ := result.RowsAffected()
		if removed > 0 {
			_, err = tx.ExecContext(r.Context(), "UPDATE portal_community_issues SET vote_count=GREATEST(vote_count-1,0) WHERE id=?", id)
		} else {
			_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_community_issue_votes(issue_id,account_id) VALUES(?,?)", id, a.ID)
			if err == nil {
				_, err = tx.ExecContext(r.Context(), "UPDATE portal_community_issues SET vote_count=vote_count+1 WHERE id=?", id)
				voted = true
			}
		}
	}
	var count uint32
	if err == nil {
		err = tx.QueryRowContext(r.Context(), "SELECT vote_count FROM portal_community_issues WHERE id=?", id).Scan(&count)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not update vote")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"voted": voted, "voteCount": count})
}

func (s *Server) communityIssueComment(w http.ResponseWriter, r *http.Request) {
	a, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in to join the discussion")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid tracker item")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Body = strings.TrimSpace(in.Body)
	if len(in.Body) < 2 || len(in.Body) > 4000 {
		problem(w, http.StatusUnprocessableEntity, "Comment must be 2–4000 characters")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		ensureMockCommunityLocked(s.mock)
		for index := range s.mock.communityIssues {
			issue := &s.mock.communityIssues[index]
			if issue.ID != id {
				continue
			}
			if issue.Status == "closed" || issue.Status == "declined" {
				problem(w, http.StatusConflict, "This discussion is closed")
				return
			}
			comment := communityComment{ID: uint64(len(issue.Comments) + 1), IssueID: id, AccountID: a.ID, Author: a.Username, AuthorRole: "player", Body: in.Body, CreatedAt: time.Now()}
			issue.Comments = append(issue.Comments, comment)
			issue.CommentCount++
			issue.UpdatedAt = comment.CreatedAt
			jsonOut(w, http.StatusCreated, comment)
			return
		}
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var status string
	var ownerID uint32
	if err = tx.QueryRowContext(r.Context(), "SELECT status,account_id FROM portal_community_issues WHERE id=? AND realm_key=? FOR UPDATE", id, s.c.RealmKey).Scan(&status, &ownerID); err != nil {
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	if status == "closed" || status == "declined" {
		problem(w, http.StatusConflict, "This discussion is closed")
		return
	}
	result, err := tx.ExecContext(r.Context(), "INSERT INTO portal_community_issue_comments(issue_id,account_id,author_role,body) VALUES(?,?,'player',?)", id, a.ID, in.Body)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE portal_community_issues SET comment_count=comment_count+1 WHERE id=?", id)
	}
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not add comment")
		return
	}
	commentID, _ := result.LastInsertId()
	if ownerID != a.ID {
		s.notifyAccount(r.Context(), ownerID, "community", "New tracker comment", a.Username+" replied to your community submission.", "/tracker/"+strconv.FormatUint(id, 10))
	}
	jsonOut(w, http.StatusCreated, communityComment{ID: uint64(commentID), IssueID: id, AccountID: a.ID, Author: a.Username, AuthorRole: "player", Body: in.Body, CreatedAt: time.Now()})
}

func (s *Server) adminCommunityIssues(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "content"); !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	s.communityIssues(w, r)
}

func (s *Server) adminCommunityIssue(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, http.StatusBadRequest, "Invalid tracker item")
		return
	}
	var in struct{ Status, Priority, Labels, StaffResponse string }
	if !decode(w, r, &in) {
		return
	}
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	in.Priority = strings.ToLower(strings.TrimSpace(in.Priority))
	in.StaffResponse = strings.TrimSpace(in.StaffResponse)
	labels := normalizeCommunityLabels(in.Labels)
	if !communityStatuses[in.Status] || !communityPriorities[in.Priority] || len(in.StaffResponse) > 10000 {
		problem(w, http.StatusUnprocessableEntity, "Choose a valid status and priority")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		ensureMockCommunityLocked(s.mock)
		for index := range s.mock.communityIssues {
			issue := &s.mock.communityIssues[index]
			if issue.ID != id {
				continue
			}
			issue.Status, issue.Priority, issue.Labels, issue.StaffResponse, issue.UpdatedAt = in.Status, in.Priority, labels, in.StaffResponse, time.Now()
			s.mock.notifications = append([]notification{{ID: uint64(len(s.mock.notifications) + 1), Kind: "community", Title: "Submission updated", Message: "Your submission is now " + strings.ReplaceAll(in.Status, "_", " ") + ".", ActionURL: "/tracker/" + strconv.FormatUint(id, 10), Created: time.Now()}}, s.mock.notifications...)
			jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	var ownerID uint32
	if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT account_id FROM portal_community_issues WHERE id=? AND realm_key=?", id, s.c.RealmKey).Scan(&ownerID); err != nil {
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_community_issues SET status=?,priority=?,labels=?,staff_response=? WHERE id=? AND realm_key=?", in.Status, in.Priority, strings.Join(labels, ","), in.StaffResponse, id, s.c.RealmKey)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not update tracker item")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusNotFound, "Tracker item not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'community.triage',?,?)", actor.ID, strconv.FormatUint(id, 10), "status="+in.Status+" priority="+in.Priority+" labels="+strings.Join(labels, ","))
	s.notifyAccount(r.Context(), ownerID, "community", "Submission updated", "Your submission is now "+strings.ReplaceAll(in.Status, "_", " ")+".", "/tracker/"+strconv.FormatUint(id, 10))
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}
