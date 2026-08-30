import {esc, pageFromURL, publicLink, qs, qsa, renderPagination, setMessage, updateURLQuery} from "/js/ui.js";
const page = document.body.dataset.page;
for (const link of qsa('.nav nav a')) {
	const path = new URL(link.href, location.origin).pathname.replace(/\/$/, "") || "/";
	const current = location.pathname.replace(/\/$/, "") || "/";
	if (current === path || (path !== "/" && current.startsWith(path + "/")) || (path === "/community" && current.startsWith("/tracker")))
		link.setAttribute("aria-current", "page");
}
const classes = {
	1: "Warrior",
	2: "Paladin",
	3: "Hunter",
	4: "Rogue",
	5: "Priest",
	6: "Death Knight",
	7: "Shaman",
	8: "Mage",
	9: "Warlock",
	11: "Druid",
};
const classSetPreviewItems = {
	1: 46151,
	2: 46156,
	3: 46143,
	4: 46125,
	5: 46172,
	6: 46115,
	7: 46212,
	8: 46129,
	9: 46140,
	11: 46161,
};
const races = {
	1: "Human",
	2: "Orc",
	3: "Dwarf",
	4: "Night Elf",
	5: "Undead",
	6: "Tauren",
	7: "Gnome",
	8: "Troll",
	10: "Blood Elf",
	11: "Draenei",
};
const slots = {
	0: "Head",
	1: "Neck",
	2: "Shoulders",
	3: "Shirt",
	4: "Chest",
	5: "Waist",
	6: "Legs",
	7: "Feet",
	8: "Wrists",
	9: "Hands",
	10: "Ring",
	11: "Ring",
	12: "Trinket",
	13: "Trinket",
	14: "Back",
	15: "Main hand",
	16: "Off hand",
	17: "Ranged",
	18: "Tabard",
};
const qualityNames = {
	0: "Poor",
	1: "Common",
	2: "Uncommon",
	3: "Rare",
	4: "Epic",
	5: "Legendary",
};
const statNames = {
	1: "Health",
	3: "Agility",
	4: "Strength",
	5: "Intellect",
	6: "Spirit",
	7: "Stamina",
	12: "Defense Rating",
	13: "Dodge Rating",
	14: "Parry Rating",
	15: "Block Rating",
	16: "Melee Hit",
	17: "Ranged Hit",
	18: "Spell Hit",
	19: "Melee Critical Strike",
	20: "Ranged Critical Strike",
	21: "Spell Critical Strike",
	28: "Melee Haste",
	29: "Ranged Haste",
	30: "Spell Haste",
	31: "Hit Rating",
	32: "Critical Strike Rating",
	35: "Resilience Rating",
	36: "Haste Rating",
	37: "Expertise Rating",
	38: "Attack Power",
	45: "Spell Power",
	47: "Spell Penetration",
	48: "Block Value",
};
const iconBase = "https://wow.zamimg.com/images/wow/icons/large/";
const localItemIcon = "/images/item-placeholder.svg";
const characterDisplays = {
	1: [49, 50],
	2: [51, 52],
	3: [53, 54],
	4: [55, 56],
	5: [57, 58],
	6: [59, 60],
	7: [1563, 1564],
	8: [1478, 1479],
	10: [15476, 15475],
	11: [16125, 16126],
};
const requestedRealm = new URLSearchParams(location.search).get("realm") || "";

function setTheme(theme) {
	document.documentElement.dataset.theme = theme;
	localStorage.setItem("portal-theme", theme);
	const toggle = qs("#theme-toggle"),
		light = theme === "light";
	if (toggle) {
		toggle.setAttribute(
			"aria-label",
			light ? "Use dark mode" : "Use light mode",
		);
		toggle.title = light ? "Use dark mode" : "Use light mode";
	}
}
setTheme(document.documentElement.dataset.theme || "dark");
qs("#theme-toggle")?.addEventListener("click", () =>
	setTheme(
		document.documentElement.dataset.theme === "light" ? "dark" : "light",
	),
);
const accountMenu = qs(".account-menu"),
	accountTrigger = qs(".account-menu-trigger");
