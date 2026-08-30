import {esc, qs, setMessage} from "/js/ui.js";

const iso = (value) => value ? new Date(value).toISOString() : null;

export function createAdminNews({api, askAction}) {
	const form = qs("#gm-news-form");
	let bound = false;

	function bind() {
		if (!form || bound) return;
		bound = true;
		form.onsubmit = async (event) => {
			event.preventDefault();
			const button = qs('button[type="submit"]', form),
				values = Object.fromEntries(new FormData(form)),
				id = values.id;
			delete values.id;
			values.active = values.status === "published";
			values.publishAt = iso(values.publishAt);
			values.expiresAt = iso(values.expiresAt);
			button.disabled = true;
			setMessage(form, "");
			try {
				await api(id ? "/api/admin/news/" + id : "/api/admin/news", {method: id ? "PUT" : "POST", body: JSON.stringify(values)});
				form.reset();
				form.elements.id.value = "";
				qs("#news-editor-title").textContent = "New article";
				setMessage(form, id ? "Article updated." : "Article created.", true);
				await load();
			} catch (error) { setMessage(form, error.message); }
			finally { button.disabled = false; }
		};
		qs("#news-editor-reset").onclick = () => {
			form.reset();
			form.elements.id.value = "";
			qs("#news-editor-title").textContent = "New article";
			qs("#news-preview-panel").classList.add("hidden");
		};
		qs("#news-preview").onclick = () => {
			const values = Object.fromEntries(new FormData(form)),
				panel = qs("#news-preview-panel");
			panel.innerHTML = "";
			if (values.coverUrl) {
				const image = document.createElement("img");
				image.src = values.coverUrl;
				image.alt = "";
				panel.append(image);
			}
			const meta = document.createElement("p"),
				title = document.createElement("h2"),
				summary = document.createElement("p"),
				body = document.createElement("div");
			meta.className = "eyebrow";
			meta.textContent = [values.kind, values.authorName].filter(Boolean).join(" · ");
			title.textContent = values.title || "Untitled article";
			summary.className = "article-lead";
			summary.textContent = values.summary || "";
			body.className = "article-body";
			body.textContent = values.body || "No article body yet.";
			panel.append(meta, title, summary, body);
			panel.classList.remove("hidden");
		};
	}

	async function load() {
		if (!form) return;
		bind();
		const box = qs("#admin-news");
		try {
			const {news} = await api("/api/admin/news");
			box.innerHTML = "";
			for (const article of news || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(article.title)}</b><small>${esc(article.kind)} · ${esc(article.status || (article.active ? "published" : "archived"))}${article.slug ? ` · /news/${esc(article.slug)}` : ""}</small></span><span class="row-actions"></span>`;
				const edit = document.createElement("button"),
					revisions = document.createElement("button"),
					archive = document.createElement("button");
				for (const button of [edit, revisions, archive]) { button.type = "button"; button.className = "ghost-button"; }
				edit.textContent = "Edit";
				revisions.textContent = "History";
				archive.textContent = "Archive";
				edit.onclick = () => {
					for (const [key, value] of Object.entries(article)) {
						const input = form.elements[key];
						if (!input) continue;
						input.value = input.type === "datetime-local" && value ? new Date(value).toISOString().slice(0, 16) : value ?? "";
					}
					form.elements.id.value = article.id;
					qs("#news-editor-title").textContent = "Edit " + article.title;
					form.scrollIntoView({behavior: "smooth", block: "start"});
				};
				revisions.onclick = async () => {
					const data = await api(`/api/admin/news/${article.id}/revisions`),
						history = qs("#admin-news-revisions");
					history.classList.remove("hidden");
					history.innerHTML = `<h4>${esc(article.title)} revision history</h4>` + (data.revisions || []).map((revision) => `<div class="admin-row"><span><b>${esc(revision.snapshot.status)} · ${esc(revision.snapshot.title)}</b><small>${new Date(revision.createdAt).toLocaleString()} · editor account ${revision.editorId}</small></span></div>`).join("");
				};
				archive.onclick = async () => {
					if (await askAction({title: "Archive article", message: article.title, label: "Type ARCHIVE", expected: "ARCHIVE", confirmText: "Archive article"}) !== "ARCHIVE") return;
					await api("/api/admin/news/" + article.id, {method: "DELETE"});
					await load();
				};
				qs(".row-actions", row).append(edit, revisions, archive);
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No articles yet. Draft the realm’s first update above.</p>';
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}

	return {load};
}
