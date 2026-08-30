import {qs, setMessage} from "/js/ui.js";

export function mountRegister(context) {
	const { api, publicConfigPromise, submitJSON } = context;
	const form = qs("#register-form"),
		turnstile = qs("#turnstile", form),
		referral = document.createElement("label");
	referral.innerHTML =
		'Referral code <small>(optional)</small><input name="referralCode" maxlength="40" placeholder="FRIEND-ABC123">';
	form.insertBefore(referral, turnstile);
	qs('[name="referralCode"]', referral).value = new URLSearchParams(location.search).get("ref") || "";
	submitJSON(qs("#register-form"), "/api/auth/register", (result) => {
		setMessage(qs("#register-form"), result.message, true);
		if (!result.verificationRequired)
			setTimeout(() => (location.href = "/login"), 900);
	});
	publicConfigPromise
		.then((c) => {
			if (!c.turnstileSiteKey) return;
			const script = document.createElement("script");
			script.src =
				"https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
			script.async = true;
			script.onload = () =>
				window.turnstile.render("#turnstile", {
					sitekey: c.turnstileSiteKey,
					callback: (token) => (qs("[name=turnstileToken]").value = token),
				});
			document.head.append(script);
		})
		.catch(() => {});

}