accountTrigger?.addEventListener("click", (event) => {
	event.stopPropagation();
	const open = accountMenu.classList.toggle("open");
	accountTrigger.setAttribute("aria-expanded", String(open));
});
const accountMenuItems = () => qsa('[role="menuitem"]', accountMenu).filter((item) => !item.classList.contains("hidden"));
accountTrigger?.addEventListener("keydown", (event) => {
	if (!["ArrowDown", "ArrowUp"].includes(event.key)) return;
	event.preventDefault();
	accountMenu.classList.add("open");
	accountTrigger.setAttribute("aria-expanded", "true");
	const items = accountMenuItems();
	(items[event.key === "ArrowUp" ? items.length - 1 : 0] || accountTrigger).focus();
});
accountMenu?.addEventListener("keydown", (event) => {
	const items = accountMenuItems(), current = items.indexOf(document.activeElement);
	if (event.key === "Escape") {
		event.preventDefault();
		accountMenu.classList.remove("open");
		accountTrigger.setAttribute("aria-expanded", "false");
		accountTrigger.focus();
		return;
	}
	if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key) || current < 0) return;
	event.preventDefault();
	let next = event.key === "Home" ? 0 : event.key === "End" ? items.length - 1 : event.key === "ArrowDown" ? (current + 1) % items.length : (current - 1 + items.length) % items.length;
	items[next].focus();
});
document.addEventListener("click", (event) => {
	if (accountMenu && !accountMenu.contains(event.target)) {
		accountMenu.classList.remove("open");
		accountTrigger?.setAttribute("aria-expanded", "false");
	}
});
document.addEventListener("keydown", (event) => {
	if (event.key === "Escape") {
		accountMenu?.classList.remove("open");
		accountTrigger?.setAttribute("aria-expanded", "false");
	}
});
qs(".account-signout")?.addEventListener("click", async () => {
	try {
		await api("/api/auth/logout", { method: "POST", body: "{}" });
	} finally {
		location.href = "/";
	}
});

async function api(path, options = {}) {
	const { stepUpRetried = false, headers = {}, ...fetchOptions } = options;
	if (requestedRealm && path.startsWith("/api/")) {
		const url = new URL(path, location.origin);
		if (!url.searchParams.has("realm"))
			url.searchParams.set("realm", requestedRealm);
		path = url.pathname + url.search;
	}
	const response = await fetch(path, {
		...fetchOptions,
		headers: fetchOptions.body instanceof FormData ? headers : { "Content-Type": "application/json", ...headers },
	});
	const body = await response.json().catch(() => ({}));
	const requestId = response.headers.get("X-Request-ID") || "";
	if (response.status === 428 && !stepUpRetried && (path.startsWith("/api/admin/") || path.startsWith("/api/identity/") || path.startsWith("/api/security/passkeys/"))) {
		if (await requestStepUp()) return api(path, { ...fetchOptions, headers, stepUpRetried: true });
	}
	if (!response.ok)
		throw Object.assign(new Error((body.error || "Something went wrong") + (response.status >= 500 && requestId ? ` · Request ${requestId}` : "")), {
			status: response.status,
			requestId,
		});
	if (body && typeof body === "object" && requestId) body.requestId ||= requestId;
	return body;
}

