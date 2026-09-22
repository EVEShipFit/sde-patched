// One attribute: what it is, the rules that calculate it, and what they came
// to item by item. Every edit redraws the formula and the table together.

let host = null;
let sheet = null;
let editing = "";
let latest = 0;

views.attribute = {
  async draw(where, at) {
    host = where;
    const mine = ++latest;
    if (!at.n) return welcome();

    replace(host, h("div", { class: "load" }, "Calculating…"));
    let answer;
    try {
      answer = await api(`/api/sheet/${encodeURIComponent(at.n)}?${query()}${at.all ? "&all=1" : ""}`);
    } catch (err) {
      if (mine === latest) replace(host, h("div", { class: "empty" }, h("p", {}, err.message)));
      return;
    }
    // A slow answer for a page already left must not paint over the new one.
    if (mine !== latest) return;
    sheet = answer;
    editing = "";
    paint();
  },
};

function welcome() {
  replace(host, h("div", { class: "empty" },
    h("p", {}, "Pick an attribute on the left, or search for an item."),
    h("p", {}, "Try ",
      h("a", { on: { click: () => go({ v: "attribute", n: "alignTime" }) } }, "alignTime"), ", ",
      h("a", { on: { click: () => go({ v: "attribute", n: "shieldEhp" }) } }, "shieldEhp"), " or ",
      h("a", { on: { click: () => go({ v: "item", n: "Rifter" }) } }, "Rifter"), ".")));
}

// ------------------------------------------------------------- the formula

// Dogma runs these in this order whatever order they are written in, so the
// list is the order, and the words say where in it each one lands.
const OPS = [
  ["from", "= ", "start from"],
  ["mulFirst", "× ", "multiply by, before adding"],
  ["divFirst", "÷ ", "divide by, before adding"],
  ["add", "+ ", "add"],
  ["sub", "− ", "subtract"],
  ["mul", "× ", "multiply by"],
  ["div", "÷ ", "divide by"],
  ["percent", "+ ", "add a percentage of"],
  ["assign", "= ", "set it to, after everything"],
];

const SIGN = Object.fromEntries(OPS.map(([op, sign]) => [op, sign.trim()]));
const assigns = (op) => op === "from" || op === "assign";

// badge marks a thing the patches made or changed; anything else gets no mark.
function badge(kind) {
  if (kind !== "new" && kind !== "patched") return null;
  return h("span", { class: "tag ours" }, kind);
}

const items = (n) => `${n.toLocaleString()} item${n === 1 ? "" : "s"}`;

// start is where a sum begins: the item's own value, which is only the
// default when the item carries none. The default itself is under Definition.
function start() {
  return h("span", { class: "start", title: "the item's own value, or the default if it has none" }, "<value>");
}

// line writes one effect's rules: the starting number, then every rule in the
// order dogma applies them. An assignment throws away everything before it, so
// the line starts again from there.
function line(terms) {
  let parts = [start()];

  for (const at of terms) {
    const name = attributeLink(at.modifying, "ref" + (at.ours ? " ours" : ""));

    if (assigns(at.op)) {
      parts = [name];
      continue;
    }
    parts.push(h("span", { class: "op" }, SIGN[at.op] || at.op), name);
    if (at.op === "percent") parts.push(h("span", { class: "op" }, "%"));
  }
  return parts;
}

// onto writes a rule that lands on another item. It can never be a number on
// a page about one item at a time, so it is said in words instead.
function onto(at) {
  return h("div", { class: "away" },
    h("span", { class: "op" }, SIGN[at.op] || at.op),
    attributeLink(at.modifying, "ref" + (at.ours ? " ours" : "")),
    h("span", { class: "onto" }, "onto ", landsOn(at.domain)));
}

// ------------------------------------------------------------------- paint

function paint() {
  const body = sheet;
  const suffix = unit(body.unitID);

  replace(host,
    h("div", { class: "head" },
      h("h1", {}, body.name, suffix ? h("span", { class: "unit" }, suffix) : null),
      badge(body.kind),
      body.displayName ? h("span", { class: "tiny" }, body.displayName) : null,
      h("span", { class: "spacer" }),
      body.definition
        ? h("button", { class: "bare danger", on: { click: () => removeAttribute(body.name) } }, "Delete")
        : null),

    warnings(body),
    definition(body),
    calculation(body),
    table(body),
    feeds(body));
}

