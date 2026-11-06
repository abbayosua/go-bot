// Minimal i18n: English + Indonesian, no framework. Strings live in STR, the
// active language in localStorage, and t() is the only lookup. Ship the two
// languages we can stand behind; adding one is one object.
const STR = {
  en: {
    "spaces.all": "All spaces",
    "run.stop": "Stop",
    "approval.allow": "Allow",
    "approval.deny": "Deny",
    "memory.empty": "Nothing remembered yet. The bot stores facts with the remember tool.",
    "memory.forget": "Forget",
    "memory.scope.you": "this bot",
    "memory.scope.shared": "shared with every bot",
    "notes.empty": "No working notes yet. Jot one above, or the bot stores them with the note tool.",
    "skills.empty": "No skills yet.",
    "rules.empty": "No rules. Everything asks, except reads.",
    "instructions.empty": "No instructions yet. Saved versions appear here.",
    "routines.empty": "No routines yet.",
    "search.empty": "No matches.",
    "search.pick": "Pick a backup file first.",
    "plugins.empty": "No plugins. Install a folder from plugins/ to add tools to this bot only.",
    "admin.title": "Admin",
    "admin.language": "Language",
  },
  id: {
    "spaces.all": "Semua space",
    "run.stop": "Berhenti",
    "approval.allow": "Izinkan",
    "approval.deny": "Tolak",
    "memory.empty": "Belum ada yang diingat. Bot menyimpan fakta dengan tool remember.",
    "memory.forget": "Lupakan",
    "memory.scope.you": "bot ini",
    "memory.scope.shared": "dibagikan ke semua bot",
    "notes.empty": "Belum ada catatan. Tulis di atas, atau bot menyimpannya dengan tool note.",
    "skills.empty": "Belum ada skill.",
    "rules.empty": "Belum ada aturan. Semua bertanya, kecuali bacaan.",
    "instructions.empty": "Belum ada instruksi. Versi tersimpan muncul di sini.",
    "routines.empty": "Belum ada rutinitas.",
    "search.empty": "Tidak ada hasil.",
    "search.pick": "Pilih file backup dulu.",
    "plugins.empty": "Belum ada plugin. Instal folder dari plugins/ untuk menambah tool ke bot ini saja.",
    "admin.title": "Admin",
    "admin.language": "Bahasa",
  },
};

let lang = localStorage.getItem("gobot_lang") || "en";
if (!STR[lang]) lang = "en";

export function t(key) {
  return (STR[lang] && STR[lang][key]) || STR.en[key] || key;
}

export function setLang(next) {
  if (!STR[next]) return;
  lang = next;
  localStorage.setItem("gobot_lang", next);
}

export function currentLang() {
  return lang;
}
