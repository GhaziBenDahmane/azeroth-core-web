import {esc, qs} from "/js/ui.js";

export function mountVote(context) {
	const { api, toast } = context;
	const sitesBox = qs("#vote-sites"), leaderboard = qs("#vote-leaderboard"), campaignsBox = qs("#vote-campaigns"), historyBox = qs("#vote-history");
	const remaining = (date) => {
		const seconds = Math.max(0, Math.ceil((new Date(date).getTime() - Date.now()) / 1000));
		if (!seconds) return "Ready now";
		const hours = Math.floor(seconds / 3600), minutes = Math.ceil((seconds % 3600) / 60);
		return `${hours ? hours + "h " : ""}${minutes}m remaining`;
	};
	async function loadVotes() {
		try {
			const data = await api("/api/votes");
			sitesBox.innerHTML = "";
			(data.sites || []).forEach((site) => {
				const card = document.createElement("article");
				card.className = "vote-site";
				card.innerHTML = `<div><h3>${esc(site.name)}</h3><p>${esc(site.description || `Vote every ${Math.round(site.cooldownMinutes / 60)} hours.`)}</p><small>${site.available ? "Available now" : site.availableAt ? remaining(site.availableAt) : "Sign in to vote"}</small></div><div class="vote-site-reward"><strong>+${site.rewardCredits} credits</strong><button class="button small" ${site.available ? "" : "disabled"}>Vote</button></div>`;
				qs("button", card).onclick = async () => {
					try {
						const result = await api(`/api/votes/${site.id}/visit`, { method: "POST", body: "{}" });
						location.href = result.url;
					} catch (error) { toast(error.message); }
				};
				sitesBox.append(card);
			});
			if (!sitesBox.children.length) sitesBox.innerHTML = '<p class="empty">No voting sites are configured for this realm.</p>';
		} catch (error) { sitesBox.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}
	api("/api/votes/leaderboard").then((data) => {
		leaderboard.innerHTML = "";
		(data.leaders || []).forEach((entry) => {
			const row = document.createElement("div");
			row.className = "compact-rank-row";
			row.innerHTML = `<strong>#${entry.rank}</strong><span><b>${esc(entry.username)}</b><small>${entry.votes} verified votes</small></span><b>${entry.credits} credits</b>`;
			leaderboard.append(row);
		});
		if (!leaderboard.children.length) leaderboard.innerHTML = '<p class="muted">No verified votes this month.</p>';
	}).catch((error) => leaderboard.innerHTML = `<p class="empty">${esc(error.message)}</p>`);
	api("/api/votes/campaigns").then(({ campaigns }) => {
		campaignsBox.innerHTML = "";
		(campaigns || []).forEach((campaign) => {
			const card = document.createElement("article");
			card.className = "campaign-card";
			const winners = (campaign.winners || []).map((winner) => `<li>#${winner.rank} ${esc(winner.username)} · ${winner.votes} votes</li>`).join("");
			const goal = campaign.targetEntries ? `<div class="community-goal"><div class="panel-title"><strong>Community goal</strong><span>${campaign.totalEntries.toLocaleString()} / ${campaign.targetEntries.toLocaleString()}</span></div><div class="meter" role="progressbar" aria-label="Community vote goal" aria-valuemin="0" aria-valuemax="${campaign.targetEntries}" aria-valuenow="${Math.min(campaign.totalEntries,campaign.targetEntries)}"><span style="width:${Math.min(100,Math.round(campaign.totalEntries/campaign.targetEntries*100))}%"></span></div><small>${campaign.goalReached ? "Unlocked" : "Unlock"}: ${esc(campaign.communityRewardDescription)}</small></div>` : "";
			card.innerHTML = `<p class="eyebrow">${esc(campaign.status)}</p><h3>${esc(campaign.name)}</h3><p>${esc(campaign.description)}</p><p><strong>${esc(campaign.prizeDescription)}</strong></p><div class="campaign-stats"><span>${campaign.totalEntries} entries</span><span>${campaign.participantCount} supporters</span><span>${campaign.viewerEntries} yours</span></div>${goal}<small>Ends ${new Date(campaign.endsAt).toLocaleString()} · minimum ${campaign.minimumVotes} votes</small><details><summary>Verify draw fairness</summary><p class="muted">Commitment published before the draw:</p><code title="${esc(campaign.commitment)}">${esc(campaign.commitment)}</code>${campaign.seed ? `<p class="muted">Revealed seed:</p><code title="${esc(campaign.seed)}">${esc(campaign.seed)}</code>` : '<p class="muted">The seed is revealed only after the campaign closes.</p>'}${winners ? `<ol>${winners}</ol>` : ""}</details>`;
			campaignsBox.append(card);
		});
		if (!campaignsBox.children.length) campaignsBox.innerHTML = '<p class="empty">No voting campaigns are scheduled.</p>';
	}).catch((error) => campaignsBox.innerHTML = `<p class="empty">${esc(error.message)}</p>`);
	api("/api/votes/history").then(({history, summary}) => {
		const streak = Number(summary?.currentStreakDays || 0);
		qs("#vote-history-summary").innerHTML = `<span>${Number(summary?.thisMonth || 0).toLocaleString()} this month</span><span>${Number(summary?.total || 0).toLocaleString()} lifetime</span><span>${streak} day${streak === 1 ? "" : "s"} current streak</span>`;
		historyBox.innerHTML = "";
		(history || []).forEach((entry) => {
			const row = document.createElement("div");
			row.className = "compact-rank-row";
			row.innerHTML = `<span><b>${esc(entry.siteName)}</b><small>${new Date(entry.votedAt).toLocaleString()}</small></span><strong>+${Number(entry.credits).toLocaleString()} credits</strong>`;
			historyBox.append(row);
		});
		if (!historyBox.children.length) historyBox.innerHTML = '<p class="muted">No verified votes yet.</p>';
	}).catch((error) => historyBox.innerHTML = `<p class="empty">${esc(error.message)}</p>`);
	loadVotes();

}
