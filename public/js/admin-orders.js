import {esc, pageFromURL, qs, qsa, renderPagination, setMessage} from "/js/ui.js";

export function createAdminOrders({api, askAction, toast}) {
	const dialog = qs("#order-reconcile-dialog"),
		form = qs("#order-step-form");

	async function openSteps(orderID) {
		qs("#order-reconcile-title").textContent = `Order #${orderID} delivery`;
		const list = qs("#order-step-list");
		list.innerHTML = '<div class="skeleton"></div>';
		form.classList.add("hidden");
		dialog.showModal();
		try {
			const data = await api(`/api/admin/orders/${orderID}/steps`);
			list.innerHTML = "";
			for (const step of data.steps || []) {
				const row = document.createElement("article");
				row.className = "order-step";
				row.innerHTML = `<span><b>${esc(step.kind.replaceAll("_", " "))}</b><small>${esc(step.key)} · ${step.attempts} attempt${step.attempts === 1 ? "" : "s"}</small>${step.response ? `<small>${esc(step.response)}</small>` : ""}</span><div class="row-actions"><strong class="status-${esc(step.status)}">${esc(step.status)}</strong></div>`;
				if (["failed", "executing"].includes(step.status)) {
					const button = document.createElement("button");
					button.type = "button";
					button.className = "ghost-button";
					button.textContent = "Reconcile";
					button.onclick = () => {
						form.elements.orderId.value = orderID;
						form.elements.stepKey.value = step.key;
						form.elements.resolution.value = step.status === "failed" ? "retry" : "completed";
						form.elements.reason.value = "";
						form.elements.confirmation.value = "";
						form.classList.remove("hidden");
						form.elements.reason.focus();
					};
					qs(".row-actions", row).append(button);
				}
				list.append(row);
			}
			if (!list.children.length) list.innerHTML = '<p class="muted">No delivery step has started. The order can be safely retried or refunded.</p>';
		} catch (error) {
			list.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}

	async function load() {
		try {
			const [orders, ledger] = await Promise.all([
				api(`/api/admin/orders?page=${pageFromURL("ordersPage")}&perPage=10`),
				api(`/api/admin/ledger?page=${pageFromURL("ledgerPage")}&perPage=10`),
			]);
			const box = qs("#admin-orders");
			box.innerHTML = "";
			for (const order of orders.orders || []) {
				const row = document.createElement("div"),
					id = order.id || order.ID,
					status = order.status || order.Status,
					retryable = ["review", "failed"].includes(status);
				row.className = "admin-row";
				row.innerHTML = `<span class="order-select"><input type="checkbox" value="${id}" ${retryable ? "" : "disabled"} aria-label="Select order ${id}"><span><b>#${id} · ${esc(order.product || "Shop order")}</b><small>${esc(order.username || "DEMO")} · ${esc(status)}</small>${order.steps ? `<small class="order-progress">${order.completedSteps || 0}/${order.steps} delivery steps completed</small>` : ""}</span></span><span class="row-actions"></span>`;
				if (order.steps || retryable) {
					const inspect = document.createElement("button");
					inspect.className = "ghost-button";
					inspect.textContent = "Delivery details";
					inspect.onclick = () => openSteps(id);
					qs(".row-actions", row).append(inspect);
				}
				if (retryable) for (const action of ["retry", "refund"]) {
					const button = document.createElement("button");
					button.className = "ghost-button";
					button.textContent = action;
					button.onclick = async () => {
						if (action === "refund" && await askAction({title: `Refund order #${id}`, message: "This returns the credits. Orders with completed fulfillment steps cannot be refunded automatically.", label: "Type REFUND", expected: "REFUND", confirmText: "Refund order"}) !== "REFUND") return;
						button.disabled = true;
						try {
							await api(`/api/admin/orders/${id}/${action}`, {method: "POST", body: "{}"});
							toast(`Order ${action} accepted`);
							await load();
						} finally { button.disabled = false; }
					};
					qs(".row-actions", row).append(button);
				}
				box.append(row);
			}
			renderPagination(box, orders.pagination, "ordersPage", load);
			const ledgerBox = qs("#credit-ledger");
			ledgerBox.innerHTML = "";
			for (const entry of ledger.entries || []) {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(entry.Target || entry.target)}</b><small>${esc(entry.Reason || entry.reason)}</small></span><strong>+${entry.Amount || entry.amount}</strong>`;
				ledgerBox.append(row);
			}
			renderPagination(ledgerBox, ledger.pagination, "ledgerPage", load);
		} catch (error) {
			toast(error.message);
		}
	}

	if (dialog && form) {
		qs(".dialog-close", dialog).onclick = () => dialog.close();
		qs("[data-cancel-reconcile]", dialog).onclick = () => form.classList.add("hidden");
		form.onsubmit = async (event) => {
			event.preventDefault();
			if (form.elements.confirmation.value !== "RECONCILE") return setMessage(form, "Type RECONCILE to confirm this decision.");
			const button = qs('button[type="submit"]', form),
				orderID = form.elements.orderId.value,
				stepKey = form.elements.stepKey.value;
			button.disabled = true;
			try {
				await api(`/api/admin/orders/${orderID}/steps/${encodeURIComponent(stepKey)}`, {method: "POST", body: JSON.stringify({resolution: form.elements.resolution.value, reason: form.elements.reason.value})});
				toast("Fulfillment step reconciled and order resumed");
				dialog.close();
				await load();
			} catch (error) { setMessage(form, error.message); }
			finally { button.disabled = false; }
		};
	}

	qs("#orders-select-all").onclick = () => qsa("#admin-orders input[type=checkbox]:not(:disabled)").forEach((input) => { input.checked = true; });
	qs("#orders-bulk-retry").onclick = async () => {
		const ids = qsa("#admin-orders input:checked").map((input) => input.value);
		if (!ids.length) return toast("Select at least one retryable order");
		if (await askAction({title: `Retry ${ids.length} orders`, message: "Only incomplete fulfillment steps will be retried.", label: "Type RETRY", expected: "RETRY", confirmText: "Retry selected"}) !== "RETRY") return;
		const report = await api("/api/admin/orders/bulk-retry", {method: "POST", body: JSON.stringify({ids: ids.map(Number)})}),
			output = qs("#orders-bulk-results");
		output.classList.remove("hidden");
		output.innerHTML = `<p><b>${report.succeeded} of ${report.requested} orders queued.</b> Request ${esc(report.requestId || "—")}</p><div class="activity-list">${(report.results || []).map((item) => `<div><span>Order #${item.id}</span><strong class="status-${item.status === "queued" ? "executed" : "failed"}">${esc(item.status)}</strong><small>${esc(item.message)}</small></div>`).join("")}</div>`;
		load();
	};

	return {load};
}
