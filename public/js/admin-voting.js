import { esc, qs, setMessage } from "/js/ui.js";

export function mountAdminVoting({ api, askAction, toast }) {
	const box = qs("#admin-vote-campaigns"), form = qs("#vote-campaign-form");
	if (!box || !form) return;
	if (form.dataset.initialized === "true") return;
	form.dataset.initialized = "true";
	const missionForm = qs("#mission-form"), missionBox = qs("#admin-missions");
	const siteForm = qs("#gm-vote-site-form"), siteBox = qs("#admin-vote-sites");
	const missionCategories = {raid_kills:"pve", achievements:"pve", honorable_kills:"pvp", verified_votes:"community"};
	const resetMissionForm = () => {
		if (!missionForm) return;
		missionForm.reset();
		missionForm.elements.id.value = "";
		missionForm.elements.active.checked = true;
		missionForm.elements.category.value = missionCategories[missionForm.elements.metric.value];
		qs("#mission-form-title").textContent = "Add player mission";
	};
	async function loadMissions() {
		if (!missionBox) return;
		try {
			const {missions} = await api("/api/admin/missions");
			missionBox.innerHTML = "";
			for (const mission of missions || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(mission.name)}</b><small>${esc(mission.category)} · ${esc(mission.metric.replaceAll("_", " "))} · target ${mission.target} · ${mission.rewardCredits} credits${mission.active ? "" : " · disabled"}</small></span><span class="row-actions"><button class="ghost-button small" data-edit type="button">Edit</button><button class="danger-button small" data-disable type="button" ${mission.active ? "" : "disabled"}>Disable</button></span>`;
				qs("[data-edit]", row).onclick = () => {
					for (const key of ["id","slug","name","description","category","metric","target","rewardCredits","sortOrder"]) if (missionForm.elements[key]) missionForm.elements[key].value = mission[key] ?? "";
					missionForm.elements.active.checked = Boolean(mission.active);
					qs("#mission-form-title").textContent = `Edit ${mission.name}`;
					missionForm.scrollIntoView({behavior:"smooth", block:"center"});
				};
				qs("[data-disable]", row).onclick = async () => {
					const confirmed = await askAction({title:"Disable monthly mission", message:`${mission.name} will disappear from player dashboards. Existing claims remain in the ledger.`, input:false, confirmText:"Disable mission"});
					if (!confirmed) return;
					try { await api(`/api/admin/missions/${mission.id}`, {method:"DELETE"}); toast("Mission disabled"); await loadMissions(); } catch (error) { toast(error.message); }
				};
				missionBox.append(row);
			}
			if (!missionBox.children.length) missionBox.innerHTML = '<p class="muted">No monthly missions configured.</p>';
		} catch (error) { missionBox.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}
	async function loadVoteCampaigns() {
		try {
			const { campaigns } = await api("/api/admin/vote-campaigns");
			box.innerHTML = "";
			for (const campaign of campaigns || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(campaign.name)}</b><small>${esc(campaign.status)} · ${campaign.totalEntries} entries${campaign.targetEntries ? ` / ${campaign.targetEntries} goal` : ""} · ends ${new Date(campaign.endsAt).toLocaleString()}</small></span><span class="row-actions"><button class="ghost-button small" type="button" ${campaign.status === "drawn" || new Date(campaign.endsAt) > new Date() ? "disabled" : ""}>Draw</button></span>`;
				qs("button", row).onclick = async () => {
					const confirmed = await askAction({ title:"Draw campaign winners", message:"The published commitment will be revealed and winners become permanent.", label:"Type DRAW", expected:"DRAW", confirmText:"Draw winners" });
					if (confirmed !== "DRAW") return;
					try {
						await api(`/api/admin/vote-campaigns/${campaign.id}/draw`, { method:"POST", body:"{}" });
						toast("Campaign winners selected");
						await loadVoteCampaigns();
					} catch (error) { toast(error.message); }
				};
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No voting campaigns configured.</p>';
		} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}
	async function loadVoteSites() {
		if (!siteBox) return;
		try {
			const {sites} = await api("/api/admin/vote-sites");
			siteBox.innerHTML = "";
			for (const site of sites || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(site.name)}</b><small>${site.rewardCredits} credits · ${site.cooldownMinutes} minute cooldown · ${site.active ? "active" : "disabled"}</small></span><span class="row-actions"></span>`;
				if (site.active) {
					const disable = document.createElement("button");
					disable.type = "button";
					disable.className = "ghost-button";
					disable.textContent = "Disable";
					disable.onclick = async () => {
						if (!(await askAction({title: "Disable voting site", message: site.name, input: false, confirmText: "Disable"}))) return;
						disable.disabled = true;
						try { await api(`/api/admin/vote-sites/${site.id}`, {method: "DELETE"}); await loadVoteSites(); }
						catch (error) { toast(error.message); }
						finally { disable.disabled = false; }
					};
					qs(".row-actions", row).append(disable);
				}
				siteBox.append(row);
			}
			if (!siteBox.children.length) siteBox.innerHTML = '<p class="muted">No voting sites configured.</p>';
		} catch (error) { siteBox.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}
	form.onsubmit = async (event) => {
		event.preventDefault();
		const values = Object.fromEntries(new FormData(form));
		values.startsAt = new Date(values.startsAt).toISOString();
		values.endsAt = new Date(values.endsAt).toISOString();
		values.minimumVotes = Number(values.minimumVotes);
		values.winnerCount = Number(values.winnerCount);
		values.targetEntries = Number(values.targetEntries || 0);
		try {
			await api("/api/admin/vote-campaigns", { method:"POST", body:JSON.stringify(values) });
			setMessage(form, "Campaign published with a committed draw seed.", true);
			form.reset();
			await loadVoteCampaigns();
		} catch (error) { setMessage(form, error.message); }
	};
	if (missionForm) {
		missionForm.elements.metric.onchange = () => { missionForm.elements.category.value = missionCategories[missionForm.elements.metric.value]; };
		qs("#mission-form-reset").onclick = resetMissionForm;
		missionForm.onsubmit = async (event) => {
			event.preventDefault();
			const values = Object.fromEntries(new FormData(missionForm));
			values.target = Number(values.target); values.rewardCredits = Number(values.rewardCredits); values.sortOrder = Number(values.sortOrder); values.active = missionForm.elements.active.checked;
			const id = values.id; delete values.id;
			try {
				await api(id ? `/api/admin/missions/${id}` : "/api/admin/missions", {method:id ? "PUT" : "POST", body:JSON.stringify(values)});
				setMessage(missionForm, id ? "Mission updated." : "Mission created.", true);
				resetMissionForm(); await loadMissions();
			} catch (error) { setMessage(missionForm, error.message); }
		};
		loadMissions();
	}
	if (siteForm) siteForm.onsubmit = async (event) => {
		event.preventDefault();
		const button = qs('button[type="submit"]', siteForm),
			values = Object.fromEntries(new FormData(siteForm));
		values.rewardCredits = Number(values.rewardCredits);
		values.cooldownMinutes = Number(values.cooldownMinutes);
		values.sortOrder = Number(values.sortOrder);
		values.active = true;
		button.disabled = true;
		setMessage(siteForm, "");
		try {
			await api("/api/admin/vote-sites", {method: "POST", body: JSON.stringify(values)});
			siteForm.reset();
			setMessage(siteForm, "Voting site added.", true);
			await loadVoteSites();
		} catch (error) { setMessage(siteForm, error.message); }
		finally { button.disabled = false; }
	};
	loadVoteCampaigns();
	loadVoteSites();
}
