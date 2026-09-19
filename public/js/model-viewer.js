/**
 * Character 3D viewer.
 *
 * Wowhead's model viewer cannot be embedded cross-origin: `wow.zamimg.com`
 * returns 403 to any request carrying an `Origin` header, so `contentPath`
 * pointing at their CDN always fails. The Go backend therefore exposes an
 * allow-listed, same-origin reverse proxy at `/modelviewer/`, and this module
 * mounts the viewer against it.
 *
 * The viewer also expects a `WH` namespace and a jQuery-shaped API at load time,
 * so both are stubbed here rather than pulling jQuery from a CDN.
 *
 * Resolves `true` when a canvas is present, `false` otherwise, so callers can
 * show an honest fallback instead of a spinner that never resolves.
 */

/** Model-viewer display id: character models start at 49 (Human male). */
export function characterDisplayId(race, gender) {
	return (Number(race) - 1) * 2 + Number(gender) + 1;
}

let viewerScriptPromise;

function loadViewerScript() {
	if (window.ZamModelViewer) return Promise.resolve(window.ZamModelViewer);
	if (!viewerScriptPromise) {
		viewerScriptPromise = new Promise((resolve, reject) => {
			const script = document.createElement("script");
			script.src = "/modelviewer/viewer.min.js";
			script.async = true;
			script.onload = () =>
				window.ZamModelViewer
					? resolve(window.ZamModelViewer)
					: reject(new Error("viewer unavailable"));
			script.onerror = () => reject(new Error("viewer unavailable"));
			document.head.append(script);
		}).catch((error) => {
			viewerScriptPromise = undefined;
			throw error;
		});
	}
	return viewerScriptPromise;
}

/** The viewer reads `WH.*` during load; unknown members must not throw. */
function installWH() {
	if (window.WH && window.WH.__portalStub) return;
	const wh = {
		__portalStub: true,
		debug() {},
		warn() {},
		error() {},
		defaultAnimation: "Stand",
		REMOTE: 0,
		staticUrl: "https://wow.zamimg.com/",
		ge: (id) => document.getElementById(id),
		// Load-bearing: without this the viewer throws
		// "WH.WebP.getImageExtension is not a function" and renders untextured.
		WebP: { getImageExtension: () => ".webp", feature() {}, supportsFeature: () => true },
	};
	window.WH = new Proxy(wh, {
		get(target, prop) {
			if (prop in target) return target[prop];
			return typeof prop === "string" ? () => {} : undefined;
		},
	});
}

/**
 * Minimal jQuery-shaped shim. The viewer calls `jQuery(node)`, reads
 * `.width()`/`.height()`, binds events, appends nodes and its transport layer
 * awaits a jQuery deferred — so `ajax` must return a real thenable whose
 * `done`/`fail` callbacks accumulate rather than overwrite each other.
 */
