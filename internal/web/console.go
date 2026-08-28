package web

import (
	"net/http"
	"strings"
	"time"
)

type consoleEntry struct {
	ID       uint64    `json:"id"`
	Actor    string    `json:"actor"`
	Command  string    `json:"command"`
	Response string    `json:"response"`
	Success  bool      `json:"success"`
	IP       string    `json:"ip"`
	Created  time.Time `json:"created"`
}

func (s *Server) requireConsoleGM(r *http.Request) (account, bool) {
	a, err := s.auth(r)
	if err != nil {
		return a, false
	}
	required := s.c.GMConsoleLevel
	if required < 1 {
		required = 3
	}
	if s.c.GMConsoleAllowAll && required < 3 {
		required = 3
	}
	return a, int(s.gmLevel(r.Context(), a.ID)) >= required
}

func normalizeConsoleCommand(command string) (string, bool) {
	command = strings.TrimSpace(command)
	command = strings.TrimSpace(strings.TrimPrefix(command, "."))
	if command == "" || len(command) > 255 {
		return "", false
	}
	for _, r := range command {
		if r < 32 || r == 127 {
			return "", false
		}
	}
	return command, true
}

func consoleCommandAllowed(command string, allowAll bool, prefixes []string) bool {
	if allowAll {
		return true
	}
	command = strings.ToLower(command)
	for _, prefix := range prefixes {
		prefix = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(prefix, ".")))
		if command == prefix || strings.HasPrefix(command, prefix+" ") {
			return true
		}
	}
	return false
}

func auditConsoleCommand(command string) string {
	lower := strings.ToLower(command)
	for _, sensitive := range []string{"account create", "account set password", "account set email", "account set regemail"} {
		if lower == sensitive || strings.HasPrefix(lower, sensitive+" ") {
			return sensitive + " [arguments redacted]"
		}
	}
	return command
}

func limitConsoleText(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 16000 {
		runes = append(runes[:16000], []rune("\n[response truncated]")...)
	}
	return string(runes)
}

func (s *Server) adminConsoleExecute(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireConsoleGM(r)
	if !ok {
		problem(w, http.StatusForbidden, "GM console access required")
		return
	}
	if !s.soap.Enabled() {
		problem(w, http.StatusServiceUnavailable, "AzerothCore SOAP is not configured")
		return
	}
	var in struct {
		Command string `json:"command"`
	}
	if !decode(w, r, &in) {
		return
	}
	command, valid := normalizeConsoleCommand(in.Command)
	if !valid {
		problem(w, http.StatusUnprocessableEntity, "Command must contain 1–255 characters on one line")
		return
	}
	if !consoleCommandAllowed(command, s.c.GMConsoleAllowAll, s.c.GMConsoleAllowed) {
		problem(w, http.StatusForbidden, "Command is not included in GM_CONSOLE_ALLOWED_PREFIXES")
		return
	}
	result, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_command_log(actor_account_id,command,response,success,ip_address) VALUES(?,?,'Pending',0,?)", actor.ID, auditConsoleCommand(command), s.clientIP(r))
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not create command audit entry")
		return
	}
	logID, _ := result.LastInsertId()
	response, commandErr := s.soap.Command(r.Context(), command)
	response = limitConsoleText(response)
	if commandErr != nil {
		response = limitConsoleText(commandErr.Error())
	}
	if response == "" {
		response = "Command completed with no output."
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_command_log SET response=?,success=? WHERE id=?", response, commandErr == nil, logID)
	if commandErr != nil {
		problem(w, http.StatusBadGateway, "Worldserver rejected the command; review the console audit log")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "command": command, "output": response, "auditId": logID})
}

func (s *Server) adminConsoleHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireConsoleGM(r); !ok {
		problem(w, http.StatusForbidden, "GM console access required")
		return
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT l.id,a.username,l.command,l.response,l.success,l.ip_address,l.created_at FROM portal_command_log l JOIN `"+s.c.AuthDB+"`.account a ON a.id=l.actor_account_id ORDER BY l.id DESC LIMIT 50")
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load command history")
		return
	}
	defer rows.Close()
	entries := []consoleEntry{}
	for rows.Next() {
		var entry consoleEntry
		if rows.Scan(&entry.ID, &entry.Actor, &entry.Command, &entry.Response, &entry.Success, &entry.IP, &entry.Created) == nil {
			entries = append(entries, entry)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"entries": entries, "allowAll": s.c.GMConsoleAllowAll, "allowedPrefixes": s.c.GMConsoleAllowed})
}
