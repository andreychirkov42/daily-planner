"use strict";
// Каркас страницы и карточки задач.

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
      <span class="count">${n || ""}</span><span class="list-del" data-del-list="${l.id}" title="Удалить список">${icon("x")}</span></button>`;
  }).join("") + `<div id="newListSlot"></div>`;

  const tags = {};
  for (const t of a) for (const g of t.tags) tags[g] = (tags[g] || 0) + 1;
  $("#tagsCloud").innerHTML = Object.entries(tags).sort((x, y) => y[1] - x[1])
    .map(([g, n]) => `<button class="chip tag" data-view="tag:${esc(g)}">#${esc(g)} ${n}</button>`).join("");
}

const TITLES = {
  today: "Сегодня", upcoming: "Предстоящие", inbox: "Входящие", all: "Все задачи", calendar: "Календарь",
  board: "Доска", schedule: "Расписание дня", sketch: "Холст",
  matrix: "Приоритеты", habits: "Привычки", stats: "Статистика", completed: "Выполненные",
};

function setView(v) {
  if (S.view === "sketch" && v !== "sketch") sketch.flush(); // уходим с холста — сохраняем сразу
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
  document.title = title + " · Clepsy";

  const noAdd = ["habits", "stats", "completed", "matrix", "sketch"].includes(v) && !S.search;
  $("#quickAdd").style.display = noAdd ? "none" : "";
  $("#qaPreview").style.display = noAdd ? "none" : "";

  const showSort = !["calendar", "habits", "stats", "matrix", "completed", "schedule", "sketch"].includes(v) || S.search;
  let head = showSort ? `<select class="input btn-sm" id="sortSel" style="width:auto" title="Сортировка">
      <option value="manual">Вручную</option><option value="smart">Умная (срочность)</option>
      <option value="priority">По приоритету</option><option value="due">По сроку</option>
    </select>` : "";

  const el = $("#view");
  if (S.search) el.innerHTML = viewSearch();
  else if (v === "today") { el.innerHTML = viewToday(); head = progressRing() + `<button class="btn btn-sm btn-primary" data-act="plan" title="P">${icon("sunrise")} Спланировать день</button>` + head; }
  else if (v === "board") {
    el.innerHTML = viewBoard();
    head = `<select class="input btn-sm" id="boardListSel" style="width:auto"><option value="">Все списки</option>
      ${S.lists.map(l => `<option value="${l.id}" ${S.boardList === l.id ? "selected" : ""}>${esc(l.name)}</option>`).join("")}</select>` + head;
  }
  else if (v === "schedule") el.innerHTML = viewSchedule();
  else if (v === "sketch") { el.innerHTML = ""; sketch.mount(el); }
  else if (v === "upcoming") el.innerHTML = viewUpcoming();
  else if (v === "inbox") el.innerHTML = viewGrouped(active().filter(t => !t.listId), "inbox", "Входящие пусты — всё разобрано");
  else if (v === "all") el.innerHTML = viewGrouped(active(), "list", "Нет активных задач");
  else if (v.startsWith("list:")) el.innerHTML = viewGrouped(active().filter(t => t.listId === v.slice(5)), "list", "В этом списке пока пусто");
  else if (v.startsWith("tag:")) el.innerHTML = viewGrouped(active().filter(t => t.tags.includes(v.slice(4))), "search", "Нет задач с этим тегом");
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
    meta.push(`<span class="chip ${t.done ? "" : cls}">${icon("date")} ${humanDate(t.due)}${t.dueTime ? " · " + t.dueTime : ""}</span>`);
  }
  if (t.repeat) meta.push(`<span class="chip" title="${REPEAT_NAMES[t.repeat]}">${icon("repeat")}</span>`);
  if (t.carry && !t.done) meta.push(`<span class="chip" title="${CARRY_NAMES[t.carry]}">${icon(t.carry === "shift" ? "shift" : "move")}</span>`);
  if (t.subtasks.length) meta.push(`<span class="chip">${icon("subtasks")} ${t.subtasks.filter(s => s.done).length}/${t.subtasks.length}</span>`);
  if (l && !opts.hideList) meta.push(`<span class="chip"><i class="dot" style="background:${l.color};margin:0"></i>${esc(l.name)}</span>`);
  for (const g of t.tags) meta.push(`<button class="chip tag" data-view="tag:${esc(g)}">#${esc(g)}</button>`);
  if (t.notes) meta.push(`<span class="chip" title="Есть заметка">${icon("note")}</span>`);
  if (t.timerStart) meta.unshift(`<span class="chip live">${icon("timer")} <span data-live="${t.id}">${fmtClock(spentNow(t))}</span></span>`);
  else if (t.estimate || t.spent) {
    const sp = Math.round((t.spent || 0) / 60);
    meta.push(`<span class="chip ${t.estimate && sp > t.estimate ? "over" : ""}" title="Затрачено / оценка">${icon("hourglass")} ${t.spent ? fmtMin(sp) + (t.estimate ? " / " : "") : ""}${t.estimate ? fmtMin(t.estimate) : ""}</span>`);
  }
  if (t.status === "doing" && !t.done && S.view !== "board") meta.unshift(`<span class="chip soon">${icon("progress")} В работе</span>`);
  if (S.sort === "smart" && !t.done) meta.push(`<span class="chip" title="Срочность по формуле Taskwarrior">${icon("zap")} ${urgency(t)}</span>`);
  const drag = opts.drag || (S.sort === "manual" && !t.done);

  return `<div class="task p${t.priority} ${S.enter.has(t.id) ? "enter" : ""} ${t.done ? "done" : ""} ${t.timerStart ? "running" : ""} ${S.selected === t.id ? "selected" : ""}" data-id="${t.id}"
      ${drag ? 'draggable="true"' : ""} style="--list-color:${l ? l.color : "transparent"}">
    <button class="check" data-act="toggle" title="Отметить">${CHECK_SVG}</button>
    <div class="t-body">
      <div class="t-title">${t.pinned ? '<span class="t-pin">' + icon("pin") + '</span>' : ""}<span class="t-text">${esc(t.title)}</span></div>
      ${meta.length ? `<div class="t-meta">${meta.join("")}</div>` : ""}
    </div>
    <div class="t-actions">
      ${t.done ? "" : `<button class="icon-btn ${t.timerStart ? "on" : ""}" data-act="timer" title="Учёт времени">${icon(t.timerStart ? "pause" : "play")}</button>`}
      <button class="icon-btn" data-act="pin" title="${t.pinned ? "Открепить" : "Закрепить"}">${icon("pin")}</button>
      <button class="icon-btn" data-act="del" title="Удалить">${TRASH_SVG}</button>
    </div>
  </div>`;
}

