"use strict";
// Телефон: нижняя панель, разделы «План», «Таймер», «Ещё», меню «⋮» и шторка добавления.
// Включается на узком экране (в приложении Android всегда); на компьютере ничего не меняется.

const mobileMQ = matchMedia("(max-width: 760px)");
function mobile() { return document.documentElement.classList.contains("m"); }
function applyMobile() {
  document.documentElement.classList.toggle("m", mobileMQ.matches || location.search.includes("mobile"));
}

// разделы, которые на телефоне не нужны: перетаскивание мышью, широкие таблицы, рисование
const DESKTOP_ONLY = ["board", "schedule", "matrix", "sketch"];
const MOBILE_ONLY = ["plan", "timer", "more"];
const SORTS = [["manual", "Вручную"], ["smart", "Умная (срочность)"], ["priority", "По приоритету"], ["due", "По сроку"]];

function tabOf(v) { return ["today", "plan", "timer"].includes(v) ? v : "more"; }
function renderTabs() {
  const tab = tabOf(S.view);
  for (const b of $$("#mTabs [data-tab]")) b.classList.toggle("on", b.dataset.tab === tab);
}

function shortDate() {
  return cap(new Intl.DateTimeFormat("ru", { weekday: "short", day: "numeric", month: "long" }).format(new Date()));
}

/* ================= разделы ================= */
function viewPlan() {
  const month = S.planMode === "month";
  if (month && !S.calPick) S.calPick = today();
  return `<div class="seg m-seg"><button class="${month ? "" : "on"}" data-plan="week">Неделя</button>
    <button class="${month ? "on" : ""}" data-plan="month">Месяц</button></div>` + (month ? viewCalendar() : viewUpcoming());
}

// таймер — тот же блок Помодоро, что в боковой панели: переносим его в раздел и обратно
function viewTimer() { return `<div class="m-timer" id="mTimerSlot"></div>`; }
function mountTimer() { $("#mTimerSlot")?.appendChild($("#pomodoro")); }
function restoreTimer() {
  const p = $("#pomodoro");
  if (p.parentElement !== $("#sidebar")) $("#sidebar").appendChild(p);
}

function viewMore() {
  const c = counts(), a = active();
  const row = (view, ic, name, n) => `<button class="m-row" data-view="${view}">${ic}<span>${name}</span>${n ? `<b>${n}</b>` : ""}</button>`;
  const tags = {};
  for (const t of a) for (const g of t.tags) tags[g] = (tags[g] || 0) + 1;
  return `<label class="m-search">${icon("search")}<input id="mSearch" type="search" placeholder="Поиск задач" value="${esc(S.mQuery || "")}"></label>
    <div id="mSearchRes"></div>
    <div id="mMore">
      <div class="m-group">
        ${row("inbox", icon("inbox"), "Входящие", c.inbox)}
        ${row("all", icon("list"), "Все задачи", c.all)}
        ${row("habits", icon("flame"), "Привычки")}
        ${row("stats", icon("chart"), "Статистика")}
        ${row("completed", icon("done"), "Выполненные")}
      </div>
      <div class="group-head">Списки</div>
      <div class="m-group">
        ${S.lists.map(l => row("list:" + l.id, `<i class="dot" style="background:${l.color}"></i>`, esc(l.name), a.filter(t => t.listId === l.id).length)).join("")}
        <button class="m-row m-muted" data-m-newlist>${icon("plus")}<span>Новый список</span></button>
      </div>
      ${Object.keys(tags).length ? `<div class="group-head">Теги</div><div class="tags-cloud">${Object.entries(tags).sort((x, y) => y[1] - x[1])
        .map(([g, n]) => `<button class="chip tag" data-view="tag:${esc(g)}">#${esc(g)} ${n}</button>`).join("")}</div>` : ""}
      <div class="m-group">${row("settings", icon("settings"), "Настройки")}</div>
    </div>`;
}

// поиск во «Ещё» перерисовывает только результаты, чтобы поле ввода не теряло фокус
function fillSearch() {
  const q = S.mQuery || "";
  const more = $("#mMore");
  if (!more) return;
  more.hidden = !!q;
  $("#mSearchRes").innerHTML = q ? viewSearch(q) : "";
}

function viewHabitsMobile() {
  const td = today();
  const days = Array.from({ length: 7 }, (_, i) => addDays(td, i - 6));
  return `<div class="m-habits">${S.habits.map(h => `<div class="m-habit">
      <div class="m-habit-top"><i class="dot" style="background:${h.color}"></i><b>${esc(h.name)}</b>
        <span class="streak">${icon("flame")} ${streak(h)}</span>
        <button class="icon-btn" data-del-habit="${h.id}" aria-label="Удалить привычку">${TRASH_SVG}</button></div>
      <div class="m-week">${days.map(d => `<button class="hcheck ${h.done[d] ? "on" : ""} ${d === td ? "today" : ""}" style="--c:${h.color}"
        data-habit="${h.id}" data-date="${d}"><small>${fmtDow.format(parseYmd(d))}</small>${parseYmd(d).getDate()}</button>`).join("")}</div>
    </div>`).join("") || empty("flame", "Добавь привычку, которую хочешь выработать: зарядка, чтение, вода…")}</div>
    <form class="add-habit" id="addHabit"><input class="input" name="name" placeholder="Новая привычка" required>
    <button class="btn btn-primary">Добавить</button></form>`;
}

