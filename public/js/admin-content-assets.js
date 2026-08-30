import {esc, qs, setMessage} from "/js/ui.js";

export function createAdminContentAssets({api, askAction, toast}) {
	let mediaPreviewURL = "";
	async function loadMedia() {
		const box = qs("#admin-media");
		if (!box) return;
		try {
			const data = await api("/api/admin/media");
			box.innerHTML = "";
			for (const asset of data.assets || []) {
				const card = document.createElement("figure"),
					image = document.createElement("img"),
					caption = document.createElement("figcaption");
				card.className = "media-card";
				image.src = asset.url;
				image.alt = asset.alt || "";
				image.loading = "lazy";
				caption.innerHTML = `<b>${esc(asset.name)}</b><small>${asset.width}×${asset.height} · ${esc(asset.mime)}${asset.uploader ? ` · ${esc(asset.uploader)}` : ""}</small><small>${esc(asset.alt || "No alternative text")}</small><span class="row-actions"></span>`;
				const actions = qs(".row-actions", caption),
					copy = document.createElement("button"),
					edit = document.createElement("button"),
					archive = document.createElement("button");
				copy.type = edit.type = archive.type = "button";
				copy.className = edit.className = "ghost-button";
				archive.className = "danger-button small";
				copy.textContent = "Copy URL";
				edit.textContent = "Edit alt text";
				archive.textContent = "Archive";
				copy.onclick = async () => { await navigator.clipboard.writeText(new URL(asset.url, location.origin).href); toast("Image URL copied"); };
				edit.onclick = async () => {
					const alt = await askAction({title: "Edit alternative text", message: asset.name, label: "Alternative text", defaultValue: asset.alt || "", confirmText: "Save"});
					if (alt === null) return;
					try { await api(`/api/admin/media/${asset.id}`, {method: "PATCH", body: JSON.stringify({alt})}); await loadMedia(); }
					catch (error) { toast(error.message); }
				};
				archive.onclick = async () => {
					const confirmed = await askAction({title: "Archive image", message: "Existing pages using this URL will stop displaying the image.", label: "Type ARCHIVE", expected: "ARCHIVE", confirmText: "Archive image"});
					if (confirmed !== "ARCHIVE") return;
					try { await api(`/api/admin/media/${asset.id}`, {method: "DELETE"}); await loadMedia(); }
					catch (error) { toast(error.message); }
				};
				actions.append(copy, edit, archive);
				card.append(image, caption);
				box.append(card);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No managed images yet.</p>';
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}

	async function loadNavigation() {
		const box = qs("#admin-navigation");
		if (!box) return;
		try {
			const data = await api("/api/admin/navigation");
			box.innerHTML = "";
			for (const item of data.items || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(item.label)}</b><small>${item.area === "primary" ? "Header" : "Footer"} · ${esc(item.url)} · order ${item.sortOrder}${item.active ? "" : " · archived"}</small></span><span class="row-actions"></span>`;
				const edit = document.createElement("button");
				edit.type = "button";
				edit.className = "ghost-button";
				edit.textContent = "Edit";
				edit.onclick = () => {
					const form = qs("#navigation-form");
					for (const [key, value] of Object.entries(item)) {
						const input = form.elements[key];
						if (!input) continue;
						if (input.type === "checkbox") input.checked = Boolean(value);
						else input.value = value ?? "";
					}
					form.scrollIntoView({behavior: "smooth", block: "center"});
				};
				qs(".row-actions", row).append(edit);
				if (item.active) {
					const archive = document.createElement("button");
					archive.type = "button";
					archive.className = "ghost-button";
					archive.textContent = "Archive";
					archive.onclick = async () => {
						if (!(await askAction({title: "Archive navigation link", message: item.label, input: false, confirmText: "Archive"}))) return;
						await api(`/api/admin/navigation/${item.id}`, {method: "DELETE"});
						loadNavigation();
					};
					qs(".row-actions", row).append(archive);
				}
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">Using the built-in header and footer links.</p>';
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}

	const mediaForm = qs("#media-upload-form"), mediaInput = mediaForm?.elements.image, mediaPreview = qs("#media-upload-preview");
	if (mediaInput && mediaForm.dataset.initialized !== "true") {
		mediaForm.dataset.initialized = "true";
		mediaInput.onchange = () => {
			if (mediaPreviewURL) URL.revokeObjectURL(mediaPreviewURL);
			const file = mediaInput.files?.[0];
			if (!file) { mediaPreview.classList.add("hidden"); return; }
			mediaPreviewURL = URL.createObjectURL(file);
			qs("img", mediaPreview).src = mediaPreviewURL;
			qs("span", mediaPreview).textContent = `${file.name} · ${(file.size / 1024 / 1024).toFixed(2)} MB`;
			mediaPreview.classList.remove("hidden");
		};
		mediaForm.onsubmit = async (event) => {
			event.preventDefault();
			const button = qs('button[type="submit"]', mediaForm);
			button.disabled = true;
			setMessage(mediaForm, "");
			try {
				await api("/api/admin/media", {method: "POST", body: new FormData(mediaForm)});
				mediaForm.reset();
				mediaPreview.classList.add("hidden");
				if (mediaPreviewURL) URL.revokeObjectURL(mediaPreviewURL);
				mediaPreviewURL = "";
				setMessage(mediaForm, "Image uploaded.", true);
				await loadMedia();
			} catch (error) { setMessage(mediaForm, error.message); }
			finally { button.disabled = false; }
		};
	}

	const navigationForm = qs("#navigation-form");
	if (navigationForm && navigationForm.dataset.initialized !== "true") {
		navigationForm.dataset.initialized = "true";
		navigationForm.onsubmit = async (event) => {
			event.preventDefault();
			const data = new FormData(navigationForm), values = Object.fromEntries(data), id = values.id;
			delete values.id;
			values.sortOrder = Number(values.sortOrder || 0);
			values.newTab = data.has("newTab");
			values.active = data.has("active");
			try {
				await api(id ? `/api/admin/navigation/${id}` : "/api/admin/navigation", {method: id ? "PUT" : "POST", body: JSON.stringify(values)});
				navigationForm.reset();
				navigationForm.elements.id.value = "";
				navigationForm.elements.active.checked = true;
				setMessage(navigationForm, id ? "Navigation link updated." : "Navigation link added.", true);
				await loadNavigation();
			} catch (error) { setMessage(navigationForm, error.message); }
		};
		qs("#navigation-reset")?.addEventListener("click", () => {
			navigationForm.reset();
			navigationForm.elements.id.value = "";
			navigationForm.elements.active.checked = true;
		});
	}

	return {loadMedia, loadNavigation};
}
