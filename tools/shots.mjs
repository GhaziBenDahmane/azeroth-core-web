import { chromium } from "playwright-core";
const base = process.env.BASE_URL || "http://127.0.0.1:8080";
const browser = await chromium.launch({ executablePath: "/usr/bin/chromium", args: ["--no-sandbox"] });
const page = await browser.newPage({ viewport: { width: 1440, height: 950 }, colorScheme: "dark" });
await page.goto(`${base}/login`, { waitUntil: "networkidle" });
await page.fill('#login-form input[name="username"]', "DEMO");
await page.fill('#login-form input[name="password"]', "demo1234");
await page.click('#login-form button[type="submit"]');
await page.waitForURL(/account/).catch(() => {});
for (const [name, url] of [
  ["i-home", "/"],
  ["i-account", "/account"],
  ["i-admin", "/admin"],
  ["i-shop", "/shop"],
  ["i-rankings", "/rankings"],
  ["i-community", "/community"],
]) {
  await page.goto(base + url, { waitUntil: "networkidle" });
  await page.waitForTimeout(1800);
  await page.screenshot({ path: `/tmp/shots/${name}.png` });
  console.log("shot", name);
}
await browser.close();