function installJQueryShim() {
	if (window.jQuery && window.jQuery.__portalShim) return;
	const listeners = new WeakMap();
	const transports = new Map();
	const sizeOf = (el, dimension) =>
		!el ? 0 : dimension === "width" ? el.clientWidth || el.offsetWidth || 0 : el.clientHeight || el.offsetHeight || 0;

	function wrap(nodes) {
		return {
			0: nodes[0],
			length: nodes.length,
			jquery: "3.7.1",
			each(fn) {
				nodes.forEach((node, index) => fn.call(node, index, node));
				return this;
			},
			get: (index) => nodes[index],
			on(name, handler) {
				nodes.forEach((node) => {
					const stored = listeners.get(node) || new Map();
					stored.set(`${name}:${stored.size}`, handler);
					listeners.set(node, stored);
					node.addEventListener(name, handler);
				});
				return this;
			},
			off(name) {
				nodes.forEach((node) => {
					const stored = listeners.get(node);
					stored?.forEach((handler, key) => {
						if (!name || key.startsWith(`${name}:`)) node.removeEventListener(name, handler);
					});
				});
				return this;
			},
			bind(name, handler) {
				return this.on(name, handler);
			},
			width: () => sizeOf(nodes[0], "width"),
			height: () => sizeOf(nodes[0], "height"),
			innerWidth: () => sizeOf(nodes[0], "width"),
			innerHeight: () => sizeOf(nodes[0], "height"),
			css(property, value) {
				if (typeof property === "object") {
					nodes.forEach((node) => Object.assign(node.style, property));
					return this;
				}
				if (value === undefined) return nodes[0] ? getComputedStyle(nodes[0])[property] : "";
				nodes.forEach((node) => (node.style[property] = value));
				return this;
			},
			attr(name, value) {
				if (value === undefined) return nodes[0]?.getAttribute?.(name) ?? undefined;
				nodes.forEach((node) => node.setAttribute?.(name, value));
				return this;
			},
			append(child) {
				const node = child?.[0] ?? child;
				nodes.forEach((parent) => parent.append(node));
				return this;
			},
			remove() {
				nodes.forEach((node) => node.remove());
				return this;
			},
			detach() {
				nodes.forEach((node) => node.remove());
				return this;
			},
			is: (selector) => Boolean(nodes[0]?.matches?.(selector)),
			show() {
				nodes.forEach((node) => (node.style.display = ""));
				return this;
			},
			hide() {
				nodes.forEach((node) => (node.style.display = "none"));
				return this;
			},
			toggle(visible) {
				nodes.forEach((node) => (node.style.display = visible ? "" : "none"));
				return this;
			},
			fadeIn() {
				return this.show();
			},
			fadeOut() {
				return this.hide();
			},
			addClass(name) {
				nodes.forEach((node) => node.classList.add(name));
				return this;
			},
			removeClass(name) {
				nodes.forEach((node) => node.classList.remove(name));
				return this;
			},
			hasClass: (name) => Boolean(nodes[0]?.classList?.contains(name)),
		};
	}

	const jQuery = (target) => {
		if (target === window || target === document) target = document.documentElement;
		if (typeof target === "string") {
			if (target.trim().startsWith("<")) {
				const holder = document.createElement("div");
				holder.innerHTML = target.trim();
				return wrap([...holder.childNodes]);
			}
			return wrap([...document.querySelectorAll(target)]);
		}
		if (!target) return wrap([]);
		if (target instanceof Element) return wrap([target]);
		if (target instanceof NodeList || Array.isArray(target)) return wrap([...target]);
		return wrap([target]);
	};

	jQuery.fn = {};
	jQuery.support = { cors: true, ajax: true };
	jQuery.ajaxSettings = { flatOptions: {} };
	jQuery.ajaxSetup = (options) => Object.assign(jQuery.ajaxSettings, options);
	jQuery.ajaxTransport = (name, factory) => {
		if (factory) transports.set(name, factory);
		else transports.delete(name);
		return factory;
	};
	jQuery.extend = (target, ...sources) => Object.assign(target || {}, ...sources);
	jQuery.each = (collection, fn) => {
		if (Array.isArray(collection) || collection instanceof NodeList) [...collection].forEach((value, index) => fn(index, value));
		else Object.entries(collection || {}).forEach(([key, value]) => fn(key, value));
		return collection;
	};
	jQuery.isFunction = (value) => typeof value === "function";
	jQuery.isArray = Array.isArray;
	jQuery.isEmptyObject = (value) => !value || Object.keys(value).length === 0;
	jQuery.inArray = (value, array) => (array || []).indexOf(value);
	jQuery.trim = (value) => String(value ?? "").trim();
	jQuery.param = (object) =>
		Object.entries(object || {})
			.map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
			.join("&");
	jQuery.getJSON = (url, data, success) => jQuery.ajax({ url, data, dataType: "json", success });
	jQuery.get = (url, success) => jQuery.ajax({ url, success });
	jQuery.post = (url, data, success) => jQuery.ajax({ url, data, type: "POST", success });
	jQuery.parseJSON = (value) => JSON.parse(value);

	jQuery.ajax = (options = {}) => {
		const callbacks = { done: [], fail: [], always: [] };
		const settle = (ok, value) => {
			callbacks[ok ? "done" : "fail"].forEach((fn) => fn(value));
			callbacks.always.forEach((fn) => fn(value));
		};
		const deferred = {
			done(fn) {
				if (fn) callbacks.done.push(fn);
				return deferred;
			},
			fail(fn) {
				if (fn) callbacks.fail.push(fn);
				return deferred;
			},
			always(fn) {
				if (fn) callbacks.always.push(fn);
				return deferred;
			},
			then(resolve, reject) {
				return Promise.resolve().then(() => deferred.__promise).then(resolve, reject);
			},
			catch(reject) {
				return deferred.then(undefined, reject);
			},
		};

		const binary = options.dataType === "binary" || options.data instanceof ArrayBuffer || options.data instanceof Blob;
		const transport = transports.get("+binary");
		if (binary && transport) {
			const instance = transport({ ...options }, () => {}, {});
			instance?.send?.({}, (status, response) => {
				const ok = status === "success";
				if (ok) options.success?.(response, status, {});
				else options.error?.({}, status);
				options.complete?.({}, status);
				settle(ok, response);
			});
			deferred.__promise = new Promise((resolve) => callbacks.done.push(resolve));
			return deferred;
		}

		const method = (options.type || options.method || "GET").toUpperCase();
		const query = typeof options.data === "string" && method === "GET" && options.data ? `?${options.data}` : "";
		deferred.__promise = fetch(`${options.url}${query}`, {
			method,
			headers: options.contentType ? { "Content-Type": options.contentType } : undefined,
			body: method === "GET" ? undefined : options.data,
		})
			.then((response) => {
				if (!response.ok) throw new Error(`${response.status} ${response.statusText}`);
				return options.dataType === "json" ? response.json() : response.text();
			})
			.then((data) => {
				options.success?.(data, "success", {});
				options.complete?.({}, "success");
				settle(true, data);
				return data;
			})
			.catch((error) => {
				options.error?.({}, "error", error);
				options.complete?.({}, "error");
				settle(false, error);
				throw error;
			});
		deferred.__promise.catch((error) => {
			if (!callbacks.fail.length) console.warn("[viewer] request failed", options.url, error.message);
		});
		return deferred;
	};

	jQuery.__portalShim = true;
	window.jQuery = window.$ = jQuery;
}

