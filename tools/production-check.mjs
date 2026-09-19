/** Verify the deployed production site: shell, routes, console health, 3D proxy. */
import { chromium } from "playwright-core";
const base = process.env.BASE_URL || "https://valenoza.com";
const browser = await chromium.launch({
  executablePath: "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage", "--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: "dark" });
const errors = [];
page.on("pageerror", (e) => errors.push(`pageerror ${e.message.slice(0, 110)}`));
page.on("console", (m) => { if (m.type() === "error" && !/401|403|404|favicon/.test(m.text())) errors.push(m.text().slice(0, 110)); });

let failures = 0;
for (const path of ["/", "/play", "/armory", "/rankings", "/community", "/tools", "/vote", "/tracker", "/shop", "/realm", "/guilds", "/news", "/security", "/login"]) {
  errors.length = 0;
  await page.goto(base + path, { waitUntil: "networkidle", timeout: 45000 });
  await page.waitForTimeout(1600);
  const r = await page.evaluate(() => ({
    rail: Math.round(document.querySelector(".rail")?.getBoundingClientRect().width ?? 0),
    overflow: document.documentElement.scrollWidth > window.innerWidth + 1,
    brand: document.querySelector("[data-brand-name]")?.textContent?.trim(),
    accent: getComputedStyle(document.documentElement).getPropertyValue("--accent").trim(),
    chars: (document.querySelector("main")?.innerText || "").trim().length,
    // Pages may legitimately have no content yet and render an empty state.
    empty: Boolean(document.querySelector(".empty, [class*='empty']")),
  }));
  const ok = r.rail > 0 && !r.overflow && (r.chars > 150 || r.empty) && errors.length === 0;
  if (!ok) failures++;
  console.log(`${ok ? "PASS" : "FAIL"} ${path.padEnd(12)} rail=${r.rail} brand=${r.brand} accent=${r.accent} chars=${String(r.chars).padStart(5)}${r.overflow ? " OVERFLOW" : ""}`);
  for (const e of errors.slice(0, 2)) console.log(`      ! ${e}`);
}

// 3D armory preview on production.
errors.length = 0;
const assets = [];
page.on("response", (res) => { if (res.url().includes("/modelviewer/auto/")) assets.push(res.status()); });
// Pick a character that exists on this realm rather than assuming a fixture.
const roster = await page.evaluate(async () => (await (await fetch("/api/armory")).json()).characters || []);
const target = roster[0]?.name;
console.log(`      (3D check uses real character: ${target})`);
await page.goto(`${base}/armory?q=${encodeURIComponent(target)}`, { waitUntil: "networkidle", timeout: 45000 });
await page.waitForTimeout(15000);
const model = await page.evaluate(() => {
  const canvas = document.querySelector(".wowhead-model canvas");
  const host = document.querySelector(".wowhead-model");
  return {
    canvas: canvas ? [canvas.width, canvas.height] : null,
    host: host ? [host.clientWidth, host.clientHeight] : null,
    provider: document.querySelector(".model-provider")?.textContent?.trim(),
  };
});
const sized = model.canvas && model.host && Math.abs(model.canvas[0] - model.host[0]) <= 2;
const modelOk = Boolean(sized) && model.canvas[0] > 100 && assets.length > 3;
if (!modelOk) failures++;
console.log(`${modelOk ? "PASS" : "FAIL"} 3D preview  canvas=${model.canvas ? model.canvas.join("x") : "none"} assets=${assets.filter((s) => s === 200).length} provider=${JSON.stringify(model.provider)}`);

await page.screenshot({ path: "/tmp/shots/prod-home.png" });
await page.goto(`${base}/`, { waitUntil: "networkidle" });
await page.waitForTimeout(2500);
await page.screenshot({ path: "/tmp/shots/prod-home.png" });

await browser.close();
console.log(failures ? `\n${failures} check(s) failed` : "\nproduction healthy");
process.exit(failures ? 1 : 0);
