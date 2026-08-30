import { mkdir, rm, writeFile } from "node:fs/promises";

const cdpURL = process.env.CDP_URL || "http://127.0.0.1:9222";
const portalURL = (process.env.PORTAL_BROWSER_URL || "http://host.docker.internal:8787").replace(/\/$/, "");
const loginURL = (process.env.PORTAL_BROWSER_LOGIN_URL || "http://127.0.0.1:8787").replace(/\/$/, "");
const outputDir = process.env.BROWSER_AUDIT_OUTPUT || "test-results/browser";

class CDP {
	constructor(socketURL) {
		this.id = 0;
		this.pending = new Map();
		this.events = [];
		this.socket = new WebSocket(socketURL);
	}
	async open() {
		await new Promise((resolve, reject) => {
			this.socket.addEventListener("open", resolve, { once: true });
			this.socket.addEventListener("error", reject, { once: true });
		});
		this.socket.addEventListener("message", ({ data }) => {
			const message = JSON.parse(data);
			if (message.id) {
				const pending = this.pending.get(message.id);
				if (!pending) return;
				this.pending.delete(message.id);
				if (message.error) pending.reject(new Error(message.error.message));
				else pending.resolve(message.result);
			} else if (message.method) this.events.push(message);
		});
	}
	call(method, params = {}) {
		const id = ++this.id;
		return new Promise((resolve, reject) => {
			this.pending.set(id, { resolve, reject });
			this.socket.send(JSON.stringify({ id, method, params }));
		});
	}
	close() { this.socket.close(); }
}

