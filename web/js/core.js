"use strict";
// Состояние, даты, время, срочность, API и выборки задач.

/* ================= состояние ================= */
const S = {
  tasks: [], lists: [], habits: [], notes: {},
  view: "today",
  selected: null,
  search: "",
  sort: safeGet("sort") || "manual",
  calMonth: firstOfMonth(new Date()),
  calPick: null,
  schedDay: null,
  enter: new Set(), // задачи, которым при следующей отрисовке нужна анимация появления
  boardList: "",
};

const PALETTE = ["#7c6cf2", "#2fb59a", "#f2a33c", "#e2534a", "#4a9df0", "#d45fb8", "#8a9a5b", "#5bc0de"];
const PRI_NAMES = ["Нет", "Низкий", "Средний", "Высокий"];
const REPEAT_NAMES = { "": "Не повторять", daily: "Каждый день", weekdays: "По будням", weekly: "Каждую неделю", monthly: "Каждый месяц", yearly: "Каждый год" };
const CARRY_NAMES = { "": "Остаётся просроченной", move: "Переносится на сегодня", shift: "Сдвигается весь план" };
function seriesOf(t) { return (S.series || []).find(x => x.id === t.series); }
function carryHint(t) {
  const sr = seriesOf(t);
  const days = ["вс", "пн", "вт", "ср", "чт", "пт", "сб"];
  if (t.carry === "shift" && sr) return `План «${esc(sr.name)}»: ${[1, 2, 3, 4, 5, 6, 0].map(d => days[d] + " " + sr.perDay[d]).join(", ")}. Пропущенная задача встаёт на сегодня, остальные сдвигаются по сетке.`;
  if (t.carry === "move") return "Каждый новый день невыполненная задача сама переезжает на сегодня.";
  if (sr) return `Задача из плана «${esc(sr.name)}» — можно включить сдвиг всего плана.`;
  return "Сдвиг плана доступен для задач из плана (например, курса).";
}
const VIEW_KEYS = ["today", "upcoming", "inbox", "all", "board", "schedule", "calendar", "matrix", "habits"];

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

function safeGet(k) { try { return localStorage.getItem(k); } catch { return null; } }
function safeSet(k, v) { try { localStorage.setItem(k, v); } catch { /* приватный режим */ } }

/* ================= даты ================= */
function firstOfMonth(d) { return new Date(d.getFullYear(), d.getMonth(), 1); }
const fmtShort = new Intl.DateTimeFormat("ru", { day: "numeric", month: "short" });
const fmtLong = new Intl.DateTimeFormat("ru", { weekday: "long", day: "numeric", month: "long" });
const fmtDow = new Intl.DateTimeFormat("ru", { weekday: "short" });
const fmtWeekday = new Intl.DateTimeFormat("ru", { weekday: "long" });

function humanDate(s) {
  const n = diffDays(today(), s);
  if (n === 0) return "Сегодня";
  if (n === 1) return "Завтра";
  if (n === -1) return "Вчера";
  if (n > 1 && n < 7) return cap(fmtWeekday.format(parseYmd(s)));
  const d = parseYmd(s);
  return fmtShort.format(d).replace(".", "") + (d.getFullYear() !== new Date().getFullYear() ? " " + d.getFullYear() : "");
}
function cap(s) { return s.charAt(0).toUpperCase() + s.slice(1); }

