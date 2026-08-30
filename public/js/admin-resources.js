import {esc, qs, qsa, setMessage} from "/js/ui.js";

export function mountAdminResources({api, askAction, mutationSuccess}) {
	const host = qs('[data-admin-view="content"]');
	if (!host || qs("#resource-manager")) return;
	const panel = document.createElement("article");
	panel.id = "resource-manager";
	panel.className = "account-panel cms-panel";
	panel.innerHTML = '<form id="resource-editor"><input name="id" type="hidden"><div class="panel-title"><div><p class="eyebrow">TOOLS LIBRARY</p><h3 id="resource-editor-title">New addon or WeakAura</h3></div><button id="resource-editor-reset" class="ghost-button" type="button">New</button></div><div class="gm-fields"><label>Type<select name="kind"><option value="addon">Addon</option><option value="weakaura">WeakAura</option></select></label><label>Status<select name="status"><option value="draft">Draft</option><option value="published">Published</option><option value="archived">Archived</option></select></label></div><div class="gm-fields"><label>Title<input name="title" maxlength="160" required></label><label>Slug<input name="slug" maxlength="160" pattern="[a-z0-9]+(?:-[a-z0-9]+)*" required></label></div><label>Summary<textarea name="summary" maxlength="1000" rows="2"></textarea></label><label>Installation notes<textarea name="body" maxlength="100000" rows="5" required></textarea></label><div class="gm-fields"><label>Version<input name="version" maxlength="40"></label><label>Tags<input name="tags" maxlength="500" placeholder="raids, healing"></label></div><label>Verified HTTPS source<input name="downloadUrl" type="url"></label><label>Image URL<input name="imageUrl" type="url"></label><label>Display order<input name="sortOrder" type="number" value="0"></label><button class="button" type="submit">Save resource</button><p class="form-message" role="status"></p></form><div id="admin-resources" class="admin-table"><p class="muted">Loading…</p></div>';
	host.append(panel);
	const form = qs("#resource-editor"), box = qs("#admin-resources");
	const reset = () => { form.reset(); form.elements.id.value = ""; form.elements.status.value = "draft"; qs("#resource-editor-title").textContent = "New addon or WeakAura"; setMessage(form, ""); };
	const load = async () => {
		try {
			const data = await api("/api/admin/resources");
			box.innerHTML = "";
			for (const item of data.resources || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(item.title)}</b><small>${esc(item.kind)} · ${esc(item.status)} · ${esc(item.version || "No version")}</small></span><span class="row-actions"></span>`;
				const edit = document.createElement("button"), remove = document.createElement("button");
				edit.className = "ghost-button"; edit.type = "button"; edit.textContent = "Edit";
				edit.onclick = () => { for (const field of ["id","kind","status","title","slug","summary","body","version","tags","downloadUrl","imageUrl","sortOrder"]) if (form.elements[field]) form.elements[field].value = item[field] ?? ""; qs("#resource-editor-title").textContent = "Edit " + item.title; form.scrollIntoView({behavior:"smooth"}); };
				remove.className = "ghost-button danger"; remove.type = "button"; remove.textContent = "Delete";
				remove.onclick = async () => { if (!(await askAction({title:"Delete resource", message:item.title, input:false, confirmText:"Delete"}))) return; await api("/api/admin/resources/" + item.id, {method:"DELETE"}); await load(); };
				qs(".row-actions", row).append(edit, remove); box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No resources yet.</p>';
		} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	};
	form.onsubmit = async (event) => {
		event.preventDefault();
		const values = Object.fromEntries(new FormData(form)), id = values.id;
		delete values.id; values.sortOrder = Number(values.sortOrder) || 0;
		try { const result = await api(id ? "/api/admin/resources/" + id : "/api/admin/resources", {method:id ? "PUT" : "POST", body:JSON.stringify(values)}); reset(); mutationSuccess(form, "Resource saved.", result); await load(); }
		catch (error) { setMessage(form, error.message); }
	};
	qs("#resource-editor-reset").onclick = reset;
	load();
}
