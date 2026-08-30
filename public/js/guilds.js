import {esc, publicLink, qs, setMessage} from "/js/ui.js";

export function mountGuilds(context) {
	const { api, classes, initial } = context;
	const list = qs("#guild-list"),
		detail = qs("#guild-detail");
	async function showGuild(id, updateHistory = true) {
		list.classList.add("hidden");
		detail.innerHTML = '<div class="skeleton"></div>';
		if (updateHistory && location.pathname !== "/guilds/" + id)
			history.pushState({ guild: id }, "", "/guilds/" + id);
		try {
			const [{ guild, members }, recruitmentData] = await Promise.all([
				api("/api/guilds/" + id),
				api("/api/guilds/" + id + "/recruitment"),
			]);
			detail.innerHTML = `<button class="ghost-button" id="back-guilds">← All guilds</button><article class="guild-profile"><p class="eyebrow">GUILD ROSTER</p><h2>&lt;${esc(guild.name)}&gt;</h2><p>${esc(guild.motd || "No message of the day.")}</p><div id="guild-recruitment"></div><div class="roster"></div></article>`;
			const recruitmentBox = qs("#guild-recruitment", detail), recruitment = recruitmentData.recruitment, application = recruitmentData.application;
			if (recruitment) {
				recruitmentBox.className = "guild-recruitment";
				const discordURL = publicLink(recruitment.discordUrl);
				recruitmentBox.innerHTML = `<div><p class="eyebrow">RECRUITING</p><h3>${esc(recruitment.headline)}</h3><p>${esc(recruitment.description)}</p><dl>${recruitment.lookingFor ? `<div><dt>Looking for</dt><dd>${esc(recruitment.lookingFor)}</dd></div>` : ""}${recruitment.schedule ? `<div><dt>Schedule</dt><dd>${esc(recruitment.schedule)}</dd></div>` : ""}${recruitment.contact ? `<div><dt>Contact</dt><dd>${esc(recruitment.contact)}</dd></div>` : ""}</dl>${discordURL ? `<a class="button small" href="${esc(discordURL)}" target="_blank" rel="noreferrer">Join guild Discord</a>` : ""}</div><div id="guild-application"></div>`;
				const applicationBox = qs("#guild-application", recruitmentBox);
				if (application) {
					applicationBox.innerHTML = `<div class="application-status"><span>Your application</span><strong>${esc(application.status.replaceAll("_", " "))}</strong><small>Updated ${new Date(application.updatedAt).toLocaleDateString()}</small>${application.response ? `<p>${esc(application.response)}</p>` : ""}</div>`;
				} else {
					try {
						const { characters } = await api("/api/characters"), eligible = characters.filter((character) => !character.guild);
						if (!eligible.length) applicationBox.innerHTML = '<p class="notice-inline">You need a guildless character before applying.</p>';
						else {
							applicationBox.innerHTML = `<form id="guild-application-form"><h3>Apply to this guild</h3><label>Character<select name="characterGuid">${eligible.map((character) => `<option value="${character.guid}">${esc(character.name)} · level ${character.level} ${classes[character.class] || "Hero"}</option>`).join("")}</select></label><label>Introduction<textarea name="message" minlength="20" maxlength="2000" rows="4" placeholder="Share your role, experience, and availability." required></textarea></label><button class="button small" type="submit">Send application</button><p class="form-message" role="status"></p></form>`;
							qs("#guild-application-form", applicationBox).onsubmit = async (event) => { event.preventDefault(); const form=event.currentTarget,values=Object.fromEntries(new FormData(form));values.characterGuid=Number(values.characterGuid);try{await api(`/api/guilds/${id}/applications`,{method:"POST",body:JSON.stringify(values)});setMessage(form,"Application sent.",true);showGuild(id,false)}catch(error){setMessage(form,error.message)} };
						}
					} catch (error) {
						applicationBox.innerHTML = `<p class="notice-inline"><a href="/login?next=${encodeURIComponent(location.pathname)}">Sign in</a> to apply with one of your characters.</p>`;
					}
				}
			}
			const roster = qs(".roster", detail);
			members.forEach((m) => {
				const row = document.createElement("a");
				row.className = "roster-row";
				row.href = "/armory/" + encodeURIComponent(m.name);
				row.innerHTML = `<span class="avatar">${initial(m.name)}</span><span><b>${esc(m.name)}</b><small>${esc(m.rank || "Member")} · Level ${m.level} ${classes[m.class] || "Hero"}</small></span><i class="${m.online ? "is-online" : ""}">${m.online ? "Online" : "Offline"}</i>`;
				roster.append(row);
			});
			qs("#back-guilds").onclick = () => {
				history.pushState(null, "", "/guilds");
				detail.innerHTML = "";
				list.classList.remove("hidden");
			};
		} catch (e) {
			detail.innerHTML = `<p class="empty">${esc(e.message)}</p>`;
		}
	}
	api("/api/guilds")
		.then(({ guilds }) => {
			list.innerHTML = "";
			guilds.forEach((g) => {
				const card = document.createElement("button");
				card.className = "guild-card";
				card.innerHTML = `<span class="guild-crest">${initial(g.name)}</span><span><b>${esc(g.name)}</b><small>Led by ${esc(g.leader)} · ${g.members} members</small></span><strong>${g.online}<small> online</small></strong>`;
				card.onclick = () => showGuild(g.id);
				list.append(card);
			});
			const pathGuild = location.pathname.match(/^\/guilds\/(\d+)\/?$/),
				id = pathGuild ? pathGuild[1] : new URLSearchParams(location.search).get("id");
			if (id) showGuild(id, false);
		})
		.catch((e) => (list.innerHTML = `<p class="empty">${esc(e.message)}</p>`));
	window.addEventListener("popstate", () => {
		const match = location.pathname.match(/^\/guilds\/(\d+)\/?$/);
		if (match) showGuild(match[1], false);
		else {
			detail.innerHTML = "";
			list.classList.remove("hidden");
		}
	});

}
