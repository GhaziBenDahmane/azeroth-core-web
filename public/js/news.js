import {esc, publicLink, qs} from "/js/ui.js";

export function mountNews(context) {
	const { api } = context;
	const index = qs("#news-index"), article = qs("#news-article"),
		match = location.pathname.match(/^\/news\/([^/]+)\/?$/);
	const renderArticle = (item) => {
		index.classList.add("hidden");
		article.classList.remove("hidden");
		article.innerHTML = "";
		const back = document.createElement("a"); back.className = "text-action"; back.href = "/news"; back.textContent = "← All realm news";
		article.append(back);
		if (item.coverUrl) { const cover = document.createElement("img"); cover.className = "article-cover"; cover.src = item.coverUrl; cover.alt = ""; article.append(cover); }
		const meta = document.createElement("p"); meta.className = "eyebrow"; meta.textContent = [item.kind, item.authorName, item.publishAt ? new Date(item.publishAt).toLocaleDateString() : ""].filter(Boolean).join(" · ");
		const title = document.createElement("h1"); title.textContent = item.title;
		const summary = document.createElement("p"); summary.className = "article-lead"; summary.textContent = item.summary || "";
		const body = document.createElement("div"); body.className = "article-body"; body.textContent = item.body || "";
		article.append(meta, title, summary, body);
		if (item.tags) { const tags = document.createElement("div"); tags.className = "article-tags"; for (const value of item.tags.split(",").map((x) => x.trim()).filter(Boolean)) { const tag = document.createElement("span"); tag.textContent = value; tags.append(tag); } article.append(tags); }
		const external = publicLink(item.url); if (external) { const link = document.createElement("a"); link.className = "button small"; link.href = external; link.rel = "noreferrer"; link.textContent = "Related link →"; article.append(link); }
		document.title = item.title + " — Realm news";
		qs("#news-page-title").textContent = item.title;
		qs("#news-page-intro").textContent = item.summary || "Realm news";
	};
	if (match) {
		api("/api/news/" + encodeURIComponent(decodeURIComponent(match[1])))
			.then(renderArticle)
			.catch((e) => { index.innerHTML = `<p class="empty">${esc(e.message)}</p>`; });
	} else {
		api("/api/news").then(({news}) => {
			index.innerHTML = "";
			for (const item of news || []) {
				const card = document.createElement("article"); card.className = "news-card";
				const date = document.createElement("time"); date.textContent = item.publishAt ? new Date(item.publishAt).toLocaleDateString() : "";
				const title = document.createElement("h2"); title.textContent = item.title;
				const summary = document.createElement("p"); summary.textContent = item.summary || "";
				const link = document.createElement("a"); link.href = item.slug && item.body ? "/news/" + encodeURIComponent(item.slug) : (publicLink(item.url) || "/community"); link.textContent = "Read more →";
				card.append(date, title, summary, link); index.append(card);
			}
			if (!index.children.length) index.innerHTML = '<p class="empty">No published articles yet.</p>';
		}).catch((e) => { index.innerHTML = `<p class="empty">${esc(e.message)}</p>`; });
	}

}