/** A canvas created without an explicit pixel size never initialises WebGL. */
function sizeCanvas(canvas, host) {
	const width = Math.max(1, Math.round(host.clientWidth || 420));
	const height = Math.max(1, Math.round(host.clientHeight || 420));
	if (canvas.width !== width) canvas.width = width;
	if (canvas.height !== height) canvas.height = height;
	canvas.style.width = `${width}px`;
	canvas.style.height = `${height}px`;
}

/** The viewer only uses four methods on its container. */
function viewerContainer(host) {
	const $ = window.jQuery;
	const wrapper = $(host);
	wrapper.width = () => host.clientWidth || 420;
	wrapper.height = () => host.clientHeight || 420;
	wrapper.css = (properties) => {
		if (properties && typeof properties === "object") Object.assign(host.style, properties);
		return wrapper;
	};
	wrapper.append = (child) => {
		const node = child?.[0] ?? child;
		if (node?.tagName === "CANVAS") sizeCanvas(node, host);
		host.append(node);
		return wrapper;
	};
	return wrapper;
}

function ensureCanvasSized(host) {
	const canvas = host.querySelector("canvas");
	if (!canvas) return;
	const width = Math.round(host.clientWidth || 0);
	const height = Math.round(host.clientHeight || 0);
	if (!width || !height) return;
	if (canvas.width !== width || canvas.height !== height) sizeCanvas(canvas, host);
}

/**
 * Mount the character viewer into `host`.
 * @returns {Promise<boolean>} true when a canvas mounted, false otherwise.
 */
export async function mountCharacterModel(host, character, equipment) {
	installWH();
	installJQueryShim();
	host.classList.remove("hidden");

	const ready = await Promise.race([
		loadViewerScript().then(() => true).catch(() => false),
		new Promise((resolve) => setTimeout(() => resolve(false), 8000)),
	]);
	if (!ready) return false;

	const items = equipment
		.filter((item) => item.displayId && item.slot !== 3 && item.slot !== 18)
		.map((item) => [Number(item.slot) + 1, Number(item.displayId)]);

	const width = host.clientWidth || 420;
	const height = host.clientHeight || 420;

	let viewer;
	try {
		viewer = new window.ZamModelViewer({
			container: viewerContainer(host),
			contentPath: "/modelviewer/auto/",
			type: 2,
			aspect: width / Math.max(1, height),
			hd: false,
			dataEnv: "classic",
			env: "classic",
			gameDataEnv: "classic",
			models: { id: characterDisplayId(character.race, character.gender), type: 16 },
			items,
		});
	} catch (error) {
		console.warn("[viewer] constructor failed", error);
		return false;
	}

	const mounted = await new Promise((resolve) => {
		const started = Date.now();
		const poll = () => {
			if (host.querySelector("canvas")) {
				ensureCanvasSized(host);
				return resolve(true);
			}
			if (Date.now() - started > 9000) return resolve(false);
			setTimeout(poll, 200);
		};
		poll();
	});
	if (!mounted) {
		try {
			viewer?.destroy?.();
		} catch {}
		return false;
	}

	const fit = () => {
		ensureCanvasSized(host);
		try {
			viewer?.setAspect?.((host.clientWidth || 420) / Math.max(1, host.clientHeight || 420));
		} catch {}
	};
	window.addEventListener("resize", fit);
	setTimeout(fit, 250);
	return true;
}
