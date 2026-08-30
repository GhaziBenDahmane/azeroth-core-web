import {esc, qs, setMessage} from "/js/ui.js";

export function createAdminCommunity({api}) {
	const issueForm = qs("#admin-community-issue-form");
	const recruitmentForm = qs("#guild-recruitment-form");
	let initialized = false;

	async function load() {
		const status = qs("#admin-tracker-status")?.value || "";
		const [communityIssues, guildRecruitment] = await Promise.all([
			api("/api/admin/community/issues?status=" + encodeURIComponent(status)),
			api("/api/admin/guild-recruitment"),
		]);
		const recruitmentBox = qs("#admin-guild-recruitment");
		if (recruitmentBox) {
			recruitmentBox.innerHTML = "";
			for (const profile of guildRecruitment.profiles || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(profile.guildName || "Guild " + profile.guildId)}</b><small>${esc(profile.headline)} · ${profile.active ? "accepting applications" : "paused"}</small></span><span class="row-actions"></span>`;
				const edit = document.createElement("button");
				edit.className = "ghost-button";
				edit.type = "button";
				edit.textContent = "Edit";
				edit.onclick = () => {
					for (const [key, value] of Object.entries(profile)) {
						const input = recruitmentForm?.elements[key];
						if (!input) continue;
						if (input.type === "checkbox") input.checked = Boolean(value);
						else input.value = value ?? "";
					}
					recruitmentForm?.scrollIntoView({behavior: "smooth", block: "center"});
				};
				qs(".row-actions", row).append(edit);
				recruitmentBox.append(row);
			}
			if (!recruitmentBox.children.length) recruitmentBox.innerHTML = '<p class="muted">No guild recruitment profiles.</p>';
		}
		const trackerBox = qs("#admin-community-issues");
		if (trackerBox) {
			trackerBox.innerHTML = "";
			for (const issue of communityIssues.issues || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(issue.title)}</b><small>${esc(issue.kind)} · ${esc(issue.status.replaceAll("_", " "))} · ${issue.voteCount} votes · ${issue.commentCount} comments</small></span><span class="row-actions"></span>`;
				const open = document.createElement("a");
				open.className = "ghost-button";
				open.href = "/tracker/" + issue.id;
				open.textContent = "View";
				const triage = document.createElement("button");
				triage.className = "button small";
				triage.type = "button";
				triage.textContent = "Triage";
				triage.onclick = () => {
					issueForm.elements.id.value = issue.id;
					issueForm.elements.status.value = issue.status;
					issueForm.elements.priority.value = issue.priority;
					issueForm.elements.labels.value = (issue.labels || []).join(", ");
					issueForm.elements.staffResponse.value = issue.staffResponse || "";
					qs("#admin-community-issue-title").textContent = issue.title;
					issueForm.classList.remove("hidden");
					issueForm.scrollIntoView({behavior: "smooth", block: "center"});
				};
				qs(".row-actions", row).append(open, triage);
				trackerBox.append(row);
			}
			if (!trackerBox.children.length) trackerBox.innerHTML = '<p class="muted">No matching community submissions.</p>';
		}
	}

	if (!initialized) {
		initialized = true;
		qs("#admin-tracker-status").onchange = load;
		qs("#admin-tracker-refresh").onclick = load;
		qs("#admin-community-issue-close").onclick = () => issueForm.classList.add("hidden");
		issueForm.onsubmit = async (event) => {
			event.preventDefault();
			const values = Object.fromEntries(new FormData(issueForm)), id = values.id;
			delete values.id;
			try {
				await api("/api/admin/community/issues/" + id, {method: "PUT", body: JSON.stringify(values)});
				setMessage(issueForm, "Triage saved and published.", true);
				issueForm.classList.add("hidden");
				await load();
			} catch (error) { setMessage(issueForm, error.message); }
		};
		qs("#guild-recruitment-reset").onclick = () => recruitmentForm.reset();
		recruitmentForm.onsubmit = async (event) => {
			event.preventDefault();
			const data = new FormData(recruitmentForm), values = Object.fromEntries(data);
			values.guildId = Number(values.guildId);
			values.active = data.has("active");
			try {
				await api("/api/admin/guild-recruitment", {method: "POST", body: JSON.stringify(values)});
				setMessage(recruitmentForm, "Recruitment profile saved.", true);
				await load();
			} catch (error) { setMessage(recruitmentForm, error.message); }
		};
	}
	return {load};
}
