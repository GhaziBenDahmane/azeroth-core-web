import {esc, qs, qsa} from "/js/ui.js";

export function mountArmory(context) {
	const { api, toast, classes, races, slots, qualityNames, statNames, iconBase, localItemIcon, useLocalItemFallback, initial } = context;
	const form = qs("#armory-search"),
		grid = qs("#armory-results"),
		detail = qs("#character-detail"),
		tooltip = qs("#item-tooltip");
	const fallbackIcon = localItemIcon;
	const formatDate = (value) =>
		value
			? new Intl.DateTimeFormat(undefined, {
					day: "numeric",
					month: "short",
					year: "numeric",
				}).format(new Date(value * 1000))
			: "Not recorded";
	function itemIcon(item) {
		return item.icon ? iconBase + item.icon + ".jpg" : fallbackIcon;
	}
	let jqueryLoader, wowheadLoader;
	function loadJQuery() {
		if (window.jQuery) return Promise.resolve(window.jQuery);
		if (jqueryLoader) return jqueryLoader;
		jqueryLoader = new Promise((resolve, reject) => {
			const script = document.createElement("script");
			script.src = "https://code.jquery.com/jquery-3.7.1.min.js";
			script.integrity = "sha256-/JqT3SQfawRcv/BIHPThkBvs0OEvtFFmqPF/lYI/Cxo=";
			script.crossOrigin = "anonymous";
			script.async = true;
			script.onload = () =>
				window.jQuery
					? resolve(window.jQuery)
					: reject(new Error("jQuery API unavailable"));
			script.onerror = () => reject(new Error("jQuery CDN unavailable"));
			document.head.append(script);
		});
		return jqueryLoader;
	}
	function loadWowhead() {
		if (window.ZamModelViewer) return Promise.resolve(window.ZamModelViewer);
		if (wowheadLoader) return wowheadLoader;
		wowheadLoader = loadJQuery().then(
			() =>
				new Promise((resolve, reject) => {
					const script = document.createElement("script");
					script.src =
						"https://wow.zamimg.com/modelviewer/live/viewer/viewer.min.js";
					script.async = true;
					script.onload = () =>
						window.ZamModelViewer
							? resolve(window.ZamModelViewer)
							: reject(new Error("Viewer API unavailable"));
					script.onerror = () => reject(new Error("Wowhead CDN unavailable"));
					document.head.append(script);
				}),
		);
		return wowheadLoader;
	}
	async function mountWowheadModel(container, c, equipment) {
		const stage = qs(".wowhead-model", container),
			status = qs(".model-provider", container);
		try {
			const Viewer = await loadWowhead();
			const display = (characterDisplays[c.race] || characterDisplays[1])[
				c.gender === 1 ? 1 : 0
			];
			const model = {
				id: display,
				type: 8,
				items: equipment
					.filter((i) => i.displayId)
					.map((i) => [Number(i.slot) + 1, Number(i.displayId)]),
			};
			const options = {
				type: 1,
				contentPath: "https://wow.zamimg.com/modelviewer/live/",
				container: stage,
				aspect: 0.72,
				hd: true,
				models: [model],
			};
			let viewer;
			try {
				viewer = new Viewer(options);
			} catch {
				viewer = new Viewer({ ...options, models: model });
			}
			await Promise.resolve(viewer);
			stage.classList.add("ready");
			status.textContent = "Interactive 3D · Wowhead model viewer";
		} catch (error) {
			status.textContent = "Wowhead model viewer failed to initialize";
			stage.innerHTML =
				'<p class="model-error">Unable to load the 3D model.</p>';
		}
	}
	async function resolveIcon(img, item) {
		if (item.icon) return;
		try {
			const cached = localStorage.getItem("item-icon-" + item.entry);
			if (cached) {
				img.src = iconBase + cached + ".jpg";
				return;
			}
			const data = await fetch(
				`https://nether.wowhead.com/tooltip/item/${item.entry}?dataEnv=8&locale=0`,
			).then((r) => r.json());
			if (data.icon) {
				localStorage.setItem("item-icon-" + item.entry, data.icon);
				img.src = iconBase + data.icon + ".jpg";
			}
		} catch {}
	}
	function showTooltip(item, event) {
		const stats = (item.stats || []).map((x) =>
			typeof x === "string"
				? x
				: `+${x.value} ${statNames[x.type] || "Stat " + x.type}`,
		), enhancements = item.enhancements || [];
		tooltip.innerHTML = `<strong class="q${item.quality || 0}">${esc(item.name)}</strong><span>${qualityNames[item.quality] || "Item"} · Item Level ${item.itemLevel || "—"}</span>${item.requiredLevel ? `<span>Requires Level ${item.requiredLevel}</span>` : ""}${item.armor ? `<span>${item.armor} Armor</span>` : ""}${stats.map((x) => `<span class="item-stat">${esc(x)}</span>`).join("")}${enhancements.map((x) => `<span class="item-enhancement ${esc(x.kind)}">${x.kind === "gem" ? "◆" : "+"} ${esc(x.name || `${x.kind} ${x.enchantmentId}`)}</span>`).join("")}${item.maxDurability ? `<span>Durability ${item.durability} / ${item.maxDurability}</span>` : ""}<small>Item ${item.entry}</small>`;
		tooltip.classList.add("visible");
		moveTooltip(event);
	}
	function moveTooltip(event) {
		const pad = 18,
			w = tooltip.offsetWidth || 260,
			h = tooltip.offsetHeight || 180;
		tooltip.style.left =
			Math.min(event.clientX + 16, innerWidth - w - pad) + "px";
		tooltip.style.top =
			Math.min(event.clientY + 16, innerHeight - h - pad) + "px";
	}
	function gearSlot(slot, item, onSelect) {
		const el = document.createElement("div");
		el.className = `gear-slot gear-slot-${slot} q-border-${item?.quality || 0}`;
		el.innerHTML = `<div class="gear-icon">${item ? '<img alt=""/>' : "<span>+</span>"}${item?.enhancements?.some((x) => x.kind === "gem") ? `<div class="gear-gems" aria-label="${item.enhancements.filter((x) => x.kind === "gem").length} socketed gems">${item.enhancements.filter((x) => x.kind === "gem").map(() => "<i></i>").join("")}</div>` : ""}</div><div class="gear-label"><small>${slots[slot]}</small><strong>${item ? esc(item.name) : "Empty slot"}</strong>${item ? `<i>Item level ${item.itemLevel || "—"}</i>` : ""}</div>`;
		if (item) {
			el.tabIndex = 0;
			el.setAttribute("role", "button");
			el.setAttribute("aria-label", `${slots[slot]}: ${item.name}. Add to item comparison`);
			const img = qs("img", el);
			img.src = itemIcon(item);
			useLocalItemFallback(img);
			resolveIcon(img, item);
			el.onmouseenter = (e) => showTooltip(item, e);
			el.onmousemove = moveTooltip;
			el.onmouseleave = () => tooltip.classList.remove("visible");
			el.onclick = () => onSelect?.(item, el);
			el.onkeydown = (event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onSelect?.(item, el); } };
		}
		return el;
	}
	function renderPaperdoll(c, equipment, onSelect) {
		const bySlot = new Map(equipment.map((i) => [Number(i.slot), i]));
		const doll = document.createElement("div");
		doll.className = "paperdoll";
		const left = document.createElement("div");
		left.className = "gear-column left";
		[0, 1, 2, 14, 4, 3, 8, 9].forEach((slot) =>
			left.append(gearSlot(slot, bySlot.get(slot), onSelect)),
		);
		const center = document.createElement("div");
		center.className = `character-model class-${c.class}`;
		center.innerHTML = `<div class="model-glow"></div><div class="wowhead-model"><div class="model-loading"><i></i><span>Loading character model</span></div></div><span class="model-provider">Connecting to Wowhead 3D…</span><div class="model-caption"><p>${races[c.race] || ""}</p><strong>${classes[c.class] || "Hero"}</strong><small>Level ${c.level}</small></div>`;
		mountWowheadModel(center, c, equipment);
		const right = document.createElement("div");
		right.className = "gear-column right";
		[5, 6, 7, 10, 11, 12, 13, 18].forEach((slot) =>
			right.append(gearSlot(slot, bySlot.get(slot), onSelect)),
		);
		const weapons = document.createElement("div");
		weapons.className = "weapon-row";
		[15, 16, 17].forEach((slot) =>
			weapons.append(gearSlot(slot, bySlot.get(slot), onSelect)),
		);
		doll.append(left, center, right, weapons);
		return doll;
	}
	function renderItemComparison(selected, target) {
		const items = [...selected.values()].map((entry) => entry.item);
		target.classList.toggle("hidden", items.length === 0);
		if (!items.length) { target.innerHTML = ""; return; }
		const statTypes = [...new Set(items.flatMap((item) => (item.stats || []).filter((stat) => typeof stat !== "string").map((stat) => Number(stat.type))))];
		target.innerHTML = `<div class="panel-title"><div><p class="eyebrow">ITEM COMPARISON</p><h3>${items.length} selected item${items.length === 1 ? "" : "s"}</h3><p class="muted">Select up to three equipped pieces. Select one again to remove it.</p></div><button class="ghost-button" type="button" data-clear-comparison>Clear</button></div><div class="data-table-wrap"><table class="data-table item-comparison"><thead><tr><th>Attribute</th>${items.map((item) => `<th><a href="https://www.wowhead.com/wotlk/item=${item.entry}" data-wowhead="item=${item.entry}&domain=wotlk" target="_blank" rel="noreferrer" class="q${item.quality || 0}">${esc(item.name)}</a></th>`).join("")}</tr></thead><tbody><tr><th>Item level</th>${items.map((item) => `<td>${item.itemLevel || "—"}</td>`).join("")}</tr><tr><th>Armor</th>${items.map((item) => `<td>${Number(item.armor || 0).toLocaleString()}</td>`).join("")}</tr>${statTypes.map((type) => `<tr><th>${esc(statNames[type] || `Stat ${type}`)}</th>${items.map((item) => { const value=(item.stats||[]).filter((stat)=>typeof stat!=="string"&&Number(stat.type)===type).reduce((sum,stat)=>sum+Number(stat.value||0),0);return `<td>${value ? (value > 0 ? "+" : "") + value.toLocaleString() : "—"}</td>`; }).join("")}</tr>`).join("")}</tbody></table></div>`;
		qs("[data-clear-comparison]", target).onclick = () => { for (const entry of selected.values()) entry.element.classList.remove("selected"); selected.clear(); renderItemComparison(selected, target); };
	}
	function renderProgression(data, target) {
		target.innerHTML = "";
		const summary = document.createElement("div");
		summary.className = "progress-summary";
		const done = data.progression.filter((x) => x.characterDate).length;
		summary.innerHTML = `<div><p class="eyebrow">RECORDED PROGRESSION</p><h3>${esc(data.character)}${data.guild ? ` · &lt;${esc(data.guild)}&gt;` : ""}</h3></div><strong>${done}<small> / ${data.progression.length} sections</small></strong>`;
		target.append(summary);
		const groups = {};
		data.progression.forEach((p) => (groups[p.raid] ??= []).push(p));
		Object.entries(groups).forEach(([raid, entries]) => {
			const section = document.createElement("section");
			section.className = "raid-card";
			const completed = entries.filter((x) => x.characterDate).length;
			section.innerHTML = `<header><div><h3>${esc(raid)}</h3><p>${completed} of ${entries.length} recorded sections</p></div><span>${Math.round((completed / entries.length) * 100)}%</span></header><div class="raid-progress"><i style="width:${(completed / entries.length) * 100}%"></i></div><div class="raid-encounters"></div>`;
			const list = qs(".raid-encounters", section);
			entries.forEach((e) => {
				const row = document.createElement("article");
				row.className = e.characterDate ? "down" : "pending";
				row.innerHTML = `<span class="kill-mark">${e.characterDate ? "✓" : "○"}</span><div><strong>${esc(e.section)}</strong><small>${esc(e.difficulty)} · ${e.bosses.map(esc).join(", ")}</small></div><time>${formatDate(e.characterDate)}</time>${data.guild ? `<em>Guild first: ${formatDate(e.guildDate)}</em>` : ""}`;
				list.append(row);
			});
			target.append(section);
		});
	}
	async function search(term = "") {
		detail.innerHTML = "";
		grid.innerHTML = "";
		try {
			const { characters } = await api(
				"/api/armory?q=" + encodeURIComponent(term),
			);
			if (!characters.length) {
				grid.innerHTML =
					'<p class="empty">No heroes found. Try another name.</p>';
				return;
			}
			characters.forEach((c) => {
				const card = characterCard(c);
				card.addEventListener("click", () => show(c.name));
				grid.append(card);
			});
		} catch (e) {
			grid.innerHTML = `<p class="empty">${esc(e.message)}</p>`;
		}
	}
	async function show(name, updateHistory = true) {
		grid.innerHTML = "";
		detail.innerHTML = '<div class="skeleton"></div>';
		if (updateHistory && location.pathname !== "/armory/" + encodeURIComponent(name)) {
			const characterURL = new URL(location.href);
			characterURL.pathname = "/armory/" + encodeURIComponent(name);
			characterURL.searchParams.delete("q");
			history.pushState({ character: name }, "", characterURL.pathname + characterURL.search);
		}
		try {
			const [
				{ character: c, equipment, itemSets = [], profile, arenaTeams },
				progress,
				insights,
			] = await Promise.all([
				api("/api/armory/" + encodeURIComponent(name)),
				api("/api/progression/" + encodeURIComponent(name)),
				api("/api/armory/" + encodeURIComponent(name) + "/insights"),
			]);
			const wrap = document.createElement("article");
			wrap.className = "character-profile";
			wrap.innerHTML = `<button class="ghost-button" id="back-armory">← Back to heroes</button><div class="profile-head"><div class="avatar">${initial(c.name)}</div><div><h2></h2><p>Level ${c.level} ${races[c.race] || ""} ${classes[c.class] || ""}</p><a class="guild-name" href="${c.guildId ? `/guilds/${c.guildId}` : "/guilds"}">${c.guild ? "‹" + esc(c.guild) + "›" : "Unaffiliated"}</a></div><div class="profile-vitals"><span class="${c.online ? "is-online" : ""}">${c.online ? "Online now" : "Offline"}</span><strong>${Math.floor(c.totalTime / 3600).toLocaleString()}h <small>played</small></strong></div></div><nav class="profile-tabs" role="tablist" aria-label="Character profile"><button id="profile-tab-gear" role="tab" aria-controls="gear-panel" aria-selected="false" tabindex="-1" data-profile="gear">Equipment</button><button id="profile-tab-activity" role="tab" aria-controls="activity-panel" aria-selected="false" tabindex="-1" data-profile="activity">Recent activity</button><button id="profile-tab-talents" role="tab" aria-controls="talents-panel" aria-selected="false" tabindex="-1" data-profile="talents">Talents & glyphs</button><button id="profile-tab-achievements" role="tab" aria-controls="achievements-panel" aria-selected="false" tabindex="-1" data-profile="achievements">Achievements & raids</button><button id="profile-tab-collections" role="tab" aria-controls="collections-panel" aria-selected="false" tabindex="-1" data-profile="collections">Collections</button><button id="profile-tab-pvp" role="tab" aria-controls="pvp-panel" aria-selected="false" tabindex="-1" data-profile="pvp">PvP history</button><button id="profile-tab-guild" role="tab" aria-controls="guild-panel" aria-selected="false" tabindex="-1" data-profile="guild">Guild activity</button></nav><div id="gear-panel" role="tabpanel" aria-labelledby="profile-tab-gear"></div><div id="activity-panel" role="tabpanel" aria-labelledby="profile-tab-activity" class="hidden"></div><div id="talents-panel" role="tabpanel" aria-labelledby="profile-tab-talents" class="hidden"></div><div id="achievements-panel" role="tabpanel" aria-labelledby="profile-tab-achievements" class="hidden"></div><div id="collections-panel" role="tabpanel" aria-labelledby="profile-tab-collections" class="hidden"></div><div id="pvp-panel" role="tabpanel" aria-labelledby="profile-tab-pvp" class="hidden"></div><div id="guild-panel" role="tabpanel" aria-labelledby="profile-tab-guild" class="hidden"></div>`;
			qs("h2", wrap).textContent = c.name;
			const gear = qs("#gear-panel", wrap),
				summary = document.createElement("div"),
				professions = profile?.professions || [];
			const equipped = equipment.filter((item) => Number(item.itemLevel) > 0),
				averageItemLevel = equipped.length
					? Math.round(equipped.reduce((total, item) => total + Number(item.itemLevel), 0) / equipped.length)
					: 0,
				totalArmor = equipment.reduce((total, item) => total + (Number(item.armor) || 0), 0),
				equipmentStats = new Map();
			equipment.forEach((item) =>
				(item.stats || []).forEach((stat) => {
					if (typeof stat === "string" || !Number.isFinite(Number(stat.value))) return;
					const type = Number(stat.type), value = Number(stat.value);
					equipmentStats.set(type, (equipmentStats.get(type) || 0) + value);
				}),
			);
			const aggregateStats = [...equipmentStats.entries()]
				.filter(([, value]) => value !== 0)
				.sort((a, b) => Math.abs(b[1]) - Math.abs(a[1]));
			summary.className = "profile-overview";
			summary.innerHTML = `<div class="profile-stat"><span>Average item level</span><b>${averageItemLevel || "—"}</b></div><div class="profile-stat"><span>Total armor</span><b>${totalArmor.toLocaleString()}</b></div><div class="profile-stat"><span>Achievements</span><b>${profile?.achievements || 0}</b></div><div class="profile-stat"><span>Exalted reputations</span><b>${profile?.exaltedReputations || 0}</b></div><section class="equipment-stat-summary"><h3>Equipped item stats</h3>${aggregateStats.length ? `<div>${aggregateStats.map(([type, value]) => `<p><b>${esc(statNames[type] || `Stat ${type}`)}</b><span>${value > 0 ? "+" : ""}${value.toLocaleString()}</span></p>`).join("")}</div>` : '<p class="muted">Detailed item stats are unavailable for this realm.</p>'}</section><section><h3>Professions</h3>${professions.length ? professions.map((p) => `<p><b>${esc(p.name)}</b><span>${p.value} / ${p.maximum}</span></p>`).join("") : '<p class="muted">No primary professions recorded.</p>'}</section>`;
			const comparison=document.createElement("section"),selectedItems=new Map();comparison.className="account-panel item-comparison-panel hidden";
			const selectItem=(item,element)=>{if(selectedItems.has(item.entry)){selectedItems.delete(item.entry);element.classList.remove("selected")}else{if(selectedItems.size>=3){toast("Compare up to three items at a time");return}selectedItems.set(item.entry,{item,element});element.classList.add("selected")}renderItemComparison(selectedItems,comparison)};
			const setSummary=document.createElement("section");setSummary.className="item-set-summary";setSummary.innerHTML=`<div class="panel-title"><div><p class="eyebrow">EQUIPPED SETS</p><h3>Set bonuses</h3></div></div>${itemSets.length?itemSets.map(set=>`<article><div><b>${esc(set.name)}</b><small>${set.equipped} pieces equipped${set.metadata?"":" · DBC metadata unavailable"}</small></div><ul>${(set.bonuses||[]).map(bonus=>`<li class="${bonus.active?"active":""}"><span>${bonus.active?"✓":"○"} ${bonus.pieces} pieces</span><a href="https://www.wowhead.com/wotlk/spell=${bonus.spellId}" data-wowhead="spell=${bonus.spellId}&domain=wotlk" target="_blank" rel="noreferrer">${esc(bonus.name||`Spell ${bonus.spellId}`)}</a></li>`).join("")||'<li class="muted">Bonus metadata is unavailable.</li>'}</ul></article>`).join(""):'<p class="muted">No equipped item set bonuses.</p>'}`;
			gear.append(renderPaperdoll(c, equipment, selectItem), comparison, setSummary, summary);
			const talents = qs("#talents-panel", wrap),
				metadata = insights.capabilities || {};
			talents.innerHTML = `${metadata.dbcMetadata ? "" : `<div class="notice-box warning"><p><b>Limited metadata.</b> ${esc(metadata.source || "This realm has not imported the optional WotLK DBC metadata tables.")}</p></div>`}<div class="insight-grid"></div>`;
			const talentGrid = qs(".insight-grid", talents);
			(insights.talents || []).forEach((t) => {
				const card = document.createElement("section");
				card.className = "insight-card";
				const trees = (t.trees || []).map((tree) => `${esc(tree.name || "Tree " + tree.id)} ${tree.points}`).join(" / ");
				card.innerHTML = `<p class="eyebrow">${t.active ? "ACTIVE SPECIALIZATION" : "SECONDARY SPECIALIZATION"}</p><h3>${t.pointsKnown ? `${t.points} talent points` : `${(t.spells || []).length} learned talents`}</h3>${trees ? `<p class="talent-distribution">${trees}</p>` : ""}<div class="spell-chip-list">${(t.spells || []).map((spell) => { const id=typeof spell === "number" ? spell : spell.id, name=typeof spell === "number" ? `Spell ${id}` : spell.name || `Spell ${id}`, detail=typeof spell === "number" ? "" : [spell.rank ? `Rank ${spell.rank}` : spell.rankName, spell.treeName].filter(Boolean).join(" · "); return `<a href="https://www.wowhead.com/wotlk/spell=${id}" data-wowhead="spell=${id}&domain=wotlk" target="_blank" rel="noreferrer"${spell.description ? ` title="${esc(spell.description)}"` : ""}><b>${esc(name)}</b>${detail ? `<small>${esc(detail)}</small>` : ""}</a>`; }).join("")}</div>`;
				talentGrid.append(card);
			});
			const glyphCard = document.createElement("section");
			glyphCard.className = "insight-card";
			glyphCard.innerHTML = `<p class="eyebrow">GLYPHS</p><h3>${(insights.glyphs || []).length} active glyphs</h3><div class="spell-chip-list">${(insights.glyphs || []).map((g) => { const id=g.spellId || g.id; return `<a href="https://www.wowhead.com/wotlk/spell=${id}" data-wowhead="spell=${id}&domain=wotlk" target="_blank" rel="noreferrer"${g.description ? ` title="${esc(g.description)}"` : ""}><b>${esc(g.name || `Glyph ${g.id}`)}</b><small>Slot ${g.slot + 1}</small></a>`; }).join("") || '<span class="muted">No glyphs recorded.</span>'}</div>`;
			talentGrid.append(glyphCard);
			renderProgression(progress, qs("#achievements-panel", wrap));
			const achievements = document.createElement("section");
			achievements.className = "achievement-browser";
			achievements.innerHTML = `<div class="view-heading"><div><p class="eyebrow">ACHIEVEMENT BROWSER</p><h3>Recently earned</h3></div><div class="row-actions"><label><span class="sr-only">Achievement category</span><select aria-label="Achievement category"><option value="">All categories</option></select></label><input type="search" placeholder="Filter achievements" aria-label="Filter achievements"></div></div><div class="achievement-list"></div>`;
			const achievementList = qs(".achievement-list", achievements),
				achievementCategory = qs("select", achievements),
				categoryOptions = new Map(),
				renderAchievements = (term) => {
					achievementList.innerHTML = "";
					(insights.achievements || [])
						.filter((a) => !achievementCategory.value || String(a.parentCategory || a.category || 0) === achievementCategory.value)
						.filter((a) => `${a.id} ${a.name || ""} ${a.description || ""} ${a.categoryName || ""} ${a.parentCategoryName || ""}`.toLowerCase().includes(term.toLowerCase()))
						.forEach((a) => {
							const row = document.createElement("a");
							row.href = `https://www.wowhead.com/wotlk/achievement=${a.id}`;
							row.dataset.wowhead = `achievement=${a.id}&domain=wotlk`;
							row.target = "_blank";
							row.rel = "noreferrer";
							row.className = "achievement-row";
							row.innerHTML = `<span><strong>${esc(a.name || `Achievement ${a.id}`)}</strong><small>${esc([a.parentCategoryName,a.categoryName].filter(Boolean).join(" › ") || `ID ${a.id}`)}</small>${a.description ? `<small>${esc(a.description)}</small>` : ""}${a.criteria?.length ? `<small>${a.criteria.filter((criterion)=>criterion.complete).length}/${a.criteria.length} objectives · ${esc(a.criteria.map((criterion)=>criterion.description||`Criterion ${criterion.id}`).join(" · "))}</small>` : ""}</span><span>${a.points ? `<b>${a.points} points</b>` : ""}<time>${formatDate(a.date)}</time></span>`;
							achievementList.append(row);
						});
					if (!achievementList.children.length)
						achievementList.innerHTML =
							'<p class="muted">No matching achievements.</p>';
				};
			(insights.achievements || []).forEach((a) => { const id=String(a.parentCategory || a.category || 0),name=a.parentCategoryName || a.categoryName; if(id!=="0"&&name)categoryOptions.set(id,name); });
			for (const [id,name] of [...categoryOptions].sort((a,b)=>a[1].localeCompare(b[1]))) { const option=document.createElement("option");option.value=id;option.textContent=name;achievementCategory.append(option); }
			qs("input", achievements).oninput = (e) =>
				renderAchievements(e.target.value.trim());
			achievementCategory.onchange = () => renderAchievements(qs('input[type="search"]', achievements).value.trim());
			renderAchievements("");
			qs("#achievements-panel", wrap).prepend(achievements);
			const collections=insights.collections||{},collectionPanel=qs("#collections-panel",wrap),collectionSpells=(items,empty)=>items?.length?`<div class="spell-chip-list">${items.map(item=>`<a href="https://www.wowhead.com/wotlk/spell=${item.id}" data-wowhead="spell=${item.id}&domain=wotlk" target="_blank" rel="noreferrer"><b>${esc(item.name||"Spell "+item.id)}</b></a>`).join("")}</div>`:`<p class="muted">${empty}</p>`;
			collectionPanel.innerHTML=`<div class="insight-grid"><section class="insight-card"><p class="eyebrow">PROFESSIONS</p><h3>${(insights.professions||[]).length} learned skills</h3>${(insights.professions||[]).map(skill=>`<details><summary><b>${esc(skill.name)}</b><span>${skill.value} / ${skill.maximum} · ${(skill.recipes||[]).length} recipes</span></summary><div class="spell-chip-list">${(skill.recipes||[]).map(recipe=>`<a href="https://www.wowhead.com/wotlk/spell=${recipe.spellId}" data-wowhead="spell=${recipe.spellId}&domain=wotlk" target="_blank" rel="noreferrer"><b>${esc(recipe.name||"Spell "+recipe.spellId)}</b><small>${recipe.requiredSkill?`Requires ${recipe.requiredSkill}`:"Learned"}</small></a>`).join("")||'<span class="muted">Recipe metadata is unavailable.</span>'}</div></details>`).join("")||'<p class="muted">No professions recorded.</p>'}</section><section class="insight-card"><p class="eyebrow">REPUTATIONS</p><h3>${(collections.reputations||[]).length} tracked factions</h3><div class="activity-list">${(collections.reputations||[]).map(item=>`<div><span>${esc(item.name||"Faction "+item.factionId)}</span><b>${Number(item.standing).toLocaleString()}</b></div>`).join("")||'<p class="muted">Reputation data is unavailable.</p>'}</div></section><section class="insight-card"><p class="eyebrow">TITLES</p><h3>${(collections.titles||[]).length} earned titles</h3>${(collections.titles||[]).length?`<div class="spell-chip-list">${collections.titles.map(item=>`<span><b>${esc(item.name||"Title "+item.id)}</b></span>`).join("")}</div>`:'<p class="muted">Title metadata is unavailable.</p>'}</section><section class="insight-card"><p class="eyebrow">MOUNTS</p><h3>${(collections.mounts||[]).length} mounts</h3>${collectionSpells(collections.mounts,"Mount metadata is unavailable.")}</section><section class="insight-card"><p class="eyebrow">COMPANIONS</p><h3>${(collections.companions||[]).length} companions</h3>${collectionSpells(collections.companions,"Companion metadata is unavailable.")}</section></div>`;
			const pvp = qs("#pvp-panel", wrap),
				teams = arenaTeams || [];
			pvp.innerHTML = '<div class="personal-arena"></div>';
			teams.forEach((t) => {
				const row = document.createElement("article");
				row.className = "personal-team";
				row.innerHTML = `<strong>#${t.rank}</strong><div><h3>${esc(t.name)}</h3><p>${t.bracket}v${t.bracket} · Team rating ${t.rating}</p></div><span><b>${t.personalRating}</b><small> personal rating</small><em>${t.personalWins}W / ${t.personalGames - t.personalWins}L</em></span>`;
				qs(".personal-arena", pvp).append(row);
			});
			const matches = document.createElement("section");
			matches.className = "insight-card";
			matches.innerHTML = `<p class="eyebrow">RECENT MATCHES</p><h3>Arena match history</h3><p class="muted">${esc(insights.pvpHistorySource || "")}</p><div class="activity-list">${(insights.pvpMatches || []).map((m) => `<div><strong class="status-${m.result === "win" ? "executed" : "failed"}">${esc(m.result)}</strong><span><b>${m.bracket}v${m.bracket} · ${esc(m.team || "Your team")} vs ${esc(m.opponent)}</b><small>${esc(m.season || "Current season")}${m.durationSeconds ? ` · ${Math.floor(m.durationSeconds / 60)}m ${m.durationSeconds % 60}s` : ""}${m.source === "signed_ingest" ? " · verified event" : ""}</small></span><b title="Rating movement">${m.ratingBefore || m.ratingAfter ? `${m.ratingBefore} → ${m.ratingAfter} ` : ""}${m.ratingChange > 0 ? "+" : ""}${m.ratingChange}</b><time>${new Date(m.playedAt).toLocaleString()}</time></div>`).join("") || '<p class="muted">No per-match history is available.</p>'}</div>`;
			pvp.append(matches);
			const battlegrounds = document.createElement("section");
			battlegrounds.className = "insight-card";
			battlegrounds.innerHTML = `<p class="eyebrow">BATTLEGROUNDS</p><h3>Recent battleground matches</h3><div class="activity-list">${(insights.battlegroundMatches||[]).map((m)=>`<div><strong class="status-${m.result==="win"?"executed":"failed"}">${esc(m.result)}</strong><span><b>${esc(m.battleground)}</b><small>${m.killingBlows} killing blows · ${m.honorableKills} honorable kills · ${m.deaths} deaths</small></span><span><b>${Number(m.damageDone).toLocaleString()}</b><small> damage · ${Number(m.healingDone).toLocaleString()} healing</small></span><time>${new Date(m.playedAt).toLocaleString()}</time></div>`).join("")||'<p class="muted">No verified battleground history is available.</p>'}</div>`;
			pvp.append(battlegrounds);
			if (!teams.length)
				qs(".personal-arena", pvp).innerHTML =
					'<p class="empty">This character is not on an active arena team.</p>';
			const guild = qs("#guild-panel", wrap),
				achievementNames = new Map((insights.achievements || []).map((achievement) => [Number(achievement.id), achievement.name]));
			guild.innerHTML = `<div class="insight-grid"><section class="insight-card"><p class="eyebrow">RAID COMPOSITION</p><h3>${esc(c.guild || "No guild")}</h3><div class="activity-list">${(insights.raidComposition || []).map((m) => `<a href="/armory/${encodeURIComponent(m.name)}"><span>${esc(m.name)}</span><small>${classes[m.class] || "Hero"} · level ${m.level}</small><i class="${m.online ? "is-online" : ""}">${m.online ? "Online" : "Offline"}</i></a>`).join("") || '<p class="muted">No guild roster data.</p>'}</div></section><section class="insight-card"><p class="eyebrow">GUILD ACTIVITY</p><h3>Recent achievements</h3><div class="activity-list">${(insights.guildActivity || []).map((a) => `<div><span>${esc(a.character)}</span><a href="https://www.wowhead.com/wotlk/achievement=${a.achievement}" data-wowhead="achievement=${a.achievement}&domain=wotlk" target="_blank" rel="noreferrer">${esc(achievementNames.get(Number(a.achievement)) || `Achievement ${a.achievement}`)}</a><time>${formatDate(a.date)}</time></div>`).join("") || '<p class="muted">No recent guild activity.</p>'}</div></section></div>`;
			const timeline = [
				...(insights.achievements || []).map((item) => ({ kind:"achievement", title:item.name || `Achievement ${item.id}`, detail:[item.parentCategoryName,item.categoryName].filter(Boolean).join(" › "), at:new Date(Number(item.date) * 1000), href:`https://www.wowhead.com/wotlk/achievement=${item.id}` })),
				...(insights.pvpMatches || []).map((item) => ({ kind:"arena", title:`${item.result === "win" ? "Won" : "Lost"} ${item.bracket}v${item.bracket} arena`, detail:`vs ${item.opponent} · ${item.ratingChange > 0 ? "+" : ""}${item.ratingChange} rating`, at:new Date(item.playedAt) })),
				...(insights.battlegroundMatches || []).map((item) => ({ kind:"battleground", title:`${item.result === "win" ? "Won" : "Lost"} ${item.battleground}`, detail:`${item.killingBlows} killing blows · ${item.honorableKills} honorable kills`, at:new Date(item.playedAt) })),
			].filter((item)=>!Number.isNaN(item.at.getTime())).sort((a,b)=>b.at-a.at).slice(0,50);
			const activityPanel=qs("#activity-panel",wrap);activityPanel.innerHTML=`<div class="view-heading"><div><p class="eyebrow">RECENT ACTIVITY</p><h3>A single character timeline</h3><p>Achievement data comes from the core database. PvP events require the signed competitive ingestion integration.</p></div></div><div class="activity-list character-timeline">${timeline.map(item=>`<${item.href?`a href="${item.href}" target="_blank" rel="noreferrer"`:"div"}><strong>${esc(item.kind)}</strong><span><b>${esc(item.title)}</b><small>${esc(item.detail)}</small></span><time>${item.at.toLocaleString()}</time></${item.href?"a":"div"}>`).join("")||'<p class="muted">No public activity is recorded for this character.</p>'}</div>`;
			const profileTabs = ["gear", "activity", "talents", "achievements", "collections", "pvp", "guild"],
				activateProfileTab = (id, updateURL = false) => {
					const active = profileTabs.includes(id) ? id : "gear";
					qsa("[data-profile]", wrap).forEach((button) => {
						const selected = button.dataset.profile === active;
						button.classList.toggle("active", selected);
						button.setAttribute("aria-selected", String(selected));
						button.tabIndex = selected ? 0 : -1;
					});
					profileTabs.forEach((panel) =>
						qs("#" + panel + "-panel", wrap).classList.toggle("hidden", panel !== active),
					);
					if (updateURL) {
						const url = new URL(location.href);
						if (active === "gear") url.searchParams.delete("tab");
						else url.searchParams.set("tab", active);
						history.pushState({ character: name, tab: active }, "", url.pathname + url.search);
					}
				};
			qsa("[data-profile]", wrap).forEach((button, index, buttons) => {
				button.onclick = () => activateProfileTab(button.dataset.profile, true);
				button.onkeydown = (event) => {
					if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
					event.preventDefault();
					let target = index;
					if (event.key === "ArrowLeft") target = (index - 1 + buttons.length) % buttons.length;
					if (event.key === "ArrowRight") target = (index + 1) % buttons.length;
					if (event.key === "Home") target = 0;
					if (event.key === "End") target = buttons.length - 1;
					buttons[target].focus();
					activateProfileTab(buttons[target].dataset.profile, true);
				};
			});
			activateProfileTab(new URLSearchParams(location.search).get("tab") || "gear");
			detail.innerHTML = "";
			detail.append(wrap);
			qs("#back-armory").onclick = () => {
				history.pushState(null, "", "/armory");
				search(form.q.value);
			};
		} catch (e) {
			detail.innerHTML = `<p class="empty">${esc(e.message)}</p>`;
		}
	}
	form.addEventListener("submit", (e) => {
		e.preventDefault();
		search(form.q.value);
	});
	const pathCharacter = location.pathname.match(/^\/armory\/([^/]+)\/?$/),
		initialQuery = pathCharacter ? decodeURIComponent(pathCharacter[1]) : new URLSearchParams(location.search).get("q") || "";
	form.q.value = initialQuery;
	if (initialQuery) show(initialQuery, false);
	else search("");
	window.addEventListener("popstate", () => {
		const match = location.pathname.match(/^\/armory\/([^/]+)\/?$/);
		if (match) show(decodeURIComponent(match[1]), false);
		else search(new URLSearchParams(location.search).get("q") || "");
	});

}
