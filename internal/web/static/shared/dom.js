// The handful of things both views need. They hang off the global scope so a
// page can pull them in with a plain script tag, in order, with no bundler.

globalThis.$ = (id) => document.getElementById(id);

// The views register themselves here as they load, before the shell runs.
globalThis.views = {};

globalThis.h = function h(tag, attrs, ...kids) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(attrs || {})) {
    if (value === null || value === undefined || value === false) continue;
    if (key === "on") for (const [name, fn] of Object.entries(value)) node.addEventListener(name, fn);
    else if (key === "class") node.className = value;
    else if (writable(node, key)) node[key] = value;
    else node.setAttribute(key, value);
  }
  return fill(node, ...kids);
};

// Some properties only reflect an attribute and cannot be written; "list" on
// an input is one. Those have to go through setAttribute instead.
function writable(node, key) {
  for (let at = node; at; at = Object.getPrototypeOf(at)) {
    const found = Object.getOwnPropertyDescriptor(at, key);
    if (found) return found.writable === true || found.set !== undefined;
  }
  return false;
}

globalThis.fill = function fill(node, ...kids) {
  for (const kid of kids.flat(9)) {
    if (kid === null || kid === undefined || kid === false) continue;
    node.appendChild(kid instanceof Node ? kid : document.createTextNode(String(kid)));
  }
  return node;
};

globalThis.replace = (node, ...kids) => (node.textContent = "", fill(node, ...kids));

globalThis.api = async function api(path, options) {
  const response = await fetch(path, options);
  const body = await response.json();
  if (!response.ok) throw new Error(body.error || response.statusText);
  return body;
};

globalThis.put = (path, body) => api(path, {
  method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
});

globalThis.debounce = function debounce(fn, wait) {
  let timer;
  return (...args) => { clearTimeout(timer); timer = setTimeout(() => fn(...args), wait); };
};

globalThis.attributeLink = (name, className) => h("a", {
  class: className, on: { click: () => go({ v: "attribute", n: name }) },
}, name);

// Where a rule that needs a whole fit lands.
const DOMAINS = {
  shipID: "the ship it is fitted to",
  charID: "the pilot",
  otherID: "the charge in it, or the module it sits in",
  structureID: "the structure",
  target: "whatever it is used on",
  targetID: "whatever it is used on",
};

globalThis.landsOn = (domain) => DOMAINS[domain] || domain;

// A number the server could not put in JSON comes back as a word, which is
// passed straight through.
globalThis.num = function num(value) {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return "–";
  if (value === 0) return "0";
  const size = Math.abs(value);
  if (size >= 1e9 || size < 1e-4) return value.toExponential(2);
  if (Number.isInteger(value)) return value.toLocaleString();
  return value.toLocaleString(undefined, { maximumFractionDigits: size >= 100 ? 1 : size >= 1 ? 2 : 4 });
};

globalThis.isBroken = (value) => typeof value === "string";

// Only the units that are never in doubt. A wrong unit reads worse than none.
const UNITS = {
  1: "m", 2: "kg", 3: "s", 4: "m³", 9: "m³", 11: "m/s", 101: "ms", 102: "mm",
  106: "tf", 107: "MW", 113: "HP", 114: "GJ", 121: "%", 122: "slots", 124: "%",
  127: "%", 137: "", 139: "", 141: "hardpoints",
};

globalThis.unit = (id) => UNITS[id] || "";
