import {esc, qs} from "/js/ui.js";

export function mountContentPage(context) {
	const { api } = context;
	const slugMatch=location.pathname.match(/^\/pages\/([^/]+)\/?$/), body=qs("#content-page-body");
	if (!slugMatch) body.innerHTML='<p class="empty">Page not found.</p>';
	else api("/api/pages/"+encodeURIComponent(decodeURIComponent(slugMatch[1]))).then((item)=>{
		qs("#content-page-title").textContent=item.title;qs("#content-page-summary").textContent=item.summary||"";body.innerHTML="";
		const content=document.createElement("div");content.className="article-body";content.textContent=item.body;body.append(content);
		document.title=(item.seoTitle||item.title)+" — "+document.title.split(" — ").pop();const description=qs('meta[name="description"]');if(description&&item.seoDescription)description.content=item.seoDescription;
	}).catch((error)=>{body.innerHTML=`<p class="empty">${esc(error.message)}</p>`});

}
