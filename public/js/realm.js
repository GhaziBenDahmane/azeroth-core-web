import { esc, qs } from "/js/ui.js";

export async function mountRealm({ api, publicConfigPromise }) {
	const box = qs("#realm-overview");
	try {
		const [realm, config] = await Promise.all([api("/api/realm"), publicConfigPromise]);
		const hours = Math.floor(realm.uptime / 3600), total = Math.max(1, realm.allianceOnline + realm.hordeOnline), alliance = Math.round((realm.allianceOnline / total) * 100), profile = config.realmProfile || {}, rates = profile.rates || {};
		box.innerHTML = `<div class="realm-title"><i></i><div><p class="eyebrow">${esc(profile.type || "PvE")} REALM</p><h2>${esc(realm.name)}</h2><code>set realmlist ${esc(realm.address)}</code>${profile.description ? `<p>${esc(profile.description)}</p>` : ""}</div><strong>${Number(realm.online).toLocaleString()}<small> players</small></strong></div><div class="realm-stats"><article><span>Total characters</span><b>${Number(realm.characters).toLocaleString()}</b></article><article><span>Current uptime</span><b>${hours.toLocaleString()}h</b></article><article><span>Online record</span><b>${Number(realm.recordOnline).toLocaleString()}</b></article><article><span>Level range</span><b>${profile.startLevel || 1}–${profile.maxLevel || 80}</b></article></div><div class="realm-rate-grid"><article><span>Quest XP</span><b>${esc(rates.questXp || config.experienceRate || "1×")}</b></article><article><span>Kill XP</span><b>${esc(rates.killXp || config.experienceRate || "1×")}</b></article><article><span>Exploration XP</span><b>${esc(rates.explorationXp || config.experienceRate || "1×")}</b></article><article><span>Drops</span><b>${esc(rates.drop || "1×")}</b></article><article><span>Reputation</span><b>${esc(rates.reputation || "1×")}</b></article><article><span>Honor</span><b>${esc(rates.honor || "1×")}</b></article><article><span>Professions</span><b>${esc(rates.profession || "1×")}</b></article><article><span>Faction policy</span><b>${esc(profile.factionPolicy || "both")}${profile.crossFaction ? " · Cross-faction" : ""}</b></article></div>${profile.season ? `<div class="realm-season"><span>Current season / phase</span><strong>${esc(profile.season)}</strong><small>${esc(profile.timezone || "UTC")}</small></div>` : ""}<div class="faction-balance"><div><span>Alliance · ${realm.allianceOnline}</span><span>Horde · ${realm.hordeOnline}</span></div><i style="--alliance:${alliance}%"></i></div>`;
		const enabledCrossFaction = Object.entries(profile.crossFactionFeatures || {}).filter(([, enabled]) => enabled).map(([feature]) => feature);
		if (enabledCrossFaction.length) {
			const capabilities = document.createElement("section");
			capabilities.className = "realm-capabilities";
			capabilities.innerHTML = `<span>Cross-faction enabled</span><div>${enabledCrossFaction.map((feature) => `<b>${esc(feature)}</b>`).join("")}</div>`;
			box.append(capabilities);
		}
	} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
}