let stepUpPromise;
function requestStepUp() {
	if (stepUpPromise) return stepUpPromise;
	const dialog = qs("#step-up-dialog"), form = qs("#step-up-form");
	if (!dialog || !form) return Promise.resolve(false);
	setMessage(form, "");
	form.reset();
	dialog.showModal();
	form.elements.password.focus();
	stepUpPromise = new Promise((resolve) => {
		let settled = false;
		const finish = (value) => {
			if (settled) return;
			settled = true;
			dialog.close();
			stepUpPromise = null;
			resolve(value);
		};
		form.onsubmit = async (event) => {
			event.preventDefault();
			const button = qs('button[type="submit"]', form);
			button.disabled = true;
			setMessage(form, "");
			try {
				const response = await fetch("/api/security/step-up" + (requestedRealm ? "?realm=" + encodeURIComponent(requestedRealm) : ""), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(Object.fromEntries(new FormData(form))) });
				const body = await response.json().catch(() => ({}));
				if (!response.ok) throw new Error(body.error || "Confirmation failed");
				finish(true);
			} catch (error) {
				setMessage(form, error.message);
			} finally {
				button.disabled = false;
			}
		};
		qs(".dialog-close", dialog).onclick = () => finish(false);
		qs("[data-step-up-cancel]", dialog).onclick = () => finish(false);
		dialog.oncancel = (event) => { event.preventDefault(); finish(false); };
	});
	return stepUpPromise;
}
function base64URLToBuffer(value) {
	const padded = value.replaceAll("-", "+").replaceAll("_", "/") + "===".slice((value.length + 3) % 4);
	const binary = atob(padded);
	return Uint8Array.from(binary, (character) => character.charCodeAt(0)).buffer;
}
function bufferToBase64URL(value) {
	const bytes = new Uint8Array(value || new ArrayBuffer(0));
	let binary = "";
	for (let index = 0; index < bytes.length; index += 0x8000)
		binary += String.fromCharCode(...bytes.subarray(index, index + 0x8000));
	return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}