function card(name, onEdit, inner, note) {
  return h("section", { class: "card" },
    h("div", { class: "card-head" },
      h("h3", {}, name),
      note ? h("span", { class: "tiny" }, note) : null,
      h("span", { class: "spacer" }),
      onEdit ? h("button", { class: "bare", on: { click: onEdit } }, "Edit") : null),
    h("div", { class: "card-body" }, inner));
}

function open(what) {
  editing = editing === what ? "" : what;
  paint();
}

// ---------------------------------------------------------------- the facts

function definition(body) {
  if (editing === "definition") return card("Definition", null, definitionForm(body));

  const marks = [
    mark("=", `default ${num(body.defaultValue)}`),
    mark(body.highIsGood ? "↑" : "↓", body.highIsGood ? "higher is better" : "lower is better"),
    mark(body.stackable ? "✓" : "✕", body.stackable ? "no stacking penalty" : "stacking penalty"),
  ];
  return card("Definition", body.definition ? () => open("definition") : null,
    h("div", { class: "marks" }, marks));
}

function mark(glyph, label) {
  return h("span", { class: "mark" }, h("i", {}, glyph), label);
}

function warnings(body) {
  if (!body.flags.length) return null;
  return h("div", { class: "warnings" }, body.flags.map((flag) =>
    h("p", { class: rank(flag.kind) }, flag.why.charAt(0).toUpperCase() + flag.why.slice(1) + ".")));
}

const RANKS = {
  broken: "bad", negative: "bad",
  zero: "warn", flat: "warn", default: "warn", spread: "warn",
  orphan: "warn", input: "info", onlyOnFits: "info",
};
const rank = (kind) => RANKS[kind] || "info";

// -------------------------------------------------------- the calculation

// Who an effect applies to and its rules are one block, edited together.
function calculation(body) {
  const blocks = body.appliesTo.map((entry, i) => block(body, entry, i));
  const note = body.appliesTo.length > 1
    ? "items matching several of these get all of them, in dogma order"
    : "";

  return card("Calculation", null, [
    blocks.length ? blocks : h("p", { class: "none" },
      `No rules set this, so it always stays at its default of ${num(body.defaultValue)}.`),
    shared(body),
  ], note);
}

function block(body, entry, i) {
  const terms = body.formula.filter((at) => at.effect === entry.effect);
  const here = terms.filter((at) => !at.elsewhere);
  const away = terms.filter((at) => at.elsewhere);
  const place = entry.where || terms.find((at) => at.where)?.where.place;
  const key = `rule:${i}`;

  const count = entry.matched ?? entry.count;
  const kind = entry.ours ? "new" : terms.some((at) => at.ours) ? "patched" : "";

  if (editing === key) {
    return h("div", { class: "block editing" },
      h("div", { class: "who" },
        h("span", { class: "lead-in" }, place.section === "addTo" ? "Rules added to" : "Edit rule"),
        place.section === "addTo" ? h("code", {}, entry.effect) : null,
        h("span", { class: "tiny" }, entry.category),
        badge(kind)),
      ruleForm(entry, place));
  }

  const head = h("div", { class: "who" },
    h("span", { class: "lead-in" }, entry.on ? "Applies to" : "Items with"),
    h("code", {}, entry.on || entry.effect),
    h("span", { class: "n" }, items(count)),
    h("span", { class: "tiny" }, entry.category),
    badge(kind),
    h("span", { class: "spacer" }),
    place ? h("button", { class: "bare", on: { click: () => open(key) } }, "Edit") : null);

  return h("div", { class: "block" },
    head,
    here.length
      ? h("div", { class: "formula" },
        h("span", { class: "lhs" }, body.name), h("span", { class: "eq" }, "="), line(here))
      : null,
    away.map(onto));
}

// shared warns that an effect on this page moves other numbers too, so a sum
// above cannot be changed on its own.
function shared(body) {
  if (!body.alsoWrites.length) return null;
  return h("p", { class: "why" }, "These effects also change ",
    join(body.alsoWrites.map((entry) => attributeLink(entry.name))), ", so editing them affects those too.");
}

// ------------------------------------------------------------- the numbers

