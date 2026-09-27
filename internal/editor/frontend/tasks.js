// Open-tasks popup: arrows move, Space checks, typing filters, Enter opens
// the task's link, Tab switches today/all, Esc closes.
import { Call, Events } from "/wails/runtime.js";
import { tagIndex, matches, inView } from "./filter.js";

const svc = "github.com/pashagolub/pagotask/internal/editor.App.";
const api = {
  Catalog: () => Call.ByName(svc + "Catalog"),
  Tasks: () => Call.ByName(svc + "Tasks"),
  Toggle: (id, done) => Call.ByName(svc + "Toggle", id, done),
  OpenLink: (url) => Call.ByName(svc + "OpenLink", url),
  CloseTasks: () => Call.ByName(svc + "CloseTasks"),
};

(function () {
  const $ = (id) => document.getElementById(id);
  const filterEl = $("filter"), rowsEl = $("rows"), viewEl = $("view"), noteEl = $("note");

  let tags = {};
  let rows = [];            // from the backend
  let backendNote = "";
  let touched = new Map();  // id -> row checked or unchecked since the popup opened
  let view = "today";
  let selID = "";
  let lastTyped = false;    // the last key went into the filter
  let flash = "";           // a one-off message, e.g. "no link"

  // Rows checked here stay visible, struck through, until the popup closes;
  // rows checked in an earlier opening that Google has not confirmed are hidden.
  function merged() {
    const seen = new Set();
    const out = [];
    for (const r of rows) {
      seen.add(r.id);
      if (touched.has(r.id)) out.push({ ...r, done: touched.get(r.id).done });
      else if (!r.done) out.push(r);
    }
    for (const [id, r] of touched) if (!seen.has(id)) out.push(r);
    return out;
  }

  function visible() {
    return merged().filter((r) => inView(r, view) && matches(r, filterEl.value, tags));
  }

  function dueLabel(r) {
    if (!r.due) return "";
    if (r.when === "today") return "today";
    const [y, m, d] = r.due.split("-").map(Number);
    const dt = new Date(y, m - 1, d);
    const opts = { month: "short", day: "numeric" };
    if (y !== new Date().getFullYear()) opts.year = "numeric";
    return dt.toLocaleDateString(undefined, opts);
  }

  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  function render() {
    const vis = visible();
    if (!vis.some((r) => r.id === selID)) selID = vis.length ? vis[0].id : "";
    rowsEl.innerHTML = "";
    for (const r of vis) {
      const li = el("li", [r.id === selID ? "sel" : "", r.done ? "done" : "", r.syncing ? "syncing" : ""].join(" ").trim());
      const box = el("span", "box", r.done ? "☑" : "☐");
      box.onclick = (ev) => { ev.stopPropagation(); selID = r.id; toggle(); };
      const t = el("span", "t", r.title);
      if (r.parent) t.appendChild(el("span", "parent", "↳ " + r.parent));
      if (r.syncing) t.title = "Not in Google yet";
      li.append(box, t, el("span", "badge", r.listTitle), el("span", "due " + (r.when || ""), dueLabel(r)));
      li.onclick = () => { selID = r.id; render(); filterEl.focus(); };
      li.ondblclick = () => { selID = r.id; openLink(); };
      rowsEl.appendChild(li);
      if (r.id === selID) requestAnimationFrame(() => li.scrollIntoView({ block: "nearest" }));
    }
    if (!vis.length) {
      const msg = filterEl.value.trim() ? "Nothing matches"
        : view === "today" ? "Nothing due today. Tab shows all open tasks." : "No open tasks";
      rowsEl.appendChild(el("li", "empty", msg));
    }
    const all = merged();
    const n = all.filter((r) => inView(r, view)).length;
    viewEl.textContent = (view === "today" ? "Today" : "All") + " · " + n;
    noteEl.textContent = flash || backendNote;
  }

  function move(delta) {
    const vis = visible();
    if (!vis.length) return;
    let i = vis.findIndex((r) => r.id === selID);
    i = Math.max(0, Math.min(vis.length - 1, (i < 0 ? 0 : i) + delta));
    selID = vis[i].id;
    render();
  }

  function selected() { return visible().find((r) => r.id === selID); }

  async function toggle() {
    const r = selected();
    if (!r) return;
    if (r.syncing) { flash = "Not in Google yet, try again in a moment"; render(); return; }
    const done = !r.done;
    touched.set(r.id, { ...r, done });
    flash = "";
    render();
    const err = await api.Toggle(r.id, done);
    if (err) {
      touched.set(r.id, { ...r, done: !done });
      flash = err;
      render();
    }
  }

  async function openLink() {
    const r = selected();
    if (!r) return;
    if (!r.link) { flash = "This task has no link"; render(); return; }
    const err = await api.OpenLink(r.link);
    if (err) { flash = err; render(); }
  }

  async function load() {
    const v = await api.Tasks();
    rows = v.rows || [];
    backendNote = v.note || "";
    render();
  }

  async function reset() {
    tags = tagIndex((await api.Catalog()).tags);
    touched = new Map();
    view = "today";
    selID = "";
    flash = "";
    lastTyped = false;
    filterEl.value = "";
    await load();
    filterEl.focus();
  }

  document.addEventListener("keydown", (ev) => {
    switch (ev.key) {
      case "Escape":
        ev.preventDefault(); api.CloseTasks(); return;
      case "Tab":
        ev.preventDefault(); view = view === "today" ? "all" : "today"; render(); return;
      case "ArrowDown":
        ev.preventDefault(); lastTyped = false; move(1); return;
      case "ArrowUp":
        ev.preventDefault(); lastTyped = false; move(-1); return;
      case "PageDown":
        ev.preventDefault(); lastTyped = false; move(10); return;
      case "PageUp":
        ev.preventDefault(); lastTyped = false; move(-10); return;
      case "Enter":
        ev.preventDefault(); openLink(); return;
      case " ":
        // Mid-word, Space separates filter words ("pr pgw"); otherwise,
        // and always right after the arrows, it checks the highlighted task.
        if (filterEl.value && lastTyped && !filterEl.value.endsWith(" ")) return;
        ev.preventDefault(); lastTyped = false; toggle(); return;
    }
  });
  filterEl.addEventListener("input", () => { lastTyped = true; flash = ""; selID = ""; render(); });

  Events.On("tasks-open", reset);
  Events.On("tasks-changed", load);
  reset();
})();
