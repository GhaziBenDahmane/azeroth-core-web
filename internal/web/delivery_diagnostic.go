package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const deliveryDiagnosticItemID uint32 = 117 // Tough Jerky: a fixed, low-value WotLK consumable.

func (s *Server) adminDeliveryDiagnostic(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "realm")
	if !ok {
		problem(w, http.StatusForbidden, "Realm operator access required")
		return
	}
	character := strings.TrimSpace(s.c.DeliveryDiagnosticCharacter)
	if character == "" {
		problem(w, http.StatusConflict, "Set DELIVERY_DIAGNOSTIC_CHARACTER to a dedicated disposable character first")
		return
	}
	var in struct {
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Confirm != character {
		problem(w, http.StatusUnprocessableEntity, "Type the designated character name exactly to confirm")
		return
	}
	requestID := RequestID(r.Context())
	cleanup := fmt.Sprintf("Delete the diagnostic mail without opening it. If collected, run: character delete item %s %d 1", character, deliveryDiagnosticItemID)
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"ok": true, "character": character, "itemId": deliveryDiagnosticItemID, "correlationId": requestID, "auditId": 0, "response": "Mock SOAP delivery accepted", "cleanup": cleanup, "simulated": true})
		return
	}
	if !s.soap.Enabled() {
		problem(w, http.StatusServiceUnavailable, "AzerothCore SOAP is not configured")
		return
	}
	if strings.ContainsAny(character, " \t\r\n\"\\") {
		problem(w, http.StatusConflict, "The configured diagnostic character name is unsafe")
		return
	}
	var online bool
	query := fmt.Sprintf("SELECT online FROM `%s`.characters WHERE name=? AND deleteDate IS NULL", s.c.CharactersDB)
	if err := s.s.Characters.QueryRowContext(r.Context(), query, character).Scan(&online); err != nil {
		problem(w, http.StatusConflict, "The designated diagnostic character was not found")
		return
	}
	if online {
		problem(w, http.StatusConflict, "The designated diagnostic character must be offline")
		return
	}
	var itemName string
	if err := s.s.World.QueryRowContext(r.Context(), "SELECT name FROM item_template WHERE entry=?", deliveryDiagnosticItemID).Scan(&itemName); err != nil {
		problem(w, http.StatusConflict, "The fixed diagnostic item is unavailable in this world database")
		return
	}
	command := fmt.Sprintf(`send items %s "Portal delivery diagnostic" "Correlation %s. Delete this mail after verification." %d:1`, character, requestID, deliveryDiagnosticItemID)
	result, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_command_log(actor_account_id,realm_key,command,response,success,ip_address) VALUES(?,?,?,'Pending',0,?)", actor.ID, s.c.RealmKey, "delivery diagnostic to "+character, s.clientIP(r))
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not create diagnostic audit record")
		return
	}
	auditID, _ := result.LastInsertId()
	response, commandErr := s.soapCommand(r.Context(), command)
	response = limitConsoleText(response)
	if commandErr != nil {
		response = limitConsoleText(commandErr.Error())
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_command_log SET response=?,success=? WHERE id=?", response, commandErr == nil, auditID)
	metadata, _ := json.Marshal(map[string]any{"character": character, "itemId": deliveryDiagnosticItemID, "correlationId": requestID, "commandLogId": auditID, "success": commandErr == nil})
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key,request_id,ip_address,user_agent,metadata_json) VALUES(?,'delivery.diagnostic',?,'Fixed low-value SOAP delivery probe',?,?,?,?,?)`, actor.ID, character, s.c.RealmKey, requestID, s.clientIP(r), truncate(r.UserAgent(), 500), metadata)
	if commandErr != nil {
		problem(w, http.StatusBadGateway, "Worldserver rejected the diagnostic; use the correlation ID in the audit log")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"ok": true, "character": character, "itemId": deliveryDiagnosticItemID, "itemName": itemName, "correlationId": requestID, "auditId": auditID, "response": response, "cleanup": cleanup})
}
