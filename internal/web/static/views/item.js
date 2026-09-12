// One item, and every number it ends up with, each sum written out with this
// item's own numbers in it.

let host = null;
let body = null;
let latest = 0;

views.item = {
  async draw(where, at) {
    host = where;
    const mine = ++latest;
    if (!at.n) return welcome();

    replace(host, h("div", { class: "load" }, "Loading…"));
    let answer;
    try {
      answer = await api(`/api/live/${encodeURIComponent(at.n)}?active=${settings.active ? 1 : 0}`);
    } catch (err) {
      if (mine === latest) replace(host, h("div", { class: "empty" }, h("p", {}, err.message)));
      return;
    }
    // A slow answer for a page already left must not paint over the new one.
    if (mine !== latest) return;
    body = answer;
    alarm(body.error || "");
    paint();
  },
};

function welcome() {
  replace(host, h("div", { class: "empty" },
    h("p", {}, "Search for an item at the top."),
    h("p", {}, "Try ",
      h("a", { on: { click: () => go({ v: "item", n: "Rifter" }) } }, "Rifter"), ", ",
      h("a", { on: { click: () => go({ v: "item", n: "Drake" }) } }, "Drake"), " or ",
      h("a", { on: { click: () => go({ v: "item", n: "Warp Disruptor II" }) } }, "Warp Disruptor II"), ".")));
}

const SIGN = {
  preAssign: "=", preMul: "×", preDiv: "÷", modAdd: "+", modSub: "−",
  postMul: "×", postDiv: "÷", postPercent: "+%", postAssign: "=",
};

// touched is a value the patches had a hand in, either because the attribute
// is one of ours or because a rule of ours moved the number.
const touched = (entry) => entry.ours || entry.added || entry.was !== undefined ||
  (entry.steps || []).some((step) => step.ours);

const kindOf = (entry) => entry.ours ? "new" : touched(entry) ? "patched" : "";

// ------------------------------------------------------------------- paint

function paint() {
  replace(host,
    h("div", { class: "head" },
      h("h1", {}, body.name),
      h("span", { class: "tiny" }, `${body.group.name} · ${body.category.name} · #${body.id}`),
      body.published ? null : h("span", { class: "tag" }, "not published")),

    needsFit(),
    values());
}

const byName = (a, b) => a.name.localeCompare(b.name);

// Ours first, then the rest, each in name order.
function sorted() {
  const mine = body.values.filter(touched).sort(byName);
  const rest = body.values.filter((entry) => !touched(entry)).sort(byName);
  return { mine, all: [...mine, ...rest] };
}

function values() {
  const { mine, all } = sorted();

  // Only the list is redrawn while filtering, so the box keeps its focus.
  const list = h("div", { class: "card-body" });
  const box = h("input", { class: "search", placeholder: "Filter by name…" });
  const draw = () => {
    const low = box.value.trim().toLowerCase();
    const shown = low ? all.filter((entry) => entry.name.toLowerCase().includes(low)) : all;
    replace(list, shown.length ? shown.map(row) : h("p", { class: "none" }, "No values match that name."));
  };
  box.addEventListener("input", debounce(draw, 120));
  draw();

  const note = mine.length
    ? `${mine.length.toLocaleString()} of ${all.length.toLocaleString()} changed by patches`
    : `${all.length.toLocaleString()} values, none changed by patches`;

  return h("section", { class: "card" },
    h("div", { class: "card-head" },
      h("h3", {}, "All values"),
      h("span", { class: "tiny" }, note),
      h("span", { class: "spacer" }),
      box),
    list);
}

function row(entry) {
  const suffix = unit(entry.unitID);
  const bad = isBroken(entry.value) || entry.loop;
  const kind = kindOf(entry);

  return h("div", { class: "value" + (bad ? " bad" : "") },
    h("div", { class: "value-head" },
      attributeLink(entry.name, "name"),
      kind ? h("span", { class: "tag ours" }, kind) : null,
      h("span", { class: "spacer" }),
      h("b", { class: "answer" }, num(entry.value), suffix ? h("span", { class: "unit" }, suffix) : null)),
    working(entry),
    entry.loop ? h("p", { class: "why bad" }, "This value depends on itself, so the calculation was stopped.") : null);
}

// working writes the sum out with this item's own numbers in it. A number the
// item simply carries gets no line, so the rows with a sum stand out.
function working(entry) {
  const steps = entry.steps || [];
  if (!steps.length) {
    return entry.origin === "type"
      ? null
      : h("div", { class: "why" }, "No rules apply here, so it uses the default.");
  }

  let parts = [];
  for (const step of steps) {
    if (step.op === "base") {
      parts = [h("span", { class: "start" }, num(step.value))];
      continue;
    }
    const x = h("span", { class: "x" + (step.ours ? " ours" : "") }, num(step.x),
      h("small", {}, step.modifying));
    const penalty = step.penalty !== undefined && Math.abs(step.penalty - 1) > 1e-9
      ? h("span", { class: "pen", title: "stacking penalty" }, `@${Math.round(step.penalty * 100)}%`)
      : null;

    // An assignment throws away everything before it.
    parts = step.op === "preAssign" || step.op === "postAssign"
      ? [x, penalty]
      : [...parts, h("span", { class: "op" }, SIGN[step.op] || step.op), x, penalty];
  }

  return h("div", { class: "working" }, parts,
    h("span", { class: "eq" }, "="), h("span", { class: "out" }, num(entry.value)));
}

// needsFit comes first: these rules do nothing at all until the item is on a
// ship, so they explain the numbers that are missing from everything below.
function needsFit() {
  if (!body.needsFit.length) return null;

  return h("section", { class: "card" },
    h("div", { class: "card-head" },
      h("h3", {}, "When fitted"),
      h("span", { class: "tiny" }, "these change the ship, not this item")),
    h("div", { class: "card-body" }, body.needsFit.map((at) => h("div", { class: "away" },
      h("span", { class: "op" }, SIGN[at.op] || at.op),
      attributeLink(at.modifying, "ref" + (at.ours ? " ours" : "")),
      h("span", { class: "onto" }, "to"),
      attributeLink(at.modified, "ref"),
      h("span", { class: "onto" }, "on ", landsOn(at.domain)),
      at.ours ? h("span", { class: "tag ours" }, "patched") : null))));
}