function publicKeyCredentialJSON(credential, name = "") {
	const response = credential.response;
	return {
		id: credential.id, rawId: bufferToBase64URL(credential.rawId), type: credential.type, name,
		response: {
			clientDataJSON: bufferToBase64URL(response.clientDataJSON),
			attestationObject: response.attestationObject ? bufferToBase64URL(response.attestationObject) : "",
			authenticatorData: response.authenticatorData ? bufferToBase64URL(response.authenticatorData) : "",
			signature: response.signature ? bufferToBase64URL(response.signature) : "",
			userHandle: response.userHandle ? bufferToBase64URL(response.userHandle) : "",
			transports: typeof response.getTransports === "function" ? response.getTransports() : [],
		},
	};
}
function applyPublicConfig(c) {
	const name = c.portalName || c.realmName || "Azeroth",
		mark = (c.brandMark || name.slice(0, 1) || "A").slice(0, 3);
	const color = /^#[0-9a-f]{6}$/i,
		root = document.documentElement.style;
	if (color.test(c.themePrimary)) root.setProperty("--gold", c.themePrimary);
	if (color.test(c.themeSecondary))
		root.setProperty("--gold2", c.themeSecondary);
	if (color.test(c.themeAccent)) root.setProperty("--teal", c.themeAccent);
	if (color.test(c.themeBackground))
		root.setProperty("--bg", c.themeBackground);
	document.documentElement.lang = c.locale || "en";
	qsa("[data-brand-name]").forEach((x) => {
		const small = qs("small", x);
		if (small) {
			x.firstChild.textContent = name.toUpperCase();
		} else {
			x.textContent = name.toUpperCase();
		}
	});
	qsa("[data-brand-mark]").forEach((x) => (x.textContent = mark.toUpperCase()));
	qsa("[data-realm-name]").forEach(
		(x) => (x.textContent = (c.realmName || name).toUpperCase() + "."),
	);
	qsa("[data-home-realm]").forEach(
		(x) => (x.textContent = c.realmName || name),
	);
	if (document.title.includes("Azeroth"))
		document.title = document.title.replace("Azeroth", name);
	const description = qs("meta[name=description]");
	if (description && c.tagline) description.content = c.tagline;
	if (qs("#portal-tagline")) qs("#portal-tagline").textContent = c.tagline;
	if (qs("#home-headline") && c.homeHeadline)
		qs("#home-headline").textContent = c.homeHeadline;
	if (qs("#home-eyebrow") && c.homeEyebrow)
		qs("#home-eyebrow").textContent = c.homeEyebrow;
	if (qs("#home-primary-cta") && c.homePrimaryCta)
		qs("#home-primary-cta").textContent = c.homePrimaryCta;
	if (qs("#home-connect-title") && c.homeConnectTitle)
		qs("#home-connect-title").textContent = c.homeConnectTitle;
	if (qs("#home-guide-text") && c.homeGuideText)
		qs("#home-guide-text").textContent = c.homeGuideText;
	for (const [card, content, value] of [
		["#home-rules-card", "#home-rules", c.homeRules],
		["#home-discord-card", "#home-discord-status", c.discordStatus],
		["#home-changelog-card", "#home-changelog", c.homeChangelog],
	])
		if (value && qs(card)) {
			qs(content).textContent = value;
			qs(card).classList.remove("hidden");
			qs("#home-community")?.classList.remove("hidden");
		}
	if (qs("#client-version"))
		qs("#client-version").textContent = c.clientVersion;
	if (qs("#experience-rate"))
		qs("#experience-rate").textContent = c.experienceRate;
	if (qs("#uptime-label")) qs("#uptime-label").textContent = c.uptimeLabel;
	if (qs("#expansion-name"))
		qs("#expansion-name").textContent = c.expansionName;
	if (qs("#client-description"))
		qs("#client-description").textContent =
			`${c.expansionName} ${c.clientVersion}`;
	if (qs("#client-build")) qs("#client-build").textContent = c.clientBuild;
	if (qs("#troubleshooting-client-build"))
		qs("#troubleshooting-client-build").textContent = `${c.clientVersion} build ${c.clientBuild}`;
	if (qs("#realm-address")) qs("#realm-address").textContent = c.realmAddress;
	if (qs("#play-realm-type"))
		qs("#play-realm-type").textContent = c.realmProfile?.type || "PvE";
	if (qs("#play-level-cap"))
		qs("#play-level-cap").textContent = c.realmProfile?.maxLevel || 80;
	if (qs("#download-unavailable") && publicLink(c.downloadUrl))
		qs("#download-unavailable").classList.add("hidden");
	if (qs("#community-changelog") && c.homeChangelog)
		qs("#community-changelog").classList.remove("hidden");
	qsa("[data-footer-text]").forEach((x) => (x.textContent = c.footerText));
	qsa("[data-i18n]").forEach((x) => {
		const translated = c.translations?.[x.dataset.i18n];
		if (translated) x.textContent = translated;
	});
	for (const [selector, dataKey, attribute] of [
		["[data-i18n-placeholder]", "i18nPlaceholder", "placeholder"],
		["[data-i18n-aria-label]", "i18nAriaLabel", "aria-label"],
		["[data-i18n-title]", "i18nTitle", "title"],
	]) qsa(selector).forEach((element) => {
		const translated = c.translations?.[element.dataset[dataKey]];
		if (translated) element.setAttribute(attribute, translated);
	});
	const logo = publicLink(c.logoUrl);
	if (logo) {
		const candidate = new Image();
		candidate.onload = () => {
			qsa("[data-brand-logo]").forEach((x) => {
				x.src = logo;
				x.alt = name;
				x.classList.remove("hidden");
			});
			qsa("[data-brand-mark]").forEach((x) => x.classList.add("hidden"));
		};
		candidate.src = logo;
	}
	const hero = publicLink(c.heroImageUrl),
		heroArea = qs(".home-hero");
	if (hero && heroArea) {
		const candidate = new Image();
		candidate.onload = () => {
			heroArea.style.setProperty(
				"--hero-image",
				`url("${hero.replaceAll('"', "%22")}")`,
			);
			heroArea.classList.add("custom-hero");
			qs("#default-hero-credit")?.classList.add("hidden");
		};
		candidate.src = hero;
	}
	const favicon = publicLink(c.faviconUrl);
	if (favicon) {
		let icon = qs('link[rel="icon"]');
		if (!icon) {
			icon = document.createElement("link");
			icon.rel = "icon";
			document.head.append(icon);
		}
		icon.href = favicon;
	}
	const download = publicLink(c.downloadUrl);
	if (download && qs("#download-link")) {
		qs("#download-link").href = download;
		qs("#download-link").classList.remove("hidden");
	}
	const community = publicLink(c.communityUrl);
	qsa(".config-community").forEach((x) => {
		if (community) {
			x.href = community;
			x.classList.remove("hidden");
		}
	});
	for (const [selector, url] of [
		[".config-terms", publicLink(c.termsUrl)],
		[".config-privacy", publicLink(c.privacyUrl)],
		[".config-security-contact", publicLink(c.securityContactUrl)],
	])
		qsa(`${selector}`).forEach((x) => {
			if (url) {
				x.href = url;
				x.classList.remove("hidden");
			}
		});
	if (publicLink(c.securityContactUrl))
		qs(".security-support-fallback")?.classList.add("hidden");
	const featureRoutes = {
		registration: "/register",
		armory: "/armory",
		rankings: "/rankings",
		guilds: "/guilds",
		realm: "/realm",
		shop: "/shop",
	};
	for (const [feature, path] of Object.entries(featureRoutes))
		if (c.features?.[feature] === false)
			qsa(`a[href="${path}"]`).forEach((x) => x.classList.add("hidden"));
	if (c.features?.shop === false) {
		qsa("[data-feature-shop]").forEach((x) => x.classList.add("hidden"));
		const grid = qs(".account-grid");
		if (grid) grid.style.gridTemplateColumns = "1fr";
	}
	if (c.features?.billing === false)
		qs(".credit-store")?.classList.add("hidden");
	if (c.capabilities?.arenaSeasonArchives === false)
		qs("#ranking-season-field")?.classList.add("hidden");
	if (c.capabilities?.specializationFilters === false)
		qs("#ranking-spec-field")?.classList.add("hidden");
	if (c.capabilities?.discordOAuth === true)
		qsa("[data-discord-oauth]").forEach((x) => x.classList.remove("hidden"));
	if (c.capabilities?.googleOAuth === true)
		qsa("[data-google-oauth]").forEach((x) => x.classList.remove("hidden"));
	if (c.capabilities?.discordOAuth === true || c.capabilities?.googleOAuth === true)
		qsa("[data-social-oauth]").forEach((x) => x.classList.remove("hidden"));
	if (c.capabilities?.passkeys === true && window.isSecureContext && "PublicKeyCredential" in window)
		qsa("[data-passkeys]").forEach((x) => x.classList.remove("hidden"));
	if (c.features?.support === false)
		qsa("[data-feature-support]").forEach((x) => x.classList.add("hidden"));
	const pageFeature = {
		register: "registration",
		armory: "armory",
		rankings: "rankings",
		guilds: "guilds",
		realm: "realm",
		shop: "shop",
	}[page];
	if (pageFeature && c.features?.[pageFeature] === false) {
		qs("main").innerHTML =
			`<section class="page-hero wrap"><p class="eyebrow">MODULE DISABLED</p><h1>${esc(name)}</h1><p>This section is not enabled for this realm.</p></section>`;
	}
	if (c.maintenance?.active) {
		const banner = qs("#maintenance-banner");
		banner.textContent =
			c.maintenance.message ||
			"The realm is currently undergoing scheduled maintenance.";
		banner.classList.remove("hidden");
	}
	const analyticsURL = publicLink(c.analytics?.scriptUrl);
	if (analyticsURL && c.analytics?.domain && !document.querySelector("script[data-portal-analytics]")) {
		const script = document.createElement("script");
		script.src = analyticsURL; script.defer = true; script.dataset.domain = c.analytics.domain; script.dataset.portalAnalytics = "true";
		document.head.append(script);
	}
	if (c.navigation?.configured) {
		for (const [area, container] of [["primary", qs(".nav nav")], ["footer", qs(".footer-links")]]) {
			if (!container) continue;
			container.innerHTML = "";
			for (const item of (c.navigation.items || []).filter((entry) => entry.area === area)) {
				const link = document.createElement("a"); link.href = item.url; link.textContent = item.label;
				if (item.newTab) { link.target = "_blank"; link.rel = "noopener noreferrer"; }
				if (area === "primary") {
					const path = new URL(link.href, location.origin).pathname.replace(/\/$/, "") || "/", current = location.pathname.replace(/\/$/, "") || "/";
					if (new URL(link.href, location.origin).origin === location.origin && (current === path || (path !== "/" && current.startsWith(path + "/")))) link.setAttribute("aria-current", "page");
				}
				container.append(link);
			}
			container.dataset.managedNavigation = "true";
		}
	}
	if (c.news?.length && qs("#news-grid")) {
		const section = qs("#news-section"),
			grid = qs("#news-grid");
		section.classList.remove("hidden");
		grid.innerHTML = "";
		for (const item of [...c.news].sort(
			(a, b) => Number(Boolean(b.featured)) - Number(Boolean(a.featured)),
		)) {
			const article = document.createElement("article");
			article.className = "news-card";
			if (item.featured) {
				const badge = document.createElement("span");
				badge.className = "featured-badge";
				badge.textContent = "FEATURED";
				article.append(badge);
			}
			const date = document.createElement("time");
			date.textContent = item.publishAt
				? new Date(item.publishAt).toLocaleString()
				: item.date || "";
			const title = document.createElement("h3");
			title.textContent = item.title;
			const summary = document.createElement("p");
			summary.textContent = item.summary || "";
			article.append(date, title, summary);
			const link = item.slug && item.body
				? "/news/" + encodeURIComponent(item.slug)
				: publicLink(item.url);
			if (link) {
				const more = document.createElement("a");
				more.href = link;
				more.rel = "noreferrer";
				more.textContent = c.translations?.["news.readMore"] || "Read more →";
				article.append(more);
			}
			grid.append(article);
		}
	}
	const realmPicker = qs(".realm-switcher"),
		realmSelect = qs("#realm-switcher");
	if (
		realmPicker &&
		realmSelect &&
		Array.isArray(c.realms) &&
		c.realms.length > 1
	) {
		realmSelect.innerHTML = "";
		c.realms.forEach((realm) => {
			const option = document.createElement("option");
			option.value = realm.key;
			option.textContent = realm.name;
			option.selected = realm.key === c.realmKey;
			realmSelect.append(option);
		});
		realmSelect.onchange = () => {
			const target = new URL(location.href);
			target.searchParams.set("realm", realmSelect.value);
			if (
				["/reset-password", "/verify-email", "/setup"].includes(
					location.pathname,
				)
			) {
				target.pathname = "/";
				target.search = "?realm=" + encodeURIComponent(realmSelect.value);
			}
			target.hash = "";
			location.href = target.href;
		};
		realmPicker.classList.remove("hidden");
	}
}
const publicConfigPromise = api("/api/public-config");
publicConfigPromise.then(applyPublicConfig).catch(() => {});
Promise.all([api("/api/pages"), publicConfigPromise]).then(([{pages}, config]) => {
	if (config.navigation?.configured) return;
	for (const item of pages || []) {
		if (item.showNavigation) { const link=document.createElement("a");link.href="/pages/"+encodeURIComponent(item.slug);link.textContent=item.title;qs(".nav nav")?.append(link); }
		if (item.showFooter) { const link=document.createElement("a");link.href="/pages/"+encodeURIComponent(item.slug);link.textContent=item.title;qs(".footer-links")?.append(link); }
	}
}).catch(() => {});
const setupStatePromise = api("/api/setup/status");
if (page !== "setup")
	setupStatePromise
		.then((s) => {
			if (s.required) location.replace("/setup");
		})
		.catch(() => {});
