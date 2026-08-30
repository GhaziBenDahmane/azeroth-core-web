import {esc, pageFromURL, qs, renderPagination, updateURLQuery} from "/js/ui.js";

export function createAdminAccounts({api, classes, askAction, toast, adminCan, passwordResetEnabled}) {
	let mounted = false;
	function mount() {
		if (mounted || !qs("#gm-account-search")) return;
		mounted = true;
		const form = qs("#gm-account-search");
		form.elements.q.value = new URLSearchParams(location.search).get("accountQ") || "";
		form.onsubmit = (event) => {
			event.preventDefault();
			const query = String(new FormData(form).get("q") || "").trim();
			updateURLQuery({accountQ:query, accountsPage:1});
			load(query);
		};
	}
	async function load(query = new URLSearchParams(location.search).get("accountQ") || "") {
		mount();
		const box = qs("#admin-accounts");
		if (!box) return;
		try {
			const params = new URLSearchParams({q:query, page:String(pageFromURL("accountsPage")), perPage:"25"}),
				data = await api("/api/admin/accounts?" + params);
			box.innerHTML = "";
			for (const account of data.accounts || []) {
				const row = document.createElement("div");
				row.className = "admin-account";
				const characters = (account.characters || []).map((character) => `${esc(character.name)} · ${character.level} ${classes[character.class] || "Hero"}${character.online ? " · online" : ""}`).join("<br>") || "No characters";
				row.innerHTML = `<div><b>${esc(account.username)}</b><small>${esc(account.email)} · ${account.banned ? '<i class="danger">BANNED</i>' : "Active"}</small><small>${characters}</small>${account.banReason ? `<small>Reason: ${esc(account.banReason)}</small>` : ""}</div><span class="row-actions"></span>`;
				const actions = qs(".row-actions", row), action = account.banned ? "unban" : "ban",
					moderate = document.createElement("a");
				moderate.className = "ghost-button";
				moderate.textContent = action;
				moderate.href = `/admin/players/moderation?action=${action}&target=${encodeURIComponent(account.username)}`;
				actions.append(moderate);
				for (const character of account.characters || []) if (character.online) {
					const kick = document.createElement("a");
					kick.className = "ghost-button";
					kick.textContent = "kick " + character.name;
					kick.href = `/admin/players/moderation?action=kick&target=${encodeURIComponent(character.name)}`;
					actions.append(kick);
				}
				if (adminCan("moderation")) {
					const revoke = document.createElement("button");
					revoke.type = "button";
					revoke.className = "ghost-button danger";
					revoke.textContent = "Revoke sessions";
					revoke.onclick = async () => {
						const confirmation = await askAction({title:"Revoke all portal sessions", message:`Every browser session for ${account.username} will be signed out. This does not disconnect an active game client.`, label:`Type ${account.username} to confirm`, expected:account.username, confirmText:"Revoke sessions"});
						if (confirmation !== account.username) return;
						revoke.disabled = true;
						try {
							const result = await api(`/api/admin/accounts/${account.id}/revoke-sessions`, {method:"POST", body:"{}"});
							toast(`${result.revoked} session${result.revoked === 1 ? "" : "s"} revoked · request ${result.requestId || "recorded"}`);
						} catch (error) { toast(error.message); }
						finally { revoke.disabled = false; }
					};
					actions.append(revoke);
				}
				if (adminCan("admin") && passwordResetEnabled) {
					const forceReset = document.createElement("button");
					forceReset.type = "button";
					forceReset.className = "ghost-button danger";
					forceReset.textContent = "Require password reset";
					forceReset.onclick = async () => {
						const reason = await askAction({title:"Require a password reset", message:`${account.username} will be signed out of the portal and receive a one-hour reset link. Their password also protects the game account.`, label:"Security reason", confirmText:"Send reset link"});
						if (reason === null) return;
						forceReset.disabled = true;
						try {
							const result = await api(`/api/admin/accounts/${account.id}/require-password-reset`, {method:"POST", body:JSON.stringify({reason})});
							toast(`Password reset required · ${result.revoked} session${result.revoked === 1 ? "" : "s"} revoked · request ${result.requestId || "recorded"}`);
						} catch (error) { toast(error.message); }
						finally { forceReset.disabled = false; }
					};
					actions.append(forceReset);
				}
				box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No matching accounts.</p>';
			renderPagination(box, data.pagination, "accountsPage", () => load(query));
		} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}
	return {load};
}
