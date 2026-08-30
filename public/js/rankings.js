import {esc, qs, qsa} from "/js/ui.js";

export function mountRankings(context) {
	const { api, classes, publicConfigPromise } = context;
	const arenaPolicy = publicConfigPromise.then(config => String(config.realmProfile?.arenaRewardPolicy || "").trim()).catch(() => "");
	const factionLabel = (value) => String(value || "").toLowerCase() === "horde" ? "Horde" : String(value || "").toLowerCase() === "alliance" ? "Alliance" : "";
	const seasonDate = (value) => value ? new Date(value).toLocaleDateString(undefined, {year:"numeric",month:"short",day:"numeric"}) : "";
	const rankingURL = new URL(location.href),
		validMetrics = new Set(["honorable-kills", "achievements", "exalted-reputations", "mounts", "companions", "played-time", "level", "guild-members"]);
	let activeMetric = validMetrics.has(rankingURL.searchParams.get("metric")) ? rankingURL.searchParams.get("metric") : "honorable-kills",
		activeBracket = [2, 3, 5].includes(Number(rankingURL.searchParams.get("bracket"))) ? Number(rankingURL.searchParams.get("bracket")) : 2,
		activeSeason = rankingURL.searchParams.get("season") || "current",
		arenaPage = Math.max(1, Number(rankingURL.searchParams.get("arenaPage")) || 1),
		rankPage = Math.max(1, Number(rankingURL.searchParams.get("rankPage")) || 1);
	function syncRankingURL() {
		const url = new URL(location.href);
		url.searchParams.set("bracket", activeBracket);
		url.searchParams.set("metric", activeMetric);
		if (activeSeason !== "current") url.searchParams.set("season", activeSeason); else url.searchParams.delete("season");
		if (arenaPage > 1) url.searchParams.set("arenaPage", arenaPage); else url.searchParams.delete("arenaPage");
		if (rankPage > 1) url.searchParams.set("rankPage", rankPage); else url.searchParams.delete("rankPage");
		for (const [key, selector] of [["class", "#ranking-class"], ["faction", "#ranking-faction"], ["spec", "#ranking-spec"]]) {
			const value = qs(selector).value.trim();
			if (value && value !== "0") url.searchParams.set(key, value); else url.searchParams.delete(key);
		}
		history.replaceState(null, "", url.pathname + "?" + url.searchParams);
	}
	async function loadRankings(bracket) {
		activeBracket = bracket;
		syncRankingURL();
		const box = qs("#arena-ranking");
		box.innerHTML = '<div class="skeleton"></div>';
		try {
			const [data, rewardPolicy] = await Promise.all([api(`/api/arena?bracket=${bracket}&page=${arenaPage}&season=${encodeURIComponent(activeSeason)}`), arenaPolicy]), teams = data.teams;
			const seasonSelect=qs("#ranking-season");
			if (seasonSelect.options.length <= 1 && data.seasons?.length) { seasonSelect.innerHTML=""; for(const season of data.seasons){const option=document.createElement("option");option.value=season.slug;option.textContent=season.name;seasonSelect.append(option)} seasonSelect.value=activeSeason; }
			qs("#season-note").textContent = data.source || "Arena ranking data";
			const selectedSeason=(data.seasons||[]).find(season=>season.slug===activeSeason),dates=selectedSeason ? [seasonDate(selectedSeason.startsAt),seasonDate(selectedSeason.endsAt)].filter(Boolean).join(" – ") : "";
			qs("#arena-season-context").innerHTML=`<div><b>${esc(data.seasonName||selectedSeason?.name||"Current season")}</b><span class="season-status">${esc(selectedSeason?.status||(activeSeason==="current"?"live":"archived"))}</span></div><div><span>${bracket}v${bracket} bracket</span><span>${teams.length} team${teams.length===1?"":"s"} on this page</span>${dates?`<span>${esc(dates)}</span>`:""}</div><small>${esc(data.source||"Arena ranking data")}</small><p class="arena-reward-policy"><b>Rewards and eligibility:</b> ${esc(rewardPolicy || "Realm staff have not published a reward or rating-cutoff policy for this season.")}</p>`;
			box.innerHTML =
				'<div class="rank-row rank-head"><span>Rank</span><span>Team</span><span>Rating</span><span>Record</span></div>';
			teams.forEach((t) => {
				const row = document.createElement("article");
				row.className = "rank-row";
				const members = t.members
					.map(
						(m) =>
								`<a href="/armory/${encodeURIComponent(m.name)}"><span class="class-token class-${m.class}" aria-hidden="true">${esc((classes[m.class] || "H").slice(0,2))}</span><span><b>${esc(m.name)}</b><small>${classes[m.class] || "Hero"} · ${m.personalRating} personal rating</small></span></a>`,
					)
					.join("");
				const winRate=t.seasonGames?Math.round(t.seasonWins/t.seasonGames*100):0;
				row.innerHTML = `<strong class="rank-number">${t.rank}</strong><div><h3>${esc(t.name)}</h3><div class="team-members">${members}</div></div><strong class="team-rating"><small>Rating</small>${t.rating}</strong><span class="team-record"><b>${t.seasonWins}W · ${t.seasonGames - t.seasonWins}L</b><small>${winRate}% win rate</small><i style="--wins:${winRate}%"></i></span>`;
				box.append(row);
			});
			if (!teams.length)
				box.innerHTML = '<p class="empty">No teams in this bracket.</p>';
			qs("#arena-page").textContent = `Page ${arenaPage}`;
			qs("#arena-prev").disabled = arenaPage === 1;
			qs("#arena-next").disabled = !data.hasMore;
		} catch (e) {
			box.innerHTML = `<p class="empty">${esc(e.message)}</p>`;
		}
	}
	qsa("[data-bracket]").forEach(
		(b) =>
			(b.onclick = () => {
				arenaPage = 1;
				qsa("[data-bracket]").forEach((x) =>
					x.classList.toggle("active", x === b),
				);
				loadRankings(Number(b.dataset.bracket));
			}),
	);
	async function loadCharacterRankings(metric) {
		activeMetric = metric;
		syncRankingURL();
		const box = qs("#character-ranking"),
			guilds = metric === "guild-members",
			params = new URLSearchParams({ metric });
		if (!guilds) {
			params.set("class", qs("#ranking-class").value);
			params.set("faction", qs("#ranking-faction").value);
			params.set("spec", qs("#ranking-spec").value.trim());
		}
		box.innerHTML = '<div class="skeleton"></div>';
		try {
			params.set("page", rankPage);
			const data = await api("/api/rankings?" + params);
			box.innerHTML = `<div class="rank-row rank-head"><span>Rank</span><span>${guilds ? "Guild" : "Character"}</span><span>${guilds ? "Type" : "Level"}</span><span>${guilds ? "Members" : "Score"}</span></div>`;
			data.rows.forEach((x) => {
				const value =
					metric === "played-time"
						? Math.floor(x.value / 3600).toLocaleString() + "h"
						: Number(x.value).toLocaleString();
				const row = document.createElement("article");
				row.className = "rank-row";
				const faction=factionLabel(x.faction),className=classes[x.class]||"Hero";
				row.innerHTML = `<strong class="rank-number">${x.rank}</strong><div class="rank-identity">${guilds?"":`<span class="class-token class-${x.class}" aria-hidden="true">${esc(className.slice(0,2))}</span>`}<span><a href="${guilds ? "/guilds" : "/armory/" + encodeURIComponent(x.name)}"><h3>${esc(x.name)}</h3></a><small>${guilds ? "Guild" : `${esc(className)}${x.spec ? " · " + esc(x.spec) : ""}`}${faction?` · <i class="faction-mark faction-${faction.toLowerCase()}">${faction}</i>`:""}${x.online?' · <i class="online-label">Online</i>':""}</small></span></div><strong>${guilds ? "—" : x.level}</strong><span>${value}</span>`;
				box.append(row);
			});
			if (!data.rows.length)
				box.innerHTML =
					'<p class="empty">No ranking entries match these filters.</p>';
			qs("#ranking-page").textContent = `Page ${rankPage}`;
			qs("#ranking-prev").disabled = rankPage === 1;
			qs("#ranking-next").disabled = !data.hasMore;
		} catch (e) {
			box.innerHTML = `<p class="empty">${esc(e.message)}</p>`;
		}
	}
	qsa("[data-metric]").forEach(
		(b) =>
			(b.onclick = () => {
				rankPage = 1;
				qsa("[data-metric]").forEach((x) =>
					x.classList.toggle("active", x === b),
				);
				loadCharacterRankings(b.dataset.metric);
			}),
	);
	qs("#ranking-class").value = rankingURL.searchParams.get("class") || "0";
	qs("#ranking-faction").value = rankingURL.searchParams.get("faction") || "";
	qs("#ranking-spec").value = rankingURL.searchParams.get("spec") || "";
	qsa("[data-bracket]").forEach((x) => x.classList.toggle("active", Number(x.dataset.bracket) === activeBracket));
	qsa("[data-metric]").forEach((x) => x.classList.toggle("active", x.dataset.metric === activeMetric));
	api("/api/rankings/capabilities").then((data)=>{
		qsa("[data-metric]").forEach((button)=>button.classList.toggle("hidden",data.metrics?.[button.dataset.metric]===false));
		if(data.metrics?.[activeMetric]===false){activeMetric="honorable-kills";rankPage=1;qsa("[data-metric]").forEach((button)=>button.classList.toggle("active",button.dataset.metric===activeMetric));loadCharacterRankings(activeMetric)}
	}).catch(()=>{});
	loadRankings(activeBracket);
	loadCharacterRankings(activeMetric);
	qs("#arena-prev").onclick = () => { if (arenaPage > 1) { arenaPage--; loadRankings(activeBracket); } };
	qs("#arena-next").onclick = () => { arenaPage++; loadRankings(activeBracket); };
	qs("#ranking-prev").onclick = () => { if (rankPage > 1) { rankPage--; loadCharacterRankings(activeMetric); } };
	qs("#ranking-next").onclick = () => { rankPage++; loadCharacterRankings(activeMetric); };
	qs("#ranking-class").onchange = () => { rankPage = 1; loadCharacterRankings(activeMetric); };
	qs("#ranking-faction").onchange = () => { rankPage = 1; loadCharacterRankings(activeMetric); };
	let specTimer;
	qs("#ranking-spec").oninput = () => {
		clearTimeout(specTimer);
		specTimer = setTimeout(() => { rankPage = 1; loadCharacterRankings(activeMetric); }, 250);
	};
	qs("#ranking-season").onchange = (e) => {
		activeSeason = e.target.value || "current";
		arenaPage = 1;
		loadRankings(activeBracket);
	};
	api("/api/rankings/raids")
		.then((data) => {
			const speed = qs("#raid-speed"),
				recent = qs("#recent-kills"),
				attempts = qs("#raid-attempts");
			const rules=data.eligibility||{},source=qs("#raid-ranking-source");
			if(source)source.innerHTML=`<p><b>Verified source:</b> ${esc(data.source||"Authenticated competitive ingestion")}</p><small>10-player roster: ${rules.minMembers10||"—"}–${rules.maxMembers10||"—"} · 25-player roster: ${rules.minMembers25||"—"}–${rules.maxMembers25||"—"} · timed event: ${rules.minDurationSeconds||"—"}–${rules.maxDurationSeconds||"—"} seconds · maximum ingestion age: ${rules.maxEventAgeHours||"—"} hours${rules.requireCharacterGuids?" · character IDs required":""}</small>`;
			speed.innerHTML = "";
			(data.speed || []).forEach((x) => {
				const row = document.createElement("div");
				row.className = "compact-rank-row";
				row.innerHTML = `<strong>#${x.rank || "—"}</strong><span><b>${esc(x.guild)}</b><small>${esc(x.raid)} · ${esc(x.difficulty)} · ${x.verifiedMembers||"—"} verified players</small></span><time>${Math.floor(x.seconds / 60)}m ${x.seconds % 60}s</time>`;
				speed.append(row);
			});
			recent.innerHTML = "";
			(data.recent || []).forEach((x) => {
				const row = document.createElement("div");
				row.className = "compact-rank-row";
				row.innerHTML = `<span><b>${esc(x.guild)}</b><small>${esc(x.boss)} · ${esc(x.raid)} · ${esc(x.difficulty)} · ${x.verifiedMembers||"—"} verified players</small></span><time>${new Date(x.killedAt).toLocaleString()}</time>`;
				recent.append(row);
			});
			attempts.innerHTML = "";
			(data.attempts || []).forEach((x) => {
				const row = document.createElement("div");
				row.className = "compact-rank-row";
				const roles = Object.entries(x.roles || {}).map(([role, count]) => `${count} ${role}`).join(" · ");
				const composition = Object.entries(x.classes || {}).sort((a,b) => Number(a[0])-Number(b[0])).map(([classID, count]) => `${count} ${classes[classID] || "Hero"}`).join(" · ");
				const pull = x.attemptNumber ? `Pull ${x.attemptNumber}` : "Attempt";
				const outcome = x.result === "kill" ? "Kill" : `Wipe at ${Number(x.bossHealthPercent || 0).toFixed(1)}%`;
				row.innerHTML = `<strong class="status-${x.result === "kill" ? "executed" : "failed"}">${esc(outcome)}</strong><span><b>${esc(x.guild)} · ${esc(x.boss)}</b><small>${esc(x.raid)} · ${esc(x.difficulty)} · ${esc(pull)}${roles ? ` · ${esc(roles)}` : ` · ${x.verifiedMembers || "—"} players`} · ${x.source === "signed_ingest" ? "verified event" : esc(x.source || "unknown source")}</small>${composition ? `<small class="raid-composition"><b>Roster:</b> ${esc(composition)}</small>` : ""}</span><time>${x.seconds ? `${Math.floor(x.seconds / 60)}m ${x.seconds % 60}s · ` : ""}${new Date(x.occurredAt).toLocaleString()}</time>`;
				attempts.append(row);
			});
			if (!speed.children.length)
				speed.innerHTML = '<p class="empty">No timed clears recorded.</p>';
			if (!recent.children.length)
				recent.innerHTML = '<p class="empty">No recent kills recorded.</p>';
			if (!attempts.children.length)
				attempts.innerHTML = '<p class="empty">No progression attempts recorded.</p>';
		})
		.catch((e) => {
			qs("#raid-speed").innerHTML = qs("#recent-kills").innerHTML = qs("#raid-attempts").innerHTML =
				`<p class="empty">${esc(e.message)}</p>`;
		});

}
