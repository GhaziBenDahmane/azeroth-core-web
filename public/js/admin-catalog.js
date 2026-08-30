import {esc, pageFromURL, qs, qsa, renderPagination, setMessage, updateURLQuery} from "/js/ui.js";

export function createAdminCatalog(context) {
	const {api, askAction, classes, localItemIcon, mutationSuccess, navigateAdmin, resolveItemIcon, toast, useLocalItemFallback, adminCan} = context;
	const iso = (value) => (value ? new Date(value).toISOString() : null);
	const inventorySlots = {
		1: "Head",
		2: "Neck",
		3: "Shoulders",
		4: "Shirt",
		5: "Chest",
		6: "Waist",
		7: "Legs",
		8: "Feet",
		9: "Wrists",
		10: "Hands",
		11: "Ring",
		12: "Trinket",
		13: "One-hand weapon",
		14: "Shield",
		15: "Ranged",
		16: "Back",
		17: "Two-hand weapon",
		18: "Bag",
		19: "Tabard",
		20: "Chest",
		21: "Main hand",
		22: "Off hand",
		23: "Held off-hand",
		25: "Thrown",
		26: "Ranged",
		28: "Relic",
	};
	let catalogItems = [],
		catalogVariants = [],
		catalogSearchTimer,
		catalogTableTimer,
		adminProducts = [],
		catalogSort = { key: "id", direction: 1 };
	function catalogItemIcon(img, item) {
		useLocalItemFallback(img);
		resolveItemIcon(img, item.itemId);
	}
	function renderCatalogItems() {
		const list = qs("#catalog-items"),
			preview = qs("#catalog-equipment"),
			kind = qs("#catalog-product-kind");
		list.innerHTML = "";
		preview.innerHTML = "";
		kind.textContent =
			catalogItems.length === 0
				? "Service or gold product · no items"
				: catalogItems.length === 1
					? "Single-item product · icon comes from this item ID"
					: `Set / bundle · ${catalogItems.length} different items`;
		const equipped = catalogItems.filter(
				(i) => inventorySlots[i.inventoryType] && i.inventoryType !== 18,
			),
			bags = catalogItems.filter((i) => i.inventoryType === 18);
		if (!equipped.length)
			preview.innerHTML =
				'<p class="muted">Add equipment to preview its slots.</p>';
		equipped.forEach((item) => {
			const card = document.createElement("article");
			card.className = `catalog-slot q-border-${item.quality || 0}`;
			card.innerHTML = `<img alt=""/><span><small>${inventorySlots[item.inventoryType]}</small><b>${esc(item.name || "Item " + item.itemId)}</b><em>iLvl ${item.itemLevel || "—"}</em></span>`;
			const img = qs("img", card);
			img.src = localItemIcon;
			catalogItemIcon(img, item);
			preview.append(card);
		});
		catalogItems.forEach((item, index) => {
			const row = document.createElement("div");
			row.className = "catalog-bundle-row";
			row.innerHTML = `<img alt=""/><span><b>${esc(item.name || "Item " + item.itemId)}</b><small>#${item.itemId} · ${item.inventoryType === 18 ? "Bag · " : ""}iLvl ${item.itemLevel || "—"}</small></span><label>Qty<input type="number" min="1" max="1000" value="${item.quantity || 1}"/></label><button type="button" class="ghost-button">Remove</button>`;
			const img = qs("img", row);
			img.src = localItemIcon;
			catalogItemIcon(img, item);
			qs("input", row).onchange = (e) => {
				item.quantity = Math.max(
					1,
					Math.min(1000, Number(e.target.value) || 1),
				);
				updateCatalogChangeSummary();
			};
			qs("button", row).onclick = () => {
				catalogItems.splice(index, 1);
				renderCatalogItems();
			};
			list.append(row);
		});
		if (bags.length) {
			const note = document.createElement("p");
			note.className = "catalog-bag-count";
			note.textContent = `${bags.reduce((n, b) => n + (b.quantity || 1), 0)} bag(s) included`;
			list.prepend(note);
		}
		updateCatalogChangeSummary();
	}
	function renderCatalogVariants() {
		const list=qs("#catalog-variants");if(!list)return;list.innerHTML="";
		catalogVariants.forEach((variant,index)=>{const row=document.createElement("div");row.className="catalog-variant-row";row.innerHTML=`<label>Option name<input data-field="name" maxlength="100" value="${esc(variant.name||"")}" required></label><label>SKU<input data-field="sku" maxlength="80" value="${esc(variant.sku||"")}" required></label><label>Price adjustment<input data-field="priceAdjustment" type="number" min="-10000000" max="10000000" value="${variant.priceAdjustment||0}"></label><label>Replacement items <input data-field="items" value="${esc((variant.items||[]).map(item=>`${item.itemId}:${item.quantity||1}`).join(", "))}" placeholder="itemId:qty, itemId:qty"></label><label class="check"><input data-field="active" type="checkbox" ${variant.active!==false?"checked":""}> Active</label><button type="button" class="ghost-button danger">Remove</button>`;qsa("input[data-field]",row).forEach(input=>{input.onchange=()=>{const field=input.dataset.field;if(field==="active")variant.active=input.checked;else if(field==="priceAdjustment")variant[field]=Number(input.value)||0;else if(field==="items")variant.items=input.value.split(",").map(part=>part.trim()).filter(Boolean).map(part=>{const [itemId,quantity]=part.split(":");return {itemId:Number(itemId),quantity:Number(quantity)||1}});else variant[field]=input.value}});qs("button",row).onclick=()=>{catalogVariants.splice(index,1);renderCatalogVariants()};list.append(row)});
		if(!catalogVariants.length)list.innerHTML='<p class="muted">No variants. The base package is purchased directly.</p>';
		updateCatalogChangeSummary();
	}
	function updateCatalogChangeSummary() {
		const form = qs("#catalog-editor"), summary = qs("#catalog-change-summary");
		if (!form || !summary) return;
		const regular = Number(form.elements.price?.value || 0), sale = Number(form.elements.salePrice?.value || 0),
			itemCount = catalogItems.reduce((total, item) => total + Number(item.quantity || 1), 0),
			gold = Number(form.elements.gold?.value || 0), level = Number(form.elements.serviceLevel?.value || 0),
			state = `${form.elements.active?.checked ? "Active" : "Archived"}${form.elements.featured?.checked ? " · Featured" : ""}`,
			price = sale > 0 ? `${sale.toLocaleString()} credits on sale (regular ${regular.toLocaleString()})` : `${regular.toLocaleString()} credits`,
			delivery = [itemCount ? `${itemCount} item${itemCount === 1 ? "" : "s"}` : "No items", level ? `Level ${level}` : "", gold ? `${gold.toLocaleString()} gold` : ""].filter(Boolean).join(" · ");
		summary.innerHTML = `<span><small>Price</small><b>${esc(price)}</b></span><span><small>Delivery</small><b>${esc(delivery)}</b></span><span><small>Options</small><b>${catalogVariants.length} variant${catalogVariants.length === 1 ? "" : "s"}</b></span><span><small>Publishing</small><b>${esc(state)}</b></span>`;
		updateCatalogArtworkPreview();
	}
	function updateCatalogArtworkPreview() {
		const form = qs("#catalog-editor"), preview = qs("#catalog-art-preview");
		if (!form?.elements.imageUrl || !preview) return;
		const value = form.elements.imageUrl.value.trim(), image = qs("img", preview), status = qs("small", preview);
		preview.classList.toggle("hidden", !value);
		if (!value) { image.removeAttribute("src"); status.textContent = ""; return; }
		status.textContent = "Checking artwork…";
		image.onload = () => { status.textContent = `${image.naturalWidth}×${image.naturalHeight} preview`; };
		image.onerror = () => { status.textContent = "Artwork could not be loaded in this browser."; };
		if (image.src !== value) image.src = value;
	}
	function renderCatalogTable() {
		const box = qs("#admin-products");
		if (!box) return;
		const term = (qs("#catalog-table-search")?.value || "")
				.trim()
				.toLowerCase(),
			status = qs("#catalog-table-status")?.value || "all";
		const rows = adminProducts
			.filter(
				(p) =>
					(status === "all" || (status === "active") === Boolean(p.active)) &&
					(!term ||
						[p.id, p.name, p.category, p.tier, classes[p.classId]].some((v) =>
							String(v || "")
								.toLowerCase()
								.includes(term),
						)),
			)
			.sort((a, b) => {
				let av = a[catalogSort.key],
					bv = b[catalogSort.key];
				if (typeof av === "string" || typeof bv === "string")
					return (
						String(av || "").localeCompare(String(bv || "")) *
						catalogSort.direction
					);
				return (Number(av) - Number(bv)) * catalogSort.direction;
			});
		box.innerHTML = "";
		if (!rows.length) {
			box.innerHTML =
				'<tr><td colspan="7" class="table-empty">No matching products.</td></tr>';
			return;
		}
		rows.forEach((p) => {
			const row = document.createElement("tr");
			const packageSummary = [
					p.serviceLevel ? `Level ${p.serviceLevel}` : "",
					p.gold ? `${Number(p.gold).toLocaleString()} gold` : "",
					p.serviceAction ? String(p.serviceAction).replaceAll("_", " ") : "",
					p.itemId ? `Item #${p.itemId} × ${p.quantity}` : "Bundle",
				]
					.filter(Boolean)
					.join(" · "),
				price = p.salePrice
					? `<del>${Number(p.price).toLocaleString()}</del> <strong>${Number(p.salePrice).toLocaleString()}</strong>`
					: `<strong>${Number(p.price).toLocaleString()}</strong>`,
				stock = p.stockLimit ? `${p.soldCount}/${p.stockLimit}` : "Unlimited";
			row.innerHTML = `<td>#${p.id}</td><td><b>${esc(p.name)}</b><small>${esc(p.tier || "")} ${p.featured ? "· Featured" : ""}</small></td><td>${esc(p.category)}<small>Order ${p.categoryOrder || 0} · ${p.classId ? classes[p.classId] : "Any class"}</small></td><td>${esc(packageSummary)}<small>Stock: ${stock}</small></td><td>${price} credits</td><td><span class="table-status ${p.active ? "is-active" : "is-archived"}">${p.active ? "Active" : "Archived"}</span></td><td><button type="button" class="ghost-button">Edit</button></td>`;
			qs("button", row).onclick = () =>
				navigateAdmin(`/admin/catalog/${p.id}/edit`);
			box.append(row);
		});
	}
	function fillCatalogForm(p = {}) {
		const f = qs("#catalog-editor");
		f.reset();
		catalogItems = (p.items || []).map((i) => ({
			...i,
			quantity: i.quantity || 1,
		}));
		catalogVariants=(p.variants||[]).map(variant=>({...variant,items:(variant.items||[]).map(item=>({...item}))}));
		for (const key of [
			"id",
			"name",
			"price",
			"salePrice",
			"stockLimit",
			"categoryOrder",
			"category",
			"classId",
			"tier",
			"serviceLevel",
			"gold",
			"serviceAction",
			"description",
			"imageUrl",
			"perAccountLimit",
			"tags",
			"visibility",
			"bundleId",
		])
			if (f.elements[key])
				f.elements[key].value =
					p[key] ??
					{
						category: "Items",
						classId: 0,
						serviceLevel: 0,
						gold: 0,
						perAccountLimit: 0,
						salePrice: 0,
						stockLimit: 0,
						categoryOrder: 0,
					}[key] ??
					"";
		f.elements.active.checked = p.id ? Boolean(p.active) : true;
		f.elements.featured.checked = Boolean(p.featured);
		f.elements.variantRequired.checked = Boolean(p.variantRequired);
		f.dataset.bundleId=String(p.bundleId||0);
		qs("#catalog-sold-count").textContent = Number(
			p.soldCount || 0,
		).toLocaleString();
		f.elements.startsAt.value = p.startsAt
			? new Date(p.startsAt).toISOString().slice(0, 16)
			: "";
		f.elements.endsAt.value = p.endsAt
			? new Date(p.endsAt).toISOString().slice(0, 16)
			: "";
		qs("#catalog-editor-title").textContent = p.id
			? `Edit #${p.id} · ${p.name}`
			: "New product";
		qs("#catalog-archive").classList.toggle("hidden", !p.id || !p.active);
		qs("#catalog-item-results").innerHTML = "";
		qs("#catalog-item-search").value = "";
		renderCatalogItems();
		renderCatalogVariants();
		qs("#catalog-list").classList.add("hidden");
		f.classList.remove("hidden");
		window.scrollTo({ top: 0, behavior: "smooth" });
	}
	async function openCatalogEditor(id) {
		try {
			const { product } = await api("/api/admin/products/" + id);
			fillCatalogForm(product);
		} catch (e) {
			toast(e.message);
		}
	}
	function mountCatalogEditor() {
		const form = qs("#catalog-editor"),
			search = qs("#catalog-item-search"),
			results = qs("#catalog-item-results");
		let activeItemResult = -1;
		search.setAttribute("role", "combobox");
		search.setAttribute("aria-autocomplete", "list");
		search.setAttribute("aria-controls", "catalog-item-results");
		search.setAttribute("aria-expanded", "false");
		results.setAttribute("role", "listbox");
		results.setAttribute("aria-label", "Matching WotLK items");
		const chooseCatalogItem = (item) => {
			const existing = catalogItems.find((entry) => entry.itemId === item.itemId);
			if (existing) existing.quantity++;
			else catalogItems.push({ ...item, quantity: 1 });
			renderCatalogItems(); results.innerHTML = ""; search.value = "";
			search.setAttribute("aria-expanded", "false"); search.removeAttribute("aria-activedescendant"); activeItemResult = -1; search.focus();
		};
		search.onkeydown = (event) => {
			const options = qsa('[role="option"]', results);
			if (!options.length) return;
			if (event.key === "Escape") { results.innerHTML = ""; search.setAttribute("aria-expanded", "false"); search.removeAttribute("aria-activedescendant"); activeItemResult = -1; return; }
			if (!["ArrowDown", "ArrowUp", "Enter"].includes(event.key)) return;
			event.preventDefault();
			if (event.key === "Enter" && activeItemResult >= 0) { options[activeItemResult].click(); return; }
			activeItemResult = event.key === "ArrowUp" ? (activeItemResult <= 0 ? options.length - 1 : activeItemResult - 1) : (activeItemResult + 1) % options.length;
			options.forEach((option, index) => option.setAttribute("aria-selected", String(index === activeItemResult)));
			search.setAttribute("aria-activedescendant", options[activeItemResult].id);
			options[activeItemResult].scrollIntoView({block:"nearest"});
		};
		if (!qs("#catalog-change-summary")) {
			const summary = document.createElement("aside");
			summary.id = "catalog-change-summary";
			summary.className = "catalog-change-summary";
			summary.setAttribute("aria-label", "Product change summary");
			qs(".panel-title", form).after(summary);
		}
		if (!qs("#catalog-advanced")) {
			const advanced = document.createElement("details");
			advanced.id = "catalog-advanced";
			advanced.className = "catalog-advanced";
			advanced.innerHTML = '<summary><span><b>Advanced pricing and delivery</b><small>Sales, stock, ordering, level, gold, availability, and purchase limits</small></span></summary><div class="catalog-advanced-fields"></div>';
			const target = qs(".catalog-advanced-fields", advanced);
			for (const name of ["salePrice", "stockLimit", "categoryOrder", "serviceLevel", "gold", "startsAt", "endsAt", "perAccountLimit"]) {
				const label = form.elements[name]?.closest("label");
				if (label) target.append(label);
			}
			qs(".catalog-item-picker", form).before(advanced);
			qsa(".gm-fields", form).forEach(group => { if (!group.children.length) group.remove(); });
		}
		if (!form.dataset.summaryBound) {
			form.addEventListener("input", updateCatalogChangeSummary);
			form.addEventListener("change", updateCatalogChangeSummary);
			form.dataset.summaryBound = "true";
		}
		qs("#catalog-table-search")?.setAttribute("aria-label","Search catalog products");
		qs("#catalog-table-status")?.setAttribute("aria-label","Filter catalog by status");
		if(!qs("#catalog-validate")){const button=document.createElement("button");button.id="catalog-validate";button.type="button";button.className="ghost-button";button.textContent="Validate delivery";qs("#catalog-archive").before(button);button.onclick=async()=>{const id=Number(form.elements.id.value);if(!id)return toast("Save the product before validating delivery");try{const result=await api(`/api/admin/products/${id}/validate`,{method:"POST",body:"{}"});toast(result.valid?`${result.product}: ${result.items} items and ${result.variants} variants validated${result.deliveryConfigured?"":"; SOAP is not configured"}`:`Validation failed: ${result.error}`)}catch(error){toast(error.message)}}}
		if(!qs("#catalog-variant-editor")){const section=document.createElement("fieldset");section.id="catalog-variant-editor";section.className="catalog-variant-editor";section.innerHTML='<legend>Product options</legend><div class="gm-fields"><label>Audience<select name="visibility"><option value="all">All players</option><option value="new">New accounts</option><option value="returning">Returning customers</option><option value="veteran">Veteran accounts</option></select></label><label>Search tags<input name="tags" maxlength="500" placeholder="pvp, starter, mount"></label></div><label>Custom artwork URL<input name="imageUrl" type="url" maxlength="500" placeholder="Paste a URL from Content → Media"><small>Shown as a backdrop. Item products still use their WotLK item icon.</small></label><figure id="catalog-art-preview" class="catalog-art-preview hidden"><img alt="Product artwork preview"><small></small></figure><label>Reusable bundle<select name="bundleId"><option value="0">None</option></select></label><label class="check"><input name="variantRequired" type="checkbox"> Require the player to choose an option</label><div id="catalog-variants"></div><button id="catalog-add-variant" class="ghost-button" type="button">Add option</button><p class="muted">Replacement items use <code>itemId:quantity</code>, separated by commas. Empty replacement items keep the base package.</p>';qs(".catalog-item-picker",form).before(section);qs("#catalog-add-variant").onclick=()=>{catalogVariants.push({name:"",sku:"",priceAdjustment:0,active:true,sortOrder:catalogVariants.length,items:[]});renderCatalogVariants()}}
		updateCatalogChangeSummary();
		qs("#catalog-new").onclick = (e) => {
			e.preventDefault();
			navigateAdmin("/admin/catalog/new");
		};
		qs("#catalog-cancel").onclick = () => navigateAdmin("/admin/catalog");
		search.oninput = () => {
			clearTimeout(catalogSearchTimer);
			activeItemResult = -1;
			const term = search.value.trim();
			if (term.length < 2) {
				results.innerHTML = "";
				search.setAttribute("aria-expanded", "false");
				search.removeAttribute("aria-activedescendant");
				return;
			}
			catalogSearchTimer = setTimeout(async () => {
				try {
					const data = await api(
						"/api/admin/items?q=" + encodeURIComponent(term),
					);
					results.innerHTML = "";
					data.items.forEach((item, index) => {
						const b = document.createElement("button");
						b.type = "button";
						b.id = `catalog-item-option-${index}`;
						b.setAttribute("role", "option");
						b.setAttribute("aria-selected", "false");
						b.innerHTML = `<span class="q${item.quality || 0}">${esc(item.name)}</span><small>#${item.itemId} · ${inventorySlots[item.inventoryType] || "Item"} · iLvl ${item.itemLevel}</small>`;
						b.onclick = () => chooseCatalogItem(item);
						results.append(b);
					});
					search.setAttribute("aria-expanded", String(data.items.length > 0));
					if (!data.items.length)
						results.innerHTML = '<p class="muted">No matching items.</p>';
				} catch (e) {
					results.innerHTML = `<p class="muted">${esc(e.message)}</p>`;
				}
			}, 250);
		};
		form.onsubmit = async (e) => {
			e.preventDefault();
			const fd = new FormData(form),
				id = Number(fd.get("id") || 0),
				payload = Object.fromEntries(fd);
			for (const key of [
				"price",
				"salePrice",
				"stockLimit",
				"categoryOrder",
				"classId",
				"serviceLevel",
				"gold",
				"perAccountLimit",
				"bundleId",
			])
				payload[key] = Number(payload[key] || 0);
			payload.active = fd.has("active");
			payload.featured = fd.has("featured");
			payload.variantRequired=fd.has("variantRequired");
			payload.variants=catalogVariants;
			payload.startsAt = iso(payload.startsAt);
			payload.endsAt = iso(payload.endsAt);
			if (catalogItems.length === 1) {
				payload.itemId = catalogItems[0].itemId;
				payload.quantity = catalogItems[0].quantity;
				payload.items = [];
			} else {
				payload.itemId = 0;
				payload.quantity = 0;
				payload.items = catalogItems.map((i) => ({
					itemId: i.itemId,
					quantity: i.quantity,
				}));
			}
			delete payload.id;
			try {
				const result = await api(
					id ? "/api/admin/products/" + id : "/api/admin/products",
					{ method: id ? "PUT" : "POST", body: JSON.stringify(payload) },
				);
				mutationSuccess(form, id ? "Product updated." : `Product #${result.id} created.`, result);
				await loadPortalAdmin();
				if (!id && result.id) navigateAdmin(`/admin/catalog/${result.id}/edit`);
			} catch (err) {
				setMessage(form, err.message);
			}
		};
		qs("#catalog-archive").onclick = async () => {
			const id = Number(form.elements.id.value),
				typed = await askAction({ title: `Archive product #${id}`, message: "Existing orders remain intact.", label: "Type ARCHIVE", expected: "ARCHIVE", confirmText: "Archive product" });
			if (id && typed === "ARCHIVE") {
				await api("/api/admin/products/" + id, { method: "DELETE" });
				await loadPortalAdmin();
				navigateAdmin("/admin/catalog");
			}
		};
	}
	function mountShopMerchandising() {
		const host=qs("#catalog-list");if(!host||qs("#shop-merchandising"))return;
		const couponForm=qs("#gm-coupon-form");if(couponForm&&!couponForm.elements.minSubtotal){const fields=document.createElement("div");fields.className="gm-fields";fields.innerHTML='<label>Minimum subtotal<input name="minSubtotal" type="number" min="0" max="10000000" value="0"></label><label>Category restriction<input name="category" maxlength="40" placeholder="Any category"></label>';const toggle=document.createElement("label");toggle.className="check";toggle.innerHTML='<input name="allowSale" type="checkbox"> Allow this coupon on sale-priced products';couponForm.querySelector("button[type=submit]").before(fields,toggle)}
		const panel=document.createElement("div");panel.id="shop-merchandising";panel.className="admin-dashboard-grid";panel.innerHTML='<article class="account-panel"><form id="collection-form"><input name="id" type="hidden"><div class="panel-title"><div><p class="eyebrow">MERCHANDISING</p><h3>Collections</h3></div><button id="collection-reset" class="ghost-button" type="button">New</button></div><div class="gm-fields"><label>Name<input name="name" maxlength="100" required></label><label>Slug<input name="slug" pattern="[a-z0-9]+(?:-[a-z0-9]+)*" required></label></div><label>Description<textarea name="description" maxlength="500"></textarea></label><label>Collection artwork URL<input name="imageUrl" type="url" maxlength="500" placeholder="Paste a URL from Content → Media"></label><label>Product IDs<input name="productIds" placeholder="1, 2, 3"></label><div class="gm-fields"><label>Display order<input name="sortOrder" type="number" value="0"></label><span class="feature-switches"><label><input name="featured" type="checkbox"> Featured</label><label><input name="active" type="checkbox" checked> Active</label></span></div><button class="button" type="submit">Save collection</button><p class="form-message" role="status"></p></form><div id="admin-collections" class="admin-table"></div></article><article class="account-panel"><form id="stock-form"><p class="eyebrow">INVENTORY</p><h3>Adjust limited stock</h3><div class="gm-fields"><label>Product<select name="productId" required></select></label><label>Change<input name="delta" type="number" min="-1000000" max="1000000" placeholder="+10 or -5" required></label></div><label>Audit reason<input name="reason" minlength="3" maxlength="500" required></label><button class="button" type="submit">Adjust stock ceiling</button><p class="form-message" role="status"></p></form><div id="stock-history" class="admin-table"></div></article>';
		host.append(panel);const bundlePanel=document.createElement("article");bundlePanel.className="account-panel";bundlePanel.innerHTML='<form id="bundle-template-form"><input name="id" type="hidden"><div class="panel-title"><div><p class="eyebrow">REUSABLE CONTENT</p><h3>Bundle templates</h3></div><button id="bundle-template-reset" class="ghost-button" type="button">New</button></div><div class="gm-fields"><label>Name<input name="name" maxlength="100" required></label><label>Description<input name="description" maxlength="500"></label></div><label>Items<input name="items" placeholder="itemId:qty, itemId:qty" required></label><button class="button" type="submit">Save bundle template</button><p class="form-message" role="status"></p></form><div id="admin-bundle-templates" class="admin-table"></div>';panel.append(bundlePanel);const importPanel=document.createElement("article");importPanel.className="account-panel";importPanel.innerHTML='<form id="catalog-import-form"><p class="eyebrow">BULK CATALOG</p><h3>CSV import</h3><p class="muted">Required columns: <code>name,price,category</code>. Optional items use <code>itemId:quantity;itemId:quantity</code>.</p><label>CSV data<textarea name="csv" rows="7" maxlength="1048576" placeholder="name,price,category,item_id,quantity\nStarter Bag,10,Utility,51809,1" required></textarea></label><div class="row-actions"><button class="ghost-button" type="submit">Validate preview</button><button id="catalog-import-commit" class="button hidden" type="button">Import validated rows</button></div><p class="form-message" role="status"></p></form><div id="catalog-import-preview" class="admin-table"></div>';panel.append(importPanel);const collectionForm=qs("#collection-form"),stockForm=qs("#stock-form"),bundleForm=qs("#bundle-template-form"),importForm=qs("#catalog-import-form");
		const resetCollection=()=>{collectionForm.reset();collectionForm.elements.id.value="";collectionForm.elements.active.checked=true;setMessage(collectionForm,"")};
		const loadCollections=async()=>{const box=qs("#admin-collections");try{const {collections}=await api("/api/admin/shop/collections");box.innerHTML="";(collections||[]).forEach(collection=>{const row=document.createElement("div");row.className="admin-row";row.innerHTML=`<span><b>${esc(collection.name)}</b><small>/${esc(collection.slug)} · ${(collection.productIds||[]).length} products · ${collection.active?"Active":"Archived"}</small></span><span class="row-actions"><button type="button" class="ghost-button">Edit</button><button type="button" class="ghost-button danger">Archive</button></span>`;const [edit,archive]=qsa("button",row);edit.onclick=()=>{for(const key of ["id","name","slug","description","imageUrl","sortOrder"])collectionForm.elements[key].value=collection[key]??"";collectionForm.elements.productIds.value=(collection.productIds||[]).join(", ");collectionForm.elements.featured.checked=Boolean(collection.featured);collectionForm.elements.active.checked=Boolean(collection.active)};archive.onclick=async()=>{if(!(await askAction({title:"Archive collection",message:collection.name,input:false,confirmText:"Archive"})))return;await api(`/api/admin/shop/collections/${collection.id}`,{method:"DELETE"});loadCollections()};box.append(row)});if(!box.children.length)box.innerHTML='<p class="muted">No collections.</p>'}catch(error){box.innerHTML=`<p class="empty">${esc(error.message)}</p>`}};
		collectionForm.onsubmit=async event=>{event.preventDefault();const values=Object.fromEntries(new FormData(collectionForm)),id=values.id;delete values.id;values.productIds=String(values.productIds||"").split(",").map(Number).filter(Boolean);values.sortOrder=Number(values.sortOrder)||0;values.active=collectionForm.elements.active.checked;values.featured=collectionForm.elements.featured.checked;try{const result=await api(id?`/api/admin/shop/collections/${id}`:"/api/admin/shop/collections",{method:id?"PUT":"POST",body:JSON.stringify(values)});resetCollection();mutationSuccess(collectionForm,"Collection saved.",result);loadCollections()}catch(error){setMessage(collectionForm,error.message)}};qs("#collection-reset").onclick=resetCollection;
		const loadStock=async()=>{stockForm.elements.productId.innerHTML=adminProducts.filter(product=>product.stockLimit>0).map(product=>`<option value="${product.id}">${esc(product.name)} · ${product.soldCount}/${product.stockLimit}</option>`).join("");const box=qs("#stock-history");try{const data=await api(`/api/admin/shop/stock?page=${pageFromURL("stockPage")}&perPage=25`),movements=data.movements||[];box.innerHTML=movements.map(item=>`<div class="admin-row"><span><b>Product #${item.productId} · ${item.delta>0?"+":""}${item.delta}</b><small>${esc(item.type)} · ${esc(item.reason||item.reference||"")} · ${new Date(item.createdAt).toLocaleString()}</small></span></div>`).join("")||'<p class="muted">No stock movements.</p>';renderPagination(box,data.pagination,"stockPage",loadStock)}catch(error){box.innerHTML=`<p class="empty">${esc(error.message)}</p>`}};
		stockForm.onsubmit=async event=>{event.preventDefault();const values=Object.fromEntries(new FormData(stockForm));values.productId=Number(values.productId);values.delta=Number(values.delta);try{await api("/api/admin/shop/stock",{method:"POST",body:JSON.stringify(values)});setMessage(stockForm,"Stock adjusted.",true);stockForm.elements.delta.value="";await loadPortalAdmin();loadStock()}catch(error){setMessage(stockForm,error.message)}};
		const resetBundle=()=>{bundleForm.reset();bundleForm.elements.id.value=""};qs("#bundle-template-reset").onclick=resetBundle;
		const loadBundles=async()=>{const box=qs("#admin-bundle-templates");try{const {bundles}=await api("/api/admin/shop/bundles");box.innerHTML="";const editor=qs("#catalog-editor"),select=editor?.elements.bundleId;if(select){select.innerHTML='<option value="0">None</option>'+(bundles||[]).map(bundle=>`<option value="${bundle.id}">${esc(bundle.name)}</option>`).join("");select.value=editor.dataset.bundleId||"0"}for(const bundle of bundles||[]){const row=document.createElement("div");row.className="admin-row";row.innerHTML=`<span><b>${esc(bundle.name)}</b><small>${(bundle.items||[]).length} items · ${esc(bundle.description||"")}</small></span><span class="row-actions"><button type="button" class="ghost-button">Edit</button><button type="button" class="ghost-button danger">Delete</button></span>`;const [edit,remove]=qsa("button",row);edit.onclick=()=>{bundleForm.elements.id.value=bundle.id;bundleForm.elements.name.value=bundle.name;bundleForm.elements.description.value=bundle.description||"";bundleForm.elements.items.value=(bundle.items||[]).map(item=>`${item.itemId}:${item.quantity}`).join(", ");bundleForm.scrollIntoView({behavior:"smooth",block:"center"});bundleForm.elements.name.focus()};remove.onclick=async()=>{if(!(await askAction({title:"Delete bundle template",message:"Products using this template must be detached first.",input:false,confirmText:"Delete"})))return;await api(`/api/admin/shop/bundles/${bundle.id}`,{method:"DELETE"});resetBundle();loadBundles()};box.append(row)}if(!box.children.length)box.innerHTML='<p class="muted">No reusable bundles.</p>'}catch(error){box.innerHTML=`<p class="empty">${esc(error.message)}</p>`}};
		bundleForm.onsubmit=async event=>{event.preventDefault();const values=Object.fromEntries(new FormData(bundleForm)),id=Number(values.id)||0;delete values.id;values.items=String(values.items||"").split(",").map(part=>part.trim()).filter(Boolean).map(part=>{const [itemId,quantity]=part.split(":");return {itemId:Number(itemId),quantity:Number(quantity)||1}});try{const result=await api(id?`/api/admin/shop/bundles/${id}`:"/api/admin/shop/bundles",{method:id?"PUT":"POST",body:JSON.stringify(values)});mutationSuccess(bundleForm,id?"Bundle template updated.":"Bundle template created.",result);resetBundle();loadBundles()}catch(error){setMessage(bundleForm,error.message)}};
		const previewImport=async commit=>{const button=qs("#catalog-import-commit"),box=qs("#catalog-import-preview");try{const data=await api("/api/admin/products/import",{method:"POST",body:JSON.stringify({csv:importForm.elements.csv.value,commit})});if(commit){setMessage(importForm,`${data.imported} products imported.`,true);button.classList.add("hidden");box.innerHTML="";await loadPortalAdmin();return}box.innerHTML=(data.rows||[]).map(row=>`<div class="admin-row"><span><b>Row ${row.row} · ${esc(row.product.name||"Unnamed")}</b><small>${row.errors?.length?esc(row.errors.join(" · ")):"Ready to import"}</small></span><strong class="${row.errors?.length?"status-failed":"status-executed"}">${row.errors?.length?"Invalid":"Valid"}</strong></div>`).join("");button.classList.toggle("hidden",!data.valid);setMessage(importForm,data.valid?`${data.count} rows passed validation.`:"Fix invalid rows before importing.",data.valid)}catch(error){button.classList.add("hidden");setMessage(importForm,error.message)}};importForm.onsubmit=event=>{event.preventDefault();previewImport(false)};qs("#catalog-import-commit").onclick=async()=>{if(!(await askAction({title:"Import catalog",message:"Every validated row will become a product in this realm.",label:"Type IMPORT",expected:"IMPORT",confirmText:"Import products"})))return;previewImport(true)};loadCollections();loadStock();loadBundles();
	}
	async function loadPortalAdmin() {
		try {
			const adminPath = location.pathname.replace(/\/+$/, "") || "/admin",
				commerceView = adminPath === "/admin/catalog" || /^\/admin\/catalog\/(?:new|\d+\/edit)$/.test(adminPath);
			const productParams = new URLSearchParams({page:String(pageFromURL("catalogPage")),perPage:"25",sort:catalogSort.key,direction:catalogSort.direction < 0 ? "desc" : "asc"});
			const catalogQuery = qs("#catalog-table-search")?.value.trim() || "", catalogStatus = qs("#catalog-table-status")?.value || "all";
			if (catalogQuery) productParams.set("q", catalogQuery); if (catalogStatus !== "all") productParams.set("status", catalogStatus);
			const [products, coupons, creditPackages, giftCodes] = await Promise.all([
				adminCan("commerce") && commerceView
					? api("/api/admin/products?" + productParams)
					: Promise.resolve({ products: [] }),
				adminCan("commerce") && commerceView
					? api(`/api/admin/coupons?page=${pageFromURL("couponsPage")}&perPage=25`)
					: Promise.resolve({ coupons: [] }),
				adminCan("commerce") && commerceView
					? api("/api/admin/credit-packages")
					: Promise.resolve({ packages: [] }),
				adminCan("commerce") && commerceView
					? api(`/api/admin/gift-codes?page=${pageFromURL("giftCodesPage")}&perPage=25`)
					: Promise.resolve({ codes: [] }),
			]);
			if (adminCan("commerce") && commerceView) {
				const giftBox=qs("#admin-gift-codes");giftBox.innerHTML="";for(const code of giftCodes.codes||[]){const row=document.createElement("div");row.className="admin-row";row.innerHTML=`<span><b>${esc(code.codeHint)}</b><small>${Number(code.credits).toLocaleString()} credits · ${code.usedCount}/${code.maxUses} uses${code.expiresAt?" · expires "+new Date(code.expiresAt).toLocaleString():""}</small></span><span class="row-actions"></span>`;if(code.active){const disable=document.createElement("button");disable.className="ghost-button";disable.textContent="Disable";disable.onclick=async()=>{await api("/api/admin/gift-codes/"+code.id,{method:"DELETE"});loadPortalAdmin()};qs(".row-actions",row).append(disable)}giftBox.append(row)}if(!giftBox.children.length)giftBox.innerHTML='<p class="muted">No gift codes.</p>';renderPagination(giftBox,giftCodes.pagination,"giftCodesPage",loadPortalAdmin);
				const creditBox=qs("#admin-credit-packages");creditBox.innerHTML="";for(const pack of creditPackages.packages||[]){const row=document.createElement("div");row.className="admin-row";row.innerHTML=`<span><b>${esc(pack.name)}</b><small>${Number(pack.credits).toLocaleString()} credits · ${esc(pack.slug)} · ${pack.active?"active":"archived"}</small></span><span class="row-actions"></span>`;if(pack.id){const remove=document.createElement("button");remove.className="ghost-button";remove.textContent="Archive";remove.onclick=async()=>{await api("/api/admin/credit-packages/"+pack.id,{method:"DELETE"});loadPortalAdmin()};qs(".row-actions",row).append(remove)}creditBox.append(row)}
				adminProducts = products.products;
				renderCatalogTable();
				renderPagination(qs(".catalog-data-table")?.closest(".data-table-wrap"), products.pagination, "catalogPage", loadPortalAdmin);
				if(qs("#stock-form"))qs("#stock-form").elements.productId.innerHTML=adminProducts.filter(product=>product.stockLimit>0).map(product=>`<option value="${product.id}">${esc(product.name)} · ${product.soldCount}/${product.stockLimit}</option>`).join("");
				const couponBox = qs("#admin-coupons");
				couponBox.innerHTML = "";
				coupons.coupons.forEach((c) => {
					const row = document.createElement("div");
					row.className = "admin-row";
					row.innerHTML = `<span><b>${esc(c.code)}</b><small>${c.discountPercent || 0}% + ${c.discountCredits || 0} credits · ${c.uses||0}${c.maxUses?`/${c.maxUses}`:""} uses · ${c.allowSale?"stacks with sales":"no sale stacking"}${c.minSubtotal?` · minimum ${c.minSubtotal}`:""}${c.category?` · ${esc(c.category)} only`:""} · ${c.active ? "active" : "disabled"}</small></span>`;
					const b = document.createElement("button");
					b.className = "ghost-button";
					b.textContent = "Disable";
					b.onclick = async () => {
						if (await askAction({ title: `Disable ${c.code}`, message: "The coupon can no longer be redeemed.", label: "Type DISABLE", expected: "DISABLE", confirmText: "Disable coupon" }) !== "DISABLE")
							return;
						await api("/api/admin/coupons/" + c.id, { method: "DELETE" });
						loadPortalAdmin();
					};
					row.append(b);
					couponBox.append(row);
				});
				renderPagination(couponBox,coupons.pagination,"couponsPage",loadPortalAdmin);
			}
		} catch (e) {
			toast(e.message);
		}
	}
	let controlsBound = false;
	function exportCSV(name, fields, rows) {
		const csv = [
			fields.join(","),
			...rows.map((row) => fields.map((field) => `"${String(row[field] ?? row[field.toLowerCase()] ?? "").replaceAll('"', '""')}"`).join(",")),
		].join("\n");
		const blob = new Blob([csv], {type: "text/csv"}), link = document.createElement("a");
		link.href = URL.createObjectURL(blob);
		link.download = name;
		link.click();
		URL.revokeObjectURL(link.href);
	}
	function mount() {
		mountCatalogEditor();
		if (controlsBound) return;
		controlsBound = true;
		const catalogURL = new URLSearchParams(location.search);
		qs("#catalog-table-search").value = catalogURL.get("catalogQ") || "";
		qs("#catalog-table-status").value = catalogURL.get("catalogStatus") || "all";
		if (catalogURL.get("catalogSort")) catalogSort.key = catalogURL.get("catalogSort");
		if (catalogURL.get("catalogDirection") === "desc") catalogSort.direction = -1;
		qs("#catalog-table-search").oninput = () => {
			clearTimeout(catalogTableTimer);
			catalogTableTimer = setTimeout(() => {
				updateURLQuery({catalogQ: qs("#catalog-table-search").value.trim(), catalogPage: 1});
				loadPortalAdmin();
			}, 250);
		};
		qs("#catalog-table-status").onchange = () => {
			updateURLQuery({catalogStatus: qs("#catalog-table-status").value, catalogPage: 1});
			loadPortalAdmin();
		};
		qsa("[data-catalog-sort]").forEach((button) => {
			button.onclick = () => {
				const key = button.dataset.catalogSort;
				if (catalogSort.key === key) catalogSort.direction *= -1;
				else catalogSort = {key, direction: 1};
				updateURLQuery({catalogSort: catalogSort.key, catalogDirection: catalogSort.direction < 0 ? "desc" : "asc", catalogPage: 1});
				loadPortalAdmin();
			};
		});
		qs("#catalog-export").onclick = () => exportCSV("shop-catalog.csv", ["id", "name", "category", "price", "salePrice", "stockLimit", "soldCount", "active"], adminProducts);
		qs("#gm-coupon-form").onsubmit = async (event) => {
			event.preventDefault();
			const form = event.currentTarget, values = Object.fromEntries(new FormData(form));
			for (const key of ["discountPercent", "discountCredits", "maxUses", "perAccountLimit", "minSubtotal"]) values[key] = Number(values[key]);
			values.allowSale = form.elements.allowSale.checked;
			values.startsAt = iso(values.startsAt);
			values.endsAt = iso(values.endsAt);
			try {
				await api("/api/admin/coupons", {method: "POST", body: JSON.stringify(values)});
				form.reset();
				setMessage(form, "Coupon created.", true);
				loadPortalAdmin();
			} catch (error) { setMessage(form, error.message); }
		};
		qs("#credit-package-form").onsubmit = async (event) => {
			event.preventDefault();
			const form = event.currentTarget, values = Object.fromEntries(new FormData(form));
			values.credits = Number(values.credits);
			values.sortOrder = Number(values.sortOrder || 0);
			try {
				await api("/api/admin/credit-packages", {method: "POST", body: JSON.stringify(values)});
				form.reset(); setMessage(form, "Credit package added.", true); loadPortalAdmin();
			} catch (error) { setMessage(form, error.message); }
		};
		qs("#gift-code-admin-form").onsubmit = async (event) => {
			event.preventDefault();
			const form = event.currentTarget, values = Object.fromEntries(new FormData(form));
			values.credits = Number(values.credits);
			values.maxUses = Number(values.maxUses);
			values.expiresAt = iso(values.expiresAt);
			try {
				const result = await api("/api/admin/gift-codes", {method: "POST", body: JSON.stringify(values)}), output = qs("#generated-gift-code");
				output.textContent = result.code; output.classList.remove("hidden"); form.reset();
				setMessage(form, "Copy this code now; only its final characters are stored.", true); loadPortalAdmin();
			} catch (error) { setMessage(form, error.message); }
		};
	}
	return {fill: fillCatalogForm, load: loadPortalAdmin, mount, mountMerchandising: mountShopMerchandising, open: openCatalogEditor};
}