function toast(message) {
	const el = qs("#toast");
	el.textContent = message;
	el.classList.add("show");
	setTimeout(() => el.classList.remove("show"), 2600);
}
function askAction({ title = "Confirm action", message = "", label = "Confirmation", defaultValue = "", expected = "", input = true, confirmText = "Continue" }) {
	const dialog = qs("#action-dialog"), form = qs("#action-dialog-form"), field = qs("#action-dialog-field"), value = form.elements.value;
	qs("#action-dialog-title").textContent = title;
	qs("#action-dialog-message").textContent = message;
	field.firstChild.textContent = label;
	field.classList.toggle("hidden", !input);
	value.required = input;
	value.value = defaultValue;
	qs('button[type="submit"]', form).textContent = confirmText;
	setMessage(form, "");
	return new Promise((resolve) => {
		let settled = false;
		const finish = (result) => {
			if (settled) return;
			settled = true;
			dialog.close();
			resolve(result);
		};
		form.onsubmit = (event) => {
			event.preventDefault();
			if (expected && value.value !== expected) {
				setMessage(form, `Type ${expected} exactly to continue.`);
				return;
			}
			finish(input ? value.value : true);
		};
		qs(".dialog-close", dialog).onclick = () => finish(null);
		qs("[data-action-cancel]", dialog).onclick = () => finish(null);
		dialog.oncancel = (event) => { event.preventDefault(); finish(null); };
		dialog.showModal();
		(input ? value : qs('button[type="submit"]', form)).focus();
	});
}
function submitJSON(form, path, onSuccess) {
	form?.addEventListener("submit", async (e) => {
		e.preventDefault();
		const button = qs("button[type=submit]", form);
		button.disabled = true;
		setMessage(form, "");
		try {
			const data = Object.fromEntries(new FormData(form));
			const result = await api(path, {
				method: "POST",
				body: JSON.stringify(data),
			});
			await onSuccess(result);
			if (result?.requestId) {
				form.dataset.lastRequestId = result.requestId;
				const message = qs(".form-message", form);
				if (message?.textContent && !message.textContent.includes(result.requestId))
					message.append(document.createTextNode(` Request ${result.requestId}.`));
			}
		} catch (err) {
			setMessage(form, err.message);
		} finally {
			button.disabled = false;
		}
	});
	if(form)form.dataset.initialized="true";
}
function initial(name) {
	return (name || "?").slice(0, 1).toUpperCase();
}
function resolveItemIcon(img, itemId) {
	if (!itemId) return;
	const key = "item-icon-" + itemId,
		cached = localStorage.getItem(key);
	if (cached) {
		img.src = iconBase + cached + ".jpg";
		return;
	}
	fetch(`https://nether.wowhead.com/tooltip/item/${itemId}?dataEnv=8&locale=0`)
		.then((r) => (r.ok ? r.json() : Promise.reject()))
		.then((data) => {
			if (data.icon) {
				localStorage.setItem(key, data.icon);
				img.src = iconBase + data.icon + ".jpg";
			}
		})
		.catch(() => {});
}
function useLocalItemFallback(img) {
	img.onerror = () => {
		img.onerror = null;
		img.src = localItemIcon;
	};
}
function characterCard(c) {
	const a = document.createElement("article");
	a.className = "character-card";
	a.dataset.name = c.name;
	a.innerHTML = `<div class="avatar">${initial(c.name)}</div><div><h3></h3><p>${races[c.race] || "Unknown"} · Level ${c.level} ${classes[c.class] || "Hero"}</p><p>${c.guild ? "‹" + esc(c.guild) + "›" : "Unaffiliated"}</p></div>${c.online ? '<i class="online" title="Online"></i>' : ""}`;
	qs("h3", a).textContent = c.name;
	return a;
}

