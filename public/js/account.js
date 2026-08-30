import {esc, publicLink, qs, qsa, setMessage} from "/js/ui.js";

export function mountAccount(context) {
	const { page, api, toast, publicConfigPromise, classes, submitJSON, askAction, initial, requestStepUp, base64URLToBuffer, publicKeyCredentialJSON } = context;
	if (page === "account")
		Promise.all([
			api("/api/me"),
			api("/api/characters"),
			api("/api/orders"),
			api("/api/wallet"),
			publicConfigPromise,
		])
			.then(([me, chars, orders, wallet, cfg]) => {
				qs("#account-name").textContent = me.account.username;
				qs("#account-balance").textContent = me.balance;
				qs("#character-count").textContent =
					`${chars.characters.length} character${chars.characters.length === 1 ? "" : "s"}`;
				const box = qs("#account-characters");
				box.innerHTML = "";
				if (!chars.characters.length)
					box.innerHTML =
						'<p class="muted">No characters on this realm yet.</p>';
				chars.characters.forEach((c) => {
					const el = document.createElement("div");
					el.className = "account-character";
					el.innerHTML = `<div class="avatar">${initial(c.name)}</div><div><h3></h3><p>Level ${c.level} ${classes[c.class] || "Hero"} · ${c.online ? "Online" : "Offline"}</p><form class="character-privacy hidden"><label class="check"><input name="hidden" type="checkbox"> Hide from public armory</label><label class="check"><input name="showGear" type="checkbox"> Show equipment</label><label class="check"><input name="showActivity" type="checkbox"> Show activity and collections</label><button class="button small" type="submit">Save privacy</button><p class="form-message" role="status"></p></form></div><span class="row-actions"></span>`;
					qs("h3", el).textContent = c.name;
					const actions = qs(".row-actions", el);
					if (cfg.features?.armory !== false) {
						const inspect = document.createElement("a");
						inspect.href = "/armory/" + encodeURIComponent(c.name);
						inspect.textContent = "Inspect";
						actions.append(inspect);
					}
					if (!c.online) {
						for (const action of ["rename", "customize", "unstuck"]) {
							const b = document.createElement("button");
							b.className = "ghost-button";
							b.textContent = action;
							b.onclick = () => runCharacterService(c.guid, action);
							actions.append(b);
						}
					}
					const privacyButton=document.createElement("button");privacyButton.className="ghost-button";privacyButton.type="button";privacyButton.textContent="Privacy";privacyButton.onclick=async()=>{const form=qs(".character-privacy",el);if(!form.classList.contains("hidden")){form.classList.add("hidden");return}try{const data=await api(`/api/characters/${c.guid}/privacy`);form.elements.hidden.checked=Boolean(data.privacy.hidden);form.elements.showGear.checked=Boolean(data.privacy.showGear);form.elements.showActivity.checked=Boolean(data.privacy.showActivity);form.classList.remove("hidden")}catch(error){toast(error.message)}};actions.append(privacyButton);
					const privacyForm=qs(".character-privacy",el);privacyForm.onsubmit=async(event)=>{event.preventDefault();const values={hidden:privacyForm.elements.hidden.checked,showGear:privacyForm.elements.showGear.checked,showActivity:privacyForm.elements.showActivity.checked};try{await api(`/api/characters/${c.guid}/privacy`,{method:"PUT",body:JSON.stringify(values)});setMessage(privacyForm,"Privacy saved.",true)}catch(error){setMessage(privacyForm,error.message)}};
					box.append(el);
				});
				const summary = qs("#account-character-summary");
				summary.innerHTML = "";
				for (const c of chars.characters.slice(0, 4)) {
					const row = document.createElement("div");
					row.innerHTML = `<span><b>${esc(c.name)}</b><small> · ${classes[c.class] || "Hero"}</small></span><strong>Level ${c.level}</strong>`;
					summary.append(row);
				}
				if (!summary.children.length)
					summary.innerHTML = '<p class="muted">No characters on this realm yet.</p>';
				loadDeletedCharacters();
				const list = qs("#orders");
				list.innerHTML = "";
				if (!orders.orders.length)
					list.innerHTML = '<p class="muted">No orders yet.</p>';
				orders.orders.forEach((o) => {
					const el = document.createElement("div");
					el.className = "order";
					el.innerHTML = `<span>Order #${o.id || o.ID} · item ${o.ItemID || o.itemId}</span><b class="status-${esc(o.Status || o.status)}">${esc(o.Status || o.status)}</b>`;
					list.append(el);
				});
				const walletBox = qs("#wallet-history");
				walletBox.innerHTML = "";
				if (!wallet.transactions.length)
					walletBox.innerHTML = '<p class="muted">No credit activity yet.</p>';
			wallet.transactions.forEach((entry) => {
					const row = document.createElement("div");
					row.className = "admin-row";
					const amount = Number(entry.amount);
					row.innerHTML = `<span><b>${esc(entry.reason)}</b><small>${new Date(entry.created).toLocaleString()}</small></span><strong class="${amount < 0 ? "danger" : "success"}">${amount > 0 ? "+" : ""}${amount}</strong>`;
					walletBox.append(row);
				});
				const ordersView=qs('[data-account-view="orders"]');
				if(ordersView&&!qs("#payment-history")){const panel=document.createElement("article");panel.className="account-panel";panel.innerHTML='<h2>Payment receipts</h2><div id="payment-history" class="admin-table"><p class="muted">Loading…</p></div>';ordersView.append(panel);api("/api/billing/transactions").then(data=>{const paymentBox=qs("#payment-history");paymentBox.innerHTML="";for(const payment of data.payments||[]){const row=document.createElement("div");row.className="admin-row";const money=payment.amountTotal?`${(Number(payment.amountTotal)/100).toFixed(2)} ${String(payment.currency||"").toUpperCase()}`:"Amount unavailable",receiptURL=publicLink(payment.receiptUrl);row.innerHTML=`<span><b>${money} · ${Number(payment.credits).toLocaleString()} credits</b><small>${new Date(payment.createdAt).toLocaleString()} · ${esc(payment.status)}</small></span><span class="row-actions">${receiptURL?`<a class="ghost-button" href="${esc(receiptURL)}" target="_blank" rel="noreferrer">Receipt</a>`:""}</span>`;paymentBox.append(row)}if(!paymentBox.children.length)paymentBox.innerHTML='<p class="muted">No card payments yet.</p>'}).catch(error=>{qs("#payment-history").innerHTML=`<p class="empty">${esc(error.message)}</p>`})}
			})
			.catch((e) => {
				if (e.status === 401) location.href = "/login";
				else toast(e.message);
			});
	async function runCharacterService(guid, action) {
		if (!(await askAction({ title: `${action[0].toUpperCase() + action.slice(1)} character`, message: "The change is applied immediately by AzerothCore.", input: false, confirmText: "Confirm service" }))) return;
		try {
			await api(`/api/characters/${guid}/service`, {
				method: "POST",
				body: JSON.stringify({ action }),
			});
			toast(`Character ${action} requested`);
			loadDeletedCharacters();
		} catch (e) {
			toast(e.message);
		}
	}
	async function loadDeletedCharacters() {
		try {
			const response = await api("/api/characters/deleted"),
				characters = response.characters || [],
				box = qs("#deleted-characters");
			box.innerHTML = "";
			if (!characters.length) {
				box.innerHTML = '<p class="muted">No restorable characters.</p>';
				return;
			}
			characters.forEach((c) => {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(c.name)}</b><small>Deleted ${new Date(c.deletedAt * 1000).toLocaleString()}</small></span>`;
				const b = document.createElement("button");
				b.className = "ghost-button";
				b.textContent = "Restore";
				b.onclick = () => runCharacterService(c.guid, "restore");
				row.append(b);
				box.append(row);
			});
		} catch (e) {
			qs("#deleted-characters").innerHTML =
				`<p class="muted">${esc(e.message)}</p>`;
		}
	}
	async function loadMyTickets() {
		try {
			const { tickets } = await api("/api/tickets");
			const box = qs("#my-tickets");
			box.innerHTML = "";
			if (!tickets.length)
				box.innerHTML = '<p class="muted">No support tickets.</p>';
			tickets.forEach((t) => {
				const row = document.createElement("div");
				row.className = "ticket-row";
				const thread = (t.messages || []).map((m) => `<small><b>${m.authorRole === "staff" ? "Staff" : "You"}:</b> ${esc(m.message)}</small>`).join("") || `<small>${esc(t.message)}</small>${t.response ? `<small>Staff: ${esc(t.response)}</small>` : ""}`;
				row.innerHTML = `<div><b>#${t.id} · ${esc(t.subject)}</b>${thread}</div><span class="row-actions"><strong class="status-${esc(t.status)}">${esc(t.status)}</strong></span>`;
				if (!['closed', 'resolved'].includes(t.status)) {
					const reply = document.createElement("button");
					reply.className = "ghost-button";
					reply.textContent = "Reply";
					reply.onclick = async () => {
						const message = await askAction({ title: `Reply to ticket #${t.id}`, message: t.subject, label: "Message", confirmText: "Send reply" });
						if (message === null) return;
						try { await api(`/api/tickets/${t.id}/messages`, { method: "POST", body: JSON.stringify({ message }) }); loadMyTickets(); }
						catch (e) { toast(e.message); }
					};
					qs(".row-actions", row).append(reply);
				}
				box.append(row);
			});
		} catch (e) {
			toast(e.message);
		}
	}
	async function loadPlayerSanctions() {
		const box=qs("#player-sanctions");if(!box)return;
		try { const {sanctions}=await api("/api/moderation/sanctions");box.innerHTML="";for(const item of sanctions||[]){const row=document.createElement("div");row.className="admin-row";const appeal=item.appeal;row.innerHTML=`<span><b>${esc(item.type.replaceAll("_"," "))}</b><small>${esc(item.reason)} · ${new Date(item.startsAt).toLocaleString()}${item.expiresAt?" · expires "+new Date(item.expiresAt).toLocaleString():""}</small>${appeal?`<small>Appeal ${esc(appeal.status)}${appeal.staffResponse?" · "+esc(appeal.staffResponse):""}</small>`:""}</span><span class="row-actions"><strong class="status-${esc(item.status)}">${esc(item.status)}</strong></span>`;if(!appeal){const button=document.createElement("button");button.type="button";button.className="ghost-button";button.textContent="Appeal";button.onclick=async()=>{const message=await askAction({title:"Appeal this sanction",message:item.reason,label:"Explain why staff should review this action",confirmText:"Submit appeal"});if(message===null)return;try{await api(`/api/moderation/sanctions/${item.id}/appeal`,{method:"POST",body:JSON.stringify({message})});toast("Appeal submitted");loadPlayerSanctions()}catch(error){toast(error.message)}};qs(".row-actions",row).append(button)}box.append(row)}if(!box.children.length)box.innerHTML='<p class="muted">No sanctions are recorded for this account.</p>'; } catch(error){box.innerHTML=`<p class="empty">${esc(error.message)}</p>`}
	}
	async function loadTransfers() {
		const box=qs("#transfer-requests"); if(!box)return;
		publicConfigPromise.then((cfg)=>{const hours=Number(cfg.realmProfile?.transferSlaHours||72),days=Math.ceil(hours/24),label=hours%24===0?`${days} day${days===1?"":"s"}`:`${hours} hours`;qs("#transfer-sla").textContent=`Expected staff review: within ${label}. You can follow every status change below.`}).catch(()=>{});
		try{const [data,guildData]=await Promise.all([api("/api/transfers"),api("/api/guild-applications")]);box.innerHTML="";for(const item of data.requests||[]){const row=document.createElement("div");row.className="admin-row";row.innerHTML=`<span><b>${esc(item.characterName)} · ${esc(item.sourceRealm)}</b><small>${new Date(item.createdAt).toLocaleString()}${item.staffNote?" · "+esc(item.staffNote):""}</small></span><strong class="status-${esc(item.status)}">${esc(item.status)}</strong>`;box.append(row)}if(!box.children.length)box.innerHTML='<p class="muted">No transfer requests.</p>';const guildBox=qs("#my-guild-applications");guildBox.innerHTML="";for(const item of guildData.applications||[]){const row=document.createElement("div");row.className="admin-row";row.innerHTML=`<span><b>${esc(item.characterName)} → ${esc(item.guildName||"Guild "+item.guildId)}</b><small>${new Date(item.createdAt).toLocaleString()}${item.response?" · "+esc(item.response):""}</small></span><span class="row-actions"><strong class="status-${esc(item.status)}">${esc(item.status.replaceAll("_"," "))}</strong></span>`;if(["submitted","reviewing"].includes(item.status)){const withdraw=document.createElement("button");withdraw.className="ghost-button";withdraw.type="button";withdraw.textContent="Withdraw";withdraw.onclick=async()=>{if(!(await askAction({title:"Withdraw guild application",message:`${item.characterName} → ${item.guildName}`,input:false,confirmText:"Withdraw"})))return;await api("/api/guild-applications/"+item.id,{method:"DELETE"});loadTransfers()};qs(".row-actions",row).append(withdraw)}guildBox.append(row)}if(!guildBox.children.length)guildBox.innerHTML='<p class="muted">No guild applications.</p>'}catch(error){box.innerHTML=`<p class="empty">${esc(error.message)}</p>`}
	}
	async function loadSessions() {
		try {
			const { sessions } = await api("/api/security/sessions");
			const box = qs("#security-sessions");
			box.innerHTML = "";
			sessions.forEach((s) => {
				const row = document.createElement("div");
				row.className = "session-row";
				row.innerHTML = `<span><b>${s.Current ? "This session" : esc(s.IP || "Unknown location")}</b><small>${esc(s.UserAgent || "Unknown browser")} · last seen ${new Date(s.LastSeen).toLocaleString()}</small></span>`;
				if (!s.Current) {
					const b = document.createElement("button");
					b.className = "ghost-button";
					b.textContent = "Revoke";
					b.onclick = async () => {
						await api("/api/security/sessions/" + encodeURIComponent(s.id), {
							method: "DELETE",
						});
						loadSessions();
					};
					row.append(b);
				}
				box.append(row);
			});
		} catch (e) {
			qs("#security-sessions").innerHTML =
				`<p class="muted">${esc(e.message)}</p>`;
		}
	}
	async function loadIdentityAccounts() {
		const box = qs("#identity-accounts");
		if (!box) return;
		try {
			const [data, cfg] = await Promise.all([api("/api/identity/accounts"), publicConfigPromise]);
			qs("#identity-name").textContent = data.identity?.displayName || "Master account";
			box.innerHTML = "";
			for (const account of data.accounts || []) {
				const row = document.createElement("div");
				row.className = "admin-row identity-account-row";
				const summary = document.createElement("span");
				summary.innerHTML = `<b>${esc(account.label || account.username)}</b><small>${esc(account.username)} · ${esc(account.email || "No email")}</small>`;
				const actions = document.createElement("span");
				actions.className = "row-actions";
				if (account.active) actions.insertAdjacentHTML("beforeend", '<strong class="status-executed">Active</strong>');
				if (account.primary) actions.insertAdjacentHTML("beforeend", '<strong class="status-pending">Primary</strong>');
				const rename = document.createElement("button");
				rename.type = "button"; rename.className = "ghost-button"; rename.textContent = "Rename";
				rename.onclick = async () => {
					const label = await askAction({ title: "Rename game account", message: account.username, label: "Display label", defaultValue: account.label || account.username, confirmText: "Save label" });
					if (label === null) return;
					try { await api(`/api/identity/accounts/${account.id}`, { method: "PATCH", body: JSON.stringify({ label }) }); await loadIdentityAccounts(); }
					catch (error) { toast(error.message); }
				};
				actions.append(rename);
				if (!account.active) {
					const use = document.createElement("button");
					use.type = "button"; use.className = "button small"; use.textContent = "Switch";
					use.onclick = async () => { try { await api(`/api/identity/accounts/${account.id}/switch`, { method: "POST", body: "{}" }); location.href = "/account"; } catch (error) { toast(error.message); } };
					actions.append(use);
				}
				if (!account.primary) {
					const primary = document.createElement("button");
					primary.type = "button"; primary.className = "ghost-button"; primary.textContent = "Make primary";
					primary.onclick = async () => { try { await api(`/api/identity/accounts/${account.id}/primary`, { method: "POST", body: "{}" }); await loadIdentityAccounts(); } catch (error) { toast(error.message); } };
					actions.append(primary);
				}
				if (!account.active && !account.primary) {
					const unlink = document.createElement("button");
					unlink.type = "button"; unlink.className = "danger-button small"; unlink.textContent = "Unlink";
					unlink.onclick = async () => {
						const confirmed = await askAction({ title: "Unlink game account", message: `This removes ${account.username} from this master account. The game account itself and all characters remain intact.`, label: `Type ${account.username} to confirm`, expected: account.username, confirmText: "Unlink account" });
						if (confirmed === null) return;
						try { await api(`/api/identity/accounts/${account.id}`, { method: "DELETE" }); await loadIdentityAccounts(); } catch (error) { toast(error.message); }
					};
					actions.append(unlink);
				}
				row.append(summary, actions); box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No linked game accounts.</p>';
			const providerBox = qs("#identity-provider-list"), discordLink = qs("#discord-link"), googleLink = qs("#google-link");
			if (providerBox) {
				providerBox.innerHTML = "";
				const providers = data.providers || [];
				for (const provider of ["discord", "google"]) {
					const linked = providers.find((item) => item.provider === provider);
					const linkButton = provider === "discord" ? discordLink : googleLink;
					const enabled = cfg.capabilities?.[provider + "OAuth"] === true;
					linkButton?.classList.toggle("hidden", Boolean(linked) || !enabled);
					if (!linked) continue;
					const row = document.createElement("div");
					row.className = "identity-provider-row";
					const label = provider === "discord" ? "Discord" : "Google";
					row.innerHTML = `<span><b>${label}</b><small>${esc(linked.username || linked.email || linked.userId)}</small></span>`;
					const unlink = document.createElement("button");
					unlink.type = "button"; unlink.className = "danger-button small"; unlink.textContent = "Disconnect";
					unlink.onclick = async () => {
						const confirmed = await askAction({ title: `Disconnect ${label}`, message: `You will no longer be able to sign in with this ${label} account. Game-account login remains available.`, input: false, confirmText: "Disconnect" });
						if (!confirmed) return;
						try { await api(`/api/identity/providers/${provider}`, { method: "DELETE" }); await loadIdentityAccounts(); } catch (error) { toast(error.message); }
					};
					row.append(unlink); providerBox.append(row);
				}
				if (discordLink) discordLink.onclick = async () => { if (await requestStepUp()) location.href = "/api/auth/discord/start?mode=link"; };
				if (googleLink) googleLink.onclick = async () => { if (await requestStepUp()) location.href = "/api/auth/google/start?mode=link"; };
			}
		} catch (error) {
			box.innerHTML = `<p class="empty">${esc(error.message)}</p>`;
		}
	}
	async function loadPasskeys() {
		const box = qs("#passkey-list");
		if (!box || qs('[data-passkeys]')?.classList.contains("hidden")) return;
		try {
			const data = await api("/api/security/passkeys");
			box.innerHTML = "";
			for (const passkey of data.credentials || []) {
				const row = document.createElement("div"); row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(passkey.name)}</b><small>Added ${new Date(passkey.created).toLocaleString()}${passkey.lastUsed ? ` · last used ${new Date(passkey.lastUsed).toLocaleString()}` : ""}</small></span>`;
				const remove = document.createElement("button"); remove.type = "button"; remove.className = "danger-button small"; remove.textContent = "Remove";
				remove.onclick = async () => {
					const confirmed = await askAction({ title: "Remove passkey", message: `${passkey.name} will no longer be accepted for sign-in.`, input: false, confirmText: "Remove passkey" });
					if (!confirmed) return;
					try { await api(`/api/security/passkeys/${encodeURIComponent(passkey.id)}`, { method: "DELETE" }); await loadPasskeys(); } catch (error) { toast(error.message); }
				};
				row.append(remove); box.append(row);
			}
			if (!box.children.length) box.innerHTML = '<p class="muted">No passkeys registered yet.</p>';
		} catch (error) { box.innerHTML = `<p class="empty">${esc(error.message)}</p>`; }
	}
	qs("#passkey-register")?.addEventListener("click", async (event) => {
		const button = event.currentTarget; button.disabled = true;
		try {
			const name = await askAction({ title: "Add a passkey", message: "Your browser or security key will ask you to create a credential.", label: "Passkey name", defaultValue: "My device", confirmText: "Continue" });
			if (name === null) return;
			const options = await api("/api/security/passkeys/register/options", { method: "POST", body: "{}" });
			options.challenge = base64URLToBuffer(options.challenge);
			options.user.id = base64URLToBuffer(options.user.id);
			options.excludeCredentials = (options.excludeCredentials || []).map((item) => ({ ...item, id: base64URLToBuffer(item.id) }));
			const credential = await navigator.credentials.create({ publicKey: options });
			if (!credential) throw new Error("Passkey creation was cancelled");
			await api("/api/security/passkeys/register", { method: "POST", body: JSON.stringify(publicKeyCredentialJSON(credential, name)) });
			toast("Passkey added"); await loadPasskeys();
		} catch (error) { if (error.name !== "NotAllowedError") toast(error.message || "Could not add passkey"); }
		finally { button.disabled = false; }
	});
	async function loadPlayerDashboard() {
		try {
			const data = await api("/api/dashboard"),
				daily = data.dailyReward || {},
				claim = qs("#claim-daily");
			qs("#daily-reward-copy").textContent = daily.available
				? `Day ${daily.cycleDay}: ${daily.credits} credits are ready. Current streak: ${daily.streak} days.`
				: `Day ${daily.cycleDay} claimed · ${daily.streak} day streak.`;
			const cycle = qs("#daily-reward-cycle");
			if (cycle) {
				cycle.innerHTML = "";
				(daily.cycle || []).forEach((credits, index) => {
					const day = document.createElement("span");
					day.className = "reward-day" + (index + 1 === daily.cycleDay ? " active" : "");
					day.innerHTML = `<small>D${index + 1}</small><b>${credits}</b>`;
					cycle.append(day);
				});
			}
			claim.disabled = !daily.available;
			claim.onclick = async () => {
				claim.disabled = true;
				try {
					const result = await api("/api/rewards/daily", {
						method: "POST",
						body: "{}",
					});
					toast(`${result.credits} daily credits claimed`);
					qs("#account-balance").textContent = result.balance;
					loadPlayerDashboard();
				} catch (e) {
					toast(e.message);
				}
			};
			const loyalty = data.loyalty || {};
			qs("#loyalty-level").textContent = `${loyalty.name || "Initiate"} · ${loyalty.points || 0} points`;
			qs("#loyalty-copy").textContent = loyalty.nextName
				? `${loyalty.remaining} points until ${loyalty.nextName}. ${loyalty.description || ""}`
				: `${loyalty.description || ""} Maximum loyalty level reached.`;
			const loyaltyRange = Math.max(1, (loyalty.nextFloor || loyalty.floor || 1) - (loyalty.floor || 0));
			const loyaltyProgress = loyalty.nextName ? Math.max(0, (loyalty.points || 0) - (loyalty.floor || 0)) : loyaltyRange;
			qs("#loyalty-meter").style.width = `${Math.min(100, Math.round(loyaltyProgress * 100 / loyaltyRange))}%`;
			const missions = qs("#player-missions");
			if (missions) {
				missions.innerHTML = "";
				(data.missions || []).forEach((item) => {
					const progress = Math.min(item.progress || 0, item.target || 1), percent = Math.min(100, Math.round(progress * 100 / Math.max(1, item.target || 1)));
					const card = document.createElement("article");
					card.className = `mission-card mission-${esc(item.category)}`;
					card.innerHTML = `<div class="mission-heading"><span class="mission-kind">${esc(item.category)}</span><strong>${item.rewardCredits} credits</strong></div><h4>${esc(item.name)}</h4><p>${esc(item.description)}</p>${item.dataAvailable ? `<div class="meter" role="progressbar" aria-label="${esc(item.name)} progress" aria-valuemin="0" aria-valuemax="${item.target}" aria-valuenow="${progress}"><span style="width:${percent}%"></span></div><small>${progress} / ${item.target}</small>` : `<p class="empty compact">${esc(item.progressMessage || "Progress unavailable")}</p>`}<button class="${item.available ? "button" : "ghost-button"} small" type="button" ${item.available ? "" : "disabled"}>${item.claimed ? "Claimed" : item.available ? "Claim reward" : "In progress"}</button>`;
					if (item.available) qs("button", card).onclick = async () => { try { const result = await api(`/api/rewards/missions/${item.id}/claim`, { method: "POST", body: "{}" }); toast(`${result.credits} mission credits claimed`); qs("#account-balance").textContent = result.balance; loadPlayerDashboard(); } catch (error) { toast(error.message); } };
					missions.append(card);
				});
				if (!missions.children.length) missions.innerHTML = '<p class="muted">No missions are active for this realm.</p>';
				qs("#mission-period").textContent = (data.missions || [])[0]?.period || "Current month";
			}
			const referral = data.referral || {},
				code = qs("#referral-code");
			qs("code", code).textContent = referral.code || "Unavailable";
			code.dataset.copy = referral.code || "";
			qs("#referral-stats").textContent =
				`${referral.uses || 0} successful referrals · ${referral.creditsEarned || 0} credits earned`;
			const share = qs("#share-referral");
			if (share) share.onclick = async () => {
				const invite = `${location.origin}/register?ref=${encodeURIComponent(referral.code || "")}`;
				try { if (navigator.share) await navigator.share({ title: "Join my realm", text: "Join me in Azeroth", url: invite }); else { await navigator.clipboard.writeText(invite); toast("Invite link copied"); } }
				catch (error) { if (error.name !== "AbortError") toast("Could not share invite"); }
			};
			const milestones = qs("#referral-milestones");
			if (milestones) {
				milestones.innerHTML = "";
				(referral.milestones || []).forEach((item) => {
					const row = document.createElement("div"); row.className = "reward-milestone";
					row.innerHTML = `<span><h4>${esc(item.name)}</h4><small>${item.referralCount} recruits · ${item.rewardCredits} credits${item.remaining ? ` · ${item.remaining} remaining` : ""}</small></span><button class="${item.available ? "button" : "ghost-button"} small" type="button" ${item.available ? "" : "disabled"}>${item.claimed ? "Claimed" : item.available ? "Claim" : "Locked"}</button>`;
					if (item.available) qs("button", row).onclick = async () => { try { const result = await api(`/api/rewards/referrals/${item.id}/claim`, { method: "POST", body: "{}" }); toast(`${result.credits} referral credits claimed`); qs("#account-balance").textContent = result.balance; loadPlayerDashboard(); } catch (error) { toast(error.message); } };
					milestones.append(row);
				});
			}
			const recruits = qs("#referral-activity");
			if (recruits) {
				recruits.innerHTML = "";
				(referral.activity || []).forEach((item) => { const row = document.createElement("div"); row.className = "admin-row"; row.innerHTML = `<span><b>${esc(item.username)}</b><small>Joined ${new Date(item.joinedAt).toLocaleDateString()}</small></span><strong class="status-executed">Recruited</strong>`; recruits.append(row); });
				if (!recruits.children.length) recruits.innerHTML = '<p class="muted">Your successful referrals will appear here.</p>';
			}
			if (data.vote?.url) {
				qs("#vote-reward").classList.remove("hidden");
				qs("#vote-link").href = data.vote.url;
				qs("#vote-reward-copy").textContent =
					`Earn ${data.vote.credits || 0} credits through the configured voting provider.`;
			}
			const history = qs("#service-history");
			history.innerHTML = "";
			(data.services || []).forEach((item) => {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(String(item.Action || item.action).replaceAll("_", " "))}</b><small>${esc(item.Character || item.character)} · ${new Date(item.Created || item.created).toLocaleString()}</small></span><strong class="${item.Success || item.success ? "status-executed" : "status-failed"}">${item.Success || item.success ? "Completed" : "Failed"}</strong>`;
				history.append(row);
			});
			if (!history.children.length)
				history.innerHTML = '<p class="muted">No character services yet.</p>';
			const activity = qs("#account-activity");
			activity.innerHTML = "";
			(data.activity || []).forEach((item) => {
				const row = document.createElement("div");
				row.className = "admin-row";
				row.innerHTML = `<span><b>${esc(item.kind || "session")}</b><small>${esc(item.ip || "Unknown address")} · ${esc(item.agent || "Unknown client")}</small></span><time>${new Date(item.at).toLocaleString()}</time>`;
				activity.append(row);
			});
			if (!activity.children.length)
				activity.innerHTML = '<p class="muted">No recent account activity.</p>';
		} catch (e) {
			toast(e.message);
		}
	}
	if (page === "account") {
		if (!qs("#gift-code-form")) { const panel=document.createElement("article");panel.className="account-panel";panel.innerHTML='<h3>Redeem gift code</h3><form id="gift-code-form"><label>Code<input name="code" autocomplete="off" placeholder="GIFT-…" required></label><button class="button small" type="submit">Redeem</button><p class="form-message"></p></form>';qs(".dashboard-benefits").append(panel); }
		const accountRoutes = new Set([
			"overview",
			"characters",
			"transfers",
			"orders",
			"rewards",
			"notifications",
			"support",
			"security",
		]);
		function applyAccountRoute() {
			const part = location.pathname.split("/").filter(Boolean)[1] || "overview",
				view = accountRoutes.has(part) ? part : "overview";
			qsa("[data-account-route]").forEach((link) => {
				const active = link.dataset.accountRoute === view;
				link.classList.toggle("active", active);
				if (active) link.setAttribute("aria-current", "page");
				else link.removeAttribute("aria-current");
			});
			qsa("[data-account-view]").forEach((panel) =>
				panel.classList.toggle("active", panel.dataset.accountView === view),
			);
		}
		document.addEventListener("click", (event) => {
			const link = event.target.closest("a[data-account-route]");
			if (!link || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
			event.preventDefault();
			history.pushState(null, "", link.getAttribute("href"));
			applyAccountRoute();
		});
		window.addEventListener("popstate", applyAccountRoute);
		applyAccountRoute();
		async function loadNotifications() {
			try {
				const data = await api("/api/notifications"), box = qs("#notification-list");
				box.innerHTML = "";
				for (const item of data.notifications || []) {
					const row = document.createElement("article");
					row.className = "notification-item" + (item.readAt ? "" : " unread");
					const actionURL = publicLink(item.actionUrl);
					row.innerHTML = `<div><h3>${esc(item.title)}</h3><p>${esc(item.message)}</p>${actionURL ? `<a class="text-action" href="${esc(actionURL)}">View details →</a>` : ""}</div><time>${new Date(item.created).toLocaleString()}</time>`;
					if (!item.readAt) row.addEventListener("click", () => api(`/api/notifications/${item.id}/read`, { method: "POST", body: "{}" }).then(loadNotifications));
					box.append(row);
				}
				if (!box.children.length) box.innerHTML = '<p class="muted">You have no notifications.</p>';
				for (const badge of qsa("#notification-badge,#account-notification-badge")) {
					badge.textContent = data.unread;
					badge.classList.toggle("hidden", !data.unread);
				}
			} catch (e) { toast(e.message); }
		}
		qs("#notifications-read-all").onclick = () => api("/api/notifications/all/read", { method: "POST", body: "{}" }).then(loadNotifications).catch((e) => toast(e.message));
		loadNotifications();
		loadSessions();
		loadIdentityAccounts();
		publicConfigPromise.then((config) => { if (config.capabilities?.passkeys) loadPasskeys(); });
		loadPlayerDashboard();
		submitJSON(qs("#identity-link-form"), "/api/identity/accounts", () => {
			setMessage(qs("#identity-link-form"), "Game account linked.", true);
			qs("#identity-link-form").reset();
			loadIdentityAccounts();
		});
		submitJSON(qs("#gift-code-form"), "/api/gift-codes/redeem", (result) => { setMessage(qs("#gift-code-form"), `${result.credits} credits redeemed.`, true); qs("#gift-code-form").reset(); qs("#account-balance").textContent=result.balance; });
		loadTransfers();
		submitJSON(qs("#transfer-form"), "/api/transfers", () => { setMessage(qs("#transfer-form"), "Transfer submitted for staff review.", true); qs("#transfer-form").reset(); loadTransfers(); });
		// Bind the static support form immediately. Waiting for public configuration
		// left a short window where a quick submit performed a native page reload.
		// Capability discovery still controls visibility and data loading.
		submitJSON(qs("#ticket-form"), "/api/tickets", () => {
			setMessage(qs("#ticket-form"), "Ticket submitted.", true);
			qs("#ticket-form").reset();
			loadMyTickets();
		});
		publicConfigPromise.then((c) => {
			if (c.features?.support !== false) {
				loadMyTickets();
				loadPlayerSanctions();
			}
		});
		submitJSON(qs("#password-form"), "/api/security/password", () => {
			toast("Password changed. Please sign in again.");
			setTimeout(() => (location.href = "/login"), 900);
		});
		submitJSON(qs("#email-form"), "/api/security/email", (result) => {
			setMessage(qs("#email-form"), result.message || "Verification email sent.", true);
			qs("#email-form").reset();
		});
		async function loadTOTPStatus() {
			try {
				const status = await api("/api/security/totp/status");
				qs("#totp-status").textContent = status.enabled
					? `Enabled · ${status.recoveryCodesRemaining} recovery codes remaining`
					: status.enrollmentAvailable ? "Not enabled" : "Enrollment is unavailable until the operator configures encryption.";
				qs("#totp-setup").classList.toggle("hidden", status.enabled || !status.enrollmentAvailable);
				qs("#totp-disable-form").classList.toggle("hidden", !status.enabled);
			} catch (e) {
				qs("#totp-status").textContent = e.message;
			}
		}
		qs("#totp-setup").onclick = async () => {
			try {
				const data = await api("/api/security/totp/setup", {
					method: "POST",
					body: "{}",
				});
				qs("#totp-secret").textContent = data.secret;
				qs("#totp-enroll").classList.remove("hidden");
			} catch (e) {
				toast(e.message);
			}
		};
		submitJSON(qs("#totp-form"), "/api/security/totp/enable", (result) => {
			setMessage(qs("#totp-form"), "Authenticator enabled.", true);
			qs("#totp-enroll").classList.add("hidden");
			if (Array.isArray(result.recoveryCodes)) {
				qs("#totp-recovery-codes").textContent = result.recoveryCodes.join("\n");
				qs("#totp-recovery").classList.remove("hidden");
			}
			loadTOTPStatus();
		});
		submitJSON(qs("#totp-disable-form"), "/api/security/totp/disable", () => {
			setMessage(qs("#totp-disable-form"), "Authenticator disabled.", true);
			qs("#totp-recovery").classList.add("hidden");
			loadTOTPStatus();
		});
		loadTOTPStatus();
		async function loadPrivacyRequests() {
			const box = qs("#privacy-requests");
			try {
				const data = await api("/api/privacy/requests");
				box.innerHTML = "";
				for (const request of data.requests || []) {
					const row = document.createElement("div"); row.className = "admin-row";
					row.innerHTML = `<span><b>${esc(request.Type || request.type)} request #${request.id || request.ID}</b><small>${new Date(request.Created || request.created).toLocaleString()}${request.StaffNote || request.staffNote ? " · " + esc(request.StaffNote || request.staffNote) : ""}</small></span><span class="row-actions"><strong class="status-${esc(request.Status || request.status)}">${esc(request.Status || request.status)}</strong></span>`;
					if ((request.Status || request.status) === "pending") { const cancel = document.createElement("button"); cancel.className="ghost-button"; cancel.textContent="Cancel"; cancel.onclick=async()=>{if(!(await askAction({title:"Cancel deletion request",input:false,confirmText:"Keep my account"})))return;await api(`/api/privacy/requests/${request.id || request.ID}`,{method:"DELETE"});loadPrivacyRequests()}; qs(".row-actions",row).append(cancel); }
					box.append(row);
				}
				if (!box.children.length) box.innerHTML='<p class="muted">No privacy requests.</p>';
			} catch (error) { box.innerHTML=`<p class="muted">${esc(error.message)}</p>`; }
		}
		submitJSON(qs("#privacy-deletion-form"), "/api/privacy/deletion", () => {
			setMessage(qs("#privacy-deletion-form"), "Deletion request submitted for staff review.", true);
			qs("#privacy-deletion-form").reset(); loadPrivacyRequests();
		});
		loadPrivacyRequests();
		qs("#logout").onclick = () =>
			api("/api/auth/logout", { method: "POST", body: "{}" }).finally(
				() => (location.href = "/"),
			);
	}

}
