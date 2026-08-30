import {qs, setMessage} from "/js/ui.js";

const iso = (value) => value ? new Date(value).toISOString() : null;

export function createAdminSettings({api, mutationSuccess, toast}) {
	const form = qs("#gm-settings-form");
	let bound = false;

	function bind() {
		if (!form || bound) return;
		bound = true;
		form.onsubmit = async (event) => {
			event.preventDefault();
			const button = qs('button[type="submit"]', form),
				formData = new FormData(form),
				values = Object.fromEntries(formData);
			for (const key of ["maintenanceEnabled", "registration", "armory", "rankings", "guilds", "realm", "shop", "support", "admin", "gmConsole"]) values[key] = formData.has(key);
			for (const key of ["crossFactionAccounts", "crossFactionCalendar", "crossFactionChannels", "crossFactionGroups", "crossFactionGuilds", "crossFactionAuctions", "crossFactionMail", "crossFactionWho", "crossFactionFriends", "crossFactionTrade"]) values[key] = formData.has(key);
			values.crossFaction = values.crossFactionGroups;
			values.featuredNewsId = Number(values.featuredNewsId || 0);
			for (const key of ["startLevel", "maxLevel", "populationCap", "transferSlaHours"]) values[key] = Number(values[key] || 0);
			values.maintenanceStarts = iso(values.maintenanceStarts);
			values.maintenanceEnds = iso(values.maintenanceEnds);
			button.disabled = true;
			setMessage(form, "");
			try {
				const result = await api("/api/admin/settings", {method: "PUT", body: JSON.stringify(values)});
				mutationSuccess(form, "Website settings saved.", result);
				toast("Configuration updated");
			} catch (error) {
				setMessage(form, error.message);
			} finally {
				button.disabled = false;
			}
		};
	}

	async function load() {
		if (!form) return;
		bind();
		try {
			const {settings} = await api("/api/admin/settings");
			for (const [key, value] of Object.entries(settings || {})) {
				const input = form.elements[key];
				if (!input) continue;
				if (input.type === "checkbox") input.checked = Boolean(value);
				else if (input.type === "datetime-local") input.value = value ? new Date(value).toISOString().slice(0, 16) : "";
				else input.value = value ?? "";
			}
		} catch (error) {
			setMessage(form, error.message);
		}
	}

	return {load};
}
