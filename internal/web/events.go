package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type realmEvent struct {
	ID                   uint64             `json:"id"`
	Title                string             `json:"title"`
	Description          string             `json:"description"`
	Category             string             `json:"category"`
	Location             string             `json:"location"`
	StartsAt             time.Time          `json:"startsAt"`
	EndsAt               *time.Time         `json:"endsAt,omitempty"`
	URL                  string             `json:"url"`
	Status               string             `json:"status"`
	MaxParticipants      uint32             `json:"maxParticipants"`
	SignupEnabled        bool               `json:"signupEnabled"`
	RegistrationDeadline *time.Time         `json:"registrationDeadline,omitempty"`
	RewardCredits        uint32             `json:"rewardCredits"`
	RegisteredCount      uint32             `json:"registeredCount"`
	ViewerRegistration   *eventRegistration `json:"viewerRegistration,omitempty"`
	CreatedBy            uint32             `json:"createdBy,omitempty"`
}

type eventRegistration struct {
	EventID       uint64    `json:"eventId"`
	AccountID     uint32    `json:"accountId"`
	Username      string    `json:"username,omitempty"`
	CharacterGUID uint32    `json:"characterGuid"`
	CharacterName string    `json:"characterName,omitempty"`
	Status        string    `json:"status"`
	Rewarded      bool      `json:"rewarded"`
	RegisteredAt  time.Time `json:"registeredAt"`
}

const eventSelect = `id,title,description,category,location,starts_at,ends_at,url,status,max_participants,signup_enabled,registration_deadline,reward_credits,created_by`

func scanEvent(row rowScanner, event *realmEvent) error {
	return row.Scan(&event.ID, &event.Title, &event.Description, &event.Category, &event.Location, &event.StartsAt, &event.EndsAt, &event.URL, &event.Status, &event.MaxParticipants, &event.SignupEnabled, &event.RegistrationDeadline, &event.RewardCredits, &event.CreatedBy)
}

