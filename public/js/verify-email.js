import {qs, setMessage} from "/js/ui.js";

export function mountVerifyEmail(context) {
	const { api } = context;
	const status = qs("#verification-status"),
		resend = qs("#verification-resend"),
		token = new URLSearchParams(location.search).get("token") || "";
	const verify = async () => {
		if (!token) {
			status.textContent = "This verification link is missing its token.";
			resend.classList.remove("hidden");
			return;
		}
		try {
			const result = await api("/api/auth/email/verify", {
				method: "POST",
				body: JSON.stringify({ token }),
			});
			status.textContent = result.message;
			status.classList.add("success");
			qs("#verification-login").classList.remove("hidden");
		} catch (err) {
			status.textContent = err.message;
			resend.classList.remove("hidden");
		}
	};
	resend.onsubmit = async (e) => {
		e.preventDefault();
		const button = qs("button", resend);
		button.disabled = true;
		try {
			const result = await api("/api/auth/email/resend", {
				method: "POST",
				body: JSON.stringify({ email: new FormData(resend).get("email") }),
			});
			setMessage(resend, result.message, true);
		} catch (err) {
			setMessage(resend, err.message);
		} finally {
			button.disabled = false;
		}
	};
	verify();
}
