import {readFile} from "node:fs/promises";

const routePattern = /m\.HandleFunc\("([A-Z]+) ([^" ]+)"/g;
const load = async (file) => {
	const source = await readFile(file, "utf8"), routes = [], seen = new Set();
	for (const match of source.matchAll(routePattern)) {
		const route = `${match[1]} ${match[2]}`;
		if (seen.has(route)) throw new Error(`${file} registers ${route} more than once`);
		seen.add(route);
		routes.push(route);
	}
	return new Set(routes.filter((route) => route.includes(" /api/")));
};

const production = await load("internal/web/server.go");
const mock = await load("internal/web/mock.go");
// These callbacks deliberately require real signed third-party or collector
// input and must not be simulated by the interactive mock portal.
const productionOnly = new Set([
	"POST /api/billing/webhook",
	"POST /api/integrations/battlegrounds",
	"POST /api/integrations/pvp",
	"POST /api/integrations/raids",
]);
const missing = [...production].filter((route) => !mock.has(route) && !productionOnly.has(route));
const unexpected = [...mock].filter((route) => !production.has(route));
const staleExceptions = [...productionOnly].filter((route) => !production.has(route) || mock.has(route));
if (missing.length || unexpected.length || staleExceptions.length) {
	throw new Error([
		missing.length ? `Mock routes missing:\n${missing.join("\n")}` : "",
		unexpected.length ? `Mock-only routes:\n${unexpected.join("\n")}` : "",
		staleExceptions.length ? `Stale production-only exceptions:\n${staleExceptions.join("\n")}` : "",
	].filter(Boolean).join("\n\n"));
}
process.stdout.write(`API contract parity passed: ${production.size} production routes, ${mock.size} mock routes, ${productionOnly.size} intentionally production-only callbacks.\n`);
