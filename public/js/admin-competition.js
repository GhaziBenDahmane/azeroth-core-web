import {esc, qs, setMessage} from "/js/ui.js";

export function mountAdminCompetition({api, askAction, adminCan, mutationSuccess}) {
	const host = qs('[data-admin-subview="operations"]');
	if (!host || qs("#arena-season-manager")) return;
	const panel = document.createElement("article");
	panel.id = "arena-season-manager";
	panel.className = "account-panel cms-panel";
	panel.innerHTML = '<div class="panel-title"><div><p class="eyebrow">COMPETITIVE HISTORY</p><h3>Arena season archives</h3></div><button id="arena-seasons-refresh" class="ghost-button" type="button">Refresh</button></div><p class="muted">Capture the current 2v2, 3v3, and 5v5 ladders before resetting arena teams. Snapshots are immutable and immediately available in Rankings.</p><form id="arena-season-form" class="gm-fields"><label>Season name<input name="name" maxlength="100" placeholder="Season 8" required></label><label>URL slug<input name="slug" maxlength="100" pattern="[a-z0-9]+(?:-[a-z0-9]+)*" placeholder="season-8" required></label><button class="button" type="submit">Capture ladders</button><p class="form-message" role="status"></p></form><div id="admin-arena-seasons" class="admin-table"><p class="muted">Loading…</p></div>';
	host.append(panel);
	const load = async () => {
		const box = qs("#admin-arena-seasons");
		try {
			const data = await api("/api/admin/arena-seasons"); box.innerHTML = "";
			for (const season of data.seasons || []) {
				const row = document.createElement("div"); row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(season.name)}</b><small>${esc(season.slug)} · ${esc(season.status)}</small></span>${season.slug === "current" ? '<strong>Live</strong>' : `<a class="ghost-button" href="/rankings?season=${encodeURIComponent(season.slug)}">View</a>`}`;
				box.append(row);
			}
		} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	};
	qs("#arena-seasons-refresh").onclick = load;
	qs("#arena-season-form").onsubmit = async (event) => {
		event.preventDefault(); const form = event.currentTarget;
		if (!(await askAction({title:"Capture arena season", message:"This stores an immutable copy of every current arena ladder. Capture it before the worldserver season reset.", input:false, confirmText:"Capture snapshot"}))) return;
		try { const result = await api("/api/admin/arena-seasons", {method:"POST", body:JSON.stringify(Object.fromEntries(new FormData(form)))}); setMessage(form, `${result.capturedTeams} teams captured.`, true); form.reset(); await load(); }
		catch (error) { setMessage(form, error.message); }
	};
	load();

	if (adminCan("moderation")) mountRankingExclusions({host, api});
	if (adminCan("realm")) mountRaidEligibility({host, api, mutationSuccess});
}

function mountRankingExclusions({host, api}) {
	const rules = document.createElement("article");
	rules.id = "ranking-eligibility-manager"; rules.className = "account-panel cms-panel";
	rules.innerHTML = '<div class="panel-title"><div><p class="eyebrow">ELIGIBILITY</p><h3>Ranking exclusions</h3></div><button id="ranking-rules-refresh" class="ghost-button" type="button">Refresh</button></div><p class="muted">Temporarily or permanently exclude staff characters, test teams, or sanctioned guilds from public ladders. Every change is audited.</p><form id="ranking-rule-form" class="gm-fields"><label>Ranking scope<select name="scope"><option value="character">Character rankings</option><option value="arena_team">Arena team</option><option value="guild">Guild rankings</option></select></label><label>Exact character, team, or guild name<input name="target" maxlength="100" required></label><label>Internal reason<input name="reason" minlength="3" maxlength="500" required></label><label>Ends at (optional)<input name="endsAt" type="datetime-local"></label><button class="button" type="submit">Exclude from rankings</button><p class="form-message" role="status"></p></form><div id="admin-ranking-rules" class="admin-table"><p class="muted">Loading…</p></div>';
	host.append(rules);
	const form = qs("#ranking-rule-form"), loadRules = async () => {
		const box = qs("#admin-ranking-rules");
		try {
			const data = await api("/api/admin/ranking-exclusions"); box.innerHTML = "";
			for (const item of data.exclusions || []) {
				const row = document.createElement("div"); row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(item.target)}</b><small>${esc(item.scope.replaceAll("_", " "))} · ${esc(item.reason)}${item.endsAt ? " · until " + new Date(item.endsAt).toLocaleString() : ""}</small></span><span class="row-actions"></span>`;
				if (item.active) { const remove = document.createElement("button"); remove.type = "button"; remove.className = "ghost-button"; remove.textContent = "Restore eligibility"; remove.onclick = async () => { await api(`/api/admin/ranking-exclusions/${item.id}`, {method:"DELETE", body:"{}"}); await loadRules(); }; qs(".row-actions", row).append(remove); }
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No ranking exclusions.</p>';
		} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	};
	form.onsubmit = async (event) => {
		event.preventDefault(); const values = Object.fromEntries(new FormData(form));
		if (values.endsAt) values.endsAt = new Date(values.endsAt).toISOString(); else delete values.endsAt;
		try { await api("/api/admin/ranking-exclusions", {method:"POST", body:JSON.stringify(values)}); setMessage(form, "Ranking exclusion saved.", true); form.reset(); await loadRules(); }
		catch (error) { setMessage(form, error.message); }
	};
	qs("#ranking-rules-refresh").onclick = loadRules; loadRules();
}