async function createBrowser() {
	const response = await fetch(`${cdpURL}/json/new?${encodeURIComponent(portalURL)}`, { method: "PUT" });
	if (!response.ok) throw new Error(`Could not create browser target: ${response.status}`);
	const target = await response.json();
	const browser = new CDP(target.webSocketDebuggerUrl);
	browser.targetId = target.id;
	await browser.open();
	await browser.call("Page.enable");
	await browser.call("Runtime.enable");
	await browser.call("Log.enable");
	await browser.call("Network.enable");
	await browser.call("Accessibility.enable");
	await browser.call("Emulation.setDeviceMetricsOverride", { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
	await browser.call("Emulation.setEmulatedMedia", { features: [{ name: "prefers-reduced-motion", value: "reduce" }] });
	return browser;
}

async function accessibilityTreeAudit(browser) {
	const { nodes = [] } = await browser.call("Accessibility.getFullAXTree");
	const interactiveRoles = new Set([
		"button", "checkbox", "combobox", "link", "menuitem", "menuitemcheckbox",
		"menuitemradio", "radio", "searchbox", "slider", "spinbutton", "switch",
		"tab", "textbox", "treeitem",
	]);
	const namelessInteractive = nodes
		.filter((node) => !node.ignored && interactiveRoles.has(node.role?.value) && !String(node.name?.value || "").trim())
		.map((node) => `${node.role.value}#${node.backendDOMNodeId || "?"}`)
		.slice(0, 20);
	const unnamedDialogs = nodes
		.filter((node) => !node.ignored && ["dialog", "alertdialog"].includes(node.role?.value) && !String(node.name?.value || "").trim())
		.map((node) => `${node.role.value}#${node.backendDOMNodeId || "?"}`)
		.slice(0, 20);
	return { namelessInteractive, unnamedDialogs };
}

async function evaluate(browser, expression) {
	const result = await browser.call("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
	if (result.exceptionDetails) throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
	return result.result.value;
}

async function navigate(browser, route) {
	await browser.call("Page.navigate", { url: portalURL + route });
	for (let attempt = 0; attempt < 80; attempt++) {
		if (await evaluate(browser, "document.readyState === 'complete'")) break;
		await new Promise((resolve) => setTimeout(resolve, 50));
	}
	await new Promise((resolve) => setTimeout(resolve, 350));
}

async function waitFor(browser, expression, label, attempts = 50) {
	for (let attempt = 0; attempt < attempts; attempt++) {
		if (await evaluate(browser, expression)) return;
		await new Promise((resolve) => setTimeout(resolve, 100));
	}
	const diagnostics = browser.events
		.filter((event) => ["Runtime.exceptionThrown", "Runtime.consoleAPICalled", "Log.entryAdded"].includes(event.method))
		.slice(-5)
		.map((event) => event.params.exceptionDetails?.exception?.description || event.params.exceptionDetails?.text || event.params.entry?.text || event.params.args?.map((arg) => arg.description || arg.value).join(" "))
		.filter(Boolean);
	const pageState = await evaluate(browser, `({path:location.pathname, denied:!document.querySelector('#admin-denied')?.classList.contains('hidden'), access:!document.querySelector('#admin-access')?.classList.contains('hidden'), products:document.querySelectorAll('#admin-products tr').length, merchandising:Boolean(document.querySelector('#shop-merchandising')), body:document.body.innerText.slice(-500)})`);
	throw new Error(`Timed out waiting for ${label}\nPage state: ${JSON.stringify(pageState)}${diagnostics.length ? `\nBrowser diagnostics:\n${diagnostics.join("\n")}` : ""}`);
}

const accessibilityAudit = `(() => {
	const visible = (element) => element.getClientRects().length > 0 && getComputedStyle(element).visibility !== "hidden";
	const label = (element) => element.getAttribute("aria-label") || element.getAttribute("aria-labelledby") || element.closest("label")?.textContent?.trim() || (element.id && document.querySelector('label[for="' + CSS.escape(element.id) + '"]')?.textContent?.trim());
	const duplicateIds = [...document.querySelectorAll("[id]")].map((x) => x.id).filter((id, index, ids) => ids.indexOf(id) !== index);
	const controls = [...document.querySelectorAll("input:not([type=hidden]), select, textarea")].filter(visible).filter((x) => !label(x)).map((x) => x.outerHTML.slice(0, 100));
	const namelessButtons = [...document.querySelectorAll("button")].filter(visible).filter((x) => !(x.textContent.trim() || x.getAttribute("aria-label") || x.getAttribute("title"))).map((x) => x.outerHTML.slice(0, 100));
	const namelessLinks = [...document.querySelectorAll("a[href]")].filter(visible).filter((x) => !(x.textContent.trim() || x.getAttribute("aria-label") || x.querySelector("img[alt]"))).map((x) => x.outerHTML.slice(0, 100));
	const missingAlt = [...document.querySelectorAll("img:not([alt])")].filter(visible).map((x) => x.src);
	const brokenTabs = [...document.querySelectorAll('[role="tab"]')].filter((tab) => {
		const panel = document.getElementById(tab.getAttribute("aria-controls") || "");
		return !tab.textContent.trim() || !panel || panel.getAttribute("role") !== "tabpanel" || panel.getAttribute("aria-labelledby") !== tab.id;
	}).map((x) => x.outerHTML.slice(0, 120));
	const invalidTablists = [...document.querySelectorAll('[role="tablist"]')].filter(visible).filter((list) => list.querySelectorAll('[role="tab"][aria-selected="true"]').length !== 1).map((x) => x.outerHTML.slice(0, 120));
	const visibleRuntimeMessages = [...document.querySelectorAll('.empty')].filter(visible).map((x) => x.textContent.trim()).filter((text) => /is not defined|something went wrong|could not load/i.test(text));
	const rgb=(value)=>{
		if(!value)return null;
		if(value.startsWith('color(srgb')){const values=(value.slice(10).match(/[0-9.]+/g)||[]).map(Number);return values.length>=3?[values[0]*255,values[1]*255,values[2]*255,values[3]??1]:null}
		const values=(value.match(/[0-9.]+/g)||[]).map(Number);return values.length>=3?[values[0],values[1],values[2],values[3]??1]:null
	};
	const luminance=(color)=>{const values=color.map(value=>{value/=255;return value<=.03928?value/12.92:Math.pow((value+.055)/1.055,2.4)});return .2126*values[0]+.7152*values[1]+.0722*values[2]};
	const background=(element)=>{const layers=[];for(let node=element;node;node=node.parentElement){const style=getComputedStyle(node);if(style.backgroundImage&&style.backgroundImage!=='none')return null;const color=rgb(style.backgroundColor);if(color&&color[3]>0){layers.push(color);if(color[3]>=.999)break}}if(!layers.length)return null;let result=[255,255,255];for(const color of layers.reverse()){const alpha=color[3];result=result.map((channel,index)=>color[index]*alpha+channel*(1-alpha))}return result};
	const contrast=[...document.querySelectorAll('body *')].filter(visible).filter(element=>element.children.length===0&&element.textContent.trim()&&!['SCRIPT','STYLE'].includes(element.tagName)).map(element=>{const fg=rgb(getComputedStyle(element).color),bg=background(element);if(!fg||!bg)return null;const ratio=(Math.max(luminance(fg),luminance(bg))+.05)/(Math.min(luminance(fg),luminance(bg))+.05),style=getComputedStyle(element),large=parseFloat(style.fontSize)>=24||(parseFloat(style.fontSize)>=18&&Number(style.fontWeight)>=700);return ratio<(large?3:4.5)?element.tagName.toLowerCase()+'.'+element.className+': '+ratio.toFixed(2)+' '+element.textContent.trim().slice(0,50):null}).filter(Boolean).slice(0,20);
	return { title: document.title, h1: [...document.querySelectorAll("h1")].filter(visible).length, duplicateIds, controls, namelessButtons, namelessLinks, missingAlt, brokenTabs, invalidTablists, visibleRuntimeMessages, contrast, horizontalOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth };
})()`;

async function main() {
	await rm(outputDir, { recursive: true, force: true });
	await mkdir(outputDir, { recursive: true });
	const browser = await createBrowser();
	const failures = [];
	const reports = [];
	try {
		await navigate(browser, "/register");
		await waitFor(browser, "document.querySelector('#register-form')?.dataset.initialized === 'true'", "registration form");
		await evaluate(browser, `(() => { const form=document.querySelector('#register-form'); form.elements.username.value='BROWSERTEST'; form.elements.email.value='browser.audit@example.com'; form.elements.password.value='browserpass123'; form.querySelector('button[type="submit"]').click(); return true })()`);
		await waitFor(browser, "document.querySelector('#register-form .form-message')?.textContent.includes('Demo account created')", "completed registration");
		await waitFor(browser, "location.pathname === '/login'", "registration redirect");
		await navigate(browser, "/forgot-password");
		await waitFor(browser, "document.querySelector('#forgot-form')?.dataset.initialized === 'true'", "password recovery form");
		const recoveryResult = await evaluate(browser, `(async()=>{const response=await fetch('/api/auth/password/request',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({email:'browser.audit@example.com'})});return {ok:response.ok,status:response.status,body:await response.text()}})()`);
		if (!recoveryResult.ok || !recoveryResult.body.includes('accepted')) failures.push(`recovery journey: request failed (${recoveryResult.status}: ${recoveryResult.body})`);
		await navigate(browser, "/reset-password?token=browser-demo-token");
		await waitFor(browser, "document.querySelector('#reset-form')?.dataset.initialized === 'true'", "password reset form");
		const resetResult = await evaluate(browser, `(async()=>{const response=await fetch('/api/auth/password/reset',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({token:'browser-demo-token',password:'browserpass456'})});return {ok:response.ok,status:response.status,body:await response.text()}})()`);
		if (!resetResult.ok) failures.push(`recovery journey: reset failed (${resetResult.status}: ${resetResult.body})`);
		const login = await fetch(`${loginURL}/api/auth/login`, {method:"POST", headers:{"Content-Type":"application/json"}, body:JSON.stringify({username:"DEMO",password:"demo1234"})});
		if (!login.ok) throw new Error(`Mock login failed: ${login.status} ${await login.text()}`);
		const cookie = login.headers.get("set-cookie")?.split(";", 1)[0].split("=");
		if (!cookie || cookie.length < 2) throw new Error("Mock login did not return a session cookie");
		await browser.call("Network.setCookie", {name:cookie.shift(), value:cookie.join("="), url:portalURL, httpOnly:true, sameSite:"Lax"});

		const routes = ["/", "/play", "/armory/Arthoria", "/rankings", "/guilds", "/guilds/1", "/shop", "/community", "/vote", "/tools", "/tracker", "/tracker/1", "/security", "/account", "/account/rewards", "/account/transfers", "/account/security", "/admin", "/admin/monitoring", "/admin/catalog", "/admin/catalog/1/edit", "/admin/players/accounts", "/admin/players/credits", "/admin/players/moderation", "/admin/players/transfers", "/admin/audit", "/admin/privacy", "/admin/content/news", "/admin/content/pages", "/admin/content/media", "/admin/content/community", "/admin/content/events", "/admin/content/downloads", "/admin/content/resources", "/admin/content/voting", "/admin/settings/branding", "/admin/settings/homepage", "/admin/settings/realm", "/admin/settings/integrations", "/admin/settings/features", "/admin/settings/maintenance", "/admin/support", "/admin/staff", "/admin/realm/operations", "/admin/realm/configuration", "/admin/realm/console"];
		for (const route of routes) {
			browser.events.length = 0;
			await navigate(browser, route);
			if (route === "/") await waitFor(browser, "document.querySelectorAll('#home-features article').length >= 3 && document.querySelectorAll('#home-progression li').length >= 3 && Boolean(document.querySelector('#home-event h3')?.textContent.trim())", "homepage story content", 100);
			if (route === "/admin/catalog") await waitFor(browser, "document.querySelectorAll('#admin-products tr').length > 3 && Boolean(document.querySelector('#shop-merchandising'))", "loaded commerce workspace", 100);
			if (route === "/admin/content/community") await waitFor(browser, "document.querySelectorAll('#admin-community-issues .admin-row').length > 0", "loaded community triage", 100);
			if (route === "/admin/content/pages") await waitFor(browser, "!document.querySelector('#admin-pages')?.textContent.includes('Loading')", "loaded content pages", 100);
			if (route === "/admin/content/media") await waitFor(browser, "!document.querySelector('#admin-media')?.textContent.includes('Loading')", "loaded media library", 100);
			if (route === "/admin/content/events") await waitFor(browser, "!document.querySelector('#admin-events')?.textContent.includes('Loading')", "loaded realm events", 100);
			if (route === "/community") {
				await waitFor(browser, "document.querySelectorAll('#community-events .event-card').length > 0", "loaded community events", 100);
				const existingReservation = await evaluate(browser, "[...document.querySelectorAll('#community-events button')].some(button=>button.textContent.includes('Cancel reservation'))");
				if (existingReservation) {
					await evaluate(browser, `(() => {[...document.querySelectorAll('#community-events button')].find(button=>button.textContent.includes('Cancel reservation')).click();return true})()`);
					await waitFor(browser, "[...document.querySelectorAll('#community-events button')].some(button=>button.textContent.includes('Reserve'))", "deterministic event fixture", 100);
				}
			}
			if (route === "/admin/content/downloads") await waitFor(browser, "!document.querySelector('#admin-downloads')?.textContent.includes('Loading') && !document.querySelector('#admin-launcher-patches')?.textContent.includes('Loading')", "loaded client downloads and launcher patches", 100);
			await evaluate(browser, `(() => {
				let style=document.querySelector('#browser-audit-static-style');
				if(!style){style=document.createElement('style');style.id='browser-audit-static-style';style.textContent='*,*::before,*::after{transition:none!important;animation:none!important}';document.head.append(style)}
				document.documentElement.dataset.theme='dark';
				localStorage.setItem('portal-theme','dark');
				return true;
			})()`);
			const audit = await evaluate(browser, accessibilityAudit);
			const accessibilityTree = await accessibilityTreeAudit(browser);
			await evaluate(browser, `(async()=>{
				document.documentElement.dataset.theme='light';
				localStorage.setItem('portal-theme','light');
				await new Promise(requestAnimationFrame);
				document.getAnimations().forEach(animation=>animation.finish());
				await new Promise(requestAnimationFrame);
				return true;
			})()`);
			const lightAudit = await evaluate(browser, accessibilityAudit);
			await evaluate(browser, `(async()=>{
				document.documentElement.dataset.theme='dark';
				localStorage.setItem('portal-theme','dark');
				await new Promise(requestAnimationFrame);
				document.getAnimations().forEach(animation=>animation.finish());
				await new Promise(requestAnimationFrame);
				return true;
			})()`);
			reports.push({ route, ...audit, accessibilityTree, lightContrast:lightAudit.contrast });
			const routeFailures = [];
			if (!audit.title) routeFailures.push("missing document title");
			if (audit.h1 !== 1) routeFailures.push(`expected one h1, found ${audit.h1}`);
			if (audit.duplicateIds.length) routeFailures.push(`duplicate IDs: ${audit.duplicateIds.join(", ")}`);
			if (audit.controls.length) routeFailures.push(`unlabelled controls: ${audit.controls.join(" | ")}`);
			if (audit.namelessButtons.length) routeFailures.push(`nameless buttons: ${audit.namelessButtons.join(" | ")}`);
			if (audit.namelessLinks.length) routeFailures.push(`nameless links: ${audit.namelessLinks.join(" | ")}`);
			if (audit.missingAlt.length) routeFailures.push(`images without alt: ${audit.missingAlt.join(", ")}`);
			if (audit.brokenTabs.length) routeFailures.push(`broken tab relationships: ${audit.brokenTabs.join(" | ")}`);
			if (audit.invalidTablists.length) routeFailures.push(`tablists without one active tab: ${audit.invalidTablists.join(" | ")}`);
			if (accessibilityTree.namelessInteractive.length) routeFailures.push(`accessibility-tree controls without names: ${accessibilityTree.namelessInteractive.join(" | ")}`);
			if (accessibilityTree.unnamedDialogs.length) routeFailures.push(`accessibility-tree dialogs without names: ${accessibilityTree.unnamedDialogs.join(" | ")}`);
			if (audit.visibleRuntimeMessages.length) routeFailures.push(`visible runtime errors: ${audit.visibleRuntimeMessages.join(" | ")}`);
			if (process.env.STRICT_CONTRAST === "1" && audit.contrast.length) routeFailures.push(`low text contrast: ${audit.contrast.join(" | ")}`);
			if (process.env.STRICT_CONTRAST === "1" && lightAudit.contrast.length) routeFailures.push(`low light-theme text contrast: ${lightAudit.contrast.join(" | ")}`);
			if (audit.horizontalOverflow > 2) routeFailures.push(`horizontal overflow: ${audit.horizontalOverflow}px`);
			if (route === "/tracker" && !(await evaluate(browser, "document.querySelectorAll('.tracker-card').length > 0"))) routeFailures.push("tracker list did not render");
			if (route === "/" && !(await evaluate(browser, "document.querySelectorAll('#home-features article').length >= 3 && document.querySelectorAll('#home-progression li').length >= 3 && Boolean(document.querySelector('#home-event h3')?.textContent.trim())"))) routeFailures.push("homepage differentiators, progression, or next event did not render");
			if (route === "/") {
				const keyboardMenu = await evaluate(browser, `(() => { const trigger=document.querySelector('.account-menu-trigger');trigger.focus();trigger.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true}));const opened=trigger.getAttribute('aria-expanded')==='true',first=document.activeElement?.getAttribute('role')==='menuitem';document.activeElement?.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true}));const advanced=document.activeElement?.getAttribute('role')==='menuitem';document.activeElement?.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape',bubbles:true}));return {opened,first,advanced,closed:trigger.getAttribute('aria-expanded')==='false',returned:document.activeElement===trigger};})()`);
				if (!keyboardMenu.opened || !keyboardMenu.first || !keyboardMenu.advanced || !keyboardMenu.closed || !keyboardMenu.returned) routeFailures.push("account dropdown keyboard navigation failed");
			}
			if (route === "/play" && !(await evaluate(browser, "Boolean(document.querySelector('#download-center .download-card code[title=\"SHA-256\"]')) && document.querySelector('#download-center')?.textContent.includes('System requirements') && document.querySelector('#download-center')?.textContent.includes('VirusTotal') && document.querySelector('#download-center')?.textContent.includes('Changelog') && document.querySelector('#download-center')?.textContent.includes('Europe mirror')"))) routeFailures.push("download mirrors, verification, requirements, or release metadata did not render");
			if (route === "/tracker/1" && !(await evaluate(browser, "Boolean(document.querySelector('.tracker-issue h2')?.textContent.trim())"))) routeFailures.push("tracker detail did not render");
			if (route === "/admin/content/community" && !(await evaluate(browser, "document.querySelectorAll('#admin-community-issues .admin-row').length > 0"))) routeFailures.push("community triage queue did not render");
			if (route === "/community" && !(await evaluate(browser, "[...document.querySelectorAll('#community-events button')].some(button=>button.textContent.includes('Reserve')) && Boolean(document.querySelector('#event-registration-form')?.onsubmit) && document.querySelector('#community-events')?.textContent.includes('credits for confirmed attendance')"))) routeFailures.push("event registration or attendance reward details did not render");
			if (route === "/guilds/1" && !(await evaluate(browser, "Boolean(document.querySelector('.guild-recruitment h3')?.textContent.trim())"))) routeFailures.push("guild recruitment profile did not render");
			if (route === "/tools" && !(await evaluate(browser, "document.querySelectorAll('#resource-grid .tool-resource-card').length > 0"))) routeFailures.push("tools library did not render");
			if (route === "/tools") {
				const keyboardTabs = await evaluate(browser, `(() => { const first=document.querySelector('[data-tools-tab="resources"]');first.focus();first.dispatchEvent(new KeyboardEvent('keydown',{key:'End',bubbles:true}));const end=document.activeElement?.dataset.toolsTab==='talents'&&document.activeElement?.getAttribute('aria-selected')==='true'&&!document.querySelector('#tools-talents').classList.contains('hidden');document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Home',bubbles:true}));return {end,home:document.activeElement?.dataset.toolsTab==='resources'&&document.activeElement?.getAttribute('aria-selected')==='true'&&!document.querySelector('#tools-resources').classList.contains('hidden')};})()`);
				if (!keyboardTabs.end || !keyboardTabs.home) routeFailures.push("tools tab keyboard navigation failed");
			}
			if (route === "/account/security" && !(await evaluate(browser, "document.querySelectorAll('#identity-accounts .identity-account-row').length > 0"))) routeFailures.push("linked game accounts did not render");
			if (route === "/account/rewards" && !(await evaluate(browser, "document.querySelectorAll('#daily-reward-cycle .reward-day').length === 7 && document.querySelectorAll('#player-missions .mission-card').length >= 4 && Boolean(document.querySelector('#loyalty-level')?.textContent.includes('points'))"))) routeFailures.push("daily cycle, loyalty level, or monthly missions did not render");
			if (route === "/account" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/account.js')) && !performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/account-admin.js'))"))) routeFailures.push("account route controller was not isolated");
			if (route === "/admin" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/account-admin.js')) && !performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/account.js'))"))) routeFailures.push("admin route controller was not isolated");
			if (route === "/admin" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-analytics.js')) && performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-orders.js')) && document.querySelectorAll('#admin-analytics article').length >= 4 && document.querySelectorAll('#admin-orders .admin-row').length > 0"))) routeFailures.push("overview analytics, orders controller, or metrics did not render");
			if (route === "/admin" && (await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-support.js') || entry.name.includes('/js/admin-transfers.js') || entry.name.includes('/js/admin-audit.js') || entry.name.includes('/js/admin-staff.js') || entry.name.includes('/js/admin-privacy.js') || entry.name.includes('/js/admin-accounts.js') || entry.name.includes('/js/admin-monitoring.js') || entry.name.includes('/js/admin-realm-config.js') || entry.name.includes('/js/admin-moderation.js') || entry.name.includes('/js/admin-payments.js') || entry.name.includes('/js/admin-console.js') || entry.name.includes('/js/admin-content-assets.js') || entry.name.includes('/js/admin-settings.js') || entry.name.includes('/js/admin-news.js') || entry.name.includes('/js/admin-pages.js') || entry.name.includes('/js/admin-events.js') || entry.name.includes('/js/admin-downloads.js') || entry.name.includes('/js/admin-community.js') || entry.name.includes('/js/admin-actions.js') || entry.name.includes('/js/admin-credits.js'))"))) routeFailures.push("inactive route controller initialized on the dashboard");
			if (route === "/admin/content/news" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-news.js')) && Boolean(document.querySelector('#gm-news-form')?.onsubmit) && document.querySelectorAll('#admin-news .admin-row').length > 0"))) routeFailures.push("news controller or article list did not render");
			if (route === "/admin/content/events" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-events.js')) && Boolean(document.querySelector('#realm-event-form')?.onsubmit) && Boolean(document.querySelector('#realm-event-form [name=signupEnabled]')) && Boolean(document.querySelector('#realm-event-form [name=rewardCredits]')) && [...document.querySelectorAll('#admin-events button')].some(button=>button.textContent.includes('participants'))"))) routeFailures.push("events, registration, or attendance controller did not initialize on its route");
			if (route === "/admin/content/downloads" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-downloads.js')) && Boolean(document.querySelector('#gm-download-form')?.onsubmit) && Boolean(document.querySelector('#gm-download-form [name=virusTotalUrl]')) && Boolean(document.querySelector('#gm-download-form [name=requirements]')) && Boolean(document.querySelector('#gm-download-form [name=mirrors]')) && Boolean(document.querySelector('#launcher-patch-form')?.onsubmit) && document.querySelector('#admin-launcher-patches')?.textContent.includes('1.0.0')"))) routeFailures.push("downloads controller, mirror fields, or launcher patch controls did not initialize on its route");
			if (route.startsWith("/admin/settings/") && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-settings.js')) && Boolean(document.querySelector('#gm-settings-form')?.onsubmit)"))) routeFailures.push("settings controller did not initialize on its route");
			if (["/admin/content/media", "/admin/content/pages"].includes(route) && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-content-assets.js'))"))) routeFailures.push("content asset controller did not initialize on its route");
			if (route === "/admin/content/media" && !(await evaluate(browser, "Boolean(document.querySelector('#media-upload-form')?.onsubmit)"))) routeFailures.push("media upload controller did not initialize on its route");
			if (route === "/admin/content/pages" && !(await evaluate(browser, "Boolean(document.querySelector('#navigation-form')?.onsubmit)"))) routeFailures.push("navigation editor did not initialize on its route");
			if (route === "/admin/content/pages" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-pages.js')) && Boolean(document.querySelector('#content-page-form')?.onsubmit)"))) routeFailures.push("content pages controller did not initialize on its route");
			if (route === "/admin/catalog" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-payments.js')) && Boolean(document.querySelector('#payment-manager'))"))) routeFailures.push("payment controller did not initialize on the catalog route");
			if (route === "/admin/monitoring" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-monitoring.js')) && document.querySelectorAll('#admin-service-status .status-card').length > 0"))) routeFailures.push("monitoring controller or dependency status did not render");
			if (route === "/admin/realm/operations" && !(await evaluate(browser, "Boolean(document.querySelector('#arena-season-manager')) && performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-actions.js')) && Boolean(document.querySelector('#gm-operation-form')?.onsubmit)"))) routeFailures.push("realm operation controllers did not initialize");
			if (route === "/shop" && !(await evaluate(browser, "document.querySelectorAll('#shop-collections .collection-filter').length > 0 && document.querySelectorAll('#shop-collections .collection-preview i').length > 0 && document.querySelectorAll('.product-card').length > 3"))) routeFailures.push("shop collections, curated previews, or products did not render");
			if (route === "/shop" && !(await evaluate(browser, "document.querySelector('#shop-search')?.dataset.i18nPlaceholder === 'shop.searchPlaceholder'"))) routeFailures.push("shop discovery localization hooks are missing");
			if (route === "/account" && !(await evaluate(browser, "document.querySelector('[data-account-route=characters]')?.dataset.i18n === 'account.characters'"))) routeFailures.push("account navigation localization hooks are missing");
			if (route === "/shop" && !(await evaluate(browser, "Boolean(document.querySelector('#shop-search')) && Boolean(document.querySelector('#shop-sort')) && document.querySelectorAll('.product-card').length <= 12 && !document.querySelector('#shop-pagination')?.classList.contains('hidden')"))) routeFailures.push("shop discovery controls or pagination did not render");
			if (route === "/rankings" && !(await evaluate(browser, "document.querySelector('#arena-season-context')?.textContent.includes('2v2 bracket') && document.querySelector('#arena-season-context')?.textContent.includes('Rewards and eligibility') && document.querySelectorAll('#arena-ranking .class-token').length > 0 && document.querySelector('#raid-attempts .raid-composition')?.textContent.includes('Paladin')"))) routeFailures.push("arena season context, reward policy, class identity, or raid composition did not render");
			if (route === "/admin/catalog" && !(await evaluate(browser, "Boolean(document.querySelector('#shop-merchandising')) && document.querySelectorAll('#admin-products tr').length > 3"))) routeFailures.push("catalog merchandising tools did not render");
			if (route === "/admin/catalog" && !(await evaluate(browser, "Boolean(document.querySelector('#collection-form [name=imageUrl]'))"))) routeFailures.push("collection artwork control did not render");
			if (route === "/admin/catalog" && !(await evaluate(browser, `Boolean(document.querySelector('[data-pagination="catalogPage"]'))`))) routeFailures.push("catalog pagination did not render");
			if (route === "/admin/catalog") {
				const pagers = await evaluate(browser, `['paymentsPage','stockPage','giftCodesPage','couponsPage'].filter(key => !document.querySelector('[data-pagination="' + key + '"]'))`);
				if (pagers.length) routeFailures.push(`missing commerce pagination: ${pagers.join(", ")}`);
			}
			if (route === "/admin" && !(await evaluate(browser, "document.querySelectorAll('[data-pagination]').length >= 2"))) routeFailures.push("dashboard order or ledger pagination did not render");
			if (route === "/admin" && !(await evaluate(browser, "document.querySelectorAll('#launch-readiness .readiness-list a').length >= 8"))) routeFailures.push("public launch readiness checklist did not render");
			if (route === "/admin/settings/homepage" && !(await evaluate(browser, "Boolean(document.querySelector('[name=homeFeatures]')) && Boolean(document.querySelector('[name=homeProgression]'))"))) routeFailures.push("homepage differentiator or progression controls did not render");
			if (route === "/admin/settings/realm" && !(await evaluate(browser, "Boolean(document.querySelector('[name=arenaRewardPolicy]'))"))) routeFailures.push("arena reward policy control did not render");
			if (route === "/admin/settings/realm" && !(await evaluate(browser, "Number(document.querySelector('[name=transferSlaHours]')?.value) > 0"))) routeFailures.push("transfer SLA control did not render");
			if (route === "/account/transfers" && !(await evaluate(browser, "document.querySelector('#transfer-sla')?.textContent.includes('Expected staff review: within')"))) routeFailures.push("transfer SLA was not shown to the player");
			if (route === "/admin/settings/integrations" && !(await evaluate(browser, "Boolean(document.querySelector('[name=downloadUrl]')) && Boolean(document.querySelector('[name=communityUrl]'))"))) routeFailures.push("integration settings did not render");
			if (route === "/admin/settings/features" && !(await evaluate(browser, "Boolean(document.querySelector('.feature-switches'))"))) routeFailures.push("feature controls did not render");
			if (route === "/admin/settings/maintenance" && !(await evaluate(browser, "Boolean(document.querySelector('[name=maintenanceEnabled]'))"))) routeFailures.push("maintenance controls did not render");
			if (route === "/admin/support") {
				const pagers = await evaluate(browser, `['supportPage','applicationsPage'].filter(key => !document.querySelector('[data-pagination="' + key + '"]'))`);
				if (pagers.length) routeFailures.push(`missing support pagination: ${pagers.join(", ")}`);
				if (!(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-support.js'))"))) routeFailures.push("support controller did not load on its route");
			}
			if (route === "/admin/players/accounts" && !(await evaluate(browser, `Boolean(document.querySelector('[data-pagination="accountsPage"]')) && performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-accounts.js')) && Boolean(document.querySelector('#admin-accounts a[href^="/admin/players/moderation?"]')) && [...document.querySelectorAll('#admin-accounts button')].some(button=>button.textContent.includes('Revoke sessions')) && [...document.querySelectorAll('#admin-accounts button')].some(button=>button.textContent.includes('Require password reset'))`))) routeFailures.push("account pagination, route controller, moderation handoff, or security actions did not render");
			if (route === "/admin/players/credits" && !(await evaluate(browser, `performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-credits.js')) && Boolean(document.querySelector('#gm-credit-form')?.onsubmit)`))) routeFailures.push("credit grant controller did not initialize");
			if (route === "/admin/players/moderation" && !(await evaluate(browser, `Boolean(document.querySelector('#investigation-search-form')) && document.querySelector('#investigation-policy')?.textContent.includes('Raw IP addresses') && performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-moderation.js')) && performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-actions.js')) && Boolean(document.querySelector('#gm-moderation-form')?.onsubmit)`))) routeFailures.push("privacy-controlled moderation controller did not render");
			if (route === "/admin/players/transfers" && !(await evaluate(browser, `Boolean(document.querySelector('[data-pagination="transfersPage"]')) && performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-transfers.js'))`))) routeFailures.push("transfer pagination or route controller did not render");
			if (route === "/admin/audit" && !(await evaluate(browser, `Boolean(document.querySelector('[data-pagination="auditPage"]')) && performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-audit.js'))`))) routeFailures.push("audit pagination or route controller did not render");
			if (route === "/admin/privacy" && !(await evaluate(browser, `Boolean(document.querySelector('[data-pagination="privacyPage"]')) && performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-privacy.js'))`))) routeFailures.push("privacy pagination or route controller did not render");
			if (route === "/admin/staff" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-staff.js')) && document.querySelectorAll('#staff-list .admin-row').length > 0"))) routeFailures.push("staff controller or assignments did not render");
			if (route === "/admin/realm/configuration" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-realm-config.js')) && document.querySelectorAll('#realm-config-items tr').length > 0"))) routeFailures.push("realm configuration controller or values did not render");
			if (route === "/admin/realm/console" && !(await evaluate(browser, "Boolean(document.querySelector('#gm-command-disabled:not(.hidden)')) || (performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-console.js')) && Boolean(document.querySelector('#gm-command-enabled:not(.hidden)')) && Boolean(document.querySelector('#gm-command-form')?.onsubmit))"))) routeFailures.push("GM console capability state or route controller did not render");
			if (route === "/admin/catalog/1/edit" && !(await evaluate(browser, "document.querySelectorAll('#catalog-variants .catalog-variant-row').length === 2"))) routeFailures.push("product variants did not load in editor");
			if (route === "/admin/catalog/1/edit" && !(await evaluate(browser, "Boolean(document.querySelector('#catalog-advanced')) && document.querySelectorAll('#catalog-change-summary span').length === 4"))) routeFailures.push("catalog progressive disclosure or change summary did not render");
			if (route === "/admin/catalog/1/edit" && !(await evaluate(browser, "Boolean(document.querySelector('#catalog-editor [name=imageUrl]'))"))) routeFailures.push("catalog custom artwork control did not render");
			if (route === "/admin" && (await evaluate(browser, "Boolean(document.querySelector('#payment-manager,#shop-merchandising,#investigation-search-form,#arena-season-manager,#resource-manager')) || performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-voting.js') || entry.name.includes('/js/admin-catalog.js'))"))) routeFailures.push("inactive admin workspaces initialized eagerly");
			if (route === "/admin/catalog" && !(await evaluate(browser, "performance.getEntriesByType('resource').some(entry=>entry.name.includes('/js/admin-catalog.js'))"))) routeFailures.push("catalog controller did not load on its owning route");
			if (route === "/admin/content/news" && (await evaluate(browser, "Boolean(document.querySelector('#resource-manager'))"))) routeFailures.push("resource manager initialized outside its route");
			if (route === "/admin/content/resources" && !(await evaluate(browser, "Boolean(document.querySelector('#resource-manager'))"))) routeFailures.push("resource manager did not initialize on its route");
			if (route === "/armory/Arthoria") {
				const armory = await evaluate(browser, `(() => {
					const slots = [...document.querySelectorAll('.gear-slot[role="button"]')];
					const set = document.querySelector('.item-set-summary');
					if (slots.length < 3) return { error: 'fewer than three selectable gear slots' };
					slots[0].click(); slots[1].click(); slots[2].click();
					const selected = document.querySelectorAll('.gear-slot.selected').length;
					const columns = document.querySelectorAll('.item-comparison thead th').length;
					const visible = !document.querySelector('.item-comparison-panel')?.classList.contains('hidden');
					document.querySelector('[data-clear-comparison]')?.click();
					const activityTab=document.querySelector('[data-profile="activity"]');activityTab?.click();
					return { selected, columns, visible, cleared: document.querySelectorAll('.gear-slot.selected').length === 0, setText: set?.textContent || '', activity:document.querySelectorAll('#activity-panel .character-timeline > *').length, activityURL:location.search.includes('tab=activity') };
				})()`);
				if (armory.error) routeFailures.push(armory.error);
				if (!armory.setText?.includes("Lightsworn Battlegear") || !armory.setText?.includes("4 pieces")) routeFailures.push("equipped set bonuses did not render");
				if (armory.selected !== 3 || armory.columns !== 4 || !armory.visible) routeFailures.push("three-item comparison did not render");
				if (!armory.cleared) routeFailures.push("item comparison did not clear selected slots");
				if (!armory.activity || !armory.activityURL) routeFailures.push("character activity timeline or URL state did not render");
				const armoryTabs = await evaluate(browser, `(() => { const first=document.querySelector('[data-profile="gear"]');first.focus();first.dispatchEvent(new KeyboardEvent('keydown',{key:'End',bubbles:true}));const end=document.activeElement?.dataset.profile==='guild'&&document.activeElement?.getAttribute('aria-selected')==='true';document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Home',bubbles:true}));return {end,home:document.activeElement?.dataset.profile==='gear'&&document.activeElement?.getAttribute('aria-selected')==='true'};})()`);
				if (!armoryTabs.end || !armoryTabs.home) routeFailures.push("armory tab keyboard navigation failed");
			}
			if (route === "/vote" && !(await evaluate(browser, "document.querySelectorAll('#vote-campaigns .campaign-card').length > 0 && document.querySelectorAll('#vote-history .compact-rank-row').length > 0"))) routeFailures.push("transparent voting campaign or verified vote history did not render");
			if (route === "/account/rewards") {
				const rewards=await evaluate(browser, `({milestones:document.querySelectorAll('#referral-milestones .reward-milestone').length,recruits:document.querySelectorAll('#referral-activity .admin-row').length,share:Boolean(document.querySelector('#share-referral'))})`);
				if (rewards.milestones < 3 || rewards.recruits < 1 || !rewards.share) routeFailures.push("referral reward workflow did not render");
			}
			if (route === "/admin/content/voting" && !(await evaluate(browser, "Boolean(document.querySelector('#vote-campaign-form')) && document.querySelectorAll('#admin-vote-campaigns .admin-row').length > 0 && Boolean(document.querySelector('#mission-form')) && document.querySelectorAll('#admin-missions .admin-row').length >= 4"))) routeFailures.push("voting campaign or mission administration did not render");
			const runtimeErrors = browser.events.filter((event) => event.method === "Runtime.exceptionThrown");
			const consoleErrors = browser.events.filter((event) => event.method === "Log.entryAdded" && event.params.entry.level === "error" && event.params.entry.source === "javascript");
			if (runtimeErrors.length) routeFailures.push(`uncaught browser exceptions: ${runtimeErrors.length}`);
			if (consoleErrors.length) routeFailures.push(`browser console errors: ${consoleErrors.map((event) => event.params.entry.text).join(" | ")}`);
			const screenshot = await browser.call("Page.captureScreenshot", { format: "png", captureBeyondViewport: false });
			const name = route === "/" ? "home" : route.replace(/^\//, "").replaceAll("/", "-");
			await writeFile(`${outputDir}/${name}.png`, Buffer.from(screenshot.data, "base64"));
			if (routeFailures.length) failures.push(`${route}: ${routeFailures.join("; ")}`);
			else process.stdout.write(`PASS ${route}\n`);
		}
		// Stateful journeys run after route screenshots so mutations cannot make
		// later screenshots depend on execution order.
		const stepUp = await evaluate(browser, `(async()=>{const response=await fetch('/api/security/step-up',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({password:'demo1234'})});return {ok:response.ok,status:response.status,body:await response.text()}})()`);
		if (!stepUp.ok) failures.push(`staff journey: step-up failed (${stepUp.status}: ${stepUp.body})`);
		await navigate(browser, "/community");
		await waitFor(browser, "[...document.querySelectorAll('#community-events button')].some(button=>button.textContent.includes('Reserve') || button.textContent.includes('Cancel reservation'))", "event registration action");
		const alreadyReserved = await evaluate(browser, "[...document.querySelectorAll('#community-events button')].some(button=>button.textContent.includes('Cancel reservation'))");
		if (alreadyReserved) {
			await evaluate(browser, `(() => {[...document.querySelectorAll('#community-events button')].find(button=>button.textContent.includes('Cancel reservation')).click();return true})()`);
			await waitFor(browser, "[...document.querySelectorAll('#community-events button')].some(button=>button.textContent.includes('Reserve'))", "event reservation reset");
		}
		await evaluate(browser, `(() => {[...document.querySelectorAll('#community-events button')].find(button=>button.textContent.includes('Reserve')).click();return true})()`);
		await waitFor(browser, "document.querySelector('#event-registration-dialog')?.open && document.querySelector('#event-registration-form select')?.options.length > 1", "event registration character picker");
		await evaluate(browser, `(() => {const form=document.querySelector('#event-registration-form');form.elements.characterGuid.value='1';form.requestSubmit();return true})()`);
		await waitFor(browser, "[...document.querySelectorAll('#community-events button')].some(button=>button.textContent.includes('Cancel reservation'))", "event reservation confirmation");
		await navigate(browser, "/admin/content/events");
		await waitFor(browser, "[...document.querySelectorAll('#admin-events button')].some(button=>button.textContent.includes('1 participants'))", "event participant count");
		await evaluate(browser, `(() => {[...document.querySelectorAll('#admin-events button')].find(button=>button.textContent.includes('participants')).click();return true})()`);
		await waitFor(browser, "document.querySelectorAll('#event-participants .admin-row').length === 1", "event participant workspace");
		await evaluate(browser, `(() => {const select=document.querySelector('#event-participants select');select.value='attended';select.dispatchEvent(new Event('change',{bubbles:true}));return true})()`);
		await waitFor(browser, "!document.querySelector('#event-participants select')?.disabled && document.querySelector('#event-participants select')?.value === 'attended'", "event attendance update");
		await evaluate(browser, `(() => {const form=document.querySelector('#event-reward-form');form.elements.reason.value='Browser audit attendance';form.requestSubmit();return true})()`);
		await waitFor(browser, "document.querySelector('#event-reward-results')?.textContent.includes('awarded')", "event attendance reward");
		await navigate(browser, "/admin/settings/realm");
		await waitFor(browser, "Boolean(document.querySelector('#gm-settings-form')?.onsubmit) && document.querySelector('#gm-settings-form [name=realmType]')?.value === 'PvE'", "realm settings controller");
		await evaluate(browser, "document.querySelector('#gm-settings-form').requestSubmit(); true");
		await waitFor(browser, "document.querySelector('#gm-settings-form .form-message')?.textContent.includes('Website settings saved')", "idempotent realm settings save");
		const settingsSave = await evaluate(browser, `({saved:document.querySelector('#gm-settings-form .form-message')?.textContent.includes('Website settings saved'),request:document.querySelector('#gm-settings-form')?.dataset.lastRequestId||''})`);
		if (!settingsSave.saved || !settingsSave.request) failures.push("staff journey: realm settings save did not expose durable success and request ID");
		await navigate(browser, "/admin/players/moderation");
		await waitFor(browser, "Boolean(document.querySelector('#investigation-search-form')?.onsubmit)", "investigation workspace");
		await evaluate(browser, `(() => { const form=document.querySelector('#investigation-search-form'); form.elements.account.value='DEMO'; form.elements.reason.value='Automated review of suspected account evasion'; form.requestSubmit(); return true })()`);
		await waitFor(browser, "document.querySelector('#investigation-results')?.textContent.includes('HELPER') || Boolean(document.querySelector('#investigation-search-form .form-message')?.textContent.trim())", "linked-account investigation result");
		const investigationResult = await evaluate(browser, `({match:document.querySelector('#investigation-results')?.textContent.includes('HELPER'),rawLeak:/127\\.0\\.0\\.1|172\\.17\\./.test(document.querySelector('#investigation-results')?.textContent||'')})`);
		if (!investigationResult.match || investigationResult.rawLeak) failures.push("staff journey: privacy-aware linked-account investigation failed");
		const evidenceResult = await evaluate(browser, `(async()=>{const response=await fetch('/api/admin/investigations/evidence',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({accountId:1,caseReference:'BROWSER-AUDIT',note:'Automated browser evidence workflow verification',evidenceUrl:'https://evidence.example.test/browser-audit'})});return {ok:response.ok,status:response.status,body:await response.text()}})()`);
		if (!evidenceResult.ok) failures.push(`staff journey: evidence attachment failed (${evidenceResult.status}: ${evidenceResult.body})`);
		const moderationResult = await evaluate(browser, `(async()=>{const response=await fetch('/api/admin/moderation',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({action:'ban',target:'HELPER',duration:'30m',reason:'Automated destructive moderation journey'})});return {ok:response.ok,status:response.status,body:await response.text()}})()`);
		if (!moderationResult.ok) failures.push(`staff journey: destructive moderation failed (${moderationResult.status}: ${moderationResult.body})`);
		await navigate(browser, "/admin/support");
		await waitFor(browser, "Boolean(document.querySelector('#admin-tickets button')?.onclick)", "support triage action");
		await evaluate(browser, `(() => { const button=document.querySelector('#admin-tickets button');button.focus();button.click();return true })()`);
		await waitFor(browser, "document.querySelector('#ticket-action-dialog')?.open === true", "support dialog");
		await browser.call("Input.dispatchKeyEvent", {type:"keyDown", key:"Escape", code:"Escape", windowsVirtualKeyCode:27, nativeVirtualKeyCode:27});
		await browser.call("Input.dispatchKeyEvent", {type:"keyUp", key:"Escape", code:"Escape", windowsVirtualKeyCode:27, nativeVirtualKeyCode:27});
		await waitFor(browser, "document.querySelector('#ticket-action-dialog')?.open === false", "support dialog Escape dismissal");
		await navigate(browser, "/admin");
		await waitFor(browser, "Boolean(document.querySelector('#admin-orders input[type=checkbox]:not(:disabled)'))", "retryable delivery order");
		await evaluate(browser, `(() => { document.querySelector('#orders-select-all').click(); document.querySelector('#orders-bulk-retry').click(); return true })()`);
		await waitFor(browser, "document.querySelector('#action-dialog')?.open === true", "bulk retry confirmation");
		await evaluate(browser, `(() => { const form=document.querySelector('#action-dialog-form'); form.elements.value.value='RETRY'; form.requestSubmit(); return true })()`);
		await waitFor(browser, "!document.querySelector('#orders-bulk-results')?.classList.contains('hidden')", "structured bulk retry report");
		const bulkRetryResult = await evaluate(browser, `({request:document.querySelector('#orders-bulk-results')?.textContent.includes('Request'),rows:document.querySelectorAll('#orders-bulk-results .activity-list > div').length})`);
		if (!bulkRetryResult.request || !bulkRetryResult.rows) failures.push("staff journey: structured bulk retry result did not render");
		await evaluate(browser, `(() => { [...document.querySelectorAll('#admin-orders button')].find(button=>button.textContent.includes('Delivery details'))?.click(); return true })()`);
		await waitFor(browser, "document.querySelector('#order-reconcile-dialog')?.open === true && Boolean(document.querySelector('#order-step-list .order-step button'))", "failed order reconciliation");
		await evaluate(browser, `(() => { document.querySelector('#order-step-list .order-step button').click(); const form=document.querySelector('#order-step-form'); form.elements.reason.value='Verified against the disposable test character'; form.elements.confirmation.value='RECONCILE'; form.requestSubmit(); return true })()`);
		await waitFor(browser, "document.querySelector('#order-reconcile-dialog')?.open === false", "order reconciliation completion");
		const creditGrant = await evaluate(browser, `(async()=>{const response=await fetch('/api/admin/credits',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:'DEMO',amount:500,reason:'Automated browser audit purchase'})});return {ok:response.ok,status:response.status,body:await response.text()}})()`);
		if (!creditGrant.ok) failures.push(`shop journey: could not provision test credits (${creditGrant.status}: ${creditGrant.body})`);
		await navigate(browser, "/shop");
		await waitFor(browser, "document.querySelectorAll('.product-card').length > 3", "shop products");
		await evaluate(browser, `(() => { const search=document.querySelector('#shop-search');search.value='Shadowmourne';search.dispatchEvent(new Event('input',{bubbles:true}));return true })()`);
		await waitFor(browser, "new URLSearchParams(location.search).get('q') === 'Shadowmourne' && document.querySelectorAll('.product-card').length > 0 && [...document.querySelectorAll('.product-card')].every(card => card.textContent.toLowerCase().includes('shadowmourne'))", "URL-backed shop search");
		const shopSearchResult = await evaluate(browser, `({count:document.querySelectorAll('.product-card').length,url:new URLSearchParams(location.search).get('q'),labels:[...document.querySelectorAll('.product-card')].map(card=>card.textContent)})`);
		if (!shopSearchResult.count || shopSearchResult.url !== "Shadowmourne" || shopSearchResult.labels.some(label => !label.toLowerCase().includes("shadowmourne"))) failures.push("shop journey: text search did not isolate matching products");
		await evaluate(browser, "document.querySelector('#shop-clear-filters').click(); true");
		await waitFor(browser, "!new URLSearchParams(location.search).has('q') && document.querySelectorAll('.product-card').length > 3 && !document.querySelector('#shop-pagination')?.classList.contains('hidden')", "cleared shop discovery state");
		await evaluate(browser, "document.querySelector('.product-card:not(.is-sold-out) .buy').click(); true");
		await waitFor(browser, "document.querySelector('#purchase-dialog')?.open === true", "purchase dialog");
		await evaluate(browser, "document.querySelector('#purchase-confirm').click(); true");
		await waitFor(browser, "document.querySelector('#purchase-dialog')?.open === false || Boolean(document.querySelector('#purchase-dialog .form-message')?.textContent.trim())", "purchase completion");
		const purchaseResult = await evaluate(browser, `({closed:document.querySelector('#purchase-dialog')?.open === false,error:document.querySelector('#purchase-dialog .form-message')?.textContent.trim()||''})`);
		if (!purchaseResult.closed) failures.push(`shop journey: purchase failed (${purchaseResult.error || 'unknown error'})`);

		await navigate(browser, "/account/support");
		await waitFor(browser, "Boolean(document.querySelector('#ticket-form')) && !document.querySelector('#my-tickets')?.textContent.includes('Loading')", "initialized support form");
		await evaluate(browser, `(() => { const form=document.querySelector('#ticket-form');form.elements.subject.value='Browser journey ticket';form.elements.message.value='This message verifies the complete player support submission journey.';form.requestSubmit();return true })()`);
		await waitFor(browser, "document.querySelector('#my-tickets')?.textContent.includes('Browser journey ticket') || Boolean(document.querySelector('#ticket-form .form-message')?.textContent.trim())", "submitted support ticket");
		const ticketResult = await evaluate(browser, `({visible:document.querySelector('#my-tickets')?.textContent.includes('Browser journey ticket'),message:document.querySelector('#ticket-form .form-message')?.textContent.trim()||''})`);
		if (!ticketResult.visible) failures.push(`support journey: submission failed (${ticketResult.message || 'unknown error'})`);

		await navigate(browser, "/account/characters");
		await waitFor(browser, "[...document.querySelectorAll('#deleted-characters button')].some(button=>button.textContent.includes('Restore')) || document.querySelector('#deleted-characters')?.textContent.includes('No restorable characters')", "deleted character state");
		if (await evaluate(browser, "[...document.querySelectorAll('#deleted-characters button')].some(button=>button.textContent.includes('Restore'))")) {
			await evaluate(browser, `(() => { [...document.querySelectorAll('#deleted-characters button')].find(button=>button.textContent.includes('Restore')).click(); return true })()`);
			await waitFor(browser, "document.querySelector('#action-dialog')?.open === true", "character restore confirmation");
			await evaluate(browser, `(() => { document.querySelector('#action-dialog-form').requestSubmit(); return true })()`);
			await waitFor(browser, "document.querySelector('#toast')?.classList.contains('show')", "character restore completion");
		} else process.stdout.write("SKIP restore mutation (mock fixture already consumed)\n");

		await navigate(browser, "/admin/catalog");
		await waitFor(browser, "Boolean(document.querySelector('#catalog-import-form')?.onsubmit)", "initialized catalog importer");
		await evaluate(browser, `(() => { const form=document.querySelector('#catalog-import-form');form.elements.csv.value='name,price,category,item_id,quantity\\nBrowser Bag,10,Utility,51809,1';form.requestSubmit();return true })()`);
		await waitFor(browser, "document.querySelector('#step-up-dialog')?.open === true || document.querySelector('#catalog-import-preview')?.textContent.includes('Ready to import') || Boolean(document.querySelector('#catalog-import-form .form-message')?.textContent.trim())", "catalog validation or staff step-up");
		if (await evaluate(browser, "document.querySelector('#step-up-dialog')?.open === true")) {
			await evaluate(browser, `(() => { const form=document.querySelector('#step-up-form');form.elements.password.value='demo1234';form.requestSubmit();return true })()`);
		}
		await waitFor(browser, "document.querySelector('#catalog-import-preview')?.textContent.includes('Ready to import') || Boolean(document.querySelector('#catalog-import-form .form-message')?.textContent.trim())", "catalog validation preview");
		const importResult = await evaluate(browser, `({ready:document.querySelector('#catalog-import-preview')?.textContent.includes('Ready to import'),message:document.querySelector('#catalog-import-form .form-message')?.textContent.trim()||''})`);
		if (!importResult.ready) failures.push(`catalog journey: CSV validation failed (${importResult.message || 'unknown error'})`);
		const bundleEdit = await evaluate(browser, `(() => { const button=document.querySelector('#admin-bundle-templates .admin-row button');button?.click();return document.querySelector('#bundle-template-form [name=id]')?.value || '' })()`);
		if (!bundleEdit) failures.push("catalog journey: reusable bundle did not open for editing");
		await navigate(browser, "/admin/catalog/1/edit");
		await waitFor(browser, "Boolean(document.querySelector('#catalog-item-search')?.oninput)", "catalog item autocomplete");
		await evaluate(browser, `(() => { const search=document.querySelector('#catalog-item-search');search.value='Shadow';search.dispatchEvent(new Event('input',{bubbles:true}));return true })()`);
		await waitFor(browser, "document.querySelectorAll('#catalog-item-results [role=option]').length > 0", "catalog item suggestions");
		const autocompleteResult = await evaluate(browser, `(() => { const search=document.querySelector('#catalog-item-search');search.focus();search.dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowDown',bubbles:true}));const active=search.getAttribute('aria-activedescendant'),expanded=search.getAttribute('aria-expanded');search.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true}));return {active,expanded,cleared:search.value==='',focused:document.activeElement===search}; })()`);
		if (!autocompleteResult.active || autocompleteResult.expanded !== "true" || !autocompleteResult.cleared || !autocompleteResult.focused) failures.push("catalog journey: keyboard autocomplete selection failed");
		await writeFile(`${outputDir}/accessibility-report.json`, JSON.stringify({ generatedAt:new Date().toISOString(), strictContrast:process.env.STRICT_CONTRAST === "1", routes:reports }, null, 2));
	} finally {
		browser.close();
		if (browser.targetId)
			await fetch(`${cdpURL}/json/close/${encodeURIComponent(browser.targetId)}`).catch(() => {});
	}
	if (failures.length) {
		throw new Error(`Browser audit failed:\n${failures.join("\n")}`);
	}
}

main().catch((error) => {
	process.stderr.write(error.stack + "\n");
	process.exitCode = 1;
});
