import {esc, qs} from "/js/ui.js";

export function createAdminRealmConfig({api, askAction, toast}) {
	async function load() {
		const body=qs("#realm-config-items"),summary=qs("#realm-config-summary"),apply=qs("#realm-config-apply");
		if(!body||!summary||!apply)return;
		body.innerHTML='<tr><td colspan="4">Checking configuration…</td></tr>';apply.classList.add("hidden");
		try{
			const data=await api("/api/admin/realm-config"),items=data.items||[],drifted=items.filter((item)=>item.state==="drifted");
			summary.className=`notice-box ${data.configured?(drifted.length?"warning":"success"):"warning"}`;
			summary.innerHTML=data.configured?`<p><b>${drifted.length?`${drifted.length} setting${drifted.length===1?"":"s"} differ`:"Worldserver configuration is in sync"}.</b> ${data.mode==="mock"?"This is a safe demonstration snapshot.":`Agent ${esc(data.snapshot?.version||"connected")}.`}</p>`:`<p><b>Display metadata only.</b> ${esc(data.message||"No realm configuration agent is connected.")}</p>`;
			body.innerHTML=items.map((item)=>`<tr><td><b>${esc(item.label)}</b><small><code>${esc(item.key)}</code>${item.restartRequired?" · restart required":""}</small></td><td>${esc(String(item.desired??"—"))}</td><td>${esc(String(item.observed??"—"))}</td><td><strong class="status-${esc(item.state)}">${esc(item.state.replaceAll("_"," "))}</strong></td></tr>`).join("")||'<tr><td colspan="4">No managed settings.</td></tr>';
			apply.classList.toggle("hidden",!data.configured||!drifted.length);
			apply.onclick=async()=>{const confirmed=await askAction({title:"Apply realm configuration",message:`Apply ${drifted.length} allow-listed change${drifted.length===1?"":"s"}? The agent creates a backup first and reports settings that require a restart.`,confirmText:"Apply changes",input:false});if(!confirmed)return;apply.disabled=true;try{const result=await api("/api/admin/realm-config/apply",{method:"POST",body:"{}"});toast(`Realm configuration applied${result.requestId?` · request ${result.requestId}`:""}`);await load()}catch(error){toast(error.message)}finally{apply.disabled=false}};
		}catch(error){summary.className="notice-box error";summary.innerHTML=`<p><b>Configuration agent unavailable.</b> ${esc(error.message)}</p>`;body.innerHTML='<tr><td colspan="4">Observed worldserver values could not be loaded.</td></tr>'}
	}
	return {load};
}
