import {qs, setMessage} from "/js/ui.js";

export function mountResetPassword(context) {
	const { api } = context;
	const form = qs("#reset-form");
	form.dataset.initialized="true";
	form.onsubmit = async (e) => {
		e.preventDefault();
		const token = new URLSearchParams(location.search).get("token") || "";
		try {
			await api("/api/auth/password/reset", {
				method: "POST",
				body: JSON.stringify({
					token,
					password: new FormData(form).get("password"),
				}),
			});
			setMessage(form, "Password reset. Taking you to sign in…", true);
			setTimeout(() => (location.href = "/login"), 900);
		} catch (err) {
			setMessage(form, err.message);
		}
	};

}
