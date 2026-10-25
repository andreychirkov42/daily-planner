"use strict";

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
    <span>⏳ Запланировано <b>${fmtMin(need)}</b></span>
    <div class="track"><i style="width:${p}%"></i></div>
    <span>свободно до <button class="chip" data-act="dayEnd" title="Изменить конец дня">${dayEnd()}</button>: <b>${fmtMin(free)}</b></span>
    ${over ? `<b>⚠ Перегруз на ${fmtMin(need - free)}</b>` : ""}
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

/* ================= быстрый ввод ================= */
function viewDefaults() {
  const v = S.view;
  if (v === "today") return { due: today() };
  if (v === "calendar" && S.calPick) return { due: S.calPick };
  if (v === "schedule") return { due: S.schedDay || today() };
  if (v.startsWith("list:")) return { listId: v.slice(5) };
  if (v.startsWith("tag:")) return { tags: [v.slice(4)] };
  return {};
}

function renderPreview() {
  const raw = $("#qaInput").value;
  const box = $("#qaPreview");
  if (!raw.trim()) { box.innerHTML = ""; return; }
  const p = parseQuick(raw, S.lists);
  const chips = [];
  if (p.due) chips.push(`<span class="chip today">📅 ${humanDate(p.due)}${p.dueTime ? " " + p.dueTime : ""}</span>`);
  if (p.priority) chips.push(`<span class="chip p${p.priority}" style="color:var(--pc)">⚑ ${PRI_NAMES[p.priority]}</span>`);
  if (p.repeat) chips.push(`<span class="chip">🔁 ${REPEAT_NAMES[p.repeat]}</span>`);
  if (p.listId) { const l = listById(p.listId); chips.push(`<span class="chip"><i class="dot" style="background:${l.color};margin:0"></i>${esc(l.name)}</span>`); }
  for (const t of p.tags) chips.push(`<span class="chip tag">#${esc(t)}</span>`);
  box.innerHTML = chips.join("");
}

