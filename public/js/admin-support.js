import {esc, pageFromURL, qs, qsa, renderPagination, setMessage, updateURLQuery} from "/js/ui.js";

export function createAdminSupport({api, askAction, toast}) {
	let cannedReplies = [], searchTimer, applicationSearchTimer, mounted = false;

	async function load() {
		mount();
		const box = qs("#admin-tickets");
		if (!box) return;
		try {
			const params = new URLSearchParams({page:String(pageFromURL("supportPage")), perPage:"25"});
			const status = qs("#support-status-filter")?.value || "", priority = qs("#support-priority-filter")?.value || "", search = qs("#support-search")?.value.trim() || "";
			if (status) params.set("status", status);
			if (priority) params.set("priority", priority);
			if (search) params.set("q", search);
			const applicationParams = new URLSearchParams({page:String(pageFromURL("applicationsPage")), perPage:"25"});
			const applicationStatus = qs("#guild-application-status")?.value || "", applicationSearch = qs("#guild-application-search")?.value.trim() || "";
			if (applicationStatus) applicationParams.set("status", applicationStatus);
			if (applicationSearch) applicationParams.set("q", applicationSearch);
			const [ticketData, canned, guildApplications] = await Promise.all([
				api("/api/admin/tickets?" + params),
				api("/api/admin/canned-replies"),
				api("/api/admin/guild-applications?" + applicationParams),
			]);
			cannedReplies = (canned.replies || []).filter((reply) => reply.active !== false);
			renderCannedReplies();
			renderTickets(ticketData);
			renderGuildApplications(guildApplications);
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}

	function renderCannedReplies() {
		const select = qs("#ticket-canned-reply"), box = qs("#canned-replies");
		if (select) select.innerHTML = '<option value="">Choose a saved reply…</option>' + cannedReplies.map((reply) => `<option value="${reply.id}">${esc(reply.title)}</option>`).join("");
		if (!box) return;
		box.innerHTML = cannedReplies.map((reply) => `<div class="admin-row"><span><b>${esc(reply.title)}</b><small>${esc(reply.body)}</small></span><button class="ghost-button" data-delete-canned="${reply.id}">Delete</button></div>`).join("") || '<p class="muted">No canned replies.</p>';
		qsa("[data-delete-canned]", box).forEach((button) => button.onclick = async () => {
			if (!(await askAction({title:"Delete saved reply", message:"This removes the reply for every support agent.", input:false, confirmText:"Delete reply"}))) return;
			button.disabled = true;
			try { await api("/api/admin/canned-replies/" + button.dataset.deleteCanned, {method:"DELETE"}); await load(); }
			catch (error) { toast(error.message); button.disabled = false; }
		});
	}

	function renderTickets(data) {
		const box = qs("#admin-tickets"), tickets = data.tickets || [];
		box.innerHTML = "";
		for (const ticket of tickets) {
			const row = document.createElement("div");
			row.className = "ticket-row";
			const thread = (ticket.messages || []).map((message) => `<small class="ticket-message-${esc(message.authorRole)}"><b>${message.authorRole === "staff" ? "Staff" : message.authorRole === "internal" ? "Internal note" : esc(ticket.username || "Player")}:</b> ${esc(message.message)}</small>`).join("") || `<small>${esc(ticket.username || "Player")} · ${esc(ticket.message)}</small>${ticket.response ? `<small>Staff: ${esc(ticket.response)}</small>` : ""}`;
			const overdue = ticket.dueAt && new Date(ticket.dueAt) < new Date() && !["resolved","closed"].includes(ticket.status);
			row.innerHTML = `<div><b>#${ticket.id} · ${esc(ticket.subject)}</b><span class="ticket-meta"><i class="priority-${esc(ticket.priority || "normal")}">${esc(ticket.priority || "normal")}</i><i>${esc(ticket.category || "general")}</i><i>${esc(ticket.assignedName || "Unassigned")}</i>${ticket.dueAt ? `<i class="${overdue ? "danger" : ""}">${overdue ? "SLA overdue" : "Due " + new Date(ticket.dueAt).toLocaleString()}</i>` : ""}</span>${thread}</div><span class="row-actions"></span>`;
			const open = document.createElement("button");
			open.className = "ghost-button"; open.textContent = "Open";
			open.onclick = async () => {
				open.disabled = true;
				try {
					const dialog = qs("#ticket-action-dialog"), form = qs("#ticket-action-form");
					form.reset(); form.elements.ticketId.value = ticket.id;
					form.elements.status.value = ticket.status === "answered" ? "pending_player" : ticket.status;
					form.elements.priority.value = ticket.priority || "normal"; form.elements.category.value = ticket.category || "general"; form.elements.tags.value = ticket.tags || "";
					qs("#ticket-action-title").textContent = `Triage ticket #${ticket.id}`;
					qs("#ticket-action-context").textContent = `${ticket.username || "Player"}: ${ticket.subject}`;
					const history = await api(`/api/admin/tickets/${ticket.id}/events`);
					qs("#ticket-event-history").innerHTML = `<h4>Immutable history</h4>` + (history.events || []).map((event) => `<small><b>${esc(event.type.replaceAll("_", " "))}</b> · ${new Date(event.createdAt).toLocaleString()}<br>${esc(event.details || "")}</small>`).join("");
					dialog.showModal(); form.elements.response.focus();
				} catch (error) { toast(error.message); }
				finally { open.disabled = false; }
			};
			qs(".row-actions", row).append(open); box.append(row);
		}
		if (!tickets.length) box.innerHTML = '<p class="muted">No support tickets match these filters.</p>';
		renderPagination(box, data.pagination, "supportPage", load);
	}

	function renderGuildApplications(data) {
		const box = qs("#admin-guild-applications");
		if (!box) return;
		box.innerHTML = "";
		for (const application of data.applications || []) {
			const row = document.createElement("div"); row.className = "admin-row";
			row.innerHTML = `<span><b>${esc(application.characterName)} → ${esc(application.guildName || "Guild " + application.guildId)}</b><small>${esc(application.username)} · ${esc(application.status)} · ${esc(application.message)}</small></span><span class="row-actions"></span>`;
			const review = document.createElement("button"); review.className = "ghost-button"; review.type = "button"; review.textContent = "Review";
			review.onclick = () => { const form=qs("#guild-application-review"); form.elements.id.value=application.id; form.elements.status.value=application.status==="withdrawn"?"submitted":application.status; form.elements.response.value=application.response||""; form.elements.staffNote.value=application.staffNote||""; qs("#guild-application-review-title").textContent=`${application.characterName} → ${application.guildName || "Guild " + application.guildId}`; form.classList.remove("hidden"); form.scrollIntoView({behavior:"smooth",block:"center"}); form.elements.status.focus(); };
			qs(".row-actions", row).append(review); box.append(row);
		}
		if (!box.children.length) box.innerHTML = '<p class="muted">No matching guild applications.</p>';
		renderPagination(box, data.pagination, "applicationsPage", load);
	}

	function mount() {
		if (mounted || !qs("#ticket-action-dialog")) return;
		mounted = true;
		const dialog = qs("#ticket-action-dialog"), form = qs("#ticket-action-form");
		qs(".dialog-close", dialog).onclick = () => dialog.close();
		qs("[data-ticket-cancel]", dialog).onclick = () => dialog.close();
		form.onsubmit = async (event) => {
			event.preventDefault(); const button=qs('button[type="submit"]', form), values=Object.fromEntries(new FormData(form)); button.disabled=true; setMessage(form, "");
			try { values.assignToSelf=values.assignment==="self"; values.unassign=values.assignment==="none"; delete values.assignment; delete values.ticketId; const result=await api(`/api/admin/tickets/${form.elements.ticketId.value}`,{method:"POST",body:JSON.stringify(values)}); dialog.close(); toast(`Ticket updated${result.requestId ? ` · request ${result.requestId}` : ""}`); await load(); }
			catch(error){setMessage(form,error.message)} finally{button.disabled=false}
		};
		qs("#ticket-canned-reply").onchange = (event) => { const reply=cannedReplies.find((item)=>String(item.id)===event.target.value); if(reply)form.elements.response.value=reply.body; };
		const state=new URLSearchParams(location.search);
		qs("#support-search").value=state.get("supportQ")||""; qs("#support-status-filter").value=state.get("supportStatus")||""; qs("#support-priority-filter").value=state.get("supportPriority")||"";
		qs("#support-search").oninput=()=>{clearTimeout(searchTimer);searchTimer=setTimeout(()=>{updateURLQuery({supportQ:qs("#support-search").value.trim(),supportPage:1});load()},250)};
		qs("#support-status-filter").onchange=()=>{updateURLQuery({supportStatus:qs("#support-status-filter").value,supportPage:1});load()};
		qs("#support-priority-filter").onchange=()=>{updateURLQuery({supportPriority:qs("#support-priority-filter").value,supportPage:1});load()};
		qs("#support-refresh").onclick=load;
		qs("#guild-application-status").value=state.get("applicationsStatus")||"";
		if (!qs("#guild-application-search")) {
			const search = document.createElement("input");
			search.id = "guild-application-search";
			search.type = "search";
			search.placeholder = "Account, character, or guild";
			search.setAttribute("aria-label", "Search guild applications");
			search.value = state.get("applicationsQ") || "";
			search.oninput = () => {
				clearTimeout(applicationSearchTimer);
				applicationSearchTimer = setTimeout(() => { updateURLQuery({applicationsQ:search.value.trim(), applicationsPage:1}); load(); }, 250);
			};
			qs("#guild-application-status").before(search);
		}
		qs("#guild-application-status").onchange=()=>{updateURLQuery({applicationsStatus:qs("#guild-application-status").value,applicationsPage:1});load()};
		qs("#guild-applications-refresh").onclick=load;
		const guildForm=qs("#guild-application-review");
		if (!guildForm.elements.response) { const label=document.createElement("label"); label.textContent="Message to applicant"; label.innerHTML+='<textarea name="response" maxlength="2000" rows="4"></textarea>'; guildForm.insertBefore(label,guildForm.querySelector('[name="staffNote"]').closest("label")); }
		qs("#guild-application-review-close").onclick=()=>guildForm.classList.add("hidden");
		guildForm.onsubmit=async(event)=>{event.preventDefault();const button=qs('button[type="submit"]',guildForm),values=Object.fromEntries(new FormData(guildForm)),id=values.id;delete values.id;button.disabled=true;setMessage(guildForm,"");try{const result=await api("/api/admin/guild-applications/"+id,{method:"PUT",body:JSON.stringify(values)});setMessage(guildForm,`Application updated${result.requestId?` · request ${result.requestId}`:""}.`,true);guildForm.classList.add("hidden");await load()}catch(error){setMessage(guildForm,error.message)}finally{button.disabled=false}};
		const cannedForm=qs("#canned-reply-form");
		cannedForm.onsubmit=async(event)=>{event.preventDefault();const button=qs('button[type="submit"]',cannedForm);button.disabled=true;setMessage(cannedForm,"");try{const result=await api("/api/admin/canned-replies",{method:"POST",body:JSON.stringify(Object.fromEntries(new FormData(cannedForm)))});cannedForm.reset();setMessage(cannedForm,`Reply saved${result.requestId?` · request ${result.requestId}`:""}.`,true);await load()}catch(error){setMessage(cannedForm,error.message)}finally{button.disabled=false}};
	}

	return {load};
}
