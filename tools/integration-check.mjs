/**
 * Integration verification for the portal this branch ships:
 * every route renders inside the new rail shell, authenticated areas work, the
 * 3D armory preview loads through the same-origin proxy, and no page regresses
 * horizontally.
 */
import { chromium } from "playwright-core";
const base = process.env.BASE_URL || "http://127.0.0.1:8080";
const browser = await chromium.launch({
  executablePath: "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage", "--use-gl=angle", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--ignore-gpu-blocklist"],
});
const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, colorScheme: "dark" });
const errors = [];
page.on("pageerror", (e) => errors.push(`pageerror ${e.message.slice(0, 120)}`));
page.on("console", (m) => { if (m.type() === "error" && !/401|403|404|favicon/.test(m.text())) errors.push(m.text().slice(0, 120)); });

// Sign in so the authenticated surfaces are exercised too.
await page.goto(`${base}/login`, { waitUntil: "networkidle" });
await page.fill('#login-form input[name="username"]', "DEMO");
await page.fill('#login-form input[name="password"]', "demo1234");
await page.click('#login-form button[type="submit"]');
await page.waitForURL(/account/, { timeout: 15000 }).catch(() => {});

let failures = 0;
const routes = ["/", "/play", "/armory", "/rankings", "/community", "/tools", "/vote", "/tracker", "/shop", "/realm", "/guilds", "/news", "/security", "/account", "/admin"];
for (const path of routes) {
  errors.length = 0;
  await page.goto(base + path, { waitUntil: "networkidle" });
  await page.waitForTimeout(1400);
  const r = await page.evaluate(() => ({
    rail: Math.round(document.querySelector(".rail")?.getBoundingClientRect().width ?? 0),
    overflow: document.documentElement.scrollWidth > window.innerWidth + 1,
    chars: (document.querySelector("main")?.innerText || "").trim().length,
    h1: document.querySelector("main h1")?.textContent?.trim().slice(0, 30) || "",
  }));
  const ok = r.rail > 0 && !r.overflow && r.chars > 150 && errors.length === 0;
  if (!ok) failures++;
  console.log(`${ok ? "PASS" : "FAIL"} ${path.padEnd(12)} rail=${r.rail} chars=${String(r.chars).padStart(5)} h1=${JSON.stringify(r.h1).slice(0, 30)}${r.overflow ? " OVERFLOW" : ""}`);
  for (const e of errors.slice(0, 2)) console.log(`      ! ${e}`);
}

// 3D armory preview through the proxy.
errors.length = 0;
const assets = [];
page.on("response", (res) => { if (res.url().includes("/modelviewer/auto/")) assets.push(res.status()); });
// The armory opens a character directly from ?q=.
await page.goto(`${base}/armory?q=Arthoria`, { waitUntil: "networkidle" });
await page.waitForTimeout(14000);
const model = await page.evaluate(() => {
  const canvas = document.querySelector(".wowhead-model canvas");
  const host = document.querySelector(".wowhead-model");
  return {
    canvas: canvas ? [canvas.width, canvas.height] : null,
    host: host ? [host.clientWidth, host.clientHeight] : null,
    provider: document.querySelector(".model-provider")?.textContent?.trim(),
  };
});
// The canvas must match its host (their paper-doll column is ~250px wide, so a
// fixed ">300px" assertion would be wrong) and the model assets must have loaded.
const sized = model.canvas && model.host && Math.abs(model.canvas[0] - model.host[0]) <= 2 && Math.abs(model.canvas[1] - model.host[1]) <= 2;
const modelOk = Boolean(sized) && model.canvas[0] > 100 && model.canvas[1] > 100 && assets.length > 3;
if (!modelOk) failures++;
console.log(`${modelOk ? "PASS" : "FAIL"} 3D preview  canvas=${model.canvas ? model.canvas.join("x") : "none"} host=${model.host ? model.host.join("x") : "none"} assets=${assets.length} provider=${JSON.stringify(model.provider)}`);
for (const e of errors.slice(0, 3)) console.log(`      ! ${e}`);

await browser.close();
console.log(failures ? `\n${failures} check(s) failed` : "\nintegration healthy");
process.exit(failures ? 1 : 0);
