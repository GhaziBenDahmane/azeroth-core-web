import {esc, publicLink, qs} from "/js/ui.js";

export function mountPlay(context) {
	const { api } = context;
	api("/api/downloads").then((data) => {
		const box=qs("#download-center"); box.innerHTML="";
		for(const item of data.downloads||[]){const card=document.createElement("article");card.className="download-card";const mirrors=(item.mirrors||[]).map((mirror,index)=>{const url=publicLink(mirror.url);return url?`<a class="ghost-button" href="${esc(url)}" rel="noreferrer">${esc(mirror.label||`Mirror ${index+2}`)}</a>`:""}).join(""),primaryURL=publicLink(item.url),signatureURL=publicLink(item.signatureUrl),virusTotalURL=publicLink(item.virusTotalUrl),changelogURL=publicLink(item.changelogUrl);card.innerHTML=`<p class="eyebrow">${esc(item.platform)}</p><h3>${esc(item.name)}</h3><div class="download-meta"><span>${esc(item.version||"")}</span><span>${esc(item.fileSize||"")}</span>${item.releasedAt?`<span>Released ${esc(item.releasedAt)}</span>`:""}</div><p>${esc(item.notes||"")}</p>${item.requirements?`<details><summary>System requirements</summary><p>${esc(item.requirements)}</p></details>`:""}${item.sha256?`<code title="SHA-256">SHA-256 ${esc(item.sha256)}</code>`:""}<div class="row-actions">${primaryURL?`<a class="button small" href="${esc(primaryURL)}" rel="noreferrer">Primary download</a>`:""}${mirrors}${signatureURL?`<a class="ghost-button" href="${esc(signatureURL)}" rel="noreferrer">Signature</a>`:""}${virusTotalURL?`<a class="ghost-button" href="${esc(virusTotalURL)}" target="_blank" rel="noreferrer">VirusTotal</a>`:""}${changelogURL?`<a class="ghost-button" href="${esc(changelogURL)}" target="_blank" rel="noreferrer">Changelog</a>`:""}</div>`;box.append(card)}
		if(!box.children.length)box.innerHTML='<p class="empty">No official client mirrors are configured yet.</p>';
	}).catch((e)=>{qs("#download-center").innerHTML=`<p class="empty">${esc(e.message)}</p>`});
}