async function hydrateNav() {
	try {
		const [me, cfg] = await Promise.all([api("/api/me"), publicConfigPromise]);
		qsa(".signed-out").forEach((x) => x.classList.add("hidden"));
		qsa(".signed-in").forEach((x) => x.classList.remove("hidden"));
		if (me.permissions?.length && cfg.features?.admin !== false)
			qsa(".admin-link").forEach((x) => x.classList.remove("hidden"));
		api("/api/notifications").then((data) => {
			for (const badge of qsa("#notification-badge,#account-notification-badge")) {
				badge.textContent = data.unread;
				badge.classList.toggle("hidden", !data.unread);
			}
		}).catch(() => {});
	} catch {}
}
hydrateNav();

if (page === "tools") {
	import("/js/tools.js").then(({ mountTools }) => mountTools({ api, toast, resolveItemIcon, useLocalItemFallback })).catch((error) => { console.error("tools initialization failed", error); toast(error.message); });
}
if (page === "home") {
	import("/js/home.js").then(({ mountHome }) => mountHome({ api, toast, publicConfigPromise })).catch((error) => { console.error("home initialization failed", error); toast(error.message); });
}
if (page === "play") {
	import("/js/play.js").then(({ mountPlay }) => mountPlay({ api })).catch((error) => { console.error("play initialization failed", error); toast(error.message); });
}
if (page === "community") {
	import("/js/community.js").then(({ mountCommunity }) => mountCommunity({ api })).catch((error) => { console.error("community initialization failed", error); toast(error.message); });
}
if (page === "vote") {
	import("/js/vote.js").then(({ mountVote }) => mountVote({ api, toast })).catch((error) => { console.error("vote initialization failed", error); toast(error.message); });
}
if (page === "register") {
	import("/js/register.js").then(({ mountRegister }) => mountRegister({ api, publicConfigPromise, submitJSON })).catch((error) => { console.error("register initialization failed", error); toast(error.message); });
}
if (page === "setup") {
	import("/js/setup.js").then(({ mountSetup }) => mountSetup({ api, initial })).catch((error) => { console.error("setup initialization failed", error); toast(error.message); });
}
if (page === "login") {
	import("/js/login.js").then(({ mountLogin }) => mountLogin({ api, submitJSON })).catch((error) => { console.error("login initialization failed", error); toast(error.message); });
}
if (page === "forgot-password")
	submitJSON(qs("#forgot-form"), "/api/auth/password/request", (result) =>
		setMessage(qs("#forgot-form"), result.message, true),
	);