function mountRaidEligibility({host, api, mutationSuccess}) {
	const eligibility = document.createElement("article");
	eligibility.id = "raid-eligibility-manager"; eligibility.className = "account-panel cms-panel";
	eligibility.innerHTML = '<div class="panel-title"><div><p class="eyebrow">RAID INTEGRITY</p><h3>Verified speed-run rules</h3></div></div><p class="muted">Only signed raid events inside these bounds appear publicly. Events outside the rules are retained as ineligible evidence, never silently ranked.</p><form id="raid-eligibility-form"><div class="gm-fields"><label>10-player minimum<input name="minMembers10" type="number" min="1" max="10" required></label><label>10-player maximum<input name="maxMembers10" type="number" min="1" max="10" required></label><label>25-player minimum<input name="minMembers25" type="number" min="1" max="25" required></label><label>25-player maximum<input name="maxMembers25" type="number" min="1" max="25" required></label><label>Minimum duration (seconds)<input name="minDurationSeconds" type="number" min="10" max="86399" required></label><label>Maximum duration (seconds)<input name="maxDurationSeconds" type="number" min="11" max="86400" required></label><label>Maximum event age (hours)<input name="maxEventAgeHours" type="number" min="1" max="8760" required></label><label class="check"><input name="requireCharacterGuids" type="checkbox"> Require character GUIDs</label></div><button class="button" type="submit">Save eligibility rules</button><p class="form-message" role="status"></p></form>';
	host.append(eligibility);
	const form = qs("#raid-eligibility-form"), numberFields = ["minMembers10","maxMembers10","minMembers25","maxMembers25","minDurationSeconds","maxDurationSeconds","maxEventAgeHours"];
	const load = async () => { try { const {rules} = await api("/api/admin/raid-eligibility"); for (const key of numberFields) form.elements[key].value = rules[key]; form.elements.requireCharacterGuids.checked = Boolean(rules.requireCharacterGuids); } catch (error) { setMessage(form, error.message); } };
	form.onsubmit = async (event) => {
		event.preventDefault(); const values = Object.fromEntries(new FormData(form));
		for (const key of numberFields) values[key] = Number(values[key]); values.requireCharacterGuids = form.elements.requireCharacterGuids.checked;
		try { const result = await api("/api/admin/raid-eligibility", {method:"PUT", body:JSON.stringify(values)}); mutationSuccess(form, "Eligibility rules saved.", result); }
		catch (error) { setMessage(form, error.message); }
	};
	load();
}
