import {esc, qs, setMessage} from "/js/ui.js";

export function mountTracker(context) {
	const { api, toast } = context;
	const index = qs("#tracker-index"),
		detail = qs("#tracker-detail"),
		list = qs("#tracker-list"),
		filters = qs("#tracker-filters"),
		pager = qs("#tracker-pagination"),
		compose = qs("#tracker-compose"),
		validFilters = ["q", "kind", "status", "sort"];
	const statusLabel = (value) => String(value || "open").replaceAll("_", " ");
	const trackerDate = (value) => value ? new Date(value).toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" }) : "—";
	function trackerURL(path, values = {}) {
		const url = new URL(location.href);
		url.pathname = path;
		for (const key of [...validFilters, "page"]) url.searchParams.delete(key);
		for (const [key, value] of Object.entries(values)) if (value) url.searchParams.set(key, value);
		return url.pathname + url.search;
	}
	function renderLabels(labels) {
		return (labels || []).map((label) => `<span class="tracker-label">${esc(label)}</span>`).join("");
	}
	function issueCard(issue) {
		const card = document.createElement("article");
		card.className = `tracker-card tracker-${issue.kind}`;
		card.innerHTML = `<div class="tracker-vote-count"><strong>${issue.voteCount}</strong><span>votes</span></div><div class="tracker-card-main"><div class="tracker-card-meta"><span class="tracker-kind">${issue.kind === "bug" ? "Bug report" : "Suggestion"}</span><span class="status-${esc(issue.status)}">${esc(statusLabel(issue.status))}</span><span>${esc(issue.category)}</span></div><h2><a href="/tracker/${issue.id}">${esc(issue.title)}</a></h2><div class="tracker-labels">${renderLabels(issue.labels)}</div><footer><span>By ${esc(issue.author)}</span><span>${issue.commentCount} ${issue.commentCount === 1 ? "comment" : "comments"}</span><time datetime="${issue.updatedAt}">Updated ${trackerDate(issue.updatedAt)}</time></footer></div>`;
		qs("a", card).onclick = (event) => { event.preventDefault(); showIssue(issue.id); };
		return card;
	}
	async function loadIssues(updateHistory = false) {
		index.classList.remove("hidden"); detail.classList.add("hidden"); compose.classList.add("hidden");
		const values = Object.fromEntries(new FormData(filters));
		const current = new URL(location.href), pageNumber = Math.max(1, Number(current.searchParams.get("page")) || 1);
		if (updateHistory) history.pushState(null, "", trackerURL("/tracker", values));
		list.innerHTML = '<div class="skeleton"></div>'; pager.innerHTML = "";
		try {
			const params = new URLSearchParams();
			for (const [key, value] of Object.entries(values)) if (value) params.set(key, value);
			params.set("page", pageNumber);
			const data = await api("/api/community/issues?" + params);
			list.innerHTML = "";
			for (const issue of data.issues || []) list.append(issueCard(issue));
			if (!list.children.length) list.innerHTML = '<div class="empty"><strong>No matching submissions</strong><p>Try a broader filter or start a new discussion.</p></div>';
			const pages = Math.max(1, Math.ceil(data.total / data.pageSize));
			if (pages > 1) {
				const previous = document.createElement("button"), next = document.createElement("button"), label = document.createElement("span");
				previous.className = next.className = "ghost-button"; previous.textContent = "← Previous"; next.textContent = "Next →";
				previous.disabled = data.page <= 1; next.disabled = data.page >= pages; label.textContent = `Page ${data.page} of ${pages}`;
				const move = (page) => { const url = new URL(location.href); url.searchParams.set("page", page); history.pushState(null, "", url.pathname + url.search); loadIssues(); };
				previous.onclick = () => move(data.page - 1); next.onclick = () => move(data.page + 1); pager.append(previous, label, next);
			}
		} catch (error) { list.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}
	async function showIssue(id, updateHistory = true) {
		index.classList.add("hidden"); compose.classList.add("hidden"); detail.classList.remove("hidden"); detail.innerHTML = '<div class="skeleton"></div>';
		if (updateHistory) history.pushState({ tracker: id }, "", trackerURL("/tracker/" + id));
		try {
			const { issue } = await api("/api/community/issues/" + id);
			detail.innerHTML = `<button id="tracker-back" class="ghost-button" type="button">← Community tracker</button><article class="tracker-issue"><header><div class="tracker-card-meta"><span class="tracker-kind">${issue.kind === "bug" ? "Bug report" : "Suggestion"}</span><span class="status-${esc(issue.status)}">${esc(statusLabel(issue.status))}</span><span class="priority-${esc(issue.priority)}">${esc(issue.priority)} priority</span></div><h2>${esc(issue.title)}</h2><p>Submitted by ${esc(issue.author)} · ${trackerDate(issue.createdAt)}</p><div class="tracker-labels">${renderLabels(issue.labels)}</div></header><div class="tracker-issue-body"></div>${issue.staffResponse ? `<aside class="staff-response"><p class="eyebrow">STAFF RESPONSE</p><p>${esc(issue.staffResponse)}</p></aside>` : ""}<div class="tracker-actions"><button id="tracker-vote" class="${issue.viewerVoted ? "button" : "ghost-button"}" type="button" aria-pressed="${issue.viewerVoted}">▲ <span>${issue.voteCount}</span> ${issue.viewerVoted ? "Voted" : "Vote"}</button><span>${issue.commentCount} ${issue.commentCount === 1 ? "comment" : "comments"}</span></div><section class="tracker-discussion"><h3>Discussion</h3><div id="tracker-comments"></div>${["closed", "declined"].includes(issue.status) ? '<p class="notice-inline">This discussion is closed.</p>' : '<form id="tracker-comment-form"><label>Add to the discussion<textarea name="body" minlength="2" maxlength="4000" rows="4" required></textarea></label><button class="button small" type="submit">Post comment</button><p class="form-message" role="status"></p></form>'}</section></article>`;
			qs(".tracker-issue-body", detail).textContent = issue.body;
			const comments = qs("#tracker-comments", detail);
			for (const comment of issue.comments || []) {
				const item = document.createElement("article"); item.className = "tracker-comment" + (comment.authorRole === "staff" ? " staff" : "");
				const head = document.createElement("header"), body = document.createElement("p");
				head.innerHTML = `<strong>${esc(comment.author)}</strong>${comment.authorRole === "staff" ? '<span>Staff</span>' : ""}<time>${trackerDate(comment.createdAt)}</time>`; body.textContent = comment.body; item.append(head, body); comments.append(item);
			}
			if (!comments.children.length) comments.innerHTML = '<p class="muted">No comments yet.</p>';
			qs("#tracker-back").onclick = () => { history.pushState(null, "", "/tracker"); loadIssues(); };
			qs("#tracker-vote").onclick = async (event) => {
				const button = event.currentTarget; button.disabled = true;
				try { const result = await api(`/api/community/issues/${id}/vote`, { method: "POST", body: "{}" }); button.className = result.voted ? "button" : "ghost-button"; button.setAttribute("aria-pressed", result.voted); button.innerHTML = `▲ <span>${result.voteCount}</span> ${result.voted ? "Voted" : "Vote"}`; }
				catch (error) { if (error.status === 401) location.href = "/login?next=" + encodeURIComponent(location.pathname); else toast(error.message); }
				finally { button.disabled = false; }
			};
			const commentForm = qs("#tracker-comment-form");
			if (commentForm) commentForm.onsubmit = async (event) => { event.preventDefault(); const form=event.currentTarget; try { await api(`/api/community/issues/${id}/comments`, {method:"POST",body:JSON.stringify(Object.fromEntries(new FormData(form)))}); await showIssue(id, false); } catch(error) { if(error.status===401) location.href="/login?next="+encodeURIComponent(location.pathname); else setMessage(form,error.message); } };
		} catch (error) { detail.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}
	for (const field of filters.elements) if (field.name) field.value = new URLSearchParams(location.search).get(field.name) || field.value;
	filters.onsubmit = (event) => { event.preventDefault(); loadIssues(true); };
	qs("#tracker-new-toggle").onclick = () => { index.classList.add("hidden"); detail.classList.add("hidden"); compose.classList.remove("hidden"); qs("#tracker-create-form input[name=title]").focus(); };
	qs("#tracker-compose-close").onclick = () => { compose.classList.add("hidden"); index.classList.remove("hidden"); };
	qs("#tracker-create-form").onsubmit = async (event) => { event.preventDefault(); const form=event.currentTarget; try { const result=await api("/api/community/issues",{method:"POST",body:JSON.stringify(Object.fromEntries(new FormData(form)))}); form.reset(); setMessage(form,"Submitted.",true); showIssue(result.id); } catch(error) { if(error.status===401) location.href="/login?next="+encodeURIComponent("/tracker"); else setMessage(form,error.message); } };
	const trackerMatch = location.pathname.match(/^\/tracker\/(\d+)\/?$/);
	if (trackerMatch) showIssue(trackerMatch[1], false); else loadIssues();
	window.addEventListener("popstate", () => { const match=location.pathname.match(/^\/tracker\/(\d+)\/?$/); if(match) showIssue(match[1],false); else loadIssues(); });

}