if (page === "reset-password") {
	import("/js/reset-password.js").then(({ mountResetPassword }) => mountResetPassword({ api })).catch((error) => { console.error("reset-password initialization failed", error); toast(error.message); });
}
if (page === "verify-email") {
	import("/js/verify-email.js").then(({ mountVerifyEmail }) => mountVerifyEmail({ api })).catch((error) => { console.error("verify-email initialization failed", error); toast(error.message); });
}
if (page === "armory") {
	import("/js/armory.js")
		.then(({ mountArmory }) => mountArmory({ api, toast, classes, races, slots, qualityNames, statNames, iconBase, localItemIcon, useLocalItemFallback, initial }))
		.catch((error) => { console.error("Armory initialization failed", error); toast(`Could not initialize armory: ${error.message}`); });
}

if (page === "rankings") {
	import("/js/rankings.js")
		.then(({ mountRankings }) => mountRankings({ api, classes, publicConfigPromise }))
		.catch((error) => { console.error("Rankings initialization failed", error); toast(`Could not initialize rankings: ${error.message}`); });
}

if (page === "shop") {
	import("/js/shop.js")
		.then(({ mountShop }) => mountShop({ api, toast, classes, classSetPreviewItems, iconBase, localItemIcon, useLocalItemFallback, resolveItemIcon }))
		.catch((error) => { console.error("Shop initialization failed", error); toast(`Could not initialize shop: ${error.message}`); });
}

