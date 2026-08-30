import {qs, setMessage} from "/js/ui.js";

export function mountSetup(context) {
	const { api, initial } = context;
	const form = qs("#setup-form");
	const showSetupState = (titleText, messageText, withLogin = false) => {
		form.innerHTML = "";
		const title = document.createElement("h2");
		title.textContent = titleText;
		const message = document.createElement("p");
		message.className = "muted";
		message.textContent = messageText;
		form.append(title, message);
		if (withLogin) {
			const link = document.createElement("a");
			link.className = "button full";
			link.href = "/login";
			link.textContent = "Sign in →";
			form.append(link);
		}
	};
	setupStatePromise
		.then((state) => {
			if (!state.enabled)
				showSetupState(
					"Setup disabled",
					"Configure ENABLE_SETUP=true and a SETUP_TOKEN to use the web setup wizard.",
				);
			else if (state.complete)
				showSetupState(
					"Setup complete",
					"The initial realm administrator has already been created.",
					true,
				);
		})
		.catch((e) => setMessage(form, e.message));
	form.onsubmit = async (e) => {
		e.preventDefault();
		const values = Object.fromEntries(new FormData(form));
		if (values.password !== values.confirmPassword) {
			setMessage(form, "Passwords do not match");
			return;
		}
		delete values.confirmPassword;
		const button = qs("button[type=submit]", form);
		button.disabled = true;
		try {
			const result = await api("/api/setup", {
				method: "POST",
				body: JSON.stringify(values),
			});
			form.reset();
			form.innerHTML = "";
			const title = document.createElement("h2");
			title.textContent = "Setup complete";
			const message = document.createElement("p");
			message.className = "success";
			message.textContent = `${result.username} is now a level ${result.gmLevel} GM.`;
			const link = document.createElement("a");
			link.className = "button full";
			link.href = "/login";
			link.textContent = "Sign in →";
			form.append(title, message, link);
		} catch (err) {
			setMessage(form, err.message);
		} finally {
			button.disabled = false;
		}
	};

}
