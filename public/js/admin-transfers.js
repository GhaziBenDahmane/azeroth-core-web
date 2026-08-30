import {esc, pageFromURL, qs, renderPagination} from "/js/ui.js";

export function createAdminTransfers({api, askAction, toast}) {
	let mounted = false, searchTimer;

	function mount() {
		if (mounted || !qs("#admin-transfers")) return;
		mounted = true;
		const state = new URLSearchParams(location.search), search = qs("#transfers-search"), status = qs("#transfers-status"), refresh = qs("#transfers-refresh");
		if (search) {
			search.value = state.get("transfersQ") || "";
			search.oninput = () => { clearTimeout(searchTimer); searchTimer=setTimeout(()=>{const url=new URL(location.href);if(search.value.trim())url.searchParams.set("transfersQ",search.value.trim());else url.searchParams.delete("transfersQ");url.searchParams.set("transfersPage","1");history.replaceState(null,"",url);load()},250); };
		}
		if (status) {
			status.value = state.get("transfersStatus") || "";
			status.onchange = () => { const url=new URL(location.href);if(status.value)url.searchParams.set("transfersStatus",status.value);else url.searchParams.delete("transfersStatus");url.searchParams.set("transfersPage","1");history.replaceState(null,"",url);load(); };
		}
		if (refresh) refresh.onclick = load;
	}

	async function load() {
		mount();
		const box = qs("#admin-transfers");
		if (!box) return;
		const params = new URLSearchParams({page:String(pageFromURL("transfersPage")), perPage:"25"}), search=qs("#transfers-search")?.value.trim()||"", status=qs("#transfers-status")?.value||"";
		if(search) params.set("q",search);
		if(status) params.set("status",status);
		try {
			const data=await api("/api/admin/transfers?"+params); box.innerHTML="";
			for(const item of data.requests||[]){
				const row=document.createElement("div"); row.className="admin-row";
				row.innerHTML=`<span><b>#${item.id} · ${esc(item.characterName)}</b><small>${esc(item.username||"Player")} · from ${esc(item.sourceRealm)} · ${esc(item.playerNote||"No note")}${item.handler?" · handled by "+esc(item.handler):""}</small></span><span class="row-actions"><strong class="status-${esc(item.status)}">${esc(item.status)}</strong></span>`;
				for(const nextStatus of ["reviewing","approved","rejected","completed"]){
					if(nextStatus===item.status) continue;
					const button=document.createElement("button"); button.className="ghost-button"; button.textContent=nextStatus;
					button.onclick=async()=>{const note=await askAction({title:`Mark transfer ${nextStatus}`,message:`${item.characterName} from ${item.sourceRealm}`,label:"Staff note",defaultValue:item.staffNote||"",confirmText:"Update request"});if(note===null)return;button.disabled=true;try{const result=await api("/api/admin/transfers/"+item.id,{method:"POST",body:JSON.stringify({status:nextStatus,staffNote:note})});toast(`Transfer updated${result.requestId?` · request ${result.requestId}`:""}`);await load()}catch(error){toast(error.message);button.disabled=false}};
					qs(".row-actions",row).append(button);
				}
				box.append(row);
			}
			if(!box.children.length) box.innerHTML='<p class="muted">No transfer requests match these filters.</p>';
			renderPagination(box,data.pagination,"transfersPage",load);
		} catch(error) { box.innerHTML=`<p class="empty">${esc(error.message)}</p>`; }
	}

	return {load};
}
