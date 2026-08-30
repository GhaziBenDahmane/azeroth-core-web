import {qs, setMessage} from "/js/ui.js";

export function mountLogin(context) {
	const { api, submitJSON } = context;
	submitJSON(qs("#login-form"), "/api/auth/login", () => {
		location.href = "/account";
	});
	const oauthResult = new URLSearchParams(location.search).get("oauth");
	const provider = new URLSearchParams(location.search).get("provider") === "google" ? "Google" : "Discord";
	if (oauthResult === "unlinked") setMessage(qs("#login-form"), `That ${provider} account is not linked yet. Sign in with your game account, then connect ${provider} under Security.`);
	else if (oauthResult === "reauth") setMessage(qs("#login-form"), `Your security confirmation expired. Sign in and try linking ${provider} again.`);
	else if (oauthResult === "failed") setMessage(qs("#login-form"), `${provider} sign-in could not be completed. Please try again.`);
	qs("#passkey-login")?.addEventListener("click", async (event) => {
		const button = event.currentTarget;
		button.disabled = true;
		try {
			const options = await api("/api/auth/passkey/options", { method: "POST", body: "{}" });
			options.challenge = base64URLToBuffer(options.challenge);
			const credential = await navigator.credentials.get({ publicKey: options });
			if (!credential) throw new Error("No passkey was selected");
			await api("/api/auth/passkey", { method: "POST", body: JSON.stringify(publicKeyCredentialJSON(credential)) });
			location.href = "/account";
		} catch (error) {
			if (error.name !== "NotAllowedError") setMessage(qs("#login-form"), error.message || "Passkey sign-in failed");
		} finally { button.disabled = false; }
	});

}
