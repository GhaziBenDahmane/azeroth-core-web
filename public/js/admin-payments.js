import {esc, pageFromURL, qs, renderPagination, updateURLQuery} from "/js/ui.js";

export function mountAdminPayments({api, askAction}) {
	const host = qs('[data-admin-view="catalog"]');
	if (!host || qs("#payment-manager")) return;

	const panel = document.createElement("article");
	panel.id = "payment-manager";
	panel.className = "account-panel cms-panel";
	panel.innerHTML = `<div class="panel-title"><div><p class="eyebrow">PAYMENTS</p><h3>Receipts, refunds and disputes</h3></div><button id="payments-refresh" class="ghost-button" type="button">Refresh</button></div><div class="ranking-filterbar"><label>Search<input id="payments-search" placeholder="Checkout, account, payment"></label><label>Status<select id="payments-status"><option value="">All states</option><option value="paid">Paid</option><option value="partially_refunded">Partially refunded</option><option value="refunded">Refunded</option><option value="disputed">Disputed</option><option value="reversal_review">Review</option></select></label></div><div id="admin-payments" class="admin-table"><p class="muted">Loading…</p></div>`;
	host.append(panel);

	const state = new URLSearchParams(location.search),
		search = qs("#payments-search"),
		status = qs("#payments-status");
	search.value = state.get("paymentsQ") || "";
	status.value = state.get("paymentsStatus") || "";
	let timer;

	const load = async () => {
		const box = qs("#admin-payments"),
			params = new URLSearchParams({page: String(pageFromURL("paymentsPage")), perPage: "25"});
		if (search.value.trim()) params.set("q", search.value.trim());
		if (status.value) params.set("status", status.value);
		try {
			const data = await api("/api/admin/payments?" + params);
			box.innerHTML = "";
			for (const payment of data.payments || []) {
				const row = document.createElement("div"),
					money = payment.amountTotal
						? `${(Number(payment.amountTotal) / 100).toFixed(2)} ${String(payment.currency || "").toUpperCase()}`
						: "Amount unavailable";
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(payment.checkoutId)} · ${money}</b><small>${esc(payment.purchaser)} → ${esc(payment.recipient)} · ${Number(payment.credits).toLocaleString()} credits · ${esc(payment.status)}${payment.disputeId ? " · dispute " + esc(payment.disputeId) : ""}</small></span><span class="row-actions"></span>`;
				if (payment.status === "paid") {
					const refund = document.createElement("button");
					refund.className = "ghost-button danger";
					refund.type = "button";
					refund.textContent = "Full refund";
					refund.onclick = async () => {
						if (await askAction({title: "Refund payment", message: `${money} / ${payment.credits} credits. Stripe will issue a full refund; credits are reversed when its signed webhook arrives.`, label: "Type REFUND", expected: "REFUND", confirmText: "Request refund"}) !== "REFUND") return;
						refund.disabled = true;
						try {
							await api(`/api/admin/payments/${encodeURIComponent(payment.checkoutId)}/refund`, {method: "POST", body: "{}"});
							await load();
						} finally {
							refund.disabled = false;
						}
					};
					qs(".row-actions", row).append(refund);
				}
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No Stripe transactions match these filters.</p>';
			renderPagination(box, data.pagination, "paymentsPage", load);
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	};

	qs("#payments-refresh").onclick = load;
	search.oninput = () => {
		clearTimeout(timer);
		timer = setTimeout(() => {
			updateURLQuery({paymentsQ: search.value.trim(), paymentsPage: 1});
			load();
		}, 250);
	};
	status.onchange = () => {
		updateURLQuery({paymentsStatus: status.value, paymentsPage: 1});
		load();
	};
	load();
}