/* ================= меню «⋮» ================= */
function menuItems() {
  const v = S.view, items = [];
  if (v === "today") items.push(["plan", icon("sunrise") + " Спланировать день"]);
  if (v === "completed") items.push(["clearDone", icon("trash") + " Очистить выполненные"]);
  if (v.startsWith("list:")) items.push(["delList", icon("trash") + " Удалить список"]);
  if (["today", "inbox", "all"].includes(v) || v.startsWith("list:") || v.startsWith("tag:")) {
    items.push(["", "Сортировка"]);
    for (const [k, n] of SORTS) items.push(["sort:" + k, `<span class="m-mark">${S.sort === k ? icon("check") : ""}</span>${n}`]);
  }
  return items;
}

function menuButton() {
  return menuItems().length ? `<button class="icon-btn m-kebab" data-act="menu" aria-label="Меню">${icon("dots")}</button>` : "";
}

function openMenu() {
  $("#mSheet").innerHTML = `<div class="m-menu">${menuItems().map(([k, h]) =>
    k ? `<button data-m="${k}">${h}</button>` : `<div class="group-head">${h}</div>`).join("")}</div>`;
  $("#mSheet").hidden = false;
  $("#mBackdrop").hidden = false;
}

function closeSheets() {
  $("#mSheet").hidden = true;
  $("#mBackdrop").hidden = true;
  if (document.body.classList.contains("m-adding")) {
    document.body.classList.remove("m-adding");
    $("#qaInput").blur();
  }
}

async function onSheetClick(e) {
  const k = e.target.closest("[data-m]")?.dataset.m;
  if (!k) return;
  closeSheets();
  if (k === "plan") openPlan();
  else if (k === "clearDone") clearCompleted();
  else if (k === "delList") deleteList(S.view.slice(5));
  else if (k.startsWith("sort:")) { S.sort = k.slice(5); safeSet("sort", S.sort); renderView(); }
}

/* ================= добавление ================= */
function openAdd() {
  document.body.classList.add("m-adding");
  $("#mBackdrop").hidden = false;
  $("#qaInput").focus();
}

// клики в разделе, которые есть только на телефоне
function onMobileClick(e) {
  const plan = e.target.closest("[data-plan]");
  if (plan) { S.planMode = plan.dataset.plan; safeSet("planMode", S.planMode); renderView(); return true; }
  if (e.target.closest("[data-m-newlist]")) {
    const name = (prompt("Название списка") || "").trim();
    if (name) api("POST", "/lists", { name, color: PALETTE[S.lists.length % PALETTE.length] })
      .then(l => { S.lists.push(l); setView("list:" + l.id); });
    return true;
  }
  return false;
}

// «Назад» на Android: закрыть шторку, задачу или окно, потом подняться к «Ещё» или «Сегодня».
// true — нажатие обработано, иначе приложение сворачивается.
window.clepsyBack = () => {
  if (!$("#mBackdrop").hidden) { closeSheets(); return true; }
  if (!$("#helpModal").hidden) { $("#helpModal").hidden = true; return true; }
  if (!$("#planModal").hidden) { $("#planModal").hidden = true; render(); return true; }
  if (S.selected) { S.selected = null; render(); return true; }
  if (S.view !== "today") { setView(tabOf(S.view) === "more" && S.view !== "more" ? "more" : "today"); return true; }
  return false;
};

function initMobile() {
  S.planMode = safeGet("planMode") || "week";
  const qa = $("#qaInput");
  const desktopHint = qa.placeholder;
  const hint = () => {
    qa.placeholder = mobile() ? "Задача… завтра в 18:00 !2 #тег" : desktopHint;
    $("#helpTitle").textContent = mobile() ? "Быстрый ввод" : "Горячие клавиши и быстрый ввод";
  };
  hint();
  mobileMQ.addEventListener("change", () => {
    applyMobile();
    hint();
    closeSheets();
    if (!mobile() && MOBILE_ONLY.includes(S.view)) setView("today");
    else if (mobile() && DESKTOP_ONLY.includes(S.view)) setView("today");
    else render();
  });
  $("#mTabs").addEventListener("click", e => {
    const t = e.target.closest("[data-tab]")?.dataset.tab;
    if (!t) return;
    if (t === "add") return openAdd();
    S.selected = null;
    setView(t);
  });
  $("#mBackdrop").addEventListener("click", closeSheets);
  $("#mSheet").addEventListener("click", onSheetClick);
  $("#view").addEventListener("input", e => {
    if (e.target.id !== "mSearch") return;
    S.mQuery = e.target.value.trim();
    fillSearch();
  });
}