if (page === "account") {
	import("/js/account.js")
		.then(({ mountAccount }) => mountAccount({ page, api, toast, publicConfigPromise, classes, submitJSON, askAction, initial, requestStepUp, base64URLToBuffer, publicKeyCredentialJSON }))
		.catch((error) => { console.error("Account initialization failed", error); toast(`Could not initialize account: ${error.message}`); });
}

if (page === "admin") {
	import("/js/account-admin.js")
		.then(({ mountAccountAdmin }) => mountAccountAdmin({ page, api, toast, publicConfigPromise, classes, slots, localItemIcon, useLocalItemFallback, resolveItemIcon, submitJSON, askAction }))
		.catch((error) => { console.error("Admin initialization failed", error); toast(`Could not initialize administration: ${error.message}`); });
}

if (page === "realm")
	import("/js/realm.js").then(({ mountRealm }) => mountRealm({ api, publicConfigPromise })).catch((error) => { console.error("Realm initialization failed", error); toast(error.message); });

if (page === "news") {
	import("/js/news.js").then(({ mountNews }) => mountNews({ api })).catch((error) => { console.error("news initialization failed", error); toast(error.message); });
}

if (page === "content-page") {
	import("/js/content-page.js").then(({ mountContentPage }) => mountContentPage({ api })).catch((error) => { console.error("content-page initialization failed", error); toast(error.message); });
}

if (page === "tracker") {
	import("/js/tracker.js").then(({ mountTracker }) => mountTracker({ api, toast })).catch((error) => { console.error("tracker initialization failed", error); toast(error.message); });
}

if (page === "guilds") {
	import("/js/guilds.js").then(({ mountGuilds }) => mountGuilds({ api, classes, initial })).catch((error) => { console.error("Guilds initialization failed", error); toast(error.message); });
}
