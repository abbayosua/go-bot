// Theme registry: the collection of swappable UI themes.
// Each entry is { id, name, css }. "claude" (from the mockup) is first and
// default; future themes only append here and ship web/themes/<id>.css.
export const THEMES = [
  { id: "claude", name: "Claude", css: "/themes/claude.css" },
  { id: "plain", name: "Plain", css: "/themes/plain.css" },
];

export const DEFAULT_THEME = "claude";
const STORAGE_KEY = "gobot_theme";

export function listThemes() {
  return THEMES.map((t) => ({ ...t }));
}

export function getTheme(id) {
  return THEMES.find((t) => t.id === id) || null;
}

export function resolveTheme(id) {
  return getTheme(id) ? id : DEFAULT_THEME;
}

// loadSaved returns the persisted theme id, falling back to default.
export function loadSaved(storage) {
  try {
    const raw = (storage || defaultStorage()).getItem(STORAGE_KEY) || "";
    return resolveTheme(raw.trim());
  } catch (error) {
    return DEFAULT_THEME;
  }
}

// save persists the theme id; unknown ids are ignored.
export function save(storage, id) {
  if (!getTheme(id)) return false;
  try {
    (storage || defaultStorage()).setItem(STORAGE_KEY, id);
    return true;
  } catch (error) {
    return false;
  }
}

// apply sets the data-theme attribute and swaps the theme stylesheet link.
// Returns the resolved theme id.
export function apply(documentRef, id) {
  const resolved = resolveTheme(id);
  const doc = documentRef || (typeof document !== "undefined" ? document : null);
  if (!doc) return resolved;
  doc.documentElement.dataset.theme = resolved;
  let link = doc.querySelector("link[data-theme-sheet]");
  if (!link) {
    link = doc.createElement("link");
    link.rel = "stylesheet";
    link.setAttribute("data-theme-sheet", "");
    doc.head.appendChild(link);
  }
  const entry = getTheme(resolved);
  link.href = entry.css;
  return resolved;
}

function defaultStorage() {
  if (typeof localStorage !== "undefined") return localStorage;
  return { getItem: () => null, setItem: () => {} };
}
