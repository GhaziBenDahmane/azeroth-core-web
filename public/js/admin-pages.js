import {esc, qs, setMessage} from "/js/ui.js";

export function createAdminPages({api, askAction}) {
	const form = qs("#content-page-form");
	let bound = false;

	function reset() {
		form.reset();
		form.elements.id.value = "";
		qs("#content-page-editor-title").textContent = "New information page";
	}

	function bind() {
		if (!form || bound) return;
		bound = true;
		form.onsubmit = async (event) => {
			event.preventDefault();
			const button = qs('button[type="submit"]', form),
				data = new FormData(form),
				values = Object.fromEntries(data),
				id = values.id;
			delete values.id;
			values.showNavigation = data.has("showNavigation");
			values.showFooter = data.has("showFooter");
			values.sortOrder = Number(values.sortOrder || 0);
			button.disabled = true;
			setMessage(form, "");
			try {
				await api(id ? "/api/admin/pages/" + id : "/api/admin/pages", {method: id ? "PUT" : "POST", body: JSON.stringify(values)});
				reset();
				setMessage(form, id ? "Page updated." : "Page created.", true);
				await load();
			} catch (error) { setMessage(form, error.message); }
			finally { button.disabled = false; }
		};
		qs("#content-page-reset").onclick = reset;
	}

	async function load() {
		if (!form) return;
		bind();
		const box = qs("#admin-pages");
		try {
			const {pages} = await api("/api/admin/pages");
			box.innerHTML = "";
			for (const page of pages || []) {
				const row = document.createElement("div"),
					edit = document.createElement("button"),
					archive = document.createElement("button");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(page.title)}</b><small>/pages/${esc(page.slug)} · ${esc(page.status)}</small></span><span class="row-actions"></span>`;
				edit.type = archive.type = "button";
				edit.className = archive.className = "ghost-button";
				edit.textContent = "Edit";
				archive.textContent = "Archive";
				edit.onclick = () => {
					for (const [key, value] of Object.entries(page)) {
						const input = form.elements[key];
						if (!input) continue;
						if (input.type === "checkbox") input.checked = Boolean(value);
						else input.value = value ?? "";
					}
					form.elements.id.value = page.id;
					qs("#content-page-editor-title").textContent = "Edit " + page.title;
					form.scrollIntoView({behavior: "smooth"});
				};
				archive.onclick = async () => {
					if (await askAction({title: "Archive page", message: page.title, label: "Type ARCHIVE", expected: "ARCHIVE", confirmText: "Archive"}) !== "ARCHIVE") return;
					await api("/api/admin/pages/" + page.id, {method: "DELETE"});
					await load();
				};
				qs(".row-actions", row).append(edit, archive);
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No custom pages.</p>';
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}

	return {load};
}
