// Two pages: an attribute, and an item.
//
// Which file a rule is written in is never shown; it is only carried along so
// an edit knows where to go.

const shell = { list: [], build: "", filter: "", drawn: "" };

// Settings sit outside the address: they change what a page says, not which
// page you are on.
globalThis.settings = { published: true, active: true };

// ------------------------------------------------------------------ routing

const KEYS = ["v", "n", "all"];

globalThis.route = function route() {
  const params = new URLSearchParams(location.hash.slice(1));
  const where = {};
  for (const key of KEYS) where[key] = params.get(key) || "";
  where.v = where.v || "attribute";
  return where;
};

// Showing every row is only ever meant for the page it was asked on, so going
// anywhere without saying otherwise drops it.
globalThis.go = function go(where) {
  const next = { ...route(), all: "", ...where };
  const params = new URLSearchParams();
  for (const key of KEYS) {
    if (next[key] && !(key === "v" && next[key] === "attribute")) params.set(key, next[key]);
  }

  const hash = "#" + params.toString();
  if (hash === location.hash) return render(true);
  location.hash = hash;
};

globalThis.alarm = function alarm(message) {
  $("alarm").textContent = message || "";
  $("alarm").classList.toggle("show", !!message);
};

// changed is called once an edit has been saved. Everything on screen is
// worked out from the patches, so all of it is stale.
globalThis.changed = async function changed() {
  shell.drawn = "";
  await loadList();
  await render(true);
};

globalThis.query = () => `published=${settings.published ? 1 : 0}&active=${settings.active ? 1 : 0}`;

// ------------------------------------------------------------------- shell

async function loadList() {
  try {
    const [state, sheets] = await Promise.all([api("/api/state"), api(`/api/sheets?${query()}`)]);
    shell.list = sheets.attributes || [];
    shell.build = `build ${state.build} · ${state.types.toLocaleString()} items`;
    alarm(state.error || sheets.error || "");
  } catch (err) {
    alarm(err.message);
  }
  $("build").textContent = shell.build;
  drawRail();
}

const KINDS = [
  ["new", "New", "Attributes the patches add"],
  ["patched", "Patched", "Existing attributes the patches change"],
];

const dot = (entry) => entry.severity >= 3 ? "bad" : entry.severity >= 2 ? "warn" : "";

function drawRail() {
  const where = route();
  const low = shell.filter.toLowerCase();

  replace($("railList"), [KINDS.map(([kind, label, why]) => {
    const rows = shell.list.filter((entry) => entry.kind === kind && entry.name.toLowerCase().includes(low));
    if (!rows.length) return null;

    return h("section", {},
      h("div", { class: "rail-head", title: why }, label, h("span", { class: "n" }, rows.length)),
      rows.map((entry) => {
        const here = where.v === "attribute" && where.n === entry.name;
        return h("a", {
          class: "row" + (here ? " on" : ""),
          on: { click: () => go({ v: "attribute", n: entry.name }) },
        },
          h("span", { class: `dot ${dot(entry)}` }),
          h("b", {}, entry.name),
          h("span", { class: "n" }, entry.count ? entry.count.toLocaleString() : "–"));
      }));
  }), h("div", { class: "rail-foot" },
    h("button", { on: { click: newAttribute } }, "New attribute"))]);
}

// A new attribute is a new file, named after it. Its rules are set up on its
// own page.
async function newAttribute() {
  const name = prompt("Attribute name, as the SDE spells it (e.g. alignTime)")?.trim();
  if (!name) return;

  try {
    await api(`/api/patch/${encodeURIComponent(name)}`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ new: { highIsGood: true } }),
    });
  } catch (err) {
    return alarm(err.message);
  }
  await loadList();
  go({ v: "attribute", n: name });
}

globalThis.removeAttribute = async function removeAttribute(name) {
  if (!confirm(`Delete ${name}? Its ID stays reserved, so it is never reused for something else.`)) return;
  try {
    await api(`/api/patch/${encodeURIComponent(name)}`, { method: "DELETE" });
  } catch (err) {
    return alarm(err.message);
  }
  await loadList();
  go({ v: "attribute", n: "" });
};

function drawSettings() {
  const toggle = (key, text, why) => h("label", {
    class: settings[key] ? "on" : "", title: why,
    on: { click: () => { settings[key] = !settings[key]; changed(); } },
  }, text);

  replace($("settings"),
    toggle("published", "Published only", "Hide items no player can have"),
    toggle("active", "Active modules", "Include effects that only run while a module is active"));
}

async function render(force) {
  const where = route();
  const key = `${where.v}|${where.n}|${where.all}`;
  if (!force && shell.drawn === key) return;
  shell.drawn = key;

  drawRail();
  drawSettings();
  await (views[where.v] || views.attribute).draw($("view"), where);
}

// ------------------------------------------------------------------- lookup

let hits = [];
let chosen = -1;

async function look(text) {
  if (!text.trim()) return shut();

  let body;
  try {
    body = await api(`/api/search?q=${encodeURIComponent(text)}`);
  } catch (err) {
    return alarm(err.message);
  }

  const low = text.toLowerCase();
  hits = [
    ...(body.attributes || []).map((entry) => ({
      what: "attribute", label: entry.name, note: entry.displayName,
      to: { v: "attribute", n: entry.name },
    })),
    ...(body.types || []).map((entry) => ({
      what: "item", label: entry.name, note: `#${entry.id}`,
      to: { v: "item", n: String(entry.id) },
    })),
  ];

  // An exact word first, then what starts with it. Sorting is stable, so
  // attributes stay ahead of items within a rank.
  const rank = (label) => {
    const name = label.toLowerCase();
    return name === low ? 0 : name.startsWith(low) ? 1 : 2;
  };
  hits.sort((a, b) => rank(a.label) - rank(b.label));

  chosen = hits.length ? 0 : -1;
  replace($("hits"), hits.map((hit, i) => h("div", {
    class: i === chosen ? "on" : "",
    on: { mousedown: (e) => { e.preventDefault(); pick(hit); } },
  }, h("span", { class: "what" }, hit.what), h("b", {}, hit.label),
    h("span", { class: "note" }, hit.note || ""))));
  $("hits").classList.toggle("show", hits.length > 0);
}

function pick(hit) {
  $("find").value = "";
  shut();
  go(hit.to);
}

const shut = () => $("hits").classList.remove("show");

{
  const find = $("find");
  find.addEventListener("input", debounce(() => look(find.value), 140));
  find.addEventListener("blur", () => setTimeout(shut, 140));
  find.addEventListener("keydown", (e) => {
    const rows = [...$("hits").children];
    if (e.key === "Escape") return shut();
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      chosen = Math.max(0, Math.min(rows.length - 1, chosen + (e.key === "ArrowDown" ? 1 : -1)));
      rows.forEach((row, i) => row.classList.toggle("on", i === chosen));
      rows[chosen]?.scrollIntoView({ block: "nearest" });
    }
    if (e.key === "Enter" && hits[chosen]) { e.preventDefault(); pick(hits[chosen]); }
  });

  const railFind = $("railFind");
  railFind.addEventListener("input", debounce(() => { shell.filter = railFind.value; drawRail(); }, 100));

  document.addEventListener("keydown", (e) => {
    const typing = ["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement?.tagName);
    if (e.key === "/" && !typing) { e.preventDefault(); find.focus(); }
  });
}

window.addEventListener("hashchange", () => render());

(async () => {
  await loadList();
  await render(true);
})();
