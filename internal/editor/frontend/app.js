// Popup logic. Backend calls go through window.go.editor.App (Wails bindings).
(function () {
  const $ = (id) => document.getElementById(id);
  const titleEl = $("title"), emojiEl = $("emoji"), listName = $("list-name"), dueEl = $("due");
  const notesEl = $("notes"), notesState = $("notes-state"), errorEl = $("error"), picker = $("picker");

  let catalog = { tags: [], lists: [], defaultList: "" };
  let state = { tag: "", list: "", notes: "" };
  let tagByKey = {}, tagById = {}, listByKey = {};
  let pickerMode = null; // "list" while the list picker is open

  const backend = () => (window.go && window.go.editor && window.go.editor.App) || null;

  function index() {
    tagByKey = {}; tagById = {}; listByKey = {};
    for (const t of catalog.tags) { tagByKey[t.key.toLowerCase()] = t; tagById[t.id] = t; }
    for (const l of catalog.lists) listByKey[l.key] = l;
  }

  function render() {
    const t = tagById[state.tag];
    emojiEl.textContent = t ? t.emoji : "＋";
    const l = listByKey[state.list] || listByKey[catalog.defaultList];
    listName.textContent = l ? l.title : state.list;
    notesState.textContent = state.notes ? "notes ✓" : "no notes";
  }

  function setTag(id) {
    state.tag = id;
    const t = tagById[id];
    if (t && t.list && !state.listChosen) state.list = t.list;
    render();
  }

  // "p pgwatch #345": a known tag key followed by a space becomes the tag.
  function consumeTagKey() {
    const v = titleEl.value;
    const m = /^(\S)\s(.*)$/s.exec(v);
    if (!m) return;
    const t = tagByKey[m[1].toLowerCase()];
    if (!t) return;
    titleEl.value = m[2];
    setTag(t.id);
  }

  function showError(msg) { errorEl.textContent = msg || ""; }

  function openPicker(mode) {
    pickerMode = mode;
    picker.innerHTML = "";
    const items = mode === "list" ? catalog.lists.map((l) => ({ k: l.key, label: l.title }))
      : catalog.tags.map((t) => ({ k: t.key, label: t.emoji + " " + t.id }));
    for (const it of items) {
      const el = document.createElement("span");
      el.className = "opt";
      el.innerHTML = `<span class="k">${it.k}</span><span>${it.label}</span>`;
      el.onclick = () => pick(it.k);
      picker.appendChild(el);
    }
    picker.classList.remove("hidden");
  }

  function closePicker() { pickerMode = null; picker.classList.add("hidden"); titleEl.focus(); }

  function pick(k) {
    if (pickerMode === "list" && listByKey[k]) { state.list = k; state.listChosen = true; }
    else if (pickerMode === "tag" && tagByKey[k.toLowerCase()]) setTag(tagByKey[k.toLowerCase()].id);
    else return;
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
    if (pickerMode) {
      if (ev.key === "Escape") { closePicker(); ev.preventDefault(); return; }
      if (ev.key.length === 1) { pick(ev.key); ev.preventDefault(); }
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
  titleEl.addEventListener("input", consumeTagKey);
  notesEl.addEventListener("input", () => { state.notes = notesEl.value; render(); });
  $("list-chip").onclick = () => openPicker("list");
  $("due-chip").onclick = () => dueEl.focus();
  $("notes-chip").onclick = () => { notesEl.classList.toggle("hidden"); notesEl.focus(); };
  emojiEl.onclick = () => openPicker("tag");

  async function init() {
    const b = backend();
    if (b) { catalog = await b.Catalog(); index(); render(); }
    if (window.runtime && window.runtime.EventsOn) {
      window.runtime.EventsOn("draft", async (d) => {
        const b2 = backend();
        if (b2) { catalog = await b2.Catalog(); index(); }
        load(d);
      });
    }
  }
  init();
})();
