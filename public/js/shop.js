import {esc, qs, qsa, setMessage, updateURLQuery} from "/js/ui.js";

export function mountShop(context) {
	const { api, toast, classes, classSetPreviewItems, iconBase, localItemIcon, useLocalItemFallback, resolveItemIcon } = context;
	const grid = qs("#product-grid"),
		filters = qs("#shop-filters"),
		dialog = qs("#purchase-dialog");
	const detailMatch = location.pathname.match(/^\/shop\/(\d+)\/?$/),
		detailProductID = detailMatch ? Number(detailMatch[1]) : 0;
	let products = [],
		collections = [],
		selected = null,
			activeCollection = "",
			characters = [],
			activeCategory = "All",
			activeClass = 0,
			shopQuery = "",
			shopSort = "featured",
			shopPage = 1,
			orderHistory = [],
			wishlist = new Set(),
			comparison = new Set();
	const shopPageSize = 12,
		shopURL = new URLSearchParams(location.search);
	activeCollection = shopURL.get("collection") || "";
	activeCategory = shopURL.get("category") || "All";
	activeClass = Number(shopURL.get("class") || 0);
	shopQuery = shopURL.get("q") || "";
	shopSort = shopURL.get("sort") || "featured";
	shopPage = Math.max(1, Number(shopURL.get("page")) || 1);
	const comparisonPanel = document.createElement("section");
	comparisonPanel.className = "shop-comparison hidden";
	grid.before(comparisonPanel);
	function renderComparison() {
		const selectedProducts = products.filter((p) => comparison.has(p.id));
		comparisonPanel.classList.toggle("hidden", selectedProducts.length < 2);
		comparisonPanel.innerHTML = selectedProducts.length < 2 ? "" : `<div class="panel-title"><div><p class="eyebrow">COMPARE</p><h2>${selectedProducts.length} packages</h2></div><button class="ghost-button" type="button">Clear</button></div><div class="comparison-grid">${selectedProducts.map((p)=>`<article><h3>${esc(p.name)}</h3><strong>${Number(p.salePrice||p.price).toLocaleString()} credits</strong><p>${esc(p.description||"")}</p><small>${esc(p.category||"Package")} ${p.tier?`· ${esc(p.tier)}`:""}</small><ul>${(p.includes||[]).slice(0,6).map((item)=>`<li>${esc(item)}</li>`).join("")}</ul></article>`).join("")}</div>`;
		if (selectedProducts.length >= 2) qs("button", comparisonPanel).onclick = () => { comparison.clear(); render(); renderComparison(); };
	}
	async function toggleWishlist(product, button) {
		const saved = wishlist.has(product.id);
		try {
			await api(`/api/wishlist/${product.id}`, { method: saved ? "DELETE" : "PUT", body: "{}" });
			if (saved) wishlist.delete(product.id); else wishlist.add(product.id);
			button.textContent = saved ? "Save" : "Saved";
			button.classList.toggle("is-saved", !saved);
			button.setAttribute("aria-pressed", String(!saved));
		} catch (error) {
			if (error.status === 401) location.href = "/login?next=" + encodeURIComponent(location.pathname);
			else toast(error.message);
		}
	}
	function renderProductArt(art, p) {
		const leadItem =
			p.itemId || p.items?.[0]?.itemId || classSetPreviewItems[p.classId];
		if (p.imageUrl) {
			const backdrop = document.createElement("img");
			backdrop.className = "product-art-backdrop";
			backdrop.src = p.imageUrl;
			backdrop.alt = "";
			backdrop.loading = "lazy";
			backdrop.referrerPolicy = "no-referrer";
			backdrop.onerror = () => backdrop.remove();
			art.append(backdrop);
			art.classList.add("has-custom-art");
		}
		if (p.gold > 0 && !leadItem) {
			const img = document.createElement("img");
			img.className = "product-icon gold-icon";
			img.src = iconBase + "inv_misc_coin_01.jpg";
			img.alt = "";
			useLocalItemFallback(img);
			art.classList.add("gold-art");
			art.append(img);
			return;
		}
		if (leadItem) {
			const img = document.createElement("img");
			img.className = "product-icon";
			img.src = localItemIcon;
			img.alt = "";
			useLocalItemFallback(img);
			resolveItemIcon(img, leadItem);
			art.append(img);
			if (p.classId) {
				art.classList.add("class-art-" + p.classId);
				const label = document.createElement("span");
				label.className = "product-art-label";
				label.textContent = classes[p.classId] || p.className || "Set";
				art.append(label);
			}
			return;
		}
		art.textContent = p.classId
			? classes[p.classId] || p.className || "Class"
			: p.category || "Package";
		art.classList.add(p.classId ? "class-art-" + p.classId : "service-art");
	}
	function render() {
		grid.innerHTML = "";
		const normalizedQuery = shopQuery.trim().toLowerCase(),
			filtered = products
			.filter(
				(p) =>
					(!activeCollection || (p.collections || []).includes(activeCollection) || collections.find(c=>c.slug===activeCollection)?.productIds?.includes(p.id)) &&
					(activeCategory === "All" || p.category === activeCategory) &&
					(!activeClass || !p.classId || p.classId === activeClass) &&
					(!normalizedQuery || [p.name,p.category,p.className,classes[p.classId],p.tier,p.description,...(p.includes||[])].filter(Boolean).join(" ").toLowerCase().includes(normalizedQuery)),
			),
			sorters = {
				featured: (a,b) => Number(b.featured)-Number(a.featured) || Number(a.categoryOrder)-Number(b.categoryOrder) || Number(a.salePrice||a.price)-Number(b.salePrice||b.price),
				"price-asc": (a,b) => Number(a.salePrice||a.price)-Number(b.salePrice||b.price),
				"price-desc": (a,b) => Number(b.salePrice||b.price)-Number(a.salePrice||a.price),
				name: (a,b) => String(a.name).localeCompare(String(b.name)),
				stock: (a,b) => Number(Boolean(b.stockLimit))-Number(Boolean(a.stockLimit)) || Math.max(0,Number(a.stockLimit)-Number(a.soldCount))-Math.max(0,Number(b.stockLimit)-Number(b.soldCount)),
			},
			sorted = filtered.sort(sorters[shopSort] || sorters.featured),
			pageCount = Math.max(1, Math.ceil(sorted.length / shopPageSize));
		shopPage = Math.min(shopPage, pageCount);
		const shown = detailProductID ? sorted : sorted.slice((shopPage - 1) * shopPageSize, shopPage * shopPageSize),
			count = qs("#shop-result-count"), pager = qs("#shop-pagination"), clear = qs("#shop-clear-filters");
		if (count) count.textContent = `${sorted.length.toLocaleString()} product${sorted.length === 1 ? "" : "s"}${pageCount > 1 ? ` · page ${shopPage} of ${pageCount}` : ""}`;
		if (clear) clear.classList.toggle("hidden", !shopQuery && !activeCollection && activeCategory === "All" && !activeClass && shopSort === "featured");
		if (pager) {
			pager.classList.toggle("hidden", detailProductID || pageCount <= 1);
			qs("span", pager).textContent = `Page ${shopPage} of ${pageCount}`;
			qs("[data-shop-previous]", pager).disabled = shopPage <= 1;
			qs("[data-shop-next]", pager).disabled = shopPage >= pageCount;
		}
		if (!shown.length) {
			grid.innerHTML = '<p class="empty">No packages match these filters.</p>';
			return;
		}
		shown.forEach((p) => {
			const soldOut = p.stockLimit > 0 && p.soldCount >= p.stockLimit,
				effective = p.salePrice > 0 ? p.salePrice : p.price,
				gameplayImpact = Boolean(p.serviceLevel || p.gold || ["PvP", "PvE", "Weapons"].includes(p.category)),
				remaining =
					p.stockLimit > 0 ? Math.max(0, p.stockLimit - p.soldCount) : null;
			const card = document.createElement("article");
			card.className =
				"product-card" +
				(detailProductID ? " product-detail-card" : "") +
				(p.tier ? " package-card" : "") +
				(p.featured ? " is-featured" : "") +
				(soldOut ? " is-sold-out" : "");
			card.innerHTML = `<div class="product-art"></div><div class="product-body"><div class="product-tags"><span class="category"></span>${p.tier ? '<span class="tier"></span>' : ""}${gameplayImpact ? '<span class="impact-tag">Gameplay impact</span>' : ""}${p.featured ? '<span class="featured-tag">Featured</span>' : ""}${p.salePrice ? '<span class="sale-tag">Sale</span>' : ""}${p.variants?.length ? `<span>${p.variants.length} options</span>` : ""}</div><h3></h3><p></p><ul class="package-includes"></ul>${remaining !== null ? `<div class="stock-meter"><i style="width:${Math.min(100, (p.soldCount / p.stockLimit) * 100)}%"></i></div><small>${remaining} remaining</small>` : ""}<div class="eligibility-list"></div><div class="product-foot"><strong class="price">${p.salePrice ? `<del>${p.price}</del> ` : ""}${effective} credits${p.variants?.some(v=>v.priceAdjustment)?"+":""}</strong><span class="row-actions"><button class="ghost-button small wishlist-toggle${wishlist.has(p.id)?" is-saved":""}" type="button" aria-pressed="${wishlist.has(p.id)}">${wishlist.has(p.id)?"Saved":"Save"}</button>${!detailProductID?`<button class="ghost-button small compare-toggle${comparison.has(p.id)?" is-saved":""}" type="button" aria-pressed="${comparison.has(p.id)}">Compare</button>`:""}${detailProductID ? '<a class="ghost-button small" href="/shop">Back</a>' : `<a class="ghost-button small" href="/shop/${p.id}">Details</a>`}<button class="buy" ${soldOut ? "disabled" : ""}>${soldOut ? "Sold out" : "Purchase"}</button></span></div></div>`;
			const art = qs(".product-art", card),
				className = p.className || classes[p.classId];
			renderProductArt(art, p);
			qs(".category", card).textContent =
				p.category === "Gold" ? "Gold" : className || p.category;
			if (qs(".tier", card)) qs(".tier", card).textContent = p.tier;
			qs("h3", card).textContent = p.name;
			qs(".product-body>p", card).textContent =
				p.description || `${p.quantity} × item ${p.itemId}`;
			const includes = qs(".package-includes", card);
			const included = p.includes || [], visibleIncludes = detailProductID ? included : included.slice(0, 3);
			visibleIncludes.forEach((x) => {
				const li = document.createElement("li");
				li.textContent = x;
				includes.append(li);
			});
			if (!detailProductID && included.length > visibleIncludes.length) {
				const li = document.createElement("li"); li.textContent = `+ ${included.length - visibleIncludes.length} more in full details`; includes.append(li);
			}
			if (!soldOut) qs(".buy", card).onclick = () => openPurchase(p);
			qs(".wishlist-toggle", card).onclick = (event) => toggleWishlist(p, event.currentTarget);
			const compare = qs(".compare-toggle", card);
			if (compare) compare.onclick = () => { if (comparison.has(p.id)) comparison.delete(p.id); else if (comparison.size < 3) comparison.add(p.id); else return toast("Compare up to three packages"); render(); renderComparison(); };
			grid.append(card);
			if (detailProductID) loadEligibility(p, card);
		});
		renderComparison();
	}
	async function loadEligibility(p, card) {
		const box = qs(".eligibility-list", card);
		try {
			const data = await api(`/api/shop/${p.id}/eligibility`);
			box.innerHTML = `<h4>Character eligibility</h4>${data.characters.map((entry) => `<div class="eligibility-row ${entry.eligible ? "eligible" : "ineligible"}"><b>${esc(entry.character.name)}</b><span>${entry.eligible ? "Ready for delivery" : esc(entry.reasons.join(" · "))}</span></div>`).join("")}`;
		} catch (error) {
			box.innerHTML = error.status === 401 ? '<p class="muted">Sign in to check which characters are eligible.</p>' : `<p class="muted">${esc(error.message)}</p>`;
		}
	}
	function categoryButtons() {
		const cats = [
			"All",
			...new Set(
				[...products]
					.sort((a, b) => Number(a.categoryOrder) - Number(b.categoryOrder))
					.map((p) => p.category),
			),
		];
		filters.innerHTML = "";
		const buttons = document.createElement("div");
		buttons.className = "filter-buttons";
		cats.forEach((c) => {
			const b = document.createElement("button");
			b.className = "filter" + (activeCategory === c ? " active" : "");
			b.textContent = c;
			b.onclick = () => {
				activeCategory = c;
				shopPage = 1;
				updateURLQuery({category:c === "All" ? "" : c,page:1});
				qsa(".filter", buttons).forEach((x) =>
					x.classList.toggle("active", x === b),
				);
				render();
			};
			buttons.append(b);
		});
		const select = document.createElement("select");
		select.className = "class-filter";
		select.setAttribute("aria-label", "Filter products by class");
		select.innerHTML =
			'<option value="0">All classes</option>' +
			Object.entries(classes)
				.map(([id, name]) => `<option value="${id}">${name}</option>`)
				.join("");
		select.onchange = () => {
			activeClass = Number(select.value);
			shopPage = 1;
			updateURLQuery({class:activeClass || "",page:1});
			render();
		};
		select.value = String(activeClass);
		filters.append(buttons, select);
		const collectionBox = qs("#shop-collections");
		if (collectionBox) {
			collectionBox.innerHTML = "";
			collectionBox.classList.toggle("hidden", !collections.length);
			for (const collection of collections) {
				const button = document.createElement("button"),
					collectionProducts = (collection.productIds || [])
						.map((id) => products.find((product) => product.id === id))
						.filter(Boolean),
					preview = collectionProducts.slice(0, 3).map((product) => product.name);
				button.type = "button";
				button.className = "collection-filter" + (activeCollection === collection.slug ? " active" : "");
				button.setAttribute("aria-pressed", String(activeCollection === collection.slug));
				button.setAttribute("aria-label", `${activeCollection === collection.slug ? "Clear" : "Browse"} ${collection.name} collection`);
				button.innerHTML = `<span class="collection-heading"><span><small>Curated collection</small><b>${esc(collection.name)}</b></span><strong>${collectionProducts.length || collection.productIds?.length || 0} picks</strong></span><span class="collection-description">${esc(collection.description || "A selection chosen for this realm.")}</span>${preview.length ? `<span class="collection-preview">${preview.map((name) => `<i>${esc(name)}</i>`).join("")}</span>` : '<span class="collection-preview"><i>Products will appear here when published.</i></span>'}`;
				if (collection.imageUrl) {
					const artwork = document.createElement("img");
					artwork.className = "collection-artwork";
					artwork.src = collection.imageUrl;
					artwork.alt = "";
					artwork.loading = "lazy";
					artwork.referrerPolicy = "no-referrer";
					artwork.onerror = () => artwork.remove();
					button.prepend(artwork);
					button.classList.add("has-artwork");
				}
				button.onclick = () => {
					activeCollection = activeCollection === collection.slug ? "" : collection.slug;
					shopPage = 1;
					updateURLQuery({collection: activeCollection, page: 1});
					categoryButtons();
					render();
				};
				collectionBox.append(button);
			}
		}
	}
	async function openPurchase(p) {
		selected = p;
		try {
			const [me, chars] = await Promise.all([
				api("/api/me"),
				api("/api/characters"),
			]);
			characters = chars.characters.filter(
				(c) => !c.online && (!p.classId || c.class === p.classId),
			);
			if (!characters.length) {
				toast(
					p.classId
						? `You need an offline ${p.className || classes[p.classId]} for this package`
						: "You need an offline character for delivery",
				);
				return;
			}
			qs("#purchase-title").textContent = p.name;
			const variantField=qs("#purchase-variant-field"),variantSelect=qs("#purchase-variant"),basePrice=p.salePrice||p.price;
			variantSelect.innerHTML=(p.variants||[]).filter(v=>v.active).map(v=>`<option value="${v.id}" data-adjustment="${v.priceAdjustment||0}">${esc(v.name)}${v.priceAdjustment?` (${v.priceAdjustment>0?"+":""}${v.priceAdjustment} credits)`:""}</option>`).join("");
			variantField.classList.toggle("hidden",!variantSelect.options.length);
			const updateVariantPrice=()=>{const adjustment=Number(variantSelect.selectedOptions[0]?.dataset.adjustment||0),total=basePrice+adjustment;qs("#purchase-price").innerHTML=p.salePrice?`<del>${p.price}</del> ${total} credits`:`${total} credits`};variantSelect.onchange=updateVariantPrice;updateVariantPrice();
			qs("#purchase-character").innerHTML = "";
			characters.forEach((c) => {
				const o = document.createElement("option");
				o.value = c.guid;
				o.textContent = `${c.name} — level ${c.level} ${classes[c.class] || ""}`;
				qs("#purchase-character").append(o);
			});
			dialog.showModal();
		} catch (e) {
			if (e.status === 401) location.href = "/login?next=/shop";
			else toast(e.message);
		}
	}
	qs(".dialog-close").onclick = () => dialog.close();
	qs("#purchase-confirm").onclick = async () => {
		const btn = qs("#purchase-confirm");
		btn.disabled = true;
		setMessage(dialog, "");
		try {
			const result = await api("/api/shop/purchase", {
				method: "POST",
				body: JSON.stringify({
					productId: selected.id,
					variantId: Number(qs("#purchase-variant").value || 0),
					characterGuid: Number(qs("#purchase-character").value),
					coupon: qs("#purchase-coupon").value,
				}),
			});
			dialog.close();
			toast(result.message);
			await Promise.all([loadShop(), loadOrderHistory()]);
		} catch (e) {
			setMessage(dialog, e.message);
		} finally {
			btn.disabled = false;
		}
	};
	async function loadShop() {
		try {
			const [data, saved, collectionData] = await Promise.all([api(detailProductID ? `/api/shop/${detailProductID}` : "/api/shop"), api("/api/wishlist").catch(()=>({productIds:[]})),api("/api/shop/collections").catch(()=>({collections:[]}))]);
			wishlist = new Set(saved.productIds || []);
			collections=collectionData.collections||data.collections||[];
			products = detailProductID ? [data.product] : data.products;
			const impactful=products.filter(p=>p.serviceLevel||p.gold||["PvP","PvE","Weapons"].includes(p.category)).length,policy=qs("#shop-policy");
			if(policy){policy.classList.toggle("hidden",!impactful);policy.innerHTML=impactful?`<b>Realm monetization notice</b><span>${impactful} visible package${impactful===1?"":"s"} affect character power, progression, or currency. Review each package before purchasing.</span>`:""}
			if (detailProductID) {
				filters.classList.add("hidden");
				qs(".shop-discovery")?.classList.add("hidden");
				qs(".shop-results-heading")?.classList.add("hidden");
				grid.classList.add("product-detail-grid");
				const heading = qs(".page-hero h1"); if (heading) heading.textContent = data.product.name;
			}
			qs("#shop-meta").textContent = data.deliveryEnabled
				? "Automatic in-game delivery is online."
				: detailProductID ? "Review the complete package and character eligibility before purchase." : "Catalog preview · delivery is currently offline.";
			if (!detailProductID) categoryButtons();
			render();
		} catch (e) {
			grid.innerHTML = `<p class="empty">${esc(e.message)}</p>`;
		}
	}
	function renderOrderHistory() {
		const term = qs("#history-search").value.toLowerCase(),
			status = qs("#history-status").value,
			box = qs("#shop-history");
		box.innerHTML = "";
		orderHistory
			.filter(
				(o) =>
					(!status || String(o.status || o.Status).toLowerCase() === status) &&
					(!term || JSON.stringify(o).toLowerCase().includes(term)),
			)
			.forEach((o) => {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>Order #${o.id || o.ID}</b><small>${o.product ? esc(o.product) : "Item #" + (o.itemId || o.ItemID || "—")} · ${o.created ? new Date(o.created).toLocaleString() : ""}</small></span><span><strong>${Number(o.total || 0).toLocaleString()} credits</strong><small class="status-${esc(o.status || o.Status)}">${esc(o.status || o.Status)}</small></span>`;
				box.append(row);
			});
		if (!box.children.length)
			box.innerHTML = '<p class="muted">No orders match these filters.</p>';
	}
	async function loadOrderHistory() {
		try {
			const data = await api("/api/orders");
			orderHistory = data.orders || [];
			qs("#shop-history-section").classList.remove("hidden");
			renderOrderHistory();
		} catch (e) {
			if (e.status !== 401) toast(e.message);
		}
	}
	const shopSearch = qs("#shop-search"), shopSortControl = qs("#shop-sort");
	if (shopSearch && shopSortControl) {
		shopSearch.value = shopQuery;
		shopSortControl.value = shopSort;
		let shopSearchTimer;
		shopSearch.oninput = () => {
			clearTimeout(shopSearchTimer);
			shopSearchTimer = setTimeout(() => {
				shopQuery = shopSearch.value.trim(); shopPage = 1;
				updateURLQuery({q:shopQuery,page:1}); render();
			}, 180);
		};
		shopSortControl.onchange = () => {
			shopSort = shopSortControl.value; shopPage = 1;
			updateURLQuery({sort:shopSort === "featured" ? "" : shopSort,page:1}); render();
		};
		qs("[data-shop-previous]").onclick = () => { if (shopPage > 1) { shopPage--; updateURLQuery({page:shopPage}); render(); grid.scrollIntoView({behavior:"smooth",block:"start"}); } };
		qs("[data-shop-next]").onclick = () => { shopPage++; updateURLQuery({page:shopPage}); render(); grid.scrollIntoView({behavior:"smooth",block:"start"}); };
		qs("#shop-clear-filters").onclick = () => {
			shopQuery=""; shopSort="featured"; shopPage=1; activeCollection=""; activeCategory="All"; activeClass=0;
			shopSearch.value=""; shopSortControl.value="featured";
			updateURLQuery({q:"",sort:"",page:1,collection:"",category:"",class:""});
			categoryButtons(); render();
		};
	}
	loadShop();
	loadOrderHistory();
	qs("#history-search").oninput = renderOrderHistory;
	qs("#history-status").onchange = renderOrderHistory;
	const creditGrid=qs(".credit-packages"),giftFields=document.createElement("div");giftFields.className="credit-gift-fields";giftFields.innerHTML='<label>Gift to another account (optional)<input id="credit-recipient" maxlength="32" placeholder="Account username"></label><label>Gift message<input id="credit-gift-message" maxlength="500" placeholder="Enjoy!"></label>';creditGrid.after(giftFields);
	api("/api/billing/packages").then(({packages})=>{creditGrid.innerHTML="";for(const pack of packages||[]){const button=document.createElement("button");button.dataset.creditPackage=pack.slug;button.innerHTML=`<b>${Number(pack.credits).toLocaleString()}</b><span>${esc(pack.name||"credits")}${pack.bonusLabel?" · "+esc(pack.bonusLabel):""}</span>`;creditGrid.append(button)}if(!creditGrid.children.length)creditGrid.innerHTML='<p class="muted">Credit purchases are not configured.</p>';mountCreditButtons()}).catch(()=>{});
	function mountCreditButtons(){qsa("[data-credit-package]").forEach(
		(button) =>
			(button.onclick = async () => {
				button.disabled = true;
				try {
					const recipientUsername=qs("#credit-recipient").value.trim(),message=qs("#credit-gift-message").value.trim();
					const result = await api("/api/billing/checkout", {
						method: "POST",
						body: JSON.stringify({ package: button.dataset.creditPackage, ...(recipientUsername?{recipientUsername,message}:{}) }),
					});
					location.href = result.url;
				} catch (e) {
					if (e.status === 401) location.href = "/login?next=/shop";
					else toast(e.message);
				} finally {
					button.disabled = false;
				}
			}),
	);}

}
