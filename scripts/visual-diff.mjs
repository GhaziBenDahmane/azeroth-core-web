import {copyFile, mkdir, readdir, readFile, writeFile} from "node:fs/promises";
import {join} from "node:path";
import {inflateSync} from "node:zlib";

const currentDir = process.env.BROWSER_AUDIT_OUTPUT || "test-results/browser";
const baselineDir = process.env.BROWSER_BASELINE_DIR || "test-baselines/browser";
const allowedRatio = Number(process.env.VISUAL_DIFF_RATIO || "0.02");
const channelThreshold = Number(process.env.VISUAL_CHANNEL_THRESHOLD || "20");
const approve = process.argv.includes("--approve");

function decodePNG(buffer) {
	if (buffer.subarray(0, 8).toString("hex") !== "89504e470d0a1a0a") throw new Error("not a PNG");
	let offset = 8, width = 0, height = 0, bitDepth = 0, colorType = 0;
	const compressed = [];
	while (offset < buffer.length) {
		const length = buffer.readUInt32BE(offset), type = buffer.subarray(offset + 4, offset + 8).toString("ascii"), data = buffer.subarray(offset + 8, offset + 8 + length);
		offset += length + 12;
		if (type === "IHDR") { width = data.readUInt32BE(0); height = data.readUInt32BE(4); bitDepth = data[8]; colorType = data[9]; }
		if (type === "IDAT") compressed.push(data);
		if (type === "IEND") break;
	}
	const channels = colorType === 6 ? 4 : colorType === 2 ? 3 : 0;
	if (bitDepth !== 8 || !channels) throw new Error(`unsupported PNG format depth=${bitDepth} color=${colorType}`);
	const raw = inflateSync(Buffer.concat(compressed)), stride = width * channels, pixels = Buffer.alloc(width * height * 4);
	let source = 0, previous = Buffer.alloc(stride);
	for (let y = 0; y < height; y++) {
		const filter = raw[source++], row = Buffer.alloc(stride);
		for (let x = 0; x < stride; x++) {
			const value = raw[source++], left = x >= channels ? row[x - channels] : 0, up = previous[x], upperLeft = x >= channels ? previous[x - channels] : 0;
			if (filter === 0) row[x] = value;
			else if (filter === 1) row[x] = value + left;
			else if (filter === 2) row[x] = value + up;
			else if (filter === 3) row[x] = value + Math.floor((left + up) / 2);
			else if (filter === 4) { const p = left + up - upperLeft, pa = Math.abs(p - left), pb = Math.abs(p - up), pc = Math.abs(p - upperLeft); row[x] = value + (pa <= pb && pa <= pc ? left : pb <= pc ? up : upperLeft); }
			else throw new Error(`unsupported PNG filter ${filter}`);
		}
		for (let x = 0; x < width; x++) { const from = x * channels, to = (y * width + x) * 4; pixels[to] = row[from]; pixels[to + 1] = row[from + 1]; pixels[to + 2] = row[from + 2]; pixels[to + 3] = channels === 4 ? row[from + 3] : 255; }
		previous = row;
	}
	return {width, height, pixels};
}

async function pngNames(directory) {
	return (await readdir(directory)).filter((name) => name.endsWith(".png")).sort();
}

await mkdir(baselineDir, {recursive:true});
const currentNames = await pngNames(currentDir);
if (!currentNames.length) throw new Error(`No browser screenshots found in ${currentDir}; run npm run test:browser first.`);
if (approve) {
	for (const name of currentNames) await copyFile(join(currentDir, name), join(baselineDir, name));
	console.log(`Approved ${currentNames.length} visual baselines in ${baselineDir}. Review the images before committing them.`);
	process.exit(0);
}
const baselineNames = await pngNames(baselineDir);
if (!baselineNames.length) throw new Error(`No approved baselines in ${baselineDir}; review current screenshots, then run npm run test:visual:approve.`);
const failures = [], report = [];
for (const name of currentNames) {
	if (!baselineNames.includes(name)) { failures.push(`${name}: baseline missing`); continue; }
	const current = decodePNG(await readFile(join(currentDir, name))), baseline = decodePNG(await readFile(join(baselineDir, name)));
	if (current.width !== baseline.width || current.height !== baseline.height) { failures.push(`${name}: size ${current.width}×${current.height}, expected ${baseline.width}×${baseline.height}`); continue; }
	let changed = 0;
	for (let index = 0; index < current.pixels.length; index += 4) {
		if (Math.max(...[0,1,2,3].map((channel) => Math.abs(current.pixels[index + channel] - baseline.pixels[index + channel]))) > channelThreshold) changed++;
	}
	const ratio = changed / (current.width * current.height);
	report.push({name, changedPixels:changed, ratio});
	if (ratio > allowedRatio) failures.push(`${name}: ${(ratio * 100).toFixed(2)}% pixels changed (limit ${(allowedRatio * 100).toFixed(2)}%)`);
}
for (const name of baselineNames) if (!currentNames.includes(name)) failures.push(`${name}: current screenshot missing`);
await writeFile(join(currentDir, "visual-diff-report.json"), JSON.stringify({allowedRatio, channelThreshold, images:report}, null, 2));
for (const item of report) console.log(`${item.name}: ${(item.ratio * 100).toFixed(2)}% changed`);
if (failures.length) throw new Error(`Visual regression check failed:\n${failures.join("\n")}`);