function isOverdue(t) {
  if (t.done || !t.due) return false;
  const td = today();
  if (t.due < td) return true;
  if (t.due === td && t.dueTime) {
    const now = new Date();
    return t.dueTime < `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
  }
  return false;
}
function doneOn(t, day) { return t.done && t.completedAt && ymd(new Date(t.completedAt)) === day; }

/* ================= время (идея из Super Productivity) ================= */
function fmtMin(m) {
  m = Math.round(m);
  const h = Math.floor(m / 60), r = m % 60;
  return h ? `${h}ч${r ? " " + r + "м" : ""}` : `${r}м`;
}
function fmtClock(sec) {
  sec = Math.floor(sec);
  const h = Math.floor(sec / 3600), m = Math.floor(sec % 3600 / 60), s = sec % 60;
  const p = n => String(n).padStart(2, "0");
  return h ? `${h}:${p(m)}:${p(s)}` : `${p(m)}:${p(s)}`;
}
function spentNow(t) {
  return (t.spent || 0) + (t.timerStart ? Math.max(0, (Date.now() - new Date(t.timerStart)) / 1000) : 0);
}
function dayEnd() { return safeGet("dayEnd") || "22:00"; }

// загрузка дня: сколько осталось по оценкам и сколько свободного времени до конца дня
function dayLoad() {
  const td = today();
  const tasks = active().filter(t => t.due && t.due <= td);
  const need = tasks.reduce((s, t) => s + Math.max(0, (t.estimate || 0) - spentNow(t) / 60), 0);
  const [eh, em] = dayEnd().split(":").map(Number);
  const now = new Date();
  const free = Math.max(0, eh * 60 + em - (now.getHours() * 60 + now.getMinutes()));
  return { need, free, noEst: tasks.filter(t => !t.estimate).length };
}

function loadBar() {
  const { need, free, noEst } = dayLoad();
  const over = need > free;
  const p = free ? Math.min(100, need / free * 100) : need ? 100 : 0;
  return `<div class="load ${over ? "over" : ""}">
    <span>${icon("hourglass")} Запланировано <b>${fmtMin(need)}</b></span>
    <div class="track"><i style="width:${p}%"></i></div>
    <span>свободно до <button class="chip" data-act="dayEnd" title="Изменить конец дня">${dayEnd()}</button>: <b>${fmtMin(free)}</b></span>
    ${over ? `<b>${icon("zap")} Перегруз на ${fmtMin(need - free)}</b>` : ""}
    ${noEst ? `<span style="color:var(--ink-3)">${noEst} без оценки</span>` : ""}
  </div>`;
}

/* ================= срочность (формула Taskwarrior) ================= */
function urgency(t) {
  if (t.done) return 0;
  let u = [0, 1.8, 3.9, 6.0][t.priority] || 0;
  if (t.due) {
    const over = diffDays(t.due, today()); // дней просрочки (отрицательно — до срока)
    const f = over >= 7 ? 1 : over >= -14 ? (over + 14) * 0.8 / 21 + 0.2 : 0.2;
    u += 12 * f;
  }
  u += 2 * Math.min(1, (Date.now() - new Date(t.createdAt)) / 864e5 / 365);
  if (t.status === "doing" || t.timerStart) u += 4;
  if (t.tags.length) u += [0, 0.8, 0.9, 1][Math.min(3, t.tags.length)];
  if (t.notes) u += 1;
  if (t.pinned) u += 15; // как тег +next
  return Math.round(u * 10) / 10;
}

/* ================= API ================= */
async function api(method, path, body) {
  const res = await fetch("/api" + path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    let msg = res.statusText;
    try { msg = (await res.json()).error || msg; } catch { /* пусто */ }
    toast("Ошибка: " + msg);
    throw new Error(msg);
  }
  return res.status === 204 ? null : res.json();
}

async function load() {
  const d = await api("GET", "/state");
  S.tasks = d.tasks || [];
  S.lists = d.lists || [];
  S.habits = d.habits || [];
  S.notes = d.notes || {};
  S.series = d.series || [];
  S.pomos = d.pomos || {};
  if (!S.loaded) S.tasks.forEach(t => S.enter.add(t.id)); // анимация появления только при первой загрузке
  S.loaded = true;
  render();
}

/* ================= выборки ================= */
function listById(id) { return S.lists.find(l => l.id === id); }
function active() { return S.tasks.filter(t => !t.done); }

function sortTasks(arr) {
  const byOrder = (a, b) => a.order - b.order;
  const byDue = (a, b) => (a.due || "9999") .localeCompare(b.due || "9999") || (a.dueTime || "99").localeCompare(b.dueTime || "99");
  const cmp = {
    manual: byOrder,
    priority: (a, b) => b.priority - a.priority || byDue(a, b) || byOrder(a, b),
    due: (a, b) => byDue(a, b) || b.priority - a.priority || byOrder(a, b),
    smart: (a, b) => urgency(b) - urgency(a) || byOrder(a, b),
  }[S.sort] || byOrder;
  return [...arr].sort((a, b) => (b.pinned - a.pinned) || cmp(a, b));
}

function matchesSearch(t, q) {
  q = q.toLowerCase();
  return t.title.toLowerCase().includes(q) || (t.notes || "").toLowerCase().includes(q) ||
    t.tags.some(g => g.includes(q.replace(/^#/, ""))) || t.subtasks.some(s => s.title.toLowerCase().includes(q));
}

function counts() {
  const td = today(), a = active();
  return {
    today: a.filter(t => t.due && t.due <= td).length,
    upcoming: a.filter(t => t.due && t.due > td && t.due <= addDays(td, 7)).length,
    inbox: a.filter(t => !t.listId).length,
    all: a.length,
    doing: a.filter(t => t.status === "doing").length,
    overdue: a.filter(isOverdue).length,
  };
}