function table(body) {
  if (!body.rows.length) return null;
  const at = route();
  const range = body.count
    ? `${items(body.count)} · ${num(body.min)} to ${num(body.max)} · median ${num(body.median)}`
    : "";

  return h("section", { class: "card" },
    h("div", { class: "card-head" },
      h("h3", {}, "Items"),
      range ? h("span", { class: "tiny" }, range) : null,
      body.broken ? h("span", { class: "tag bad" }, `${body.broken.toLocaleString()} broken`) : null,
      h("span", { class: "spacer" }),
      at.all ? null : h("span", { class: "tiny" }, `sample of ${body.rows.length}`),
      body.total > body.rows.length || at.all
        ? h("button", { class: "bare", on: { click: () => go({ all: at.all ? "" : "1" }) } },
          at.all ? "Show sample" : "Show all")
        : null),
    h("div", { class: "scroll" }, h("table", { class: "rows" },
      h("thead", {}, h("tr", {},
        h("th", {}, "Item"),
        body.inputs.map((input) => h("th", { class: "n" }, attributeLink(input.name))),
        h("th", { class: "n out" }, body.name))),
      h("tbody", {}, body.rows.map((row) => h("tr", { class: isBroken(row.value) ? "bad" : "" },
        h("td", {},
          h("a", { on: { click: () => go({ v: "item", n: String(row.id) }) } }, row.name),
          h("span", { class: "tiny" }, " ", row.group)),
        row.in.map((value) => h("td", { class: "n dim" }, num(value))),
        h("td", { class: "n out" }, num(row.value))))))));
}

// feeds is what reads this attribute.
function feeds(body) {
  if (!body.readBy.length) return null;
  return card("Used by", null,
    h("div", { class: "chips" }, body.readBy.map((entry) =>
      attributeLink(entry.name, "chip" + (entry.ours ? " ours" : "")))),
    "attributes calculated from this one");
}

function join(nodes) {
  const out = [];
  nodes.forEach((node, i) => {
    if (i) out.push(i === nodes.length - 1 ? " and " : ", ");
    out.push(node);
  });
  return out;
}

// ------------------------------------------------------------------- forms

// Every form works the same way: the declaration comes back from the server
// with the page, a copy of it is edited here, and the copy is put back whole.
// Nothing is ever written as text.

async function save(where, fields) {
  const path = where.index < 0
    ? `/api/patch/${encodeURIComponent(where.patch)}/define/${where.section}`
    : `/api/patch/${encodeURIComponent(where.patch)}/${where.section}/${where.index}`;
  try {
    await put(path, { fields });
  } catch (err) {
    return alarm(err.message);
  }
  alarm("");
  editing = "";
  await changed();
}

function buttons(onSave, ...extra) {
  return h("div", { class: "buttons" },
    extra,
    h("span", { class: "spacer" }),
    h("button", { on: { click: () => open("") } }, "Cancel"),
    h("button", { class: "go", on: { click: onSave } }, "Save"));
}

function field(label, control) {
  return h("label", { class: "field" }, h("span", {}, label), control);
}

function definitionForm(body) {
  const fields = structuredClone(body.definition.fields) || {};

  const value = h("input", { class: "num", value: String(fields.default ?? body.defaultValue) });
  const good = h("input", { type: "checkbox", checked: fields.highIsGood ?? body.highIsGood });
  const stack = h("input", { type: "checkbox", checked: fields.stackable ?? body.stackable });
  const shown = h("input", { value: fields.displayName || body.displayName || "" });
  const group = h("input", { value: fields.category || body.category || "" });

  return h("div", { class: "form" },
    body.definition.section === "change"
      ? h("p", { class: "tiny" }, "Only what you change is saved. Everything else keeps its original value.")
      : null,
    field("Default value", value),
    field("Higher is better", good),
    field("No stacking penalty", stack),
    field("Display name", shown),
    field("Category", group),
    buttons(() => {
      const number = Number(value.value);
      if (!value.value.trim() || !Number.isFinite(number)) return alarm("The default value has to be a number.");

      // A change block keeps only what it overrides, as the hint above promises.
      const set = (key, next, was) => {
        const overridden = fields[key] !== null && fields[key] !== undefined && fields[key] !== "";
        if (body.definition.section === "new" || overridden || next !== was) fields[key] = next;
      };
      set("default", number, body.defaultValue);
      set("highIsGood", good.checked, body.highIsGood);
      set("stackable", stack.checked, body.stackable);
      set("displayName", shown.value, body.displayName || "");
      set("category", group.value, body.category || "");
      save(body.definition, fields);
    }));
}