async function quickAdd(e) {
  e.preventDefault();
  const input = $("#qaInput");
  const p = parseQuick(input.value, S.lists);
  if (!p.title) return;
  const def = viewDefaults();
  const task = {
    title: p.title, notes: "", done: false, subtasks: [], pinned: false,
    priority: p.priority,
    due: p.due || def.due || "",
    dueTime: p.dueTime,
    listId: p.listId || def.listId || "",
    tags: [...new Set([...(def.tags || []), ...p.tags])],
    repeat: p.repeat,
  };
  const created = await api("POST", "/tasks", task);
  S.tasks.push(created);
  S.enter.add(created.id);
  input.value = "";
  renderPreview();
  render();
  flash(created.id);
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

/* ================= рендер: каркас ================= */
function render() {
  renderNav();
  renderView();
  renderDetail();
  S.enter.clear();
}

function renderNav() {
  const c = counts();
  for (const el of $$("[data-count]")) {
    const k = el.dataset.count;
    el.textContent = c[k] || "";
    el.classList.toggle("hot", k === "today" && c.overdue > 0);
  }
  for (const b of $$("#nav button")) b.classList.toggle("active", b.dataset.view === S.view && !S.search);

  const a = active();
  $("#lists").innerHTML = S.lists.map(l => {
    const n = a.filter(t => t.listId === l.id).length;
    return `<button data-view="list:${l.id}" class="${S.view === "list:" + l.id && !S.search ? "active" : ""}">
      <i class="dot" style="background:${l.color}"></i>${esc(l.name)}
      <span class="count">${n || ""}</span><span class="list-del" data-del-list="${l.id}" title="Удалить список">✕</span></button>`;
  }).join("") + `<div id="newListSlot"></div>`;

  const tags = {};
  for (const t of a) for (const g of t.tags) tags[g] = (tags[g] || 0) + 1;
  $("#tagsCloud").innerHTML = Object.entries(tags).sort((x, y) => y[1] - x[1])
    .map(([g, n]) => `<button class="chip tag" data-view="tag:${esc(g)}">#${esc(g)} ${n}</button>`).join("");
}

const TITLES = {
  today: "Сегодня", upcoming: "Предстоящие", inbox: "Входящие", all: "Все задачи", calendar: "Календарь",
  board: "Доска", schedule: "Расписание дня",
  matrix: "Матрица Эйзенхауэра", habits: "Привычки", stats: "Статистика", completed: "Выполненные",
};

function setView(v) {
  S.view = v;
  S.search = "";
  $("#search").value = "";
  $("#sidebar").classList.remove("open");
  render();
  $(".main").scrollTop = 0;
}

function renderView() {
  const v = S.view;
  let title = TITLES[v] || "";
  if (v.startsWith("list:")) title = listById(v.slice(5))?.name || "Список";
  if (v.startsWith("tag:")) title = "#" + v.slice(4);
  if (S.search) title = "Поиск";
  $("#viewTitle").textContent = title;
  $("#eyebrow").textContent = greeting() + " · " + fmtLong.format(new Date());
  document.title = title + " · Ежедневник";

  const noAdd = ["habits", "stats", "completed", "matrix"].includes(v) && !S.search;
  $("#quickAdd").style.display = noAdd ? "none" : "";
  $("#qaPreview").style.display = noAdd ? "none" : "";

  const showSort = !["calendar", "habits", "stats", "matrix", "completed", "schedule"].includes(v) || S.search;
  let head = showSort ? `<select class="input btn-sm" id="sortSel" style="width:auto" title="Сортировка">
      <option value="manual">↕ Вручную</option><option value="smart">🧠 Умная (срочность)</option>
      <option value="priority">⚑ По приоритету</option><option value="due">📅 По сроку</option>
    </select>` : "";

  const el = $("#view");
  if (S.search) el.innerHTML = viewSearch();
  else if (v === "today") { el.innerHTML = viewToday(); head = progressRing() + `<button class="btn btn-sm btn-primary" data-act="plan" title="P">☀️ Спланировать день</button>` + head; }
  else if (v === "board") {
    el.innerHTML = viewBoard();
    head = `<select class="input btn-sm" id="boardListSel" style="width:auto"><option value="">Все списки</option>
      ${S.lists.map(l => `<option value="${l.id}" ${S.boardList === l.id ? "selected" : ""}>${esc(l.name)}</option>`).join("")}</select>` + head;
  }
  else if (v === "schedule") el.innerHTML = viewSchedule();
  else if (v === "upcoming") el.innerHTML = viewUpcoming();
  else if (v === "inbox") el.innerHTML = viewGrouped(active().filter(t => !t.listId), "📥", "Входящие пусты — всё разобрано");
  else if (v === "all") el.innerHTML = viewGrouped(active(), "🌿", "Нет активных задач");
  else if (v.startsWith("list:")) el.innerHTML = viewGrouped(active().filter(t => t.listId === v.slice(5)), "🗂️", "В этом списке пока пусто");
  else if (v.startsWith("tag:")) el.innerHTML = viewGrouped(active().filter(t => t.tags.includes(v.slice(4))), "🏷️", "Нет задач с этим тегом");
  else if (v === "completed") { el.innerHTML = viewCompleted(); head = `<button class="btn btn-sm btn-danger" id="clearDone">Очистить выполненные</button>`; }
  else if (v === "calendar") el.innerHTML = viewCalendar();
  else if (v === "matrix") el.innerHTML = viewMatrix();
  else if (v === "habits") el.innerHTML = viewHabits();
  else if (v === "stats") el.innerHTML = viewStats();

  $("#headRight").innerHTML = head;
  const sel = $("#sortSel");
  if (sel) sel.value = S.sort;
  bindDrag();
}

/* ================= рендер: задачи ================= */
const CHECK_SVG = `<svg viewBox="0 0 24 24"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>`;

function taskHTML(t, opts = {}) {
  const l = listById(t.listId);
  const meta = [];
  if (t.due) {
    const cls = isOverdue(t) ? "late" : t.due === today() ? "today" : t.due === addDays(today(), 1) ? "soon" : "";
    meta.push(`<span class="chip ${t.done ? "" : cls}">📅 ${humanDate(t.due)}${t.dueTime ? " · " + t.dueTime : ""}</span>`);
  }
  if (t.repeat) meta.push(`<span class="chip" title="${REPEAT_NAMES[t.repeat]}">🔁</span>`);
  if (t.carry && !t.done) meta.push(`<span class="chip" title="${CARRY_NAMES[t.carry]}">${t.carry === "shift" ? "⇉" : "↪"}</span>`);
  if (t.subtasks.length) meta.push(`<span class="chip">☑ ${t.subtasks.filter(s => s.done).length}/${t.subtasks.length}</span>`);
  if (l && !opts.hideList) meta.push(`<span class="chip"><i class="dot" style="background:${l.color};margin:0"></i>${esc(l.name)}</span>`);
  for (const g of t.tags) meta.push(`<button class="chip tag" data-view="tag:${esc(g)}">#${esc(g)}</button>`);
  if (t.notes) meta.push(`<span class="chip" title="Есть заметка">📝</span>`);
  if (t.timerStart) meta.unshift(`<span class="chip live">⏱ <span data-live="${t.id}">${fmtClock(spentNow(t))}</span></span>`);
  else if (t.estimate || t.spent) {
    const sp = Math.round((t.spent || 0) / 60);
    meta.push(`<span class="chip ${t.estimate && sp > t.estimate ? "over" : ""}" title="Затрачено / оценка">⏳ ${t.spent ? fmtMin(sp) + (t.estimate ? " / " : "") : ""}${t.estimate ? fmtMin(t.estimate) : ""}</span>`);
  }
  if (t.status === "doing" && !t.done && S.view !== "board") meta.unshift(`<span class="chip soon">🚧 В работе</span>`);
  if (S.sort === "smart" && !t.done) meta.push(`<span class="chip" title="Срочность по формуле Taskwarrior">🧠 ${urgency(t)}</span>`);
  const drag = opts.drag || (S.sort === "manual" && !t.done);

  return `<div class="task p${t.priority} ${S.enter.has(t.id) ? "enter" : ""} ${t.done ? "done" : ""} ${t.timerStart ? "running" : ""} ${S.selected === t.id ? "selected" : ""}" data-id="${t.id}"
      ${drag ? 'draggable="true"' : ""} style="--list-color:${l ? l.color : "transparent"}">
    <button class="check" data-act="toggle" title="Отметить">${CHECK_SVG}</button>
    <div class="t-body">
      <div class="t-title">${t.pinned ? '<span class="t-pin">📌 </span>' : ""}<span class="t-text">${esc(t.title)}</span></div>
      ${meta.length ? `<div class="t-meta">${meta.join("")}</div>` : ""}
    </div>
    <div class="t-actions">
      ${t.done ? "" : `<button class="icon-btn ${t.timerStart ? "on" : ""}" data-act="timer" title="Учёт времени">${t.timerStart ? "⏸" : "▶"}</button>`}
      <button class="icon-btn" data-act="pin" title="${t.pinned ? "Открепить" : "Закрепить"}">📌</button>
      <button class="icon-btn" data-act="del" title="Удалить">${TRASH_SVG}</button>
    </div>
  </div>`;
}

function group(title, tasks, cls = "", opts) {
  if (!tasks.length) return "";
  return `<div class="group"><div class="group-head ${cls}">${title} <span class="n">${tasks.length}</span></div>
    <div class="tasks">${sortTasks(tasks).map(t => taskHTML(t, opts)).join("")}</div></div>`;
}

function empty(icon, text) { return `<div class="empty"><span class="big">${icon}</span>${text}</div>`; }

function viewToday() {
  const td = today();
  const a = active();
  const overdue = a.filter(t => t.due && t.due < td);
  const now = a.filter(t => t.due === td);
  const done = S.tasks.filter(t => doneOn(t, td));
  let html = loadBar() + group("Просрочено", overdue, "overdue") + group("На сегодня", now) + group("Выполнено сегодня", done);
  if (!overdue.length && !now.length) html = empty("☀️", done.length ? "Всё сделано на сегодня! Отличная работа 🎉" : "Задач на сегодня нет. Добавь первую выше 👆") + html;
  html += `<div class="day-note"><h3>✍️ Заметка дня <span class="saved" id="noteSaved"></span></h3>
    <textarea id="dayNote" placeholder="Мысли, итоги дня, благодарности…">${esc(S.notes[td] || "")}</textarea></div>`;
  return html;
}

function progressRing() {
  const td = today();
  const done = S.tasks.filter(t => doneOn(t, td)).length;
  const left = active().filter(t => t.due && t.due <= td).length;
  const total = done + left;
  const p = total ? Math.round(done / total * 100) : 0;
  return `<div class="ring-wrap"><div class="ring" style="--p:${p}"><span>${p}%</span></div><div>${done} из ${total}<br>выполнено</div></div>`;
}

function viewUpcoming() {
  const td = today();
  const a = active();
  let html = "";
  for (let i = 0; i < 7; i++) {
    const d = addDays(td, i);
    const label = i === 0 ? "Сегодня" : i === 1 ? "Завтра" : cap(fmtWeekday.format(parseYmd(d)));
    html += group(`${label} · ${fmtShort.format(parseYmd(d))}`, a.filter(t => t.due === d));
  }
  html += group("Позже", a.filter(t => t.due > addDays(td, 6)));
  return html || empty("🗓️", "На ближайшую неделю ничего не запланировано");
}

function dateBuckets(tasks) {
  const td = today(), tm = addDays(td, 1), wk = addDays(td, 7);
  return [
    ["Просрочено", tasks.filter(t => t.due && t.due < td), "overdue"],
    ["Сегодня", tasks.filter(t => t.due === td)],
    ["Завтра", tasks.filter(t => t.due === tm)],
    ["На этой неделе", tasks.filter(t => t.due > tm && t.due <= wk)],
    ["Позже", tasks.filter(t => t.due > wk)],
    ["Без срока", tasks.filter(t => !t.due)],
  ];
}

function viewGrouped(tasks, icon, emptyText) {
  if (!tasks.length) return empty(icon, emptyText);
  const hideList = S.view.startsWith("list:");
  return dateBuckets(tasks).map(([t, arr, cls]) => group(t, arr, cls, { hideList })).join("");
}

function viewSearch() {
  const res = S.tasks.filter(t => matchesSearch(t, S.search));
  if (!res.length) return empty("🔍", `Ничего не найдено по запросу «${esc(S.search)}»`);
  return group("Активные", res.filter(t => !t.done)) + group("Выполненные", res.filter(t => t.done));
}

function viewCompleted() {
  const done = S.tasks.filter(t => t.done).sort((a, b) => (b.completedAt || "").localeCompare(a.completedAt || ""));
  if (!done.length) return empty("✅", "Здесь появятся выполненные задачи");
  const byDay = new Map();
  for (const t of done) {
    const d = t.completedAt ? ymd(new Date(t.completedAt)) : "—";
    if (!byDay.has(d)) byDay.set(d, []);
    byDay.get(d).push(t);
  }
  return [...byDay].map(([d, arr]) =>
    `<div class="group"><div class="group-head">${d === "—" ? "Без даты" : humanDate(d)} <span class="n">${arr.length}</span></div>
     <div class="tasks">${arr.map(t => taskHTML(t)).join("")}</div></div>`).join("");
}

/* ================= канбан (идея из Vikunja) ================= */
function viewBoard() {
  const inList = t => !S.boardList || t.listId === S.boardList;
  const a = active().filter(inList);
  const weekAgo = addDays(today(), -7);
  const done = S.tasks.filter(t => t.done && inList(t) && t.completedAt && ymd(new Date(t.completedAt)) >= weekAgo)
    .sort((x, y) => y.completedAt.localeCompare(x.completedAt));
  const cols = [
    ["todo", "📝 К выполнению", "var(--ink-3)", sortTasks(a.filter(t => t.status !== "doing"))],
    ["doing", "🚧 В работе", "var(--warn)", sortTasks(a.filter(t => t.status === "doing"))],
    ["done", "✅ Готово · 7 дней", "var(--ok)", done],
  ];
  return `<p class="hint" style="margin:0 0 12px">Перетаскивай карточки между колонками. Запуск таймера ▶ сам переносит задачу «В работу».</p>
    <div class="board">${cols.map(([k, h, c, arr]) => `<div class="col" data-status="${k}" style="--qc:${c}">
      <h3>${h} <span class="n">${arr.length}</span></h3>
      <div class="tasks">${arr.map(t => taskHTML(t, { drag: true })).join("")}</div></div>`).join("")}</div>`;
}

/* ================= расписание дня (тайм-блокинг) ================= */
const HOUR_PX = 56, START_H = 6, END_H = 24;

function viewSchedule() {
  const day = S.schedDay || today();
  const isToday = day === today();
  const unsched = active().filter(t => !t.dueTime && (t.due === day || (isToday && t.due && t.due < day)));
  const noDate = sortTasks(active().filter(t => !t.due)).slice(0, 10);
  const blocks = S.tasks.filter(t => t.due === day && t.dueTime);
  const planned = blocks.filter(t => !t.done).reduce((s, t) => s + (t.estimate || 30), 0);

  let slots = "";
  for (let h = START_H; h < END_H; h++) {
    for (const m of ["00", "30"]) {
      const time = `${String(h).padStart(2, "0")}:${m}`;
      slots += `<div class="slot ${m === "30" ? "hour" : ""}" data-time="${time}">${m === "00" ? `<span class="hour-label">${time}</span>` : ""}</div>`;
    }
  }
  const blocksHTML = blocks.map(t => {
    const [h, m] = t.dueTime.split(":").map(Number);
    const startMin = Math.max(0, (h - START_H) * 60 + m);
    const dur = t.estimate || 30;
    const end = h * 60 + m + dur;
    const endStr = `${String(Math.floor(end / 60) % 24).padStart(2, "0")}:${String(end % 60).padStart(2, "0")}`;
    const c = listById(t.listId)?.color || PRI_COLOR(t.priority);
    return `<div class="block ${t.done ? "done" : ""}" draggable="true" data-block="${t.id}"
      style="top:${startMin / 60 * HOUR_PX + 1}px;height:${Math.max(22, dur / 60 * HOUR_PX - 3)}px;--c:${c}">
      <span class="bt">${t.dueTime}–${endStr}</span> ${esc(t.title)}
      <button class="unsched" data-unsched-id="${t.id}" title="Убрать со шкалы">✕</button></div>`;
  }).join("");
  const now = new Date();
  const nowTop = ((now.getHours() - START_H) * 60 + now.getMinutes()) / 60 * HOUR_PX;

  return `<div class="cal-head">
      <button class="btn btn-sm" data-sched="-1">‹</button><h2>${cap(fmtLong.format(parseYmd(day)))}</h2>
      <button class="btn btn-sm" data-sched="1">›</button><button class="btn btn-sm" data-sched="0">Сегодня</button>
      <span class="hint">на шкале: ${fmtMin(planned)}</span></div>
    <div class="sched">
      <div class="sched-side" data-unsched-zone="1">
        <div class="group-head">Не распланировано <span class="n">${unsched.length}</span></div>
        <div class="tasks">${sortTasks(unsched).map(t => taskHTML(t, { drag: true, hideList: true })).join("") || '<p class="hint">Всё разложено по времени 👌</p>'}</div>
        ${noDate.length ? `<div class="group-head" style="margin-top:14px">Без даты</div><div class="tasks">${noDate.map(t => taskHTML(t, { drag: true })).join("")}</div>` : ""}
        <p class="hint">Перетащи задачу на шкалу. Длина блока — оценка времени (по умолчанию 30 мин). Верни блок сюда, чтобы снять время.</p>
      </div>
      <div class="timeline" id="timeline">${slots}${blocksHTML}
        ${isToday && nowTop >= 0 ? `<div class="now-line" style="top:${nowTop}px"></div>` : ""}</div>
    </div>`;
}

/* ================= планирование дня (ритуал из Sunsama) ================= */
function openPlan() { $("#planModal").hidden = false; renderPlan(); }

function renderPlan() {
  const td = today(), tm = addDays(td, 1);
  const a = active();
  const overdue = a.filter(t => t.due && t.due < td);
  const todayT = sortTasks(a.filter(t => t.due === td));
  const cand = a.filter(t => !t.due || t.due > td).sort((x, y) => urgency(y) - urgency(x)).slice(0, 8);
  const estSel = t => `<select class="input btn-sm" style="width:auto" data-plan-est="${t.id}">
    <option value="0">⏳ оценка</option>${[15, 30, 45, 60, 90, 120, 180, 240].map(m => `<option value="${m}" ${t.estimate === m ? "selected" : ""}>${fmtMin(m)}</option>`).join("")}</select>`;
  const row = (t, btns) => `<div class="plan-row"><span class="t">${t.pinned ? "📌 " : ""}${esc(t.title)}</span>${btns}</div>`;

  $("#planCard").innerHTML = `<h3>☀️ Планирование дня</h3>
    <p class="hint" style="margin:0">Разбери хвосты, выбери главное и оцени время, чтобы день не оказался перегружен.</p>
    ${overdue.length ? `<div class="plan-step">1 · Просроченные (${overdue.length}) <button class="btn btn-sm" data-plan-all="1" style="margin-left:8px">Всё на сегодня</button></div>
      ${overdue.map(t => row(t, `<span class="meta">${humanDate(t.due)}</span>
        <button class="btn btn-sm" data-pd="${t.id}" data-to="${td}">Сегодня</button>
        <button class="btn btn-sm" data-pd="${t.id}" data-to="${tm}">Завтра</button>
        <button class="btn btn-sm" data-pd="${t.id}" data-to="">Без срока</button>`)).join("")}` : ""}
    <div class="plan-step">${overdue.length ? 2 : 1} · План на сегодня (${todayT.length})</div>
    ${todayT.map(t => row(t, `${estSel(t)}<button class="btn btn-sm" data-pd="${t.id}" data-to="${tm}" title="Перенести на завтра">→ Завтра</button>`)).join("") || '<p class="hint">Пока пусто — добавь задачи из списка ниже</p>'}
    ${cand.length ? `<div class="plan-step">Что ещё взять · по срочности</div>
      ${cand.map(t => row(t, `<span class="meta">🧠 ${urgency(t)}${t.due ? " · " + humanDate(t.due) : ""}</span>
        <button class="btn btn-sm" data-pd="${t.id}" data-to="${td}">＋ Сегодня</button>`)).join("")}` : ""}
    <div class="plan-step">Загрузка</div>${loadBar()}
    <div class="plan-foot"><button class="btn btn-primary" data-plan-close="1">Готово, за работу!</button></div>`;
}

function onPlanClick(e) {
  if (e.target.id === "planModal" || e.target.closest("[data-plan-close]")) { $("#planModal").hidden = true; render(); return; }
  const b = e.target.closest("[data-pd], [data-plan-all], [data-act]");
  if (!b) return;
  if (b.dataset.act === "dayEnd") { askDayEnd(); renderPlan(); return; }
  const moves = b.dataset.planAll ? active().filter(t => t.due && t.due < today()).map(t => [t, today()])
    : [[S.tasks.find(t => t.id === b.dataset.pd), b.dataset.to]];
  for (const [t, to] of moves) { t.due = to; if (!to) t.dueTime = ""; saveTask(t, 0); }
  renderPlan(); renderNav();
}

function onPlanChange(e) {
  const id = e.target.dataset.planEst;
  if (!id) return;
  const t = S.tasks.find(t => t.id === id);
  t.estimate = +e.target.value;
  saveTask(t, 0);
  renderPlan();
}

function askDayEnd() {
  const v = prompt("Во сколько заканчивается твой день? (ЧЧ:ММ)", dayEnd());
  if (v && /^([01]?\d|2[0-3]):[0-5]\d$/.test(v.trim())) safeSet("dayEnd", v.trim().padStart(5, "0"));
}

/* ================= таймер задачи ================= */
function replaceTask(n) {
  const i = S.tasks.findIndex(t => t.id === n.id);
  if (i >= 0) S.tasks[i] = n;
}

async function timerToggle(id) {
  if (id in saveTimers) await flushSave(id);
  const t = S.tasks.find(t => t.id === id);
  if (t.timerStart) {
    replaceTask(await api("POST", `/tasks/${id}/timer/stop`));
  } else {
    (await api("POST", `/tasks/${id}/timer/start`)).forEach(replaceTask);
    toast(`⏱ Учёт времени: ${t.title}`);
  }
  render();
}

/* ================= календарь ================= */
function viewCalendar() {
  const m = S.calMonth;
  const start = new Date(m);
  start.setDate(1 - ((m.getDay() + 6) % 7)); // с понедельника
  const td = today();
  const monthName = new Intl.DateTimeFormat("ru", { month: "long", year: "numeric" }).format(m);
  let html = `<div class="cal-head">
    <button class="btn btn-sm" data-cal="-1">‹</button><h2>${monthName}</h2><button class="btn btn-sm" data-cal="1">›</button>
    <button class="btn btn-sm" data-cal="0">Сегодня</button></div><div class="cal">`;
  for (let i = 0; i < 7; i++) html += `<div class="cal-dow">${fmtDow.format(new Date(2024, 0, 1 + i))}</div>`;
  for (let i = 0; i < 42; i++) {
    const d = new Date(start); d.setDate(start.getDate() + i);
    const key = ymd(d);
    const items = sortTasks(S.tasks.filter(t => t.due === key));
    const cls = [d.getMonth() !== m.getMonth() && "other", key === td && "today", key === S.calPick && "picked"].filter(Boolean).join(" ");
    html += `<div class="cal-day ${cls}" data-day="${key}"><span class="cal-num">${d.getDate()}</span>
      ${items.slice(0, 3).map(t => `<div class="cal-task ${t.done ? "done" : ""}" draggable="true" data-cal-task="${t.id}"
        style="--c:${listById(t.listId)?.color || PRI_COLOR(t.priority)}">${t.dueTime ? t.dueTime + " " : ""}${esc(t.title)}</div>`).join("")}
      ${items.length > 3 ? `<span class="cal-more">ещё ${items.length - 3}</span>` : ""}</div>`;
    if (i === 34 && new Date(d.getFullYear(), d.getMonth(), d.getDate() + 1).getMonth() !== m.getMonth()) break; // шестая неделя не нужна
  }
  html += `</div>`;
  if (S.calPick) {
    const items = S.tasks.filter(t => t.due === S.calPick);
    html += `<div class="group"><div class="group-head">${cap(fmtLong.format(parseYmd(S.calPick)))} <span class="n">${items.length}</span></div>
      <div class="tasks">${items.length ? sortTasks(items).map(t => taskHTML(t)).join("") : '<div class="empty" style="padding:20px">Задач нет — добавь через строку выше</div>'}</div></div>`;
  }
  return html;
}
function PRI_COLOR(p) { return ["var(--accent)", "var(--p1)", "var(--p2)", "var(--p3)"][p]; }

/* ================= матрица ================= */
function viewMatrix() {
  const soon = addDays(today(), 2);
  const a = active();
  const imp = t => t.priority >= 2;
  const urg = t => t.due && t.due <= soon;
  const q = [
    ["🔥 Сделать сейчас", "Важно и срочно", "var(--danger)", a.filter(t => imp(t) && urg(t))],
    ["📌 Запланировать", "Важно, но не срочно", "var(--accent)", a.filter(t => imp(t) && !urg(t))],
    ["⚡ Делегировать", "Срочно, но не важно", "var(--warn)", a.filter(t => !imp(t) && urg(t))],
    ["🧊 Отложить", "Не важно и не срочно", "var(--ink-3)", a.filter(t => !imp(t) && !urg(t))],
  ];
  return `<p style="color:var(--ink-3);margin:0 0 12px;font-size:13px">Важные — приоритет «средний» и выше. Срочные — срок в ближайшие 2 дня или просрочен.</p>
    <div class="matrix">${q.map(([h, p, c, arr]) => `<div class="quad" style="--qc:${c}"><h3>${h} <span style="color:var(--ink-3)">${arr.length}</span></h3><p>${p}</p>
      <div class="tasks">${sortTasks(arr).map(t => taskHTML(t)).join("") || '<span style="color:var(--ink-3);font-size:13px">Пусто</span>'}</div></div>`).join("")}</div>`;
}

/* ================= привычки ================= */
function streak(h) {
  let d = today(), n = 0;
  if (!h.done[d]) d = addDays(d, -1);
  while (h.done[d]) { n++; d = addDays(d, -1); }
  return n;
}

function viewHabits() {
  const td = today();
  const days = Array.from({ length: 7 }, (_, i) => addDays(td, i - 6));
  let html = `<div class="habits"><table><thead><tr><th style="text-align:left;padding-left:16px">Привычка</th>
    ${days.map(d => `<th class="${d === td ? "today" : ""}">${fmtDow.format(parseYmd(d))}<br>${parseYmd(d).getDate()}</th>`).join("")}
    <th>Серия</th><th>30 дней</th><th></th></tr></thead><tbody>`;
  for (const h of S.habits) {
    const last30 = Array.from({ length: 30 }, (_, i) => addDays(td, i - 29));
    const rate = Math.round(last30.filter(d => h.done[d]).length / 30 * 100);
    html += `<tr><td class="name"><i class="dot" style="background:${h.color};display:inline-block"></i>${esc(h.name)}</td>
      ${days.map(d => `<td><button class="hcheck ${h.done[d] ? "on" : ""}" style="--c:${h.color}" data-habit="${h.id}" data-date="${d}">${h.done[d] ? "✓" : ""}</button></td>`).join("")}
      <td class="streak">🔥 ${streak(h)}</td>
      <td><div class="heat">${last30.map(d => `<i style="${h.done[d] ? `background:${h.color}` : ""}" title="${d}"></i>`).join("")}</div>
        <span style="font-size:11px;color:var(--ink-3)">${rate}%</span></td>
      <td><button class="icon-btn" data-del-habit="${h.id}" title="Удалить">${TRASH_SVG}</button></td></tr>`;
  }
  html += `</tbody></table>${S.habits.length ? "" : empty("🌱", "Добавь привычку, которую хочешь выработать: зарядка, чтение, вода…")}</div>
    <form class="add-habit" id="addHabit"><input class="input" name="name" placeholder="Новая привычка, напр. «Читать 20 минут»" required>
    <button class="btn btn-primary">Добавить</button></form>`;
  return html;
}

/* ================= статистика ================= */
function viewStats() {
  const td = today();
  const done = S.tasks.filter(t => t.done && t.completedAt);
  const doneDays = new Set(done.map(t => ymd(new Date(t.completedAt))));
  let st = 0, d = doneDays.has(td) ? td : addDays(td, -1);
  while (doneDays.has(d)) { st++; d = addDays(d, -1); }

  const week = Array.from({ length: 7 }, (_, i) => addDays(td, i - 6));
  const perDay = week.map(day => done.filter(t => ymd(new Date(t.completedAt)) === day).length);
  const max = Math.max(1, ...perDay);
  const weekTotal = perDay.reduce((a, b) => a + b, 0);
  const a = active();
  const all = S.tasks.length;
  const pomos = (S.pomos || {})[td] || 0;

  const cards = [
    ["Сегодня", perDay[6], "задач выполнено"],
    ["За неделю", weekTotal, `в среднем ${(weekTotal / 7).toFixed(1)} в день`],
    ["Активных", a.length, `${a.filter(isOverdue).length} просрочено`],
    ["Серия", st, "дней подряд с выполненными задачами"],
    ["Помидоры", pomos, "фокус-сессий сегодня"],
    ["Завершено", all ? Math.round(S.tasks.filter(t => t.done).length / all * 100) + "%" : "—", "от всех задач"],
  ];

  const lists = [{ id: "", name: "Входящие", color: "var(--ink-3)" }, ...S.lists];
  const maxL = Math.max(1, ...lists.map(l => a.filter(t => t.listId === l.id).length));

  return `<div class="cards">${cards.map(([k, v, s]) => `<div class="card"><div class="k">${k}</div><div class="v">${v}</div><div class="s">${s}</div></div>`).join("")}</div>
    <div class="panel"><h3>Выполнено за 7 дней</h3><div class="bars">
      ${perDay.map((n, i) => `<div class="bar"><b>${n || ""}</b><i style="height:${n / max * 100}%"></i><span>${fmtDow.format(parseYmd(week[i]))}</span></div>`).join("")}
    </div></div>
    <div class="panel"><h3>Активные задачи по спискам</h3>
      ${lists.map(l => { const n = a.filter(t => t.listId === l.id).length;
        return `<div class="hbar"><span>${esc(l.name)}</span><div class="track"><i style="width:${n / maxL * 100}%;background:${l.color}"></i></div><b>${n}</b></div>`; }).join("")}
    </div>
    <div class="panel"><h3>По приоритету</h3>
      ${[3, 2, 1, 0].map(p => { const n = a.filter(t => t.priority === p).length;
        return `<div class="hbar"><span>${PRI_NAMES[p]}</span><div class="track"><i style="width:${a.length ? n / a.length * 100 : 0}%;background:${PRI_COLOR(p)}"></i></div><b>${n}</b></div>`; }).join("")}
    </div>`;
}

/* ================= панель задачи ================= */
function renderDetail() {
  const panel = $("#detail");
  const t = S.tasks.find(t => t.id === S.selected);
  if (!t) { panel.classList.remove("open"); panel.setAttribute("aria-hidden", "true"); return; }
  panel.classList.add("open");
  panel.setAttribute("aria-hidden", "false");
  const sd = t.subtasks.filter(s => s.done).length;
  const td = today();
  panel.innerHTML = `<div class="detail-inner">
    <div class="d-top">
      <button class="btn btn-sm ${t.done ? "" : "btn-primary"}" data-d="toggle">${t.done ? "↺ Вернуть" : "✓ Выполнить"}</button>
      <span class="grow"></span>
      <button class="icon-btn" data-d="focus" title="Фокус-сессия на эту задачу">🍅</button>
      <button class="icon-btn" data-d="pin" title="Закрепить">${t.pinned ? "📍" : "📌"}</button>
      <button class="icon-btn" data-d="delete" title="Удалить">${TRASH_SVG}</button>
      <button class="icon-btn" data-d="close" title="Закрыть (Esc)">✕</button>
    </div>
    <textarea class="d-title" rows="1" data-f="title">${esc(t.title)}</textarea>

    <div class="field"><label>Срок</label>
      <div class="row"><input class="input" type="date" data-f="due" value="${t.due}"><input class="input" type="time" data-f="dueTime" value="${t.dueTime}"></div>
      <div class="quick-dates">
        <button class="chip" data-due="${td}">Сегодня</button><button class="chip" data-due="${addDays(td, 1)}">Завтра</button>
        <button class="chip" data-due="${addDays(td, (8 - new Date().getDay()) % 7 || 7)}">След. понедельник</button>
        <button class="chip" data-due="${addDays(td, 7)}">Через неделю</button><button class="chip" data-due="">Без срока</button>
      </div></div>

    <div class="field"><label>Приоритет</label><div class="seg">
      ${PRI_NAMES.map((n, i) => `<button class="p${i} ${t.priority === i ? "on" : ""}" data-pri="${i}">${i ? "⚑ " : ""}${n}</button>`).join("")}
    </div></div>

    ${t.done ? "" : `<div class="field"><label>Статус</label><div class="seg">
      <button class="${t.status !== "doing" ? "on" : ""}" data-status="">📝 К выполнению</button>
      <button class="${t.status === "doing" ? "on" : ""}" data-status="doing">🚧 В работе</button>
    </div></div>`}

    <div class="field"><label>Время</label>
      <div class="est-row">${[15, 30, 60, 120, 240].map(m => `<button class="chip ${t.estimate === m ? "on" : ""}" data-est="${m}">${fmtMin(m)}</button>`).join("")}
        <input class="input" type="number" min="0" step="5" data-f="estimate" value="${t.estimate || ""}" placeholder="мин"></div>
      <div class="timer-box">
        ${t.done ? "" : `<button class="btn btn-sm ${t.timerStart ? "" : "btn-primary"}" data-d="timer">${t.timerStart ? "⏸ Стоп" : "▶ Старт"}</button>`}
        <b data-live="${t.id}">${fmtClock(spentNow(t))}</b>
        <span class="s">${t.estimate ? `затрачено из ${fmtMin(t.estimate)} по оценке` : "затрачено · оценка не задана"}</span>
      </div></div>

    <div class="row">
      <div class="field"><label>Список</label><select class="input" data-f="listId">
        <option value="">📥 Входящие</option>${S.lists.map(l => `<option value="${l.id}" ${t.listId === l.id ? "selected" : ""}>${esc(l.name)}</option>`).join("")}
      </select></div>
      <div class="field"><label>Повтор</label><select class="input" data-f="repeat">
        ${Object.entries(REPEAT_NAMES).map(([k, n]) => `<option value="${k}" ${t.repeat === k ? "selected" : ""}>${n}</option>`).join("")}
      </select></div>
    </div>

    <div class="field"><label>Если не выполнена в срок</label><select class="input" data-f="carry">
      ${Object.entries(CARRY_NAMES).map(([k, n]) => `<option value="${k}" ${(t.carry || "") === k ? "selected" : ""} ${k === "shift" && !seriesOf(t) ? "disabled" : ""}>${n}</option>`).join("")}
    </select>
    <span class="hint">${carryHint(t)}</span></div>

    <div class="field"><label>Теги</label><input class="input" data-f="tags" value="${esc(t.tags.join(", "))}" placeholder="дом, срочно, идеи"></div>

    <div class="field"><label>Подзадачи ${t.subtasks.length ? `· ${sd}/${t.subtasks.length}` : ""}</label>
      ${t.subtasks.length ? `<div class="progress"><i style="width:${sd / t.subtasks.length * 100}%"></i></div>` : ""}
      <div class="subtasks">${t.subtasks.map((s, i) => `<div class="sub ${s.done ? "done" : ""}">
        <input type="checkbox" data-sub-check="${i}" ${s.done ? "checked" : ""}>
        <input class="input" data-sub-title="${i}" value="${esc(s.title)}">
        <button class="icon-btn" data-sub-del="${i}">✕</button></div>`).join("")}
        <input class="input" id="newSub" placeholder="＋ Добавить подзадачу и нажать Enter">
      </div></div>

    <div class="field"><label>Заметки</label><textarea class="input" data-f="notes" placeholder="Детали, ссылки, мысли…">${esc(t.notes)}</textarea></div>

    <div class="d-foot">${t.done ? "" : `🧠 Срочность ${urgency(t)} <span class="urg">(формула Taskwarrior)</span><br>`}Создана ${new Date(t.createdAt).toLocaleString("ru")}${t.completedAt ? `<br>Выполнена ${new Date(t.completedAt).toLocaleString("ru")}` : ""}</div>
  </div>`;
  autosize($(".d-title", panel));
}

function autosize(el) { if (!el) return; el.style.height = "auto"; el.style.height = el.scrollHeight + "px"; }

const saveTimers = {};
function saveTask(t, delay = 350) {
  clearTimeout(saveTimers[t.id]);
  saveTimers[t.id] = setTimeout(() => flushSave(t.id), delay);
}
// отправляет отложенное сохранение сразу (например, перед переключением выполнения)
async function flushSave(id) {
  clearTimeout(saveTimers[id]);
  delete saveTimers[id];
  const t = S.tasks.find(x => x.id === id);
  if (!t) return;
  const res = await api("PUT", "/tasks/" + id, t);
  t.completedAt = res.completedAt;
}

function current() { return S.tasks.find(t => t.id === S.selected); }

function onDetailInput(e) {
  const t = current();
  if (!t) return;
  const f = e.target.dataset.f;
  if (f) {
    let v = e.target.value;
    if (f === "title") { autosize(e.target); if (!v.trim()) return; v = v.replace(/\n/g, " "); }
    if (f === "tags") v = [...new Set(v.split(",").map(s => s.trim().replace(/^#/, "").toLowerCase()).filter(Boolean))];
    if (f === "estimate") v = Math.max(0, parseInt(v, 10) || 0);
    t[f] = v;
    if (f === "due" && !v) t.dueTime = "";
    saveTask(t);
    renderNav(); renderView();
    return;
  }
  if (e.target.dataset.subTitle !== undefined) {
    t.subtasks[+e.target.dataset.subTitle].title = e.target.value;
    saveTask(t);
  }
}

async function onDetailClick(e) {
  const t = current();
  if (!t) return;
  const b = e.target.closest("button, input[type=checkbox]");
  if (!b) return;
  const d = b.dataset;
  if (d.d === "close") { S.selected = null; render(); return; }
  if (d.d === "toggle") return toggleTask(t.id);
  if (d.d === "delete") return deleteTask(t.id);
  if (d.d === "pin") { t.pinned = !t.pinned; saveTask(t, 0); render(); return; }
  if (d.d === "focus") { pomo.focus(t.title); if (!t.timerStart && !t.done) timerToggle(t.id); return; }
  if (d.d === "timer") return timerToggle(t.id);
  if (d.due !== undefined) { t.due = d.due; if (!d.due) t.dueTime = ""; }
  else if (d.status !== undefined) t.status = d.status;
  else if (d.est !== undefined) t.estimate = t.estimate === +d.est ? 0 : +d.est;
  else if (d.pri !== undefined) t.priority = +d.pri;
  else if (d.subCheck !== undefined) t.subtasks[+d.subCheck].done = b.checked;
  else if (d.subDel !== undefined) t.subtasks.splice(+d.subDel, 1);
  else return;
  saveTask(t, 0);
  render();
}

function onDetailKey(e) {
  if (e.target.id === "newSub" && e.key === "Enter" && e.target.value.trim()) {
    const t = current();
    t.subtasks.push({ id: "", title: e.target.value.trim(), done: false });
    saveTask(t, 0);
    render();
    $("#newSub").focus();
  }
  if (e.target.dataset.f === "title" && e.key === "Enter") { e.preventDefault(); e.target.blur(); }
}

/* ================= действия с задачами ================= */
const wait = ms => new Promise(r => setTimeout(r, ms));

// схлопывает карточку перед тем, как она переедет в другую группу или исчезнет
function collapse(el) {
  const h = el.offsetHeight;
  const anim = el.animate([
    { height: h + "px", opacity: 1, transform: "none" },
    { height: "0px", opacity: 0, transform: "translateX(14px)", paddingTop: "0px", paddingBottom: "0px", marginBottom: "-6px", borderWidth: "0px" },
  ], { duration: 260, easing: "ease-in", fill: "forwards" }).finished.catch(() => {});
  // в фоновой вкладке кадры тормозят — не держим интерфейс дольше нужного
  return Promise.race([anim, wait(320)]);
}

async function toggleTask(id) {
  const el = $(`.task[data-id="${id}"]`);
  const t = S.tasks.find(x => x.id === id);
  const willBeDone = !t.done;
  // сразу показываем результат, не дожидаясь сервера
  if (el) {
    el.classList.toggle("done", willBeDone);
    if (willBeDone) $(".check", el).classList.add("burst");
  }
  if (id in saveTimers) await flushSave(id);
  const started = Date.now();
  const { task, next } = await api("POST", `/tasks/${id}/toggle`);
  replaceTask(task);
  if (next) { S.tasks.push(next); S.enter.add(next.id); }
  if (task.done) {
    chime();
    toast(next ? `Готово! Следующий повтор: ${humanDate(next.due)}` : "Задача выполнена ✓");
    if (el) maybeCelebrate(el);
  }
  if (el && el.isConnected && S.selected !== id) {
    await wait(Math.max(0, (willBeDone ? 450 : 120) - (Date.now() - started)));
    await collapse(el);
    S.enter.add(id);
  }
  render();
}

// конфетти, когда закрыта последняя задача на сегодня
function maybeCelebrate(el) {
  const td = today();
  if (active().some(t => t.due && t.due <= td)) return;
  if (!S.tasks.some(t => doneOn(t, td))) return;
  const r = $(".check", el).getBoundingClientRect();
  const colors = ["#2f6bff", "#19b5ff", "#7aa7ff", "#9fe0ff", "#ffffff", "#ffd166"];
  for (let i = 0; i < 36; i++) {
    const c = document.createElement("i");
    c.className = "confetti";
    c.style.background = colors[i % colors.length];
    c.style.left = r.left + r.width / 2 + "px";
    c.style.top = r.top + r.height / 2 + "px";
    document.body.appendChild(c);
    const ang = Math.random() * Math.PI * 2, dist = 80 + Math.random() * 160;
    c.animate([
      { transform: "translate(0,0) rotate(0)", opacity: 1 },
      { transform: `translate(${Math.cos(ang) * dist}px, ${Math.sin(ang) * dist + 120}px) rotate(${Math.random() * 720}deg)`, opacity: 0 },
    ], { duration: 900 + Math.random() * 500, easing: "cubic-bezier(.2,.7,.4,1)" }).finished.then(() => c.remove());
  }
  setTimeout(() => toast("Все задачи на сегодня выполнены! 🎉"), 500);
}

function greeting() {
  const h = new Date().getHours();
  return h < 5 ? "Доброй ночи" : h < 12 ? "Доброе утро" : h < 18 ? "Добрый день" : "Добрый вечер";
}

async function deleteTask(id) {
  const t = S.tasks.find(t => t.id === id);
  await api("DELETE", "/tasks/" + id);
  S.tasks = S.tasks.filter(x => x.id !== id);
  if (S.selected === id) S.selected = null;
  render();
  toast(`Удалено «${t.title}»`, "Вернуть", async () => {
    const back = await api("POST", "/tasks", t);
    S.tasks.push(back);
    render();
  });
}

function flash(id) {
  const el = $(`.task[data-id="${id}"]`);
  if (el) el.scrollIntoView({ block: "nearest", behavior: "smooth" });
}

async function onViewClick(e) {
  const nav = e.target.closest("[data-view]");
  if (nav) { setView(nav.dataset.view); return; }

  const cal = e.target.closest("[data-cal]");
  if (cal) {
    const n = +cal.dataset.cal;
    S.calMonth = n ? new Date(S.calMonth.getFullYear(), S.calMonth.getMonth() + n, 1) : firstOfMonth(new Date());
    if (!n) S.calPick = today();
    renderView(); return;
  }
  const gact = e.target.closest("[data-act]")?.dataset.act;
  if (gact === "plan") { openPlan(); return; }
  if (gact === "dayEnd") { askDayEnd(); renderView(); return; }

  const sched = e.target.closest("[data-sched]");
  if (sched) {
    const n = +sched.dataset.sched;
    S.schedDay = n ? addDays(S.schedDay || today(), n) : today();
    renderView(); return;
  }
  const uns = e.target.closest("[data-unsched-id]");
  if (uns) {
    const t = S.tasks.find(t => t.id === uns.dataset.unschedId);
    t.dueTime = ""; saveTask(t, 0); render(); return;
  }
  const block = e.target.closest("[data-block]");
  if (block) { S.selected = block.dataset.block; render(); return; }

  const calTask = e.target.closest("[data-cal-task]");
  if (calTask) { S.selected = calTask.dataset.calTask; render(); return; }
  const day = e.target.closest("[data-day]");
  if (day) { S.calPick = S.calPick === day.dataset.day ? null : day.dataset.day; renderView(); return; }

  const hb = e.target.closest("[data-habit]");
  if (hb) {
    const h = await api("POST", `/habits/${hb.dataset.habit}/toggle/${hb.dataset.date}`);
    S.habits[S.habits.findIndex(x => x.id === h.id)] = h;
    if (h.done[hb.dataset.date]) chime();
    renderView(); return;
  }
  const hd = e.target.closest("[data-del-habit]");
  if (hd) {
    if (!confirm("Удалить привычку вместе с историей?")) return;
    await api("DELETE", "/habits/" + hd.dataset.delHabit);
    S.habits = S.habits.filter(h => h.id !== hd.dataset.delHabit);
    renderView(); return;
  }
  if (e.target.id === "clearDone") {
    if (!confirm("Удалить все выполненные задачи навсегда?")) return;
    const { removed } = await api("POST", "/tasks/clear-completed");
    S.tasks = S.tasks.filter(t => !t.done);
    render(); toast(`Удалено задач: ${removed}`); return;
  }

  const taskEl = e.target.closest(".task");
  if (!taskEl) return;
  const id = taskEl.dataset.id;
  const act = e.target.closest("[data-act]")?.dataset.act;
  if (act === "toggle") return toggleTask(id);
  if (act === "del") return deleteTask(id);
  if (act === "timer") return timerToggle(id);
  if (act === "pin") { const t = S.tasks.find(t => t.id === id); t.pinned = !t.pinned; saveTask(t, 0); render(); return; }
  S.selected = S.selected === id ? null : id;
  render();
}

/* ================= drag & drop ================= */
let dragId = null;
function bindDrag() {
  for (const el of $$(".task[draggable], .cal-task[draggable], .block[draggable]")) {
    el.addEventListener("dragstart", e => {
      e.stopPropagation();
      dragId = el.dataset.id || el.dataset.calTask || el.dataset.block;
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", dragId);
      requestAnimationFrame(() => el.classList.add("dragging"));
    });
    el.addEventListener("dragend", () => { el.classList.remove("dragging"); $$(".drop-above,.drop").forEach(x => x.classList.remove("drop-above", "drop")); });
  }
}

// слот шкалы времени под курсором (блоки перекрывают слоты, поэтому считаем по координате)
function slotAt(e) {
  const tl = e.target.closest("#timeline");
  if (!tl) return null;
  const idx = Math.floor((e.clientY - tl.getBoundingClientRect().top) / (HOUR_PX / 2));
  return $$(".slot", tl)[Math.max(0, idx)] || null;
}

function dropTarget(e) {
  const task = e.target.closest(".task");
  const src = $(`.task[data-id="${dragId}"]`);
  if (task && task.dataset.id !== dragId && src && task.parentElement === src.parentElement) return [task, "drop-above"];
  const t = e.target.closest(".cal-day") || slotAt(e) || e.target.closest(".col") || e.target.closest("[data-unsched-zone]");
  return t ? [t, "drop"] : [null];
}

function onDragOver(e) {
  if (!dragId) return;
  const [el, cls] = dropTarget(e);
  if (!el) return;
  e.preventDefault();
  $$(".drop-above,.drop").forEach(x => x.classList.remove("drop-above", "drop"));
  el.classList.add(cls);
}

async function onDrop(e) {
  if (!dragId) return;
  e.preventDefault();
  const [el, cls] = dropTarget(e);
  const id = dragId; dragId = null;
  const t = S.tasks.find(t => t.id === id);
  if (!el) return;
  if (cls === "drop") {
    if (el.dataset.day) {
      t.due = el.dataset.day;
      toast(`Перенесено на ${humanDate(t.due)}`);
    } else if (el.dataset.time) {
      t.due = S.schedDay || today();
      t.dueTime = el.dataset.time;
    } else if (el.dataset.unschedZone) {
      t.dueTime = "";
      if (!t.due) t.due = S.schedDay || today();
    } else if (el.dataset.status) {
      const st = el.dataset.status;
      if (st === "done") { if (!t.done) await toggleTask(id); return; }
      if (t.done) await toggleTask(id);
      const cur = S.tasks.find(x => x.id === id);
      if (cur.timerStart && st === "todo") await timerToggle(id);
      cur.status = st === "doing" ? "doing" : "";
      saveTask(cur, 0); render(); return;
    }
    saveTask(t, 0);
    render();
    return;
  }
  const target = el;
  const box = target.parentElement;
  const ids = $$(".task", box).map(x => x.dataset.id).filter(x => x !== id);
  ids.splice(ids.indexOf(target.dataset.id), 0, id);
  // общий порядок: переставленные задачи встают на место, которое занимали
  const orders = ids.map(i => S.tasks.find(t => t.id === i).order).sort((a, b) => a - b);
  ids.forEach((i, k) => { S.tasks.find(t => t.id === i).order = orders[k]; });
  const all = [...S.tasks].sort((a, b) => a.order - b.order).map(t => t.id);
  all.forEach((i, k) => { S.tasks.find(t => t.id === i).order = k; });
  render();
  await api("POST", "/tasks/reorder", all);
}

/* ================= списки ================= */
function showNewList() {
  const slot = $("#newListSlot");
  slot.innerHTML = `<input class="input" id="newListName" placeholder="Название и Enter" style="margin:4px 0">`;
  const inp = $("#newListName");
  inp.focus();
  inp.addEventListener("keydown", async e => {
    if (e.key === "Escape") { slot.innerHTML = ""; }
    if (e.key !== "Enter" || !inp.value.trim()) return;
    const l = await api("POST", "/lists", { name: inp.value.trim(), color: PALETTE[S.lists.length % PALETTE.length] });
    S.lists.push(l);
    setView("list:" + l.id);
  });
  inp.addEventListener("blur", () => setTimeout(() => { if (slot.isConnected) slot.innerHTML = ""; }, 150));
}

async function onSidebarClick(e) {
  const del = e.target.closest("[data-del-list]");
  if (del) {
    e.stopPropagation();
    const l = listById(del.dataset.delList);
    if (!confirm(`Удалить список «${l.name}»? Задачи переедут во «Входящие».`)) return;
    await api("DELETE", "/lists/" + l.id);
    S.lists = S.lists.filter(x => x.id !== l.id);
    S.tasks.forEach(t => { if (t.listId === l.id) t.listId = ""; });
    if (S.view === "list:" + l.id) S.view = "inbox";
    render(); return;
  }
  const nav = e.target.closest("[data-view]");
  if (nav) setView(nav.dataset.view);
}

/* ================= помодоро (общий с мини-панелью и треем) ================= */
const pomo = (() => {
  let c;
  function draw(st) {
    if (!$("#pomoTime").querySelector("input")) $("#pomoTime").textContent = fmtMMSS(c.left());
    $("#pomoStart").textContent = st.running ? "Пауза" : st.left < st.durations[st.mode] ? "Дальше" : "Старт";
    $("#pomoLabel").textContent = st.label || { work: "Время сосредоточиться", short: "Короткий перерыв", long: "Длинный перерыв" }[st.mode];
    $$(".pomo-modes button").forEach(b => {
      b.classList.toggle("on", b.dataset.mode === st.mode);
      b.innerHTML = { work: "Фокус", short: "Перерыв", long: "Длинный" }[b.dataset.mode] + "<small>" + st.settings[b.dataset.mode] + " мин</small>";
    });
    $("#pomoDone").textContent = "🍅".repeat(Math.min(st.count, 12));
    document.title = st.running ? `${fmtMMSS(c.left())} · ${POMO_NAMES[st.mode]}` : $("#viewTitle").textContent + " · Ежедневник";
  }
  function finished(st) {
    // звук играет мини-панель в трее, здесь только подсказка
    toast(st.finished === "work" ? "Помидор готов! Перерыв уже идёт ☕" : "Перерыв окончен — за работу!");
    S.pomos[today()] = st.count;
  }
  return {
    init() {
      c = pomoClient(draw, finished);
      $("#pomoStart").onclick = () => c.toggle();
      $("#pomoReset").onclick = () => c.reset();
      $$(".pomo-modes button").forEach(b => (b.onclick = () => c.mode(b.dataset.mode)));
      $("#pomoTime").onclick = () => editClock($("#pomoTime"), c);
      const box = $("#pomoSettings");
      const close = () => { box.hidden = true; $("#pomoGear").classList.remove("on"); };
      $("#pomoGear").onclick = () => {
        if (!box.hidden) return close();
        box.innerHTML = pomoSettingsHTML(c.state().settings);
        box.hidden = false;
        $("#pomoGear").classList.add("on");
        box.querySelector("[data-pset=work]").focus();
      };
      bindPomoSettings(box, c, close);
    },
    focus(text) { c.mode("work", "🎯 " + text).then(() => c.start()); toast("Фокус-сессия запущена"); },
  };
})();

/* ================= звук, уведомления, тосты ================= */
let audioCtx;
function chime(long) {
  try {
    audioCtx = audioCtx || new AudioContext();
    const notes = long ? [660, 880, 990] : [880, 1320];
    notes.forEach((f, i) => {
      const o = audioCtx.createOscillator(), g = audioCtx.createGain();
      o.frequency.value = f; o.type = "sine";
      const t0 = audioCtx.currentTime + i * 0.09;
      g.gain.setValueAtTime(0.0001, t0);
      g.gain.exponentialRampToValueAtTime(0.12, t0 + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, t0 + 0.35);
      o.connect(g).connect(audioCtx.destination);
      o.start(t0); o.stop(t0 + 0.4);
    });
  } catch { /* без звука */ }
}

function notify(title, body) {
  if ("Notification" in window && Notification.permission === "granted") new Notification(title, { body, icon: "" });
}

function checkDeadlines() {
  const td = today();
  const now = new Date();
  const hm = `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
  const seen = JSON.parse(safeGet("notified") || "{}");
  for (const t of active()) {
    if (t.due !== td || !t.dueTime || t.dueTime > hm) continue;
    const key = t.id + t.due + t.dueTime;
    if (seen[key]) continue;
    seen[key] = 1;
    notify("⏰ Дедлайн: " + t.title, `Срок — сегодня в ${t.dueTime}`);
    toast(`⏰ Дедлайн: ${t.title}`);
  }
  safeSet("notified", JSON.stringify(seen));
  renderNav();
}

let toastTimer;
function toast(text, actionText, action) {
  const el = $("#toast");
  el.innerHTML = `<span>${esc(text)}</span>${actionText ? `<button>${esc(actionText)}</button>` : ""}`;
  el.hidden = false;
  if (actionText) el.querySelector("button").onclick = () => { el.hidden = true; action(); };
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), actionText ? 5000 : 2600);
}

/* ================= тема ================= */
function applyTheme() {
  const t = safeGet("theme");
  if (t) document.documentElement.dataset.theme = t;
  else delete document.documentElement.dataset.theme;
}
function toggleTheme() {
  const dark = document.documentElement.dataset.theme === "dark" ||
    (!document.documentElement.dataset.theme && matchMedia("(prefers-color-scheme: dark)").matches);
  safeSet("theme", dark ? "light" : "dark");
  applyTheme();
}

/* ================= заметка дня ================= */
let noteTimer;
function onNoteInput(e) {
  if (e.target.id !== "dayNote") return;
  const td = today(), text = e.target.value;
  S.notes[td] = text;
  $("#noteSaved").textContent = "сохраняю…";
  clearTimeout(noteTimer);
  noteTimer = setTimeout(async () => {
    await api("PUT", "/notes/" + td, { text });
    const s = $("#noteSaved");
    if (s) s.textContent = "сохранено ✓";
  }, 600);
}

/* ================= инициализация ================= */
function init() {
  applyTheme();
  $("#quickAdd").addEventListener("submit", quickAdd);
  $("#qaInput").addEventListener("input", renderPreview);
  // клик в любом месте строки ставит курсор в поле ввода
  $("#quickAdd").addEventListener("mousedown", e => {
    if (e.target.closest("button, input")) return;
    e.preventDefault();
    $("#qaInput").focus();
  });

  $("#sidebar").addEventListener("click", onSidebarClick);
  $("#addList").addEventListener("click", e => { e.stopPropagation(); showNewList(); });
  $("#search").addEventListener("input", e => { S.search = e.target.value.trim(); renderNav(); renderView(); });

  const view = $("#view");
  view.addEventListener("click", onViewClick);
  view.addEventListener("input", onNoteInput);
  view.addEventListener("dragover", onDragOver);
  view.addEventListener("drop", onDrop);
  view.addEventListener("submit", async e => {
    if (e.target.id !== "addHabit") return;
    e.preventDefault();
    const name = e.target.name.value.trim();
    const h = await api("POST", "/habits", { name, color: PALETTE[S.habits.length % PALETTE.length] });
    S.habits.push(h);
    renderView();
    $("#addHabit input").focus();
  });

  $("#headRight").addEventListener("change", e => {
    if (e.target.id === "boardListSel") { S.boardList = e.target.value; renderView(); return; }
    if (e.target.id !== "sortSel") return;
    S.sort = e.target.value; safeSet("sort", S.sort); renderView();
  });
  $("#headRight").addEventListener("click", onViewClick);
  $("#planModal").addEventListener("click", onPlanClick);
  $("#planModal").addEventListener("change", onPlanChange);

  // живые таймеры без полной перерисовки
  setInterval(() => {
    for (const el of $$("[data-live]")) {
      const t = S.tasks.find(x => x.id === el.dataset.live);
      if (t) el.textContent = fmtClock(spentNow(t));
    }
  }, 1000);

  const detail = $("#detail");
  detail.addEventListener("input", onDetailInput);
  detail.addEventListener("change", e => { if (e.target.matches("select, input[type=date], input[type=time]")) { onDetailInput(e); renderDetail(); } });
  detail.addEventListener("click", onDetailClick);
  detail.addEventListener("keydown", onDetailKey);

  $("#themeBtn").onclick = toggleTheme;
  $("#burger").onclick = () => $("#sidebar").classList.toggle("open");
  $("#helpBtn").onclick = () => ($("#helpModal").hidden = false);
  $("#helpModal").addEventListener("click", e => { if (e.target.id === "helpModal" || e.target.dataset.close !== undefined) $("#helpModal").hidden = true; });
  $("#notifyBtn").onclick = async () => {
    if (!("Notification" in window)) return toast("Браузер не поддерживает уведомления");
    const p = await Notification.requestPermission();
    toast(p === "granted" ? "Напоминания о дедлайнах включены 🔔" : "Уведомления запрещены в браузере");
  };
  $("#importFile").addEventListener("change", async e => {
    const f = e.target.files[0];
    if (!f) return;
    try {
      const data = JSON.parse(await f.text());
      if (!confirm("Заменить все текущие данные данными из файла?")) return;
      await api("POST", "/import", data);
      await load();
      toast("Данные импортированы");
    } catch { toast("Не удалось прочитать файл"); }
    e.target.value = "";
  });

  document.addEventListener("keydown", e => {
    const typing = e.target.matches("input, textarea, select");
    if (e.key === "Escape") {
      if (!$("#helpModal").hidden) $("#helpModal").hidden = true;
      else if (!$("#planModal").hidden) { $("#planModal").hidden = true; render(); }
      else if (S.selected) { S.selected = null; render(); }
      else if (typing) e.target.blur();
      $("#sidebar").classList.remove("open");
      return;
    }
    if (typing || e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === "n" || e.key === "т") { e.preventDefault(); $("#qaInput").focus(); }
    else if (e.key === "/" || e.key === ".") { e.preventDefault(); $("#search").focus(); }
    else if (e.key === "t" || e.key === "е") toggleTheme();
    else if (e.key === "?") $("#helpModal").hidden = false;
    else if (e.key === "p" || e.key === "з") openPlan();
    else if (/^[1-9]$/.test(e.key)) setView(VIEW_KEYS[+e.key - 1]);
  });

  pomo.init();
  // ссылка вида /#schedule открывает нужный раздел (так делает мини-панель в трее)
  const fromHash = () => {
    const v = decodeURIComponent(location.hash.slice(1));
    if (v && (TITLES[v] || v.startsWith("list:") || v.startsWith("tag:"))) setView(v);
  };
  window.addEventListener("hashchange", fromHash);
  window.openView = v => { if (v) setView(v); };
  load().then(() => {
    fromHash();
    checkDeadlines();
    setInterval(checkDeadlines, 30000);
  });
  // перерисовка после полуночи, чтобы «Сегодня» не устаревало
  let day = today();
  setInterval(() => { if (today() !== day) { day = today(); load(); } }, 60000);
  // переносы задач делает сервер — подхватываем их, когда окно снова в фокусе
  window.addEventListener("focus", () => load());
}

init();
