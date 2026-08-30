import {esc, qs, qsa, setMessage} from "/js/ui.js";
import {prepareAdminLayout} from "/js/admin-layout.js";

export function mountAccountAdmin(context) {
	const { page, api, toast, publicConfigPromise, classes, slots, localItemIcon, useLocalItemFallback, resolveItemIcon, submitJSON, askAction } = context;
	const mutationSuccess = (form, message, result) => {
		const request = result?.requestId ? ` Request ${result.requestId}.` : "";
		if (result?.requestId) form.dataset.lastRequestId = result.requestId;
		setMessage(form, message + request, true);
	};
	function renderLaunchReadiness(config) {
		const host = qs('[data-admin-view="overview"]');
		if (!host) return;
		let panel = qs("#launch-readiness");
		if (!panel) {
			panel = document.createElement("article");
			panel.id = "launch-readiness";
			panel.className = "account-panel launch-readiness";
			host.append(panel);
		}
		const checks = [
			["Client download", Boolean(config.downloadUrl), "/admin/content/downloads", "Publish a versioned client with checksum and installation notes."],
			["Discord or community", Boolean(config.communityUrl), "/admin/settings/integrations", "Add the official community destination."],
			["Terms of service", Boolean(config.termsUrl), "/admin/settings/integrations", "Publish operator-reviewed server and account terms."],
			["Privacy policy", Boolean(config.privacyUrl), "/admin/settings/integrations", "Explain retained account, session, moderation, and payment data."],
			["Security contact", Boolean(config.securityContactUrl), "/security", "Publish a private channel for responsible vulnerability reports."],
			["Realm differentiators", Boolean(String(config.homeFeatures || "").trim()), "/admin/settings/homepage", "Tell players what makes this server different."],
			["Progression roadmap", Boolean(String(config.homeProgression || "").trim()), "/admin/settings/homepage", "Publish current, completed, and upcoming content stages."],
			["News", Boolean(config.news?.length), "/admin/content/news", "Publish at least one current realm update."],
			["Password recovery", Boolean(config.passwordResetEnabled), "/admin/monitoring", "Configure SMTP before public launch."],
		];
		const complete = checks.filter(([, ready]) => ready).length;
		panel.innerHTML = `<div class="panel-title"><div><p class="eyebrow">PUBLIC LAUNCH</p><h3>Realm readiness</h3><p>${complete} of ${checks.length} player-trust essentials configured.</p></div><strong>${Math.round(complete / checks.length * 100)}%</strong></div><div class="readiness-meter"><i style="width:${complete / checks.length * 100}%"></i></div><div class="readiness-list">${checks.map(([label, ready, href, help]) => `<a href="${href}" class="${ready ? "is-ready" : "needs-action"}"><span>${ready ? "✓" : "!"}</span><b>${esc(label)}</b><small>${esc(ready ? "Configured" : help)}</small></a>`).join("")}</div>`;
	}
	let adminAccountsPromise, adminPermissions = new Set(), passwordResetEnabled = false;
	async function loadAccounts() {
		if (!adminAccountsPromise) adminAccountsPromise = import("/js/admin-accounts.js").then(({createAdminAccounts}) => createAdminAccounts({api, classes, askAction, toast, adminCan, passwordResetEnabled}));
		return (await adminAccountsPromise).load();
	}
	const adminCan = (permission) => adminPermissions.has(permission);
	let adminAnalyticsPromise;
	async function loadAdminAnalytics() {
		if (!adminCan("overview")) return;
		if (!adminAnalyticsPromise) adminAnalyticsPromise = import("/js/admin-analytics.js").then(({createAdminAnalytics}) => createAdminAnalytics({api}));
		return (await adminAnalyticsPromise).load();
	}
	let adminAuditPromise;
	async function loadAdminAudit() {
		if (!adminCan("audit")) return;
		if (!adminAuditPromise) adminAuditPromise = import("/js/admin-audit.js").then(({createAdminAudit}) => createAdminAudit({api, askAction, toast}));
		return (await adminAuditPromise).load();
	}
	let adminModerationPromise;
	async function loadAdminModeration() {
		if (!adminCan("moderation")) return;
		if (!adminModerationPromise) adminModerationPromise = import("/js/admin-moderation.js").then(({createAdminModeration}) => createAdminModeration({api, askAction, toast}));
		return (await adminModerationPromise).load();
	}
	let adminStaffPromise;
	async function loadAdminStaff() {
		if (!adminStaffPromise) adminStaffPromise = import("/js/admin-staff.js").then(({createAdminStaff}) => createAdminStaff({api, askAction, toast}));
		return (await adminStaffPromise).load();
	}
	let adminPaymentsPromise;
	async function mountPaymentManager() {
		if (!adminPaymentsPromise) adminPaymentsPromise = import("/js/admin-payments.js");
		const {mountAdminPayments} = await adminPaymentsPromise;
		mountAdminPayments({api, askAction});
	}
	let adminRealmConfigPromise;
	async function loadRealmConfig() {
		if (!adminRealmConfigPromise) adminRealmConfigPromise = import("/js/admin-realm-config.js").then(({createAdminRealmConfig}) => createAdminRealmConfig({api, askAction, toast}));
		return (await adminRealmConfigPromise).load();
	}
	let adminOrdersPromise;
	async function loadAdmin() {
		if (!adminCan("overview")) return;
		if (!adminOrdersPromise) adminOrdersPromise = import("/js/admin-orders.js").then(({createAdminOrders}) => createAdminOrders({api, askAction, toast}));
		return (await adminOrdersPromise).load();
	}
	let adminPrivacyPromise;
	async function loadAdminPrivacy() {
		if (!adminCan("players")) return;
		if (!adminPrivacyPromise) adminPrivacyPromise = import("/js/admin-privacy.js").then(({createAdminPrivacy}) => createAdminPrivacy({api, askAction, toast}));
		return (await adminPrivacyPromise).load();
	}
	let adminMonitoringPromise;
	async function loadAdminMonitoring() {
		if (!adminCan("monitoring")) return;
		if (!adminMonitoringPromise) adminMonitoringPromise = import("/js/admin-monitoring.js").then(({createAdminMonitoring}) => createAdminMonitoring({api, askAction, publicConfigPromise}));
		return (await adminMonitoringPromise).load();
	}
	let navigateAdmin;
	let adminCatalogPromise;
	async function adminCatalog() {
		if (!adminCatalogPromise) {
			adminCatalogPromise = import("/js/admin-catalog.js").then(({createAdminCatalog}) => createAdminCatalog({
				api,
				askAction,
				classes,
				localItemIcon,
				mutationSuccess,
				navigateAdmin,
				resolveItemIcon,
				toast,
				useLocalItemFallback,
				adminCan,
			}));
		}
		return adminCatalogPromise;
	}
	let adminContentAssetsPromise;
	let adminNewsPromise;
	let adminPagesPromise;
	let adminEventsPromise;
	let adminDownloadsPromise;
	let adminCommunityPromise;
	let adminActionsPromise;
	let adminCreditsPromise;
	async function mountAdminCredits() {
		if (!adminCreditsPromise) adminCreditsPromise = import("/js/admin-credits.js");
		(await adminCreditsPromise).mountAdminCredits({api});
	}
	async function mountAdminAction(kind) {
		if (!adminActionsPromise) adminActionsPromise = import("/js/admin-actions.js");
		const actions = await adminActionsPromise;
		const context = {api, askAction, mutationSuccess};
		if (kind === "moderation") actions.mountModerationAction(context);
		else actions.mountRealmOperationAction(context);
	}
	async function loadAdminCommunity() {
		if (!adminCommunityPromise) adminCommunityPromise = import("/js/admin-community.js").then(({createAdminCommunity}) => createAdminCommunity({api}));
		return (await adminCommunityPromise).load();
	}
	async function loadAdminNews() {
		if (!adminNewsPromise) adminNewsPromise = import("/js/admin-news.js").then(({createAdminNews}) => createAdminNews({api, askAction}));
		return (await adminNewsPromise).load();
	}
	async function loadAdminPages() {
		if (!adminPagesPromise) adminPagesPromise = import("/js/admin-pages.js").then(({createAdminPages}) => createAdminPages({api, askAction}));
		return (await adminPagesPromise).load();
	}
	async function loadAdminEvents() {
		if (!adminEventsPromise) adminEventsPromise = import("/js/admin-events.js").then(({createAdminEvents}) => createAdminEvents({api, askAction}));
		return (await adminEventsPromise).load();
	}
	async function loadAdminDownloads() {
		if (!adminDownloadsPromise) adminDownloadsPromise = import("/js/admin-downloads.js").then(({createAdminDownloads}) => createAdminDownloads({api, askAction}));
		return (await adminDownloadsPromise).load();
	}
	async function contentAssets() {
		if (!adminContentAssetsPromise) adminContentAssetsPromise = import("/js/admin-content-assets.js").then(({createAdminContentAssets}) => createAdminContentAssets({api, askAction, toast}));
		return adminContentAssetsPromise;
	}
	async function loadAdminMedia() {
		if (!adminCan("content")) return;
		return (await contentAssets()).loadMedia();
	}
	async function loadAdminNavigation() {
		if (!adminCan("content")) return;
		return (await contentAssets()).loadNavigation();
	}
	let adminSettingsPromise;
	async function loadAdminSettings() {
		if (!adminSettingsPromise) adminSettingsPromise = import("/js/admin-settings.js").then(({createAdminSettings}) => createAdminSettings({api, mutationSuccess, toast}));
		return (await adminSettingsPromise).load();
	}
	let adminConsolePromise, adminGMConsoleEnabled = false;
	async function loadAdminConsole() {
		if (!adminGMConsoleEnabled) return;
		if (!adminConsolePromise) adminConsolePromise = import("/js/admin-console.js");
		const {mountAdminConsole} = await adminConsolePromise;
		mountAdminConsole({api});
	}
	let adminSupportPromise;
	async function loadAdminSupport() {
		if (!adminSupportPromise) adminSupportPromise = import("/js/admin-support.js").then(({createAdminSupport}) => createAdminSupport({api, askAction, toast}));
		return (await adminSupportPromise).load();
	}
	let adminTransfersPromise;
	async function loadAdminTransfers() {
		if (!adminTransfersPromise) adminTransfersPromise = import("/js/admin-transfers.js").then(({createAdminTransfers}) => createAdminTransfers({api, askAction, toast}));
		return (await adminTransfersPromise).load();
	}
	if (page === "admin") {
		let adminRouteToken = 0;
		function parseAdminRoute(path = location.pathname) {
			const clean = path.replace(/\/+$/, "") || "/admin";
			const content = clean.match(/^\/admin\/content\/(news|pages|media|community|events|downloads|resources|voting)$/);
			if (content) return { view: "content", subview: content[1] };
			const settings = clean.match(/^\/admin\/settings\/(branding|homepage|realm|integrations|features|maintenance)$/);
			if (settings) return { view: "settings", subview: settings[1] };
			const editor = clean.match(/^\/admin\/catalog\/(\d+)\/edit$/);
			if (editor) return { view: "catalog", editor: Number(editor[1]) };
			if (clean === "/admin/catalog/new")
				return { view: "catalog", editor: "new" };
			const routes = {
				"/admin": { view: "overview" },
				"/admin/monitoring": { view: "monitoring" },
				"/admin/catalog": { view: "catalog" },
				"/admin/players/accounts": { view: "players", subview: "accounts" },
				"/admin/players/credits": { view: "players", subview: "credits" },
				"/admin/players/moderation": { view: "players", subview: "moderation" },
				"/admin/players/transfers": { view: "players", subview: "transfers" },
				"/admin/realm/operations": {
					view: "realm-admin",
					subview: "operations",
				},
				"/admin/realm/configuration": { view: "realm-admin", subview: "configuration" },
				"/admin/realm/console": { view: "realm-admin", subview: "console" },
				"/admin/content": { view: "content", subview: "news", canonical: "/admin/content/news" },
				"/admin/support": { view: "support-admin" },
				"/admin/settings": { view: "settings", subview: "branding", canonical: "/admin/settings/branding" },
				"/admin/staff": { view: "staff" },
				"/admin/audit": { view: "audit" },
				"/admin/privacy": { view: "privacy" },
			};
			return routes[clean] || { view: "overview", canonical: "/admin" };
		}
		async function loadAdminRouteData(route) {
			switch (route.view) {
				case "overview": await Promise.all([loadAdmin(), loadAdminAnalytics()]); break;
				case "monitoring": await loadAdminMonitoring(); break;
				case "catalog": await (await adminCatalog()).load(); break;
				case "settings": await loadAdminSettings(); break;
				case "content": await Promise.all([route.subview === "news" ? loadAdminNews() : Promise.resolve(), route.subview === "events" ? loadAdminEvents() : Promise.resolve(), route.subview === "downloads" ? loadAdminDownloads() : Promise.resolve(), route.subview === "media" ? loadAdminMedia() : Promise.resolve(), route.subview === "community" ? loadAdminCommunity() : Promise.resolve(), route.subview === "pages" ? Promise.all([loadAdminPages(), loadAdminNavigation()]) : Promise.resolve()]); break;
				case "players": if (route.subview === "accounts") await loadAccounts(); else if (route.subview === "credits") await mountAdminCredits(); else if (route.subview === "moderation") await Promise.all([loadAdminModeration(), mountAdminAction("moderation")]); else if(route.subview === "transfers") await loadAdminTransfers(); break;
				case "realm-admin": if (route.subview === "configuration") await loadRealmConfig(); else if (route.subview === "console") await loadAdminConsole(); else if (route.subview === "operations") await mountAdminAction("realm"); break;
				case "support-admin": await loadAdminSupport(); break;
				case "staff": await loadAdminStaff(); break;
				case "audit": await loadAdminAudit(); break;
				case "privacy": await loadAdminPrivacy(); break;
			}
		}
		async function applyAdminRoute({ focus = false } = {}) {
			const route = parseAdminRoute(),
				token = ++adminRouteToken;
			let catalog;
			if (route.view === "catalog" && adminCan("commerce")) {
				catalog = await adminCatalog();
				catalog.mount();
				if (adminCan("commerce")) {
					catalog.mountMerchandising();
					await mountPaymentManager();
				}
			}
			if (route.view === "content" && route.subview === "resources") {
				const {mountAdminResources} = await import("/js/admin-resources.js");
				mountAdminResources({api, askAction, mutationSuccess});
			}
			if (route.view === "content" && route.subview === "voting") {
				const {mountAdminVoting} = await import("/js/admin-voting.js");
				mountAdminVoting({api, askAction, toast});
			}
			if (route.view === "realm-admin" && route.subview === "operations" && adminCan("realm")) {
				const {mountAdminCompetition} = await import("/js/admin-competition.js");
				mountAdminCompetition({api, askAction, adminCan, mutationSuccess});
			}
			qsa("[data-admin-route]").forEach((x) => { const active=x.dataset.adminRoute === route.view; x.classList.toggle("active",active); if(active)x.setAttribute("aria-current","page");else x.removeAttribute("aria-current"); });
			qsa("[data-admin-view]").forEach((x) =>
				x.classList.toggle("active", x.dataset.adminView === route.view),
			);
			qsa("[data-admin-subroute]").forEach((x) =>
				x.classList.toggle(
					"active",
					x.dataset.adminSubroute === route.subview &&
						x.closest("[data-admin-view]")?.dataset.adminView === route.view,
				),
			);
			qsa("[data-admin-subview]").forEach((x) =>
				x.classList.toggle(
					"active",
					x.dataset.adminSubview === route.subview &&
						x.closest("[data-admin-view]")?.dataset.adminView === route.view,
				),
			);
			const list = qs("#catalog-list"),
				editor = qs("#catalog-editor");
			if (route.view === "catalog") {
				const editing = route.editor !== undefined;
				list.classList.toggle("hidden", editing);
				editor.classList.toggle("hidden", !editing);
				if (route.editor === "new") catalog?.fill();
				else if (Number.isInteger(route.editor)) {
					await catalog?.open(route.editor);
					if (token !== adminRouteToken) return;
				}
			} else {
				list.classList.remove("hidden");
				editor.classList.add("hidden");
			}
			if (route.canonical) history.replaceState(null, "", route.canonical);
			const activeView = qs(`[data-admin-view="${route.view}"]`), heading = qs("h2", activeView), label = heading?.textContent || "Administration";
			document.title = `${label} — Administration`;
			const breadcrumb = qs("#admin-breadcrumb span:last-child"); if (breadcrumb) breadcrumb.textContent = label;
			if (focus && heading) { heading.tabIndex = -1; heading.focus({preventScroll:true}); }
			await loadAdminRouteData(route);
			qs(".admin-workspace")?.classList.remove("route-changing");
		}
		navigateAdmin = function (path, { replace = false } = {}) {
			const destination = new URL(path, location.origin), target = destination.pathname + destination.search;
			if (target !== location.pathname + location.search)
				(replace ? history.replaceState : history.pushState).call(
					history,
					null,
					"",
					target,
				);
			qs(".admin-workspace")?.classList.add("route-changing");
			applyAdminRoute({ focus: true });
		};
		document.addEventListener("click", (event) => {
			const link = event.target.closest('a[href^="/admin"]');
			if (
				!link ||
				event.defaultPrevented ||
				event.button !== 0 ||
				event.metaKey ||
				event.ctrlKey ||
				event.shiftKey ||
				event.altKey
			)
				return;
			event.preventDefault();
			navigateAdmin(link.getAttribute("href"));
		});
		window.addEventListener("popstate", () => applyAdminRoute({ focus: true }));
		prepareAdminLayout();
		Promise.all([api("/api/me"), publicConfigPromise])
			.then(([me, cfg]) => {
				adminPermissions = new Set(me.permissions || []);
				if (!(me.permissions || []).length || cfg.features?.admin === false)
					throw Object.assign(new Error("Staff access required"), {
						status: 403,
					});
				adminGMConsoleEnabled = Boolean(cfg.features?.gmConsole);
				passwordResetEnabled = Boolean(cfg.passwordResetEnabled);
				qsa("[data-permission]").forEach((link) =>
					link.classList.toggle("hidden", !adminCan(link.dataset.permission)),
				);
				const viewPermission = {
					overview: "overview", monitoring: "monitoring", catalog: "commerce",
					players: "players", "realm-admin": "realm", content: "content",
					"support-admin": "support", settings: "settings", staff: "admin",
					audit: "audit", privacy: "players",
				};
				const route = parseAdminRoute();
				const subviewPermission = route.view === "players"
					? {accounts: "players", credits: "commerce", moderation: "moderation", transfers: "players"}[route.subview]
					: route.view === "realm-admin"
						? {operations: "realm", configuration: "realm", console: "console"}[route.subview]
						: "";
				if (!adminCan(subviewPermission || viewPermission[route.view] || "admin")) {
					const first = qsa("[data-admin-route]").find((link) => !link.classList.contains("hidden"));
					if (first) history.replaceState(null, "", first.getAttribute("href"));
				}
				qs("#admin-access").classList.remove("hidden");
				renderLaunchReadiness(cfg);
				qsa("[data-admin-refresh]").forEach((button) => button.addEventListener("click", () => loadAdminRouteData(parseAdminRoute())));
				qs("[data-monitoring-refresh]").onclick = loadAdminMonitoring;
				applyAdminRoute();
			})
			.catch((e) => {
				console.error("Admin workspace initialization failed", e);
				if (e.status === 401) {
					location.href =
						"/login?next=" + encodeURIComponent(location.pathname);
					return;
				}
				if (e.status === 403) {
					qs("#admin-access").classList.add("hidden");
					qs("#admin-denied").classList.remove("hidden");
					return;
				}
				toast(`Could not initialize administration: ${e.message}`);
			});
	}

}
