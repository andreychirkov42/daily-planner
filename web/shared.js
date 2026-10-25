"use strict";
// Общие функции для основной страницы и мини-панели в трее.

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

/* ================= даты ================= */
function ymd(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
function parseYmd(s) { const [y, m, d] = s.split("-").map(Number); return new Date(y, m - 1, d); }
function today() { return ymd(new Date()); }
function addDays(s, n) { const d = parseYmd(s); d.setDate(d.getDate() + n); return ymd(d); }
function diffDays(a, b) { return Math.round((parseYmd(b) - parseYmd(a)) / 864e5); }

/* ================= быстрый ввод ================= */
const WEEKDAYS = [
  [/^(воскресенье|вс)$/, 0], [/^(понедельник|пн)$/, 1], [/^(вторник|вт)$/, 2], [/^(среду|среда|ср)$/, 3],
  [/^(четверг|чт)$/, 4], [/^(пятницу|пятница|пт)$/, 5], [/^(субботу|суббота|сб)$/, 6],
];

function parseQuick(raw, lists = []) {
  const out = { title: "", due: "", dueTime: "", priority: 0, tags: [], listId: "", repeat: "" };
  let s = " " + raw + " ";
  const take = (re, fn) => { s = s.replace(re, (...m) => (fn(m) === false ? m[0] : " ")); };
  const end = "(?=\\s)";

  take(/\s#([\p{L}\p{N}_-]+)/gu, m => { out.tags.push(m[1].toLowerCase()); });
  take(/\s!([1-3])(?=\s)/g, m => { out.priority = +m[1]; });
  take(/\s@([\p{L}\p{N}_-]+)/gu, m => {
    const l = lists.find(l => l.name.toLowerCase().startsWith(m[1].toLowerCase()));
    if (!l) return false;
    out.listId = l.id;
  });

  const repeats = [
    ["(каждый\\s+день|ежедневно)", "daily"], ["(по\\s+будням)", "weekdays"],
    ["(каждую\\s+неделю|еженедельно)", "weekly"], ["(каждый\\s+месяц|ежемесячно)", "monthly"],
    ["(каждый\\s+год|ежегодно)", "yearly"],
  ];
  for (const [re, v] of repeats) take(new RegExp("\\s" + re + end, "iu"), () => { out.repeat = v; });

  take(/\s(?:в\s+)?([01]?\d|2[0-3]):([0-5]\d)(?=\s)/u, m => { out.dueTime = `${m[1].padStart(2, "0")}:${m[2]}`; });

  const td = today();
  take(new RegExp("\\s(сегодня)" + end, "iu"), () => { out.due = td; });
  take(new RegExp("\\s(послезавтра)" + end, "iu"), () => { out.due = addDays(td, 2); });
  take(new RegExp("\\s(завтра)" + end, "iu"), () => { out.due = addDays(td, 1); });
  take(new RegExp("\\sчерез\\s+(\\d+)\\s+(дн[яейь]+|день)" + end, "iu"), m => { out.due = addDays(td, +m[1]); });
  take(new RegExp("\\sчерез\\s+неделю" + end, "iu"), () => { out.due = addDays(td, 7); });
  take(/\s(\d{1,2})\.(\d{1,2})(?:\.(\d{2,4}))?(?=\s)/u, m => {
    const now = new Date();
    let y = m[3] ? +m[3] : now.getFullYear();
    if (y < 100) y += 2000;
    const d = new Date(y, +m[2] - 1, +m[1]);
    if (d.getMonth() !== +m[2] - 1) return false;
    if (!m[3] && ymd(d) < td) d.setFullYear(y + 1);
    out.due = ymd(d);
  });
  take(/\s(?:во?\s+)?([\p{L}]+)(?=\s)/gu, m => {
    if (out.due) return false;
    const w = WEEKDAYS.find(([re]) => re.test(m[1].toLowerCase()));
    if (!w) return false;
    let n = (w[1] - new Date().getDay() + 7) % 7;
    if (n === 0) n = 7;
    out.due = addDays(td, n);
  });

  // время без даты — значит сегодня
  if (out.dueTime && !out.due) out.due = td;
  out.title = s.replace(/\s+/g, " ").trim();
  return out;
}

