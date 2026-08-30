globalThis.location = { origin: "https://portal.example" };

const { esc, publicLink } = await import("../public/js/ui.js");

const linkCases = new Map([
	["/account", "https://portal.example/account"],
	["https://downloads.example/client.zip", "https://downloads.example/client.zip"],
	["http://mirror.example/client.zip", "http://mirror.example/client.zip"],
	["javascript:alert(1)", ""],
	["data:text/html,<script>alert(1)</script>", ""],
	["blob:https://portal.example/id", ""],
	["file:///etc/passwd", ""],
	["https://[invalid", ""],
]);

for (const [input, expected] of linkCases) {
	const actual = publicLink(input);
	if (actual !== expected) throw new Error(`publicLink(${JSON.stringify(input)}) returned ${JSON.stringify(actual)}, expected ${JSON.stringify(expected)}`);
}

const payload = `<img src=x onerror="alert('xss')">`;
const escaped = esc(payload);
if (escaped.includes("<") || escaped.includes(">") || escaped.includes('"') || escaped.includes("'")) {
	throw new Error(`esc did not neutralize HTML attribute content: ${escaped}`);
}

console.log("Frontend URL-scheme and HTML-escaping security checks passed.");
