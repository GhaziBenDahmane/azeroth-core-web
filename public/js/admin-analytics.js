import {esc, qs} from "/js/ui.js";

export function createAdminAnalytics({api}) {
	async function load() {
		const box = qs("#admin-analytics");
		if (!box) return;
		try {
			qs(".admin-chart")?.remove();
			const data = await api("/api/admin/analytics"),
				metrics = [
					["Accounts", data.accounts, `+${data.newAccounts24h} in 24h`],
					["Characters", data.characters, `${data.onlinePlayers} online`],
					["Open tickets", data.openTickets, "Awaiting staff"],
					["Orders today", data.ordersToday, `${data.ordersPending} pending`],
					["Credits · 30 days", data.credits30d, "Completed orders"],
				];
			box.innerHTML = "";
			for (const [label, value, note] of metrics) {
				const card = document.createElement("article");
				card.innerHTML = `<span>${esc(label)}</span><strong>${Number(value).toLocaleString()}</strong><small>${esc(note)}</small>`;
				box.append(card);
			}
			const chart = document.createElement("section"),
				chartData = [
					["New accounts", data.newAccounts24h],
					["Online", data.onlinePlayers],
					["Orders", data.ordersToday],
					["Tickets", data.openTickets],
				],
				max = Math.max(1, ...chartData.map((entry) => Number(entry[1])));
			chart.className = "admin-chart";
			chart.innerHTML = '<h3>Operational pulse</h3><div class="chart-bars">' + chartData.map(([label, value]) => `<div><span>${esc(label)}</span><i style="--bar:${Math.max(3, (Number(value) / max) * 100)}%"></i><b>${Number(value).toLocaleString()}</b></div>`).join("") + "</div>";
			box.after(chart);
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}
	return {load};
}
