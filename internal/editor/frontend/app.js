// Popup logic. Backend calls go through the Wails v3 runtime, which the app
// serves at /wails/runtime.js.
import { Call, Events, Window } from "/wails/runtime.js";
import { tagIndex, recentMatches } from "./filter.js";

const svc = "github.com/pashagolub/pagotask/internal/editor.App.";
const api = {
  Catalog: () => Call.ByName(svc + "Catalog"),
  Current: () => Call.ByName(svc + "Current"),
  Save: (d) => Call.ByName(svc + "Save", d),
  Cancel: () => Call.ByName(svc + "Cancel"),
};

(function () {
  const $ = (id) => document.getElementById(id);
  const titleEl = $("title"), emojiEl = $("emoji"), listName = $("list-name"), dueEl = $("due");
  const hintEl = $("tag-hint"), recentEl = $("recent");
  const notesEl = $("notes"), notesState = $("notes-state"), errorEl = $("error"), picker = $("picker");

  let catalog = { tags: [], lists: [], defaultList: "", recent: [] };
  let state = { tag: "", list: "", notes: "" };
  let tagByKey = {}, tagById = {}, listByKey = {};
  let pickerMode = null; // "list" while the list picker is open
  let tagWords = {};

  const backend = () => api;

  function index() {
    tagByKey = {}; tagById = {}; listByKey = {};
    for (const t of catalog.tags) {
      tagById[t.id] = t;
      for (const k of [t.key, ...(t.aliases || [])]) tagByKey[k.toLowerCase()] = t;
    }
    for (const l of catalog.lists) listByKey[l.key] = l;
    tagWords = tagIndex(catalog.tags);
  }

  function render() {
    const t = tagById[state.tag];
    emojiEl.textContent = t ? t.emoji : "＋";
    const l = listByKey[state.list] || listByKey[catalog.defaultList];
    listName.textContent = l ? l.title : state.list;
    notesState.textContent = state.notes ? "notes ✓" : "no notes";
    renderHint();
    renderRecent();
  }

  // Recently used tasks under the tag hint. Shown at once when the popup
  // opens empty, otherwise after Down. Typing filters, Up/Down highlight,
  // Enter or a click fills tag, title and list.
  const recentLimit = 8;
  let recentShown = false, recentSel = -1, recentRows = [];
  function renderRecent() {
    recentRows = recentShown ? recentMatches(catalog.recent, state.tag, titleEl.value, tagWords, recentLimit) : [];
    if (recentSel >= recentRows.length) recentSel = recentRows.length - 1;
    recentEl.innerHTML = "";
    recentEl.classList.toggle("hidden", recentRows.length === 0);
    recentRows.forEach((e, i) => {
      const li = document.createElement("li");
      if (i === recentSel) li.className = "sel";
      const t = tagById[e.tag], l = listByKey[e.list];
      li.innerHTML = `<span class="t"></span><span class="l"></span>`;
      li.querySelector(".t").textContent = t ? `${t.emoji} ${e.title}` : e.title;
      li.querySelector(".l").textContent = l ? l.title : e.list;
      li.onclick = () => useRecent(e);
      recentEl.appendChild(li);
    });
  }
  function moveRecent(step) {
    if (!recentShown) { recentShown = true; recentSel = -1; }
    renderRecent();
    if (recentRows.length === 0) return;
    recentSel = Math.max(-1, Math.min(recentRows.length - 1, recentSel + step));
    renderRecent();
  }
  function hideRecent() { recentShown = false; recentSel = -1; renderRecent(); }
  function useRecent(e) {
    titleEl.value = e.title;
    state.tag = "";
    setTag(e.tag);
    if (listByKey[e.list]) { state.list = e.list; state.listChosen = true; }
    hideRecent();
    render();
    titleEl.focus();
  }

  // The tag hint under the title line. With no tag yet it lists the tags
  // whose key or alias starts with the word being typed (all of them while
  // the title is empty); Tab or a click picks the first. Once a tag is set
  // it just says how to change it.
  let hintMatches = [];
  function typedWord() {
    const v = titleEl.value;
    return /\s/.test(v) ? null : v.toLowerCase();
  }
  function renderHint() {
    hintEl.innerHTML = "";
    hintMatches = [];
    const t = tagById[state.tag];
    if (t) {
      hintEl.textContent = `${t.emoji} ${t.key}  ·  Ctrl+T to change the tag`;
      return;
    }
    const word = typedWord();
    for (const tag of catalog.tags) {
      const keys = [tag.key, ...(tag.aliases || [])];
      const hit = word === null || word === "" ? tag.key : keys.find((k) => k.toLowerCase().startsWith(word));
      if (hit) hintMatches.push({ tag, k: hit });
    }
    if (word && hintMatches.length === 0) {
      hintEl.textContent = "no tag starts with that; the task is saved without a tag";
      return;
    }
    hintMatches.forEach((m, i) => {
      const el = document.createElement("span");
      el.className = "opt" + (i === 0 && word ? " first" : "");
      el.title = [m.tag.key, ...(m.tag.aliases || [])].join(", ");
      el.innerHTML = `<span>${m.tag.emoji}</span><span class="k"></span>`;
      el.querySelector(".k").textContent = m.k;
      el.onclick = () => pickHint(m);
      hintEl.appendChild(el);
    });
  }
  function pickHint(m) {
    // A partly typed key is replaced by the tag; a real title stays.
    if (typedWord() !== null) titleEl.value = "";
    setTag(m.tag.id);
    titleEl.focus();
  }

  function setTag(id) {
    state.tag = id;
    const t = tagById[id];
    if (t && t.list && !state.listChosen) state.list = t.list;
    render();
  }

  // "pr pgwatch #345": a known tag key followed by a space becomes the tag.
  function consumeTagKey() {
    const v = titleEl.value;
    const m = /^(\S+)\s(.*)$/s.exec(v);
    if (!m) return;
    const t = tagByKey[m[1].toLowerCase()];
    if (!t) return;
    titleEl.value = m[2];
    setTag(t.id);
    if (pickerMode === "tag") closePicker();
  }

  // While the tag picker is open, the first word in the title filters it.
  function filterTagPicker() {
    // With a tag already set the title is real text, not a key: show all.
    const prefix = state.tag ? "" : (titleEl.value.split(/\s/)[0] || "").toLowerCase();
    for (const el of picker.children) el.classList.toggle("hidden", !el.dataset.k.startsWith(prefix));
  }

  function showError(msg) { errorEl.textContent = msg || ""; }

  function openPicker(mode) {
    pickerMode = mode;
    picker.innerHTML = "";
    const items = mode === "list" ? catalog.lists.map((l) => ({ k: l.key, label: l.title }))
      : catalog.tags.flatMap((t) => [t.key, ...(t.aliases || [])].map((k) => ({ k: k.toLowerCase(), label: t.emoji })));
    for (const it of items) {
      const el = document.createElement("span");
      el.className = "opt";
      el.dataset.k = it.k;
      el.innerHTML = `<span class="k">${it.k}</span><span>${it.label}</span>`;
      el.onclick = () => pick(it.k);
      picker.appendChild(el);
    }
    picker.classList.remove("hidden");
    if (mode === "tag") { filterTagPicker(); titleEl.focus(); }
  }

  function closePicker() { pickerMode = null; picker.classList.add("hidden"); titleEl.focus(); }

  function pick(k) {
    if (pickerMode === "list" && listByKey[k]) { state.list = k; state.listChosen = true; }
    else if (pickerMode === "tag" && tagByKey[k.toLowerCase()]) {
      // Without a tag the first word was the filter text; with one it is the title.
      if (!state.tag) titleEl.value = titleEl.value.replace(/^\S*\s?/, "");
      setTag(tagByKey[k.toLowerCase()].id);
    } else return;
    render();
    closePicker();
  }

  async function save() {
    consumeTagKey();
    const draft = {
      tag: state.tag,
      title: titleEl.value.trim(),
      list: state.list || catalog.defaultList,
      due: dueEl.value.trim() || "tod",
      notes: notesEl.value.trim(),
    };
    if (!draft.title) { showError("title is empty"); titleEl.focus(); return; }
    const b = backend();
    if (!b) { showError("no backend"); return; }
    const err = await b.Save(draft);
    if (err) showError(err);
  }

  function cancel() { const b = backend(); if (b) b.Cancel(); }

  function load(d) {
    showError("");
    state = { tag: d.tag || "", list: d.list || catalog.defaultList, notes: d.notes || "", listChosen: !!d.list };
    recentShown = !d.title && !d.tag;
    recentSel = -1;
    titleEl.value = d.title || "";
    dueEl.value = d.due || "tod";
    notesEl.value = d.notes || "";
    notesEl.classList.add("hidden");
    closePicker();
    render();
    titleEl.focus();
    titleEl.select();
  }

  document.addEventListener("keydown", (ev) => {
    if (pickerMode === "list") {
      if (ev.key === "Escape") { closePicker(); ev.preventDefault(); return; }
      if (ev.key.length === 1) { pick(ev.key); ev.preventDefault(); }
      return;
    }
    if (pickerMode === "tag") {
      // Typing goes to the title line and filters the list; Enter picks the
      // only remaining tag, Esc closes.
      if (ev.key === "Escape") { closePicker(); ev.preventDefault(); return; }
      if (ev.key === "Enter") {
        const left = [...picker.children].filter((el) => !el.classList.contains("hidden"));
        if (left.length === 1) { pick(left[0].dataset.k); ev.preventDefault(); return; }
      }
      if (ev.key !== "Enter") return;
    }
    if (ev.key === "Tab" && !ev.shiftKey && ev.target === titleEl && !state.tag && typedWord() && hintMatches.length) {
      ev.preventDefault(); pickHint(hintMatches[0]); return;
    }
    if (ev.key === "Backspace" && ev.target === titleEl && state.tag && titleEl.selectionStart === 0 && titleEl.selectionEnd === 0) {
      // Backspace at the start of the title removes the tag.
      ev.preventDefault(); state.tag = ""; render(); return;
    }
    if (ev.target === titleEl && (ev.key === "ArrowDown" || ev.key === "ArrowUp")) {
      ev.preventDefault(); moveRecent(ev.key === "ArrowDown" ? 1 : -1); return;
    }
    if (recentSel >= 0 && (ev.key === "Enter" || ev.key === "Escape")) {
      ev.preventDefault();
      if (ev.key === "Enter") useRecent(recentRows[recentSel]); else hideRecent();
      return;
    }
    if (ev.key === "Escape") { ev.preventDefault(); cancel(); return; }
    if (ev.key === "Enter" && !(ev.shiftKey && ev.target === notesEl)) { ev.preventDefault(); save(); return; }
    if (ev.ctrlKey && !ev.altKey) {
      const k = ev.key.toLowerCase();
      if (k === "l") { ev.preventDefault(); openPicker("list"); }
      else if (k === "t") { ev.preventDefault(); openPicker("tag"); }
      else if (k === "d") { ev.preventDefault(); dueEl.focus(); dueEl.select(); }
      else if (k === "n") { ev.preventDefault(); notesEl.classList.toggle("hidden"); if (!notesEl.classList.contains("hidden")) notesEl.focus(); else titleEl.focus(); }
    }
  });
  titleEl.addEventListener("input", () => {
    consumeTagKey(); if (pickerMode === "tag") filterTagPicker(); renderHint();
    recentSel = -1; renderRecent();
  });
  notesEl.addEventListener("input", () => { state.notes = notesEl.value; render(); });
  $("list-chip").onclick = () => openPicker("list");
  $("due-chip").onclick = () => dueEl.focus();
  $("notes-chip").onclick = () => { notesEl.classList.toggle("hidden"); notesEl.focus(); };
  emojiEl.onclick = () => openPicker("tag");

  // The window follows the content: the hint line and notes change its height.
  const popupEl = $("popup");
  let lastHeight = 0;
  function fitWindow() {
    const h = Math.ceil(popupEl.getBoundingClientRect().height);
    if (h > 0 && h !== lastHeight) {
      lastHeight = h;
      Window.SetSize(document.documentElement.clientWidth, h);
    }
  }
  new ResizeObserver(fitWindow).observe(popupEl);

  async function init() {
    Events.On("draft", async (ev) => {
      catalog = await api.Catalog(); index();
      load(ev.data);
    });
    catalog = await api.Catalog(); index(); render();
    // A draft opened before this page finished loading is still waiting.
    const d = await api.Current();
    if (d) load(d);
  }
  init();
})();
