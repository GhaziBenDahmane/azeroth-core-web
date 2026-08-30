export const qs = (selector, root = document) => root.querySelector(selector);
export const qsa = (selector, root = document) => [...root.querySelectorAll(selector)];

export function publicLink(value) {
	try {
		const url = new URL(value, location.origin);
		return ["http:", "https:"].includes(url.protocol) ? url.href : "";
	} catch {
		return "";
	}
}

export const esc = (value) => String(value ?? "").replace(
	/[&<>'"]/g,
	(character) => ({"&":"&amp;","<":"&lt;",">":"&gt;","'":"&#39;",'"':"&quot;"})[character],
);

export function setMessage(form, message, success = false) {
	const element = qs(".form-message", form);
	if (!element) return;
	element.textContent = message;
	element.classList.toggle("success", success);
}

export function pageFromURL(key) {
	return Math.max(1, Number(new URLSearchParams(location.search).get(key)) || 1);
}

export function updateURLQuery(values) {
	const url = new URL(location.href);
	for (const [key, value] of Object.entries(values)) {
		if (value === "" || value === null || value === undefined || value === 1 || value === "all") url.searchParams.delete(key);
		else url.searchParams.set(key, String(value));
	}
	history.replaceState(history.state, "", url.pathname + (url.searchParams.size ? "?" + url.searchParams : ""));
}

export function renderPagination(anchor, meta, key, onChange) {
	if (!anchor || !meta) return;
	let nav = anchor.nextElementSibling;
	if (!nav?.matches(`[data-pagination="${key}"]`)) {
		nav = document.createElement("nav");
		nav.className = "data-pagination";
		nav.dataset.pagination = key;
		nav.setAttribute("aria-label", `${key.replace(/Page$/, "")} pagination`);
		anchor.after(nav);
	}
	const page = Number(meta.page) || 1;
	const pages = Number(meta.totalPages) || 0;
	const total = Number(meta.total) || 0;
	nav.innerHTML = `<button class="ghost-button" type="button" ${meta.hasPrevious ? "" : "disabled"}>← Previous</button><span>Page ${pages ? page : 0} of ${pages} · ${total.toLocaleString()} total</span><button class="ghost-button" type="button" ${meta.hasNext ? "" : "disabled"}>Next →</button>`;
	const [previous, next] = qsa("button", nav);
	previous.onclick = () => { updateURLQuery({[key]: page - 1}); onChange(); };
	next.onclick = () => { updateURLQuery({[key]: page + 1}); onChange(); };
}
