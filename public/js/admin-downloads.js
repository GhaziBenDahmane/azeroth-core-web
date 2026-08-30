import {esc, qs, setMessage} from "/js/ui.js";

export function createAdminDownloads({api, askAction}) {
	const form = qs("#gm-download-form");
	const patchForm = qs("#launcher-patch-form");
	let bound = false;
	const mirrorsFromText = (value) => String(value || "").split("\n").map((line) => {
		const separator = line.indexOf("|");
		return separator < 0 ? null : {label: line.slice(0, separator).trim(), url: line.slice(separator + 1).trim()};
	}).filter((item) => item?.label && item?.url);

	function bind() {
		if (!form || bound) return;
		bound = true;
		form.onsubmit = async (event) => {
			event.preventDefault();
			const button = qs('button[type="submit"]', form),
				values = Object.fromEntries(new FormData(form));
			values.mirrors = mirrorsFromText(values.mirrors);
			values.sortOrder = Number(values.sortOrder || 0);
			values.active = true;
			button.disabled = true;
			setMessage(form, "");
			try {
				await api("/api/admin/downloads", {method: "POST", body: JSON.stringify(values)});
				form.reset();
				setMessage(form, "Download added.", true);
				await load();
			} catch (error) { setMessage(form, error.message); }
			finally { button.disabled = false; }
		};
		if (patchForm) patchForm.onsubmit = async (event) => {
			event.preventDefault();
			const button = qs('button[type="submit"]', patchForm), values = Object.fromEntries(new FormData(patchForm));
			values.mirrors = mirrorsFromText(values.mirrors);
			values.sortOrder = Number(values.sortOrder || 0);
			values.active = true;
			button.disabled = true;
			setMessage(patchForm, "");
			try {
				await api("/api/admin/launcher-patches", {method: "POST", body: JSON.stringify(values)});
				patchForm.reset();
				setMessage(patchForm, "Launcher patch published.", true);
				await loadPatches();
			} catch (error) { setMessage(patchForm, error.message); }
			finally { button.disabled = false; }
		};
	}

	async function loadPatches() {
		const box = qs("#admin-launcher-patches");
		if (!box) return;
		try {
			const {patches} = await api("/api/admin/launcher-patches");
			box.innerHTML = "";
			for (const patch of patches || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(patch.platform)} · ${esc(patch.fromVersion)} → ${esc(patch.toVersion)}</b><small>${esc(patch.fileSize || "Size not supplied")} · SHA-256 verified · ${(patch.mirrors || []).length + 1} source${(patch.mirrors || []).length ? "s" : ""} · ${patch.active ? "active" : "disabled"}</small></span><span class="row-actions"></span>`;
				if (patch.active) {
					const disable = document.createElement("button");
					disable.type = "button";
					disable.className = "ghost-button";
					disable.textContent = "Disable";
					disable.onclick = async () => {
						if (!(await askAction({title: "Disable launcher patch", message: `${patch.platform} ${patch.fromVersion} → ${patch.toVersion}`, input: false, confirmText: "Disable"}))) return;
						disable.disabled = true;
						try { await api(`/api/admin/launcher-patches/${patch.id}`, {method: "DELETE"}); await loadPatches(); }
						finally { disable.disabled = false; }
					};
					qs(".row-actions", row).append(disable);
				}
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No incremental launcher patches configured.</p>';
		} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}

	async function load() {
		if (!form) return;
		bind();
		const box = qs("#admin-downloads");
		try {
			const {downloads} = await api("/api/admin/downloads");
			box.innerHTML = "";
			for (const download of downloads || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				const trust = [download.sha256 ? "checksum" : "", download.signatureUrl ? "signature" : "", download.virusTotalUrl ? "VirusTotal" : "", download.changelogUrl ? "changelog" : "", (download.mirrors || []).length ? `${download.mirrors.length + 1} mirrors` : ""].filter(Boolean).join(" · ") || "no verification metadata";
				row.innerHTML = `<span><b>${esc(download.name)}</b><small>${esc(download.platform)} · ${esc(download.version || "No version")}${download.releasedAt ? ` · released ${esc(download.releasedAt)}` : ""} · ${download.active ? "active" : "disabled"}</small><small>${esc(trust)}</small></span><span class="row-actions"></span>`;
				if (download.active) {
					const disable = document.createElement("button");
					disable.type = "button";
					disable.className = "ghost-button";
					disable.textContent = "Disable";
					disable.onclick = async () => {
						if (!(await askAction({title: "Disable download", message: download.name, input: false, confirmText: "Disable"}))) return;
						disable.disabled = true;
						try { await api(`/api/admin/downloads/${download.id}`, {method: "DELETE"}); await load(); }
						finally { disable.disabled = false; }
					};
					qs(".row-actions", row).append(disable);
				}
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No client downloads configured.</p>';
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
		await loadPatches();
	}

	return {load};
}
