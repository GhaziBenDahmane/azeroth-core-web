import {esc, qs, setMessage} from "/js/ui.js";

const iso = (value) => value ? new Date(value).toISOString() : null;

export function createAdminEvents({api, askAction}) {
	const form = qs("#realm-event-form");
	const participantPanel = qs("#event-participant-panel"), rewardForm = qs("#event-reward-form");
	let participants = [];
	let bound = false;

	function reset() {
		form.reset();
		form.elements.id.value = "";
		form.elements.signupEnabled.checked = false;
		qs("#realm-event-editor-title").textContent = "Schedule event";
	}

	function bind() {
		if (!form || bound) return;
		bound = true;
		form.onsubmit = async (event) => {
			event.preventDefault();
			const button = qs('button[type="submit"]', form),
				values = Object.fromEntries(new FormData(form)),
				id = values.id;
			delete values.id;
			values.startsAt = iso(values.startsAt);
			values.endsAt = iso(values.endsAt);
			values.registrationDeadline = iso(values.registrationDeadline);
			values.maxParticipants = Number(values.maxParticipants || 0);
			values.rewardCredits = Number(values.rewardCredits || 0);
			values.signupEnabled = form.elements.signupEnabled.checked;
			button.disabled = true;
			setMessage(form, "");
			try {
				await api(id ? "/api/admin/events/" + id : "/api/admin/events", {method: id ? "PUT" : "POST", body: JSON.stringify(values)});
				reset();
				setMessage(form, id ? "Event updated." : "Event scheduled.", true);
				await load();
			} catch (error) { setMessage(form, error.message); }
			finally { button.disabled = false; }
		};
		qs("#realm-event-reset").onclick = reset;
		qs("#event-participant-close").onclick = () => participantPanel.classList.add("hidden");
		rewardForm.onsubmit = async (event) => {
			event.preventDefault();
			const accountIds = participants.filter((item) => item.status === "attended" && !item.rewarded).map((item) => item.accountId), button = qs('button[type="submit"]', rewardForm);
			if (!accountIds.length) { setMessage(rewardForm, "Mark at least one unrewarded participant as attended."); return; }
			button.disabled = true; setMessage(rewardForm, "");
			try {
				const data = await api(`/api/admin/events/${rewardForm.elements.eventId.value}/rewards`, {method:"POST", body:JSON.stringify({accountIds, reason:rewardForm.elements.reason.value})});
				const results = data.results || [], box = qs("#event-reward-results");
				box.classList.remove("hidden"); box.innerHTML = results.map((item) => `<p><b>Account ${item.accountId}</b> · ${esc(item.status)} · ${esc(item.message)}</p>`).join("");
				await loadParticipants(Number(rewardForm.elements.eventId.value), qs("#event-participant-title").textContent.replace(/^Attendance · /,""));
			} catch (error) { setMessage(rewardForm, error.message); }
			finally { button.disabled = false; }
		};
	}

	async function loadParticipants(eventId, title) {
		const box = qs("#event-participants");
		participantPanel.classList.remove("hidden");
		qs("#event-participant-title").textContent = "Attendance · " + title;
		rewardForm.elements.eventId.value = eventId;
		box.innerHTML = '<p class="muted">Loading participants…</p>';
		try {
			({participants=[]} = await api(`/api/admin/events/${eventId}/participants`));
			box.innerHTML = "";
			for (const item of participants) {
				const row = document.createElement("div"); row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(item.characterName || `Character ${item.characterGuid}`)}</b><small>${esc(item.username || `Account ${item.accountId}`)} · ${new Date(item.registeredAt).toLocaleString()}${item.rewarded?" · reward granted":""}</small></span><label><span class="sr-only">Attendance status</span><select><option value="registered">Registered</option><option value="attended">Attended</option><option value="no_show">No show</option><option value="cancelled">Cancelled</option></select></label>`;
				const select = qs("select", row); select.value = item.status; select.onchange = async () => { select.disabled=true; try { await api(`/api/admin/events/${eventId}/participants/${item.accountId}`, {method:"PUT", body:JSON.stringify({status:select.value})}); item.status=select.value; } catch(error) { select.value=item.status; setMessage(rewardForm,error.message); } finally { select.disabled=false; } };
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">Nobody has registered for this event.</p>';
		} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}

	async function load() {
		if (!form) return;
		bind();
		const box = qs("#admin-events");
		try {
			const {events} = await api("/api/admin/events");
			box.innerHTML = "";
			for (const event of events || []) {
				const row = document.createElement("div"),
					edit = document.createElement("button"),
					participantsButton = document.createElement("button"),
					cancel = document.createElement("button");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(event.title)}</b><small>${new Date(event.startsAt).toLocaleString()} · ${esc(event.location || "Online")} · ${esc(event.status)}</small></span><span class="row-actions"></span>`;
				edit.type = participantsButton.type = cancel.type = "button";
				edit.className = participantsButton.className = cancel.className = "ghost-button";
				edit.textContent = "Edit";
				participantsButton.textContent = `${event.registeredCount || 0} participants`;
				cancel.textContent = "Cancel";
				edit.onclick = () => {
					for (const [key, value] of Object.entries(event)) {
						const input = form.elements[key];
						if (!input) continue;
						if (input.type === "checkbox") input.checked = Boolean(value);
						else input.value = input.type === "datetime-local" && value ? new Date(value).toISOString().slice(0, 16) : value ?? "";
					}
					form.elements.id.value = event.id;
					qs("#realm-event-editor-title").textContent = "Edit " + event.title;
					form.scrollIntoView({behavior: "smooth"});
				};
				participantsButton.onclick = () => loadParticipants(event.id, event.title);
				cancel.onclick = async () => {
					if (!(await askAction({title: "Cancel event", message: `${event.title} will no longer appear as scheduled.`, input: false, confirmText: "Cancel event"}))) return;
					cancel.disabled = true;
					try { await api("/api/admin/events/" + event.id, {method: "DELETE"}); await load(); }
					finally { cancel.disabled = false; }
				};
				qs(".row-actions", row).append(participantsButton, edit, cancel);
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No events scheduled.</p>';
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}

	return {load};
}