func (s *Server) enrichEventRegistration(r *http.Request, event *realmEvent, accountID uint32) {
	if s.c.MockMode {
		if registration, exists := s.mock.eventRegistrations[event.ID]; exists && registration.Status != "cancelled" {
			event.RegisteredCount = 1
			if accountID != 0 && registration.AccountID == accountID {
				copy := registration
				event.ViewerRegistration = &copy
			}
		}
		return
	}
	_ = s.s.Auth.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_event_registrations WHERE event_id=? AND status IN ('registered','attended')`, event.ID).Scan(&event.RegisteredCount)
	if accountID == 0 {
		return
	}
	var registration eventRegistration
	err := s.s.Auth.QueryRowContext(r.Context(), `SELECT event_id,account_id,character_guid,status,registered_at FROM portal_event_registrations WHERE event_id=? AND account_id=? AND status<>'cancelled'`, event.ID, accountID).Scan(&registration.EventID, &registration.AccountID, &registration.CharacterGUID, &registration.Status, &registration.RegisteredAt)
	if err == nil {
		event.ViewerRegistration = &registration
	}
}

func (s *Server) publicEvents(w http.ResponseWriter, r *http.Request) {
	viewer := uint32(0)
	if account, err := s.trackerAccount(r); err == nil {
		viewer = account.ID
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		events := append([]realmEvent(nil), s.mock.events...)
		out := []realmEvent{}
		for _, event := range events {
			if event.Status == "scheduled" && event.StartsAt.After(time.Now().Add(-24*time.Hour)) {
				s.enrichEventRegistration(r, &event, viewer)
				out = append(out, event)
			}
		}
		jsonOut(w, 200, map[string]any{"events": out})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+eventSelect+" FROM portal_events WHERE realm_key=? AND status='scheduled' AND starts_at>=DATE_SUB(NOW(),INTERVAL 1 DAY) ORDER BY starts_at LIMIT 100", s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not load events")
		return
	}
	defer rows.Close()
	events := []realmEvent{}
	for rows.Next() {
		var event realmEvent
		if scanEvent(rows, &event) == nil {
			s.enrichEventRegistration(r, &event, viewer)
			events = append(events, event)
		}
	}
	jsonOut(w, 200, map[string]any{"events": events})
}

func validateEvent(event *realmEvent) error {
	event.Title = strings.TrimSpace(event.Title)
	event.Description = strings.TrimSpace(event.Description)
	event.Category = strings.ToLower(strings.TrimSpace(event.Category))
	event.Status = strings.ToLower(strings.TrimSpace(event.Status))
	if event.Category == "" {
		event.Category = "community"
	}
	if event.Status == "" {
		event.Status = "scheduled"
	}
	if len(event.Title) < 2 || len(event.Title) > 160 || len(event.Description) > 2000 || len(event.Category) > 40 || len(event.Location) > 120 || len(event.URL) > 500 || event.StartsAt.IsZero() {
		return fmt.Errorf("title, start time, and valid field lengths are required")
	}
	if !map[string]bool{"scheduled": true, "cancelled": true, "completed": true}[event.Status] {
		return fmt.Errorf("invalid event status")
	}
	if event.EndsAt != nil && !event.EndsAt.After(event.StartsAt) {
		return fmt.Errorf("event end must be after its start")
	}
	if event.RegistrationDeadline != nil && event.RegistrationDeadline.After(event.StartsAt) {
		return fmt.Errorf("registration deadline cannot be after the event starts")
	}
	if event.RewardCredits > 100000 {
		return fmt.Errorf("event reward cannot exceed 100,000 credits")
	}
	if event.URL != "" {
		parsed, err := url.ParseRequestURI(event.URL)
		if err != nil || (!strings.HasPrefix(event.URL, "/") && (parsed.Scheme != "http" && parsed.Scheme != "https")) {
			return fmt.Errorf("invalid event URL")
		}
	}
	return nil
}

func (s *Server) adminEvents(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, 403, "Content permission required")
		return
	}
	if r.Method == http.MethodGet {
		if s.c.MockMode {
			s.mock.mu.Lock()
			defer s.mock.mu.Unlock()
			events := append([]realmEvent(nil), s.mock.events...)
			for index := range events {
				s.enrichEventRegistration(r, &events[index], 0)
			}
			jsonOut(w, 200, map[string]any{"events": events})
			return
		}
		rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT "+eventSelect+" FROM portal_events WHERE realm_key=? ORDER BY starts_at DESC LIMIT 200", s.c.RealmKey)
		if err != nil {
			problem(w, 500, "Could not load events")
			return
		}
		defer rows.Close()
		events := []realmEvent{}
		for rows.Next() {
			var event realmEvent
			if scanEvent(rows, &event) == nil {
				s.enrichEventRegistration(r, &event, 0)
				events = append(events, event)
			}
		}
		jsonOut(w, 200, map[string]any{"events": events})
		return
	}
	var event realmEvent
	if !decode(w, r, &event) {
		return
	}
	if err := validateEvent(&event); err != nil {
		problem(w, 422, err.Error())
		return
	}
	event.CreatedBy = a.ID
	if s.c.MockMode {
		s.mock.mu.Lock()
		event.ID = uint64(len(s.mock.events) + 1)
		s.mock.events = append(s.mock.events, event)
		s.mock.mu.Unlock()
	} else {
		res, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_events(realm_key,title,description,category,location,starts_at,ends_at,url,status,max_participants,signup_enabled,registration_deadline,reward_credits,created_by) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, s.c.RealmKey, event.Title, event.Description, event.Category, event.Location, event.StartsAt, event.EndsAt, event.URL, event.Status, event.MaxParticipants, event.SignupEnabled, event.RegistrationDeadline, event.RewardCredits, a.ID)
		if err != nil {
			problem(w, 500, "Could not create event")
			return
		}
		id, _ := res.LastInsertId()
		event.ID = uint64(id)
	}
	jsonOut(w, 201, event)
}

func (s *Server) adminEventItem(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, 403, "Content permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		problem(w, 400, "Invalid event")
		return
	}
	if r.Method == http.MethodDelete {
		if s.c.MockMode {
			s.mock.mu.Lock()
			defer s.mock.mu.Unlock()
			for i := range s.mock.events {
				if s.mock.events[i].ID == id {
					s.mock.events[i].Status = "cancelled"
					jsonOut(w, 200, map[string]bool{"ok": true})
					return
				}
			}
		} else {
			res, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_events SET status='cancelled' WHERE id=? AND realm_key=?", id, s.c.RealmKey)
			if err == nil {
				if changed, _ := res.RowsAffected(); changed > 0 {
					jsonOut(w, 200, map[string]bool{"ok": true})
					return
				}
			}
		}
		problem(w, 404, "Event not found")
		return
	}
	var event realmEvent
	if !decode(w, r, &event) {
		return
	}
	if err := validateEvent(&event); err != nil {
		problem(w, 422, err.Error())
		return
	}
	event.ID = id
	event.CreatedBy = a.ID
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for i := range s.mock.events {
			if s.mock.events[i].ID == id {
				s.mock.events[i] = event
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
		problem(w, 404, "Event not found")
		return
	}
	res, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_events SET title=?,description=?,category=?,location=?,starts_at=?,ends_at=?,url=?,status=?,max_participants=?,signup_enabled=?,registration_deadline=?,reward_credits=? WHERE id=? AND realm_key=?`, event.Title, event.Description, event.Category, event.Location, event.StartsAt, event.EndsAt, event.URL, event.Status, event.MaxParticipants, event.SignupEnabled, event.RegistrationDeadline, event.RewardCredits, id, s.c.RealmKey)
	if err != nil {
		problem(w, 500, "Could not update event")
		return
	}
	if changed, _ := res.RowsAffected(); changed == 0 {
		problem(w, 404, "Event not found")
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) eventRegistrationAction(w http.ResponseWriter, r *http.Request) {
	account, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	eventID, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || eventID == 0 {
		problem(w, http.StatusBadRequest, "Invalid event")
		return
	}
	if r.Method == http.MethodDelete {
		if s.c.MockMode {
			s.mock.mu.Lock()
			registration, exists := s.mock.eventRegistrations[eventID]
			if exists && registration.AccountID == account.ID {
				registration.Status = "cancelled"
				s.mock.eventRegistrations[eventID] = registration
			}
			s.mock.mu.Unlock()
			if !exists {
				problem(w, http.StatusNotFound, "Event registration not found")
				return
			}
			jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_event_registrations SET status='cancelled' WHERE event_id=? AND account_id=? AND status='registered'`, eventID, account.ID)
		if err != nil {
			problem(w, http.StatusInternalServerError, "Could not cancel event registration")
			return
		}
		if changed, _ := result.RowsAffected(); changed == 0 {
			problem(w, http.StatusNotFound, "Active event registration not found")
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	var input struct {
		CharacterGUID uint32 `json:"characterGuid"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.CharacterGUID == 0 {
		problem(w, http.StatusUnprocessableEntity, "Choose a character")
		return
	}
	if s.c.MockMode {
		characterName := ""
		for _, character := range mockCharacters {
			if character.GUID == input.CharacterGUID {
				characterName = character.Name
				break
			}
		}
		if characterName == "" {
			problem(w, http.StatusNotFound, "Character not found on this account")
			return
		}
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		var event *realmEvent
		for index := range s.mock.events {
			if s.mock.events[index].ID == eventID {
				event = &s.mock.events[index]
				break
			}
		}
		if event == nil || !event.SignupEnabled || event.Status != "scheduled" || !event.StartsAt.After(time.Now()) || event.RegistrationDeadline != nil && time.Now().After(*event.RegistrationDeadline) {
			problem(w, http.StatusConflict, "Registration is closed for this event")
			return
		}
		if current, exists := s.mock.eventRegistrations[eventID]; !exists || current.Status == "cancelled" {
			if event.MaxParticipants > 0 {
				registered := uint32(0)
				for _, item := range s.mock.eventRegistrations {
					if item.EventID == eventID && (item.Status == "registered" || item.Status == "attended") {
						registered++
					}
				}
				if registered >= event.MaxParticipants {
					problem(w, http.StatusConflict, "This event is full")
					return
				}
			}
		}
		registration := eventRegistration{EventID: eventID, AccountID: account.ID, Username: account.Username, CharacterGUID: input.CharacterGUID, CharacterName: characterName, Status: "registered", RegisteredAt: time.Now()}
		s.mock.eventRegistrations[eventID] = registration
		jsonOut(w, http.StatusCreated, registration)
		return
	}
	var characterName string
	query := fmt.Sprintf("SELECT name FROM %s.characters WHERE guid=? AND account=? AND deleteDate IS NULL", s.c.CharactersDB)
	if err := s.s.Characters.QueryRowContext(r.Context(), query, input.CharacterGUID, account.ID).Scan(&characterName); err != nil {
		problem(w, http.StatusNotFound, "Character not found on this account")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var event realmEvent
	if err = scanEvent(tx.QueryRowContext(r.Context(), "SELECT "+eventSelect+" FROM portal_events WHERE id=? AND realm_key=? FOR UPDATE", eventID, s.c.RealmKey), &event); err != nil || !event.SignupEnabled || event.Status != "scheduled" || !event.StartsAt.After(time.Now()) || event.RegistrationDeadline != nil && time.Now().After(*event.RegistrationDeadline) {
		problem(w, http.StatusConflict, "Registration is closed for this event")
		return
	}
	var currentStatus string
	_ = tx.QueryRowContext(r.Context(), `SELECT status FROM portal_event_registrations WHERE event_id=? AND account_id=?`, eventID, account.ID).Scan(&currentStatus)
	if event.MaxParticipants > 0 && currentStatus != "registered" && currentStatus != "attended" {
		var registered uint32
		if err = tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM portal_event_registrations WHERE event_id=? AND status IN ('registered','attended')`, eventID).Scan(&registered); err != nil || registered >= event.MaxParticipants {
			problem(w, http.StatusConflict, "This event is full")
			return
		}
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_event_registrations(event_id,account_id,character_guid,status) VALUES(?,?,?,'registered') ON DUPLICATE KEY UPDATE character_guid=VALUES(character_guid),status='registered'`, eventID, account.ID, input.CharacterGUID)
	if err != nil || tx.Commit() != nil {
		problem(w, http.StatusInternalServerError, "Could not reserve an event place")
		return
	}
	jsonOut(w, http.StatusCreated, eventRegistration{EventID: eventID, AccountID: account.ID, Username: account.Username, CharacterGUID: input.CharacterGUID, CharacterName: characterName, Status: "registered", RegisteredAt: time.Now()})
}

func (s *Server) adminEventParticipants(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "content"); !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	eventID, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || eventID == 0 {
		problem(w, http.StatusBadRequest, "Invalid event")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		items := []eventRegistration{}
		if item, exists := s.mock.eventRegistrations[eventID]; exists {
			items = append(items, item)
		}
		jsonOut(w, http.StatusOK, map[string]any{"participants": items})
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), `SELECT r.event_id,r.account_id,a.username,r.character_guid,r.status,(g.account_id IS NOT NULL),r.registered_at FROM portal_event_registrations r JOIN account a ON a.id=r.account_id LEFT JOIN portal_event_reward_grants g ON g.event_id=r.event_id AND g.account_id=r.account_id WHERE r.event_id=? ORDER BY r.registered_at`, eventID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load event participants")
		return
	}
	defer rows.Close()
	items := []eventRegistration{}
	for rows.Next() {
		var item eventRegistration
		if rows.Scan(&item.EventID, &item.AccountID, &item.Username, &item.CharacterGUID, &item.Status, &item.Rewarded, &item.RegisteredAt) == nil {
			_ = s.s.Characters.QueryRowContext(r.Context(), fmt.Sprintf("SELECT name FROM %s.characters WHERE guid=?", s.c.CharactersDB), item.CharacterGUID).Scan(&item.CharacterName)
			items = append(items, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"participants": items})
}

func (s *Server) adminEventParticipantStatus(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "content")
	if !ok {
		problem(w, http.StatusForbidden, "Content access required")
		return
	}
	eventID, eventErr := strconv.ParseUint(r.PathValue("id"), 10, 64)
	accountID, accountErr := strconv.ParseUint(r.PathValue("account"), 10, 32)
	var input struct {
		Status string `json:"status"`
	}
	if eventErr != nil || accountErr != nil || !decode(w, r, &input) {
		if eventErr != nil || accountErr != nil {
			problem(w, http.StatusBadRequest, "Invalid event participant")
		}
		return
	}
	input.Status = strings.ToLower(strings.TrimSpace(input.Status))
	if !map[string]bool{"registered": true, "attended": true, "no_show": true, "cancelled": true}[input.Status] {
		problem(w, http.StatusUnprocessableEntity, "Invalid attendance status")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		item, exists := s.mock.eventRegistrations[eventID]
		if exists && item.AccountID == uint32(accountID) {
			item.Status = input.Status
			s.mock.eventRegistrations[eventID] = item
		}
		s.mock.mu.Unlock()
		if !exists {
			problem(w, http.StatusNotFound, "Event participant not found")
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), `UPDATE portal_event_registrations SET status=? WHERE event_id=? AND account_id=?`, input.Status, eventID, accountID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not update attendance")
		return
	}
	if changed, _ := result.RowsAffected(); changed == 0 {
		problem(w, http.StatusNotFound, "Event participant not found")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'event.attendance',?,?)`, actor.ID, fmt.Sprintf("%d:%d", eventID, accountID), input.Status)
	jsonOut(w, http.StatusOK, map[string]bool{"ok": true})
}

type eventRewardResult struct {
	AccountID uint32 `json:"accountId"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

func (s *Server) adminEventRewards(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "credits")
	if !ok {
		problem(w, http.StatusForbidden, "Credit management access required")
		return
	}
	eventID, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var input struct {
		AccountIDs []uint32 `json:"accountIds"`
		Reason     string   `json:"reason"`
	}
	if err != nil || eventID == 0 || !decode(w, r, &input) {
		if err != nil || eventID == 0 {
			problem(w, http.StatusBadRequest, "Invalid event")
		}
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.AccountIDs) == 0 || len(input.AccountIDs) > 100 || len(input.Reason) < 3 || len(input.Reason) > 255 {
		problem(w, http.StatusUnprocessableEntity, "Choose 1–100 accounts and provide a reward reason")
		return
	}
	results := make([]eventRewardResult, 0, len(input.AccountIDs))
	seen := map[uint32]bool{}
	for _, accountID := range input.AccountIDs {
		if accountID == 0 || seen[accountID] {
			results = append(results, eventRewardResult{AccountID: accountID, Status: "skipped", Message: "Invalid or duplicate account"})
			continue
		}
		seen[accountID] = true
		if s.c.MockMode {
			s.mock.mu.Lock()
			registration, exists := s.mock.eventRegistrations[eventID]
			rewardCredits := uint32(0)
			for _, event := range s.mock.events {
				if event.ID == eventID {
					rewardCredits = event.RewardCredits
					break
				}
			}
			if exists && registration.AccountID == accountID && registration.Status == "attended" && !registration.Rewarded && rewardCredits > 0 {
				registration.Rewarded = true
				s.mock.eventRegistrations[eventID] = registration
				s.mock.balance += rewardCredits
				results = append(results, eventRewardResult{AccountID: accountID, Status: "awarded", Message: fmt.Sprintf("%d credits awarded", rewardCredits)})
			} else if exists && registration.Rewarded {
				results = append(results, eventRewardResult{AccountID: accountID, Status: "duplicate", Message: "Reward already granted"})
			} else {
				results = append(results, eventRewardResult{AccountID: accountID, Status: "skipped", Message: "Attendance is not confirmed"})
			}
			s.mock.mu.Unlock()
			continue
		}
		results = append(results, s.rewardEventAccount(r, actor.ID, eventID, accountID, input.Reason))
	}
	jsonOut(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) rewardEventAccount(r *http.Request, actorID uint32, eventID uint64, accountID uint32, reason string) eventRewardResult {
	result := eventRewardResult{AccountID: accountID}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		result.Status, result.Message = "failed", "Database unavailable"
		return result
	}
	defer tx.Rollback()
	var credits uint32
	if err = tx.QueryRowContext(r.Context(), `SELECT reward_credits FROM portal_events WHERE id=? AND realm_key=? FOR UPDATE`, eventID, s.c.RealmKey).Scan(&credits); err != nil || credits == 0 {
		result.Status, result.Message = "skipped", "Event has no configured credit reward"
		return result
	}
	var status string
	if err = tx.QueryRowContext(r.Context(), `SELECT status FROM portal_event_registrations WHERE event_id=? AND account_id=? FOR UPDATE`, eventID, accountID).Scan(&status); err != nil || status != "attended" {
		result.Status, result.Message = "skipped", "Attendance is not confirmed"
		return result
	}
	grant, err := tx.ExecContext(r.Context(), `INSERT IGNORE INTO portal_event_reward_grants(event_id,account_id,credits,granted_by,reason) VALUES(?,?,?,?,?)`, eventID, accountID, credits, actorID, reason)
	if err != nil {
		result.Status, result.Message = "failed", "Could not record reward"
		return result
	}
	if changed, _ := grant.RowsAffected(); changed == 0 {
		result.Status, result.Message = "duplicate", "Reward already granted"
		return result
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)`, accountID, credits); err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(?,?,?,?)`, actorID, accountID, credits, "Event reward: "+reason)
	}
	if err != nil || tx.Commit() != nil {
		result.Status, result.Message = "failed", "Could not credit wallet"
		return result
	}
	s.notifyAccount(r.Context(), accountID, "reward", "Event reward received", fmt.Sprintf("%d credits were added for %s.", credits, reason), "/account/rewards")
	result.Status, result.Message = "awarded", fmt.Sprintf("%d credits awarded", credits)
	return result
}
