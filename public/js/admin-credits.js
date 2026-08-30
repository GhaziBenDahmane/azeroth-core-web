import {qs, setMessage} from "/js/ui.js";

export function mountAdminCredits({api}) {
	const form = qs("#gm-credit-form");
	if (!form || form.dataset.initialized === "true") return;
	form.dataset.initialized = "true";
	form.onsubmit = async (event) => {
		event.preventDefault();
		const button = qs('button[type="submit"]', form), values = Object.fromEntries(new FormData(form));
		values.amount = Number(values.amount);
		button.disabled = true;
		setMessage(form, "");
		try {
			const result = await api("/api/admin/credits", {method: "POST", body: JSON.stringify(values)});
			setMessage(form, `${result.amount} credits granted to ${result.username}. New balance: ${result.balance}.${result.requestId ? ` Request ${result.requestId}.` : ""}`, true);
			form.reset();
		} catch (error) { setMessage(form, error.message); }
		finally { button.disabled = false; }
	};
}
