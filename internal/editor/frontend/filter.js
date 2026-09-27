// Filtering for the open-tasks popup, kept free of the Wails runtime so it
// can be tested on its own.

// Emoji are compared without variation selectors: "🏗️" and "🏗" are one tag.
const bare = (s) => s.replace(/️/g, "");

// tagIndex maps every tag key and alias (lower case) to its emoji.
export function tagIndex(tags) {
  const idx = {};
  for (const t of tags || []) {
    for (const k of [t.key, ...(t.aliases || [])]) if (k) idx[k.toLowerCase()] = bare(t.emoji);
  }
  return idx;
}

// matches reports whether a row passes the filter text: every word must
// match, a tag word by the title's leading emoji, any other word as text.
export function matches(row, text, tags) {
  const title = bare(row.title);
  const lower = title.toLowerCase();
  for (const w of text.trim().toLowerCase().split(/\s+/)) {
    if (!w) continue;
    const emoji = tags[w];
    if (emoji ? !title.startsWith(emoji) : !lower.includes(w)) return false;
  }
  return true;
}

// inView reports whether a row belongs to the "today" or "all" view.
export function inView(row, view) {
  return view === "all" || row.when === "overdue" || row.when === "today";
}