// ruleForm edits one effect whole: who it applies to and its rules. A rule
// added to an effect of CCP's has no filter of its own, so that part is left out.
function ruleForm(entry, place) {
  const fields = structuredClone(place.fields);
  const list = fields.rules || (fields.rules = []);
  const filter = place.section === "effects" ? filterBox(entry) : null;

  const rows = h("div", { class: "rules" });
  const draw = () => replace(rows, list.length
    ? list.map((at, i) => rule(at, list, i, draw))
    : h("p", { class: "none" }, "No rules yet."));
  draw();

  const addRule = h("button", {
    on: {
      click: () => {
        list.push({ op: "mul", by: "", domain: "", func: "", group: "", skill: "" });
        draw();
      },
    },
  }, "Add rule");

  return h("div", { class: "form" },
    filter ? h("div", { class: "part" }, h("h4", {}, "Applies to"), filter.node) : null,
    h("div", { class: "part" }, h("h4", {}, "Rules"), rows),
    buttons(() => {
      if (filter) fields.on = filter.value();
      save(place, fields);
    }, addRule));
}

// filterBox is the "on" expression with a live count of what it matches.
function filterBox(entry) {
  const box = h("input", { class: "expr", value: entry.on || "", placeholder: "isShip" });
  const count = h("span", { class: "count" }, items(entry.matched ?? 0));

  box.addEventListener("input", debounce(async () => {
    try {
      const body = await api(`/api/preview?on=${encodeURIComponent(box.value)}`);
      count.textContent = items(body.count);
      count.className = "count";
    } catch (err) {
      count.textContent = err.message;
      count.className = "count bad";
    }
  }, 200));

  const node = [
    h("div", { class: "exprrow" }, box, count),
    h("p", { class: "tiny hint" }, "A filter name like ", h("code", {}, "isShip"), ", or ",
      h("code", {}, "category(…)"), ", ", h("code", {}, "group(…)"), ", ",
      h("code", {}, "attribute(…)"), ", ", h("code", {}, "effect(…)"), ". Combine with ",
      h("code", {}, "and"), ", ", h("code", {}, "or"), ", ", h("code", {}, "not"), "."),
  ];
  return { node, value: () => box.value };
}

// Every rule in a file writes that file's attribute, so a rule only says the
// operation and what it reads.
function rule(entry, list, i, draw) {
  const op = h("select", {}, OPS.map(([name, sign, why]) =>
    h("option", { value: name, selected: entry.op === name }, `${sign} ${why}`)));
  op.addEventListener("change", () => { entry.op = op.value; });

  const by = nameBox("attribute", entry.by, (value) => { entry.by = value; });

  return h("div", { class: "rule" }, op, by,
    entry.domain && entry.domain !== "itemID"
      ? h("span", { class: "tinytag" }, `onto ${landsOn(entry.domain)}`)
      : null,
    h("span", { class: "spacer" }),
    h("button", {
      class: "bare danger", title: "Remove rule",
      on: { click: () => { list.splice(i, 1); draw(); } },
    }, "Remove"));
}

// nameBox is a plain input with the names the server knows hanging off it, so
// a name is picked rather than spelled.
function nameBox(kind, value, onChange) {
  const id = `names${Math.random().toString(36).slice(2)}`;
  const options = h("datalist", { id });
  const box = h("input", { value: value || "", list: id, placeholder: "Attribute" });

  const fetchNames = debounce(async () => {
    try {
      const body = await api(`/api/complete?kind=${kind}&q=${encodeURIComponent(box.value)}`);
      replace(options, (body.names || []).map((name) => h("option", { value: name })));
    } catch { /* a name that does not exist is caught on save */ }
  }, 150);

  box.addEventListener("input", () => { onChange(box.value); fetchNames(); });
  box.addEventListener("focus", fetchNames);
  return h("span", { class: "namebox" }, box, options);
}
