import { readFile } from "node:fs/promises";

const [migrations, readme] = await Promise.all([
	readFile("internal/store/store.go", "utf8"),
	readFile("README.md", "utf8"),
]);

const migrationTables = new Set(
	[...migrations.matchAll(/CREATE TABLE IF NOT EXISTS\s+`?(portal_[A-Za-z0-9_]+)/gi)]
		.map((match) => match[1].toLowerCase()),
);
const documented = [...readme.matchAll(/\bON\s+acore_auth\.(portal_[A-Za-z0-9_]+)\s+TO\b/gi)]
	.map((match) => match[1].toLowerCase());
const documentedTables = new Set(documented);
const missing = [...migrationTables].filter((table) => !documentedTables.has(table)).sort();
const duplicates = [...documentedTables].filter((table) => documented.filter((item) => item === table).length > 1).sort();

if (missing.length || duplicates.length) {
	const details = [
		missing.length ? `Missing README grants: ${missing.join(", ")}` : "",
		duplicates.length ? `Duplicate README grants: ${duplicates.join(", ")}` : "",
	].filter(Boolean).join("\n");
	throw new Error(details);
}

console.log(`Database grant documentation covers all ${migrationTables.size} portal tables.`);
