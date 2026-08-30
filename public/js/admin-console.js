import {qs, setMessage} from "/js/ui.js";

export function mountAdminConsole({api}) {
	const panel = qs("#gm-command-console"),
		form = qs("#gm-command-form"),
		output = qs("#gm-command-output");
	if (!panel || !form || form.dataset.initialized === "true") return;
	form.dataset.initialized = "true";
	qs("#gm-command-disabled", panel)?.classList.add("hidden");
	qs("#gm-command-enabled", panel)?.classList.remove("hidden");

	const loadHistory = async () => {
		const data = await api("/api/admin/console"),
			box = qs("#gm-command-history");
		box.innerHTML = "";
		qs("#gm-command-policy").textContent = data.allowAll
			? "Unrestricted mode: every valid one-line command is permitted."
			: "Allowed command prefixes: " + data.allowedPrefixes.join(", ");
		if (!data.entries.length) {
			box.innerHTML = '<p class="muted">No commands have been run.</p>';
			return;
		}
		for (const entry of data.entries) {
			const row = document.createElement("div"),
				info = document.createElement("span"),
				command = document.createElement("code"),
				meta = document.createElement("small"),
				result = document.createElement("strong");
			row.className = "admin-row command-history";
			command.textContent = "> " + entry.command;
			meta.textContent = `${entry.actor} · ${new Date(entry.created).toLocaleString()}\n${entry.response}`;
			result.textContent = entry.success ? "OK" : "FAILED";
			result.className = entry.success ? "status-executed" : "status-failed";
			info.append(command, meta);
			row.append(info, result);
			box.append(row);
		}
	};

	loadHistory().catch((error) => { output.textContent = error.message; });
	form.onsubmit = async (event) => {
		event.preventDefault();
		const button = qs('button[type="submit"]', form),
			command = new FormData(form).get("command");
		button.disabled = true;
		setMessage(form, "");
		output.textContent = "Executing…";
		try {
			const result = await api("/api/admin/console", {method: "POST", body: JSON.stringify({command})});
			output.textContent = result.output;
			form.reset();
			await loadHistory();
		} catch (error) {
			output.textContent = error.message;
			setMessage(form, error.message);
		} finally {
			button.disabled = false;
		}
	};
}
