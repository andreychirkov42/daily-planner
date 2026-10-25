"use strict";
// Панель задачи справа.

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
      <button class="btn btn-sm ${t.done ? "" : "btn-primary"}" data-d="toggle">${t.done ? icon("reset") + " Вернуть" : icon("check") + " Выполнить"}</button>
      <span class="grow"></span>
      <button class="icon-btn" data-d="focus" title="Фокус-сессия на эту задачу">${icon("target")}</button>
      <button class="icon-btn ${t.pinned ? "on" : ""}" data-d="pin" title="${t.pinned ? "Открепить" : "Закрепить"}">${icon("pin")}</button>
      <button class="icon-btn" data-d="delete" title="Удалить">${TRASH_SVG}</button>
      <button class="icon-btn" data-d="close" title="Закрыть (Esc)">${icon("x")}</button>
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
      ${PRI_NAMES.map((n, i) => `<button class="p${i} ${t.priority === i ? "on" : ""}" data-pri="${i}">${i ? icon("flag") + " " : ""}${n}</button>`).join("")}
    </div></div>

    <div class="field"><label>Подзадачи ${t.subtasks.length ? `· ${sd}/${t.subtasks.length}` : ""}</label>
      ${t.subtasks.length ? `<div class="progress"><i style="width:${sd / t.subtasks.length * 100}%"></i></div>` : ""}
      <div class="subtasks">${t.subtasks.map((s, i) => `<div class="sub ${s.done ? "done" : ""}">
        <input type="checkbox" data-sub-check="${i}" ${s.done ? "checked" : ""}>
        <input class="input" data-sub-title="${i}" value="${esc(s.title)}">
        <button class="icon-btn" data-sub-del="${i}" title="Удалить подзадачу">${icon("x")}</button></div>`).join("")}
        <input class="input" id="newSub" placeholder="+ Добавить подзадачу и нажать Enter">
      </div></div>

    <div class="field"><label>Заметки</label><textarea class="input" data-f="notes" placeholder="Детали, ссылки, мысли…">${esc(t.notes)}</textarea></div>

    <details class="more" data-more ${detailMoreOpen() ? "open" : ""}>
      <summary><span>Дополнительно</span><span class="more-sum">${moreSummary(t)}</span></summary>
      <div class="more-body">
    <div class="row">
      <div class="field"><label>Список</label><select class="input" data-f="listId">
        <option value="">Входящие</option>${S.lists.map(l => `<option value="${l.id}" ${t.listId === l.id ? "selected" : ""}>${esc(l.name)}</option>`).join("")}
      </select></div>
      <div class="field"><label>Повтор</label><select class="input" data-f="repeat">
        ${Object.entries(REPEAT_NAMES).map(([k, n]) => `<option value="${k}" ${t.repeat === k ? "selected" : ""}>${n}</option>`).join("")}
      </select></div>
    </div>

    ${t.done ? "" : `<div class="field"><label>Статус</label><div class="seg">
      <button class="${t.status !== "doing" ? "on" : ""}" data-status="">${icon("circle")} К выполнению</button>
      <button class="${t.status === "doing" ? "on" : ""}" data-status="doing">${icon("progress")} В работе</button>
    </div></div>`}

    <div class="field"><label>Время</label>
      <div class="est-row">${[15, 30, 60, 120, 240].map(m => `<button class="chip ${t.estimate === m ? "on" : ""}" data-est="${m}">${fmtMin(m)}</button>`).join("")}
        <input class="input" type="number" min="0" step="5" data-f="estimate" value="${t.estimate || ""}" placeholder="мин"></div>
      <div class="timer-box">
        ${t.done ? "" : `<button class="btn btn-sm ${t.timerStart ? "" : "btn-primary"}" data-d="timer">${t.timerStart ? icon("pause") + " Стоп" : icon("play") + " Старт"}</button>`}
        <b data-live="${t.id}">${fmtClock(spentNow(t))}</b>
        <span class="s">${t.estimate ? `затрачено из ${fmtMin(t.estimate)} по оценке` : "затрачено · оценка не задана"}</span>
      </div></div>

    <div class="field"><label>Если не выполнена в срок</label><select class="input" data-f="carry">
      ${Object.entries(CARRY_NAMES).map(([k, n]) => `<option value="${k}" ${(t.carry || "") === k ? "selected" : ""} ${k === "shift" && !seriesOf(t) ? "disabled" : ""}>${n}</option>`).join("")}
    </select>
    <span class="hint">${carryHint(t)}</span></div>

    <div class="field"><label>Теги</label><input class="input" data-f="tags" value="${esc(t.tags.join(", "))}" placeholder="дом, срочно, идеи"></div>
      </div>
    </details>

    <div class="d-foot">${t.done ? "" : `${icon("zap")} Срочность ${urgency(t)} <span class="urg">(формула Taskwarrior)</span><br>`}Создана ${new Date(t.createdAt).toLocaleString("ru")}${t.completedAt ? `<br>Выполнена ${new Date(t.completedAt).toLocaleString("ru")}` : ""}</div>
  </div>`;
  autosize($(".d-title", panel));
}

function detailMoreOpen() { return safeGet("detailMore") === "1"; }

// moreSummary — что выставлено в «Дополнительно», чтобы было видно и в свёрнутом виде
function moreSummary(t) {
  const parts = [];
  const l = listById(t.listId);
  if (l) parts.push(esc(l.name));
  if (t.status === "doing" && !t.done) parts.push("в работе");
  if (t.estimate) parts.push(fmtMin(t.estimate));
  if (t.repeat) parts.push(icon("repeat") + " " + REPEAT_NAMES[t.repeat].toLowerCase());
  if (t.carry === "shift") parts.push(icon("shift") + " план");
  if (t.carry === "move") parts.push(icon("move") + " перенос");
  if (t.tags.length) parts.push(t.tags.map(g => "#" + esc(g)).join(" "));
  return parts.join(" · ");
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
