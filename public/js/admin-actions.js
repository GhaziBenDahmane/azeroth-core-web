import {qs, qsa, setMessage} from "/js/ui.js";

async function submitAction({api, askAction, mutationSuccess}, form) {
	const button = qs('button[type="submit"]', form);
	button.disabled = true;
	setMessage(form, "");
	try {
		const values = Object.fromEntries(new FormData(form));
		if (["ban", "kick", "ip_ban", "shutdown", "restart"].includes(values.action)) {
			const expected = String(values.action).toUpperCase();
			const confirmed = await askAction({title: "Confirm realm operation", message: `${values.action} ${values.target || "the realm"}`, label: `Type ${expected}`, expected, confirmText: "Execute operation"});
			if (confirmed !== expected) return;
		}
		if ("level" in values) values.level = Number(values.level);
		if ("realmId" in values) values.realmId = Number(values.realmId);
		const result = await api("/api/admin/moderation", {method: "POST", body: JSON.stringify(values)});
		mutationSuccess(form, `${result.action} applied to ${result.target}.`, result);
	} catch (error) { setMessage(form, error.message); }
	finally { button.disabled = false; }
}

export function mountModerationAction(context) {
	const form = qs("#gm-moderation-form");
	if (!form) return;
	const sync = () => {
		const ban = form.elements.action.value === "ban", field = qs('[data-moderation-field="duration"]', form);
		field.classList.toggle("hidden", !ban);
		form.elements.duration.required = ban;
	};
	if (form.dataset.initialized !== "true") {
		form.dataset.initialized = "true";
		form.onsubmit = (event) => { event.preventDefault(); submitAction(context, form); };
		form.elements.action.onchange = sync;
	}
	const params = new URLSearchParams(location.search), action = params.get("action"), target = params.get("target");
	if (["ban", "unban", "kick"].includes(action)) form.elements.action.value = action;
	if (target) form.elements.target.value = target;
	sync();
}

export function mountRealmOperationAction(context) {
	const form = qs("#gm-operation-form");
	if (!form) return;
	const sync = () => {
		const action = form.elements.action.value;
		const config = {
			start: ["Start realm", "Starts the realm through the configured start webhook."],
			mute: ["Mute character", "Enter a character and a duration in minutes."],
			unmute: ["Unmute character", "Remove an active character mute."],
			ip_ban: ["Ban IP", "Duration examples: 30m, 7d, 1w, or -1."],
			ip_unban: ["Unban IP", "Remove an IP address ban."],
			gm_level: ["Set GM level", "Choose an account, security level, and realm scope."],
			announce: ["Send announcement", "Broadcast this message to online players."],
			motd: ["Set message of the day", "Replace the realm MOTD."],
			restart: ["Schedule restart", "Delay must be 10–3600 seconds."],
			shutdown: ["Schedule shutdown", "Delay must be 10–3600 seconds."],
			cancel_shutdown: ["Cancel shutdown or restart", "Cancels the pending worldserver timer."],
		}[action];
		const visible = {
			target: ["mute", "unmute", "ip_ban", "ip_unban", "gm_level"].includes(action),
			duration: ["mute", "ip_ban", "restart", "shutdown"].includes(action),
			gm: action === "gm_level",
			reason: true,
		};
		for (const [name, show] of Object.entries(visible)) {
			const field = qs(`[data-operation-field="${name}"]`, form);
			field.classList.toggle("hidden", !show);
			qsa("input,select,textarea", field).forEach((input) => { input.required = show && ["target", "duration", "reason"].includes(name); });
		}
		form.elements.target.placeholder = ["ip_ban", "ip_unban"].includes(action) ? "IPv4 or IPv6 address" : action === "gm_level" ? "Account name" : "Character name";
		form.elements.duration.placeholder = action === "mute" ? "Minutes (1–525600)" : action === "ip_ban" ? "30m, 7d, 1w, or -1" : "Seconds (10–3600)";
		form.elements.reason.placeholder = action === "announce" ? "Announcement text" : action === "motd" ? "New realm message of the day" : "Required audit reason";
		qs("#operation-title").textContent = config[0];
		qs("#operation-help").textContent = config[1];
	};
	if (form.dataset.initialized !== "true") {
		form.dataset.initialized = "true";
		form.onsubmit = (event) => { event.preventDefault(); submitAction(context, form); };
		form.elements.action.onchange = sync;
	}
	sync();
}
