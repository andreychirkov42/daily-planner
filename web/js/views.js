"use strict";
// Разделы: доска, расписание дня, календарь, матрица, привычки, статистика.

/* ================= канбан (идея из Vikunja) ================= */
function viewBoard() {
  const inList = t => !S.boardList || t.listId === S.boardList;
  const a = active().filter(inList);
  const weekAgo = addDays(today(), -7);
  const done = S.tasks.filter(t => t.done && inList(t) && t.completedAt && ymd(new Date(t.completedAt)) >= weekAgo)
    .sort((x, y) => y.completedAt.localeCompare(x.completedAt));
  const cols = [
    ["todo", icon("circle") + " К выполнению", "var(--ink-3)", sortTasks(a.filter(t => t.status !== "doing"))],
    ["doing", icon("progress") + " В работе", "var(--warn)", sortTasks(a.filter(t => t.status === "doing"))],
    ["done", icon("done") + " Готово · 7 дней", "var(--ok)", done],
  ];
  return `<p class="hint" style="margin:0 0 12px">Перетаскивай карточки между колонками. Запуск таймера ${icon("play")} сам переносит задачу «В работу».</p>
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
      <button class="unsched" data-unsched-id="${t.id}" title="Убрать со шкалы">${icon("x")}</button></div>`;
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
        <div class="tasks">${sortTasks(unsched).map(t => taskHTML(t, { drag: true, hideList: true })).join("") || '<p class="hint">Всё разложено по времени</p>'}</div>
        ${noDate.length ? `<div class="group-head" style="margin-top:14px">Без даты</div><div class="tasks">${noDate.map(t => taskHTML(t, { drag: true })).join("")}</div>` : ""}
        <p class="hint">Перетащи задачу на шкалу. Длина блока — оценка времени (по умолчанию 30 мин). Верни блок сюда, чтобы снять время.</p>
      </div>
      <div class="timeline" id="timeline">${slots}${blocksHTML}
        ${isToday && nowTop >= 0 ? `<div class="now-line" style="top:${nowTop}px"></div>` : ""}</div>
    </div>`;
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
      <div class="tasks">${items.length ? sortTasks(items).map(t => taskHTML(t)).join("") : `<div class="empty" style="padding:20px">Задач нет — ${mobile() ? "нажми «+» внизу" : "добавь через строку выше"}</div>`}</div></div>`;
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
    [icon("flame") + " Сделать сейчас", "Важно и срочно", "var(--danger)", a.filter(t => imp(t) && urg(t))],
    [icon("date") + " Запланировать", "Важно, но не срочно", "var(--accent)", a.filter(t => imp(t) && !urg(t))],
    [icon("share") + " Делегировать", "Срочно, но не важно", "var(--warn)", a.filter(t => !imp(t) && urg(t))],
    [icon("archive") + " Отложить", "Не важно и не срочно", "var(--ink-3)", a.filter(t => !imp(t) && !urg(t))],
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
      ${days.map(d => `<td><button class="hcheck ${h.done[d] ? "on" : ""}" style="--c:${h.color}" data-habit="${h.id}" data-date="${d}">${h.done[d] ? icon("check") : ""}</button></td>`).join("")}
      <td class="streak">${icon("flame")} ${streak(h)}</td>
      <td><div class="heat">${last30.map(d => `<i style="${h.done[d] ? `background:${h.color}` : ""}" title="${d}"></i>`).join("")}</div>
        <span style="font-size:11px;color:var(--ink-3)">${rate}%</span></td>
      <td><button class="icon-btn" data-del-habit="${h.id}" title="Удалить">${TRASH_SVG}</button></td></tr>`;
  }
  html += `</tbody></table>${S.habits.length ? "" : empty("flame", "Добавь привычку, которую хочешь выработать: зарядка, чтение, вода…")}</div>
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
    ["Фокус-сессии", pomos, "завершено сегодня"],
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
