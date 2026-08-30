import {esc, publicLink, qs} from "/js/ui.js";

export function mountHome(context) {
	const { api, toast, publicConfigPromise } = context;
	const parseLines = (value, columns) => String(value || "").split(/\r?\n/).map(line => line.split("|").map(part => part.trim())).filter(parts => parts[0] && parts.length >= columns);
	publicConfigPromise
		.then(async (cfg) => {
			const features = parseLines(cfg.homeFeatures, 2), progression = parseLines(cfg.homeProgression, 3);
			if (features.length) {
				const host = qs("#home-features");
				host.innerHTML = features.map(([title, description], index) => `<article><span>${String(index + 1).padStart(2, "0")}</span><h3>${esc(title)}</h3><p>${esc(description)}</p></article>`).join("");
				qs("#home-features-section").classList.remove("hidden");
			}
			if (progression.length) {
				const host = qs("#home-progression");
				host.innerHTML = progression.map(([title, status, description]) => `<li class="progression-${esc(status.toLowerCase().replace(/[^a-z0-9]+/g, "-"))}"><span></span><div><small>${esc(status)}</small><h3>${esc(title)}</h3><p>${esc(description)}</p></div></li>`).join("");
				qs("#home-progression-section").classList.remove("hidden");
			}
			const realms =
				Array.isArray(cfg.realms) && cfg.realms.length
					? cfg.realms
					: [
							{
								key: cfg.realmKey || "",
								name: cfg.realmName,
								address: cfg.realmAddress,
								experienceRate: cfg.experienceRate,
							},
						];
			const results = await Promise.all(
				realms.map(async (realm) => {
					const realmParam = realm.key
						? "?realm=" + encodeURIComponent(realm.key)
						: "";
					const [health, overview, realmConfig] = await Promise.allSettled([
						api("/api/status" + realmParam, { credentials: "omit" }),
						api("/api/realm" + realmParam, { credentials: "omit" }),
						api("/api/public-config" + realmParam, { credentials: "omit" }),
					]);
					return {
						realm,
						health: health.status === "fulfilled" ? health.value : null,
						overview: overview.status === "fulfilled" ? overview.value : null,
						config:
							realmConfig.status === "fulfilled" ? realmConfig.value : null,
					};
				}),
			);
			const grid = qs("#home-realm-grid");
			grid.innerHTML = "";
			let onlineRealms = 0,
				onlinePlayers = 0;
			results.forEach(({ realm, health, overview, config }) => {
				const online = Boolean(health?.online),
					players = Number(overview?.online || 0),
					alliance = Number(overview?.allianceOnline || 0),
					horde = Number(overview?.hordeOnline || 0);
				if (online) onlineRealms++;
				onlinePlayers += players;
				const card = document.createElement("article");
				card.className =
					"home-realm-card " + (online ? "is-online" : "is-offline");
				const href = new URL("/realm", location.origin);
				if (realm.key) href.searchParams.set("realm", realm.key);
				card.innerHTML = `<header><div><span class="realm-live-dot"></span><span>${online ? "Online" : "Offline"}</span></div>${health?.maintenance ? '<b class="realm-maintenance">Maintenance</b>' : ""}</header><h3>${esc(realm.name || health?.realm || "Realm")}</h3><p>${esc(config?.realmProfile?.type || "PvE")} · ${esc(config?.realmProfile?.season || config?.realmProfile?.timezone || "")}</p><dl><div><dt>${players.toLocaleString()}</dt><dd>Players online</dd></div><div><dt>${esc(config?.realmProfile?.rates?.questXp || realm.experienceRate || cfg.experienceRate || "1×")}</dt><dd>Quest XP</dd></div></dl>${overview ? `<div class="realm-factions"><span>Alliance ${alliance.toLocaleString()}</span><span>Horde ${horde.toLocaleString()}</span></div>` : ""}<a href="${href.pathname + href.search}">View realm details <span>→</span></a>`;
				grid.append(card);
			});
			qs("#realm-status").textContent =
				`${onlineRealms} of ${results.length} realm${results.length === 1 ? "" : "s"} online · ${onlinePlayers.toLocaleString()} players`;
			const selected =
					results.find((x) => x.realm.key === cfg.realmKey) || results[0],
				address =
					selected?.realm.address ||
					selected?.health?.address ||
					cfg.realmAddress;
			if (address) {
				qs("#realm-address").textContent = address;
				qs(".code-copy").dataset.copy = `set realmlist ${address}`;
			}
		})
		.catch(() => {
			qs("#realm-status").textContent = "Realm information unavailable";
			qs("#home-realm-grid").innerHTML =
				'<p class="empty">Realm status is temporarily unavailable.</p>';
		});
	api("/api/events", { credentials: "omit" })
		.then(({ events }) => {
			const event = (events || []).filter(item => item.status === "scheduled").sort((a, b) => new Date(a.startsAt) - new Date(b.startsAt))[0];
			if (!event) return;
			const eventURL = publicLink(event.url);
			qs("#home-event").innerHTML = `<div><span>${new Date(event.startsAt).toLocaleDateString(undefined, { month: "short", day: "numeric" })}</span><small>${new Date(event.startsAt).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}</small></div><div><h3>${esc(event.title)}</h3><p>${esc(event.description || "Join the community for this scheduled realm event.")}</p><small>${esc(event.location || "In game")}</small></div>${eventURL ? `<a class="ghost-button" href="${esc(eventURL)}" rel="noreferrer">Event details</a>` : '<a class="ghost-button" href="/community">View calendar</a>'}`;
			qs("#home-event-section").classList.remove("hidden");
		})
		.catch(() => {});
	qs(".code-copy")?.addEventListener("click", (e) => {
		navigator.clipboard.writeText(e.currentTarget.dataset.copy);
		toast("Realmlist copied");
	});
	api("/api/votes")
		.then((data) => {
			if (!data.authenticated) return;
			const ready = (data.sites || []).filter((site) => site.available).length;
			if (!ready) return;
			qs("#home-votes-ready").textContent = ready;
			qs("#home-vote-reminder").classList.remove("hidden");
		})
		.catch(() => {});
}