function group(title, tasks, cls = "", opts) {
  if (!tasks.length) return "";
  return `<div class="group"><div class="group-head ${cls}">${title} <span class="n">${tasks.length}</span></div>
    <div class="tasks">${sortTasks(tasks).map(t => taskHTML(t, opts)).join("")}</div></div>`;
}

function empty(name, text) { return `<div class="empty"><span class="big">${icon(name)}</span>${text}</div>`; }

function viewToday() {
  const td = today();
  const a = active();
  const overdue = a.filter(t => t.due && t.due < td);
  const now = a.filter(t => t.due === td);
  const done = S.tasks.filter(t => doneOn(t, td));
  let html = loadBar() + group("Просрочено", overdue, "overdue") + group("На сегодня", now) + group("Выполнено сегодня", done);
  if (!overdue.length && !now.length) html = empty("sun", done.length ? "Всё сделано на сегодня! Отличная работа" : "Задач на сегодня нет — добавь первую строкой выше") + html;
  html += `<div class="day-note"><h3>${icon("note")} Заметка дня <span class="saved" id="noteSaved"></span></h3>
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
  return html || empty("upcoming", "На ближайшую неделю ничего не запланировано");
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

function viewGrouped(tasks, iconName, emptyText) {
  if (!tasks.length) return empty(iconName, emptyText);
  const hideList = S.view.startsWith("list:");
  return dateBuckets(tasks).map(([t, arr, cls]) => group(t, arr, cls, { hideList })).join("");
}

function viewSearch() {
  const res = S.tasks.filter(t => matchesSearch(t, S.search));
  if (!res.length) return empty("search", `Ничего не найдено по запросу «${esc(S.search)}»`);
  return group("Активные", res.filter(t => !t.done)) + group("Выполненные", res.filter(t => t.done));
}

function viewCompleted() {
  const done = S.tasks.filter(t => t.done).sort((a, b) => (b.completedAt || "").localeCompare(a.completedAt || ""));
  if (!done.length) return empty("done", "Здесь появятся выполненные задачи");
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
