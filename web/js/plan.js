"use strict";
// Ритуал «Спланировать день» и учёт времени по задаче.

/* ================= планирование дня (ритуал из Sunsama) ================= */
function openPlan() { $("#planModal").hidden = false; renderPlan(); }

function renderPlan() {
  const td = today(), tm = addDays(td, 1);
  const a = active();
  const overdue = a.filter(t => t.due && t.due < td);
  const todayT = sortTasks(a.filter(t => t.due === td));
  const cand = a.filter(t => !t.due || t.due > td).sort((x, y) => urgency(y) - urgency(x)).slice(0, 8);
  const estSel = t => `<select class="input btn-sm" style="width:auto" data-plan-est="${t.id}">
    <option value="0">оценка</option>${[15, 30, 45, 60, 90, 120, 180, 240].map(m => `<option value="${m}" ${t.estimate === m ? "selected" : ""}>${fmtMin(m)}</option>`).join("")}</select>`;
  const row = (t, btns) => `<div class="plan-row"><span class="t">${t.pinned ? icon("pin", "inline") + " " : ""}${esc(t.title)}</span>${btns}</div>`;

  $("#planCard").innerHTML = `<h3>${icon("sunrise")} Планирование дня</h3>
    <p class="hint" style="margin:0">Разбери хвосты, выбери главное и оцени время, чтобы день не оказался перегружен.</p>
    ${overdue.length ? `<div class="plan-step">1 · Просроченные (${overdue.length}) <button class="btn btn-sm" data-plan-all="1" style="margin-left:8px">Всё на сегодня</button></div>
      ${overdue.map(t => row(t, `<span class="meta">${humanDate(t.due)}</span>
        <button class="btn btn-sm" data-pd="${t.id}" data-to="${td}">Сегодня</button>
        <button class="btn btn-sm" data-pd="${t.id}" data-to="${tm}">Завтра</button>
        <button class="btn btn-sm" data-pd="${t.id}" data-to="">Без срока</button>`)).join("")}` : ""}
    <div class="plan-step">${overdue.length ? 2 : 1} · План на сегодня (${todayT.length})</div>
    ${todayT.map(t => row(t, `${estSel(t)}<button class="btn btn-sm" data-pd="${t.id}" data-to="${tm}" title="Перенести на завтра">${icon("move")} Завтра</button>`)).join("") || '<p class="hint">Пока пусто — добавь задачи из списка ниже</p>'}
    ${cand.length ? `<div class="plan-step">Что ещё взять · по срочности</div>
      ${cand.map(t => row(t, `<span class="meta">${icon("zap")} ${urgency(t)}${t.due ? " · " + humanDate(t.due) : ""}</span>
        <button class="btn btn-sm" data-pd="${t.id}" data-to="${td}">${icon("plus")} Сегодня</button>`)).join("")}` : ""}
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
  if (v && /^([01]?\d|2[0-3]):[0-5]\d$/.test(v.trim())) {
    const t = v.trim().padStart(5, "0");
    S.app = { ...S.app, dayEnd: t }; // сразу в интерфейсе, сохраняем в общих настройках
    api("PUT", "/settings", { dayEnd: t });
  }
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
  }
  render();
}
