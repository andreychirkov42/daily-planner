"use strict";
// Обновления: карточка в «Настройках», окно «Что нового» и подсказка, когда новая версия готова.
// Windows — сервер сам скачивает и ставит новую версию (/api/update); Android — оболочка на Java (ClepsyAndroid).

let updateInfo = null; // current, latest, notes, available, state, error, checked, updatedFrom, canInstall
const onAndroid = () => !!(window.ClepsyAndroid && ClepsyAndroid.updateState);
let updateLoading = false, updatedRetries = 0;

async function loadUpdate() {
  if (updateLoading) return; // карточка просит данные при каждой отрисовке — не плодим запросы
  updateLoading = true;
  try {
    updateInfo = onAndroid() ? JSON.parse(ClepsyAndroid.updateState()) : await (await fetch("/api/update")).json();
  } catch { /* сервер перезапускается */ }
  updateLoading = false;
  onUpdateInfo();
}

// новое состояние: перерисовать настройки и один раз сказать о свежей или скачанной версии
function onUpdateInfo() {
  const u = updateInfo;
  if (!u) return;
  if (S.view === "settings" && !busyEditing()) renderView(); // не сбивать ввод в полях настроек
  if (u.updatedFrom && safeGet("updatedNotice") !== u.current) {
    // только что обновились: описание изменений придёт с первой проверкой — ждём её немного
    if (u.latest !== u.current && updatedRetries++ < 6) { setTimeout(loadUpdate, 5000); return; }
    safeSet("updatedNotice", u.current);
    if (u.latest === u.current && u.notes) toast(`Clepsy обновлён до версии ${u.current}`, { kind: "update", action: ["Что нового", () => showNotes(false)], ms: 15000 });
    else toast(`Clepsy обновлён до версии ${u.current}`, { kind: "update" });
  }
  if (u.state === "ready" && safeGet("updateOffered") !== u.latest) {
    safeSet("updateOffered", u.latest);
    toast(`Готова новая версия ${u.latest}`, { kind: "update", action: ["Подробнее", () => showNotes(true)], ms: 15000 });
  }
}
window.clepsyUpdate = info => { updateInfo = info; onUpdateInfo(); }; // зовёт оболочка Android

function checkUpdate() {
  if (onAndroid()) { ClepsyAndroid.checkUpdate(); return; }
  updateInfo = { ...updateInfo, state: "checking" };
  renderView();
  fetch("/api/update/check", { method: "POST" }).then(r => r.json()).then(u => { updateInfo = u; onUpdateInfo(); }).catch(() => {});
}

function installUpdate() {
  $("#notesModal").hidden = true;
  if (onAndroid()) { ClepsyAndroid.installUpdate(); return; }
  updateInfo = { ...updateInfo, state: "installing" };
  if (S.view === "settings") renderView();
  toast("Ставлю обновление — Clepsy перезапустится", { kind: "update" });
  api("POST", "/update/install").catch(() => {});
  // в браузере страница переживает перезапуск сервера — дождёмся новой версии и перезагрузимся
  const was = updateInfo.current;
  let tries = 0;
  const poll = setInterval(async () => {
    if (++tries > 40) return clearInterval(poll);
    try {
      const u = await (await fetch("/api/update")).json();
      if (u.current !== was) { clearInterval(poll); location.reload(); }
    } catch { /* сервер ещё перезапускается */ }
  }, 2000);
}

// mdLite — немного Markdown из описания релиза: заголовки ###, списки «- », **жирный**, `код`.
// Сначала экранируем всё, поэтому HTML из описания в страницу не попадёт.
function mdLite(text) {
  const inline = s => esc(s).replace(/\*\*(.+?)\*\*/g, "<b>$1</b>").replace(/`(.+?)`/g, "<code>$1</code>");
  let html = "", list = false;
  for (const raw of (text || "").split("\n")) {
    const line = raw.trim();
    const item = /^[-*] (.*)/.exec(line);
    if (!item && list) { html += "</ul>"; list = false; }
    if (item) { if (!list) { html += "<ul>"; list = true; } html += `<li>${inline(item[1])}</li>`; }
    else if (/^#{1,6} /.test(line)) html += `<h4>${inline(line.replace(/^#+ /, ""))}</h4>`;
    else if (line) html += `<p>${inline(line)}</p>`;
  }
  return html + (list ? "</ul>" : "");
}

// окно «Что нового»: для скачанной версии — с кнопкой «Обновить», для уже поставленной — просто описание
function showNotes(offer) {
  const u = updateInfo || {};
  const ready = offer && u.state === "ready";
  $("#notesCard").innerHTML = `<h3>${icon("download")} Что нового в Clepsy ${esc(u.latest || u.current)}</h3>
    <div class="notes-body">${mdLite(u.notes) || "<p>Описание изменений не пришло.</p>"}</div>
    <div class="notes-actions">
      ${ready ? `<button class="btn" data-notes-close>Позже</button>
        <button class="btn btn-primary" data-upd-install>${icon("download")} ${onAndroid() ? "Установить" : "Обновить"}</button>`
        : `<button class="btn btn-primary" data-notes-close>Понятно</button>`}
    </div>`;
  $("#notesModal").hidden = false;
}

function updateStatus(u) {
  const when = u.checked ? new Date(u.checked).toLocaleString("ru", { day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" }) : "";
  switch (u.state) {
    case "checking": return "Проверяю…";
    case "downloading": return `Скачиваю версию ${esc(u.latest)}…`;
    case "installing": return "Ставлю обновление…";
    case "ready": return `Версия ${esc(u.latest)} скачана и проверена` + (u.error ? ` · ${esc(u.error)}` : "");
    case "error": return `<span class="upd-err">${esc(u.error)}</span>`;
  }
  if (u.available && !u.canInstall) return `Есть версия ${esc(u.latest)} — скачай её на странице релизов`;
  if (u.latest) return `Это последняя версия` + (when ? ` · проверено ${when}` : "");
  return "";
}

function updateCard(a) {
  const u = updateInfo;
  if (!u) { loadUpdate(); return ""; }
  const busy = ["checking", "downloading", "installing"].includes(u.state);
  const btn = u.state === "ready"
    ? `<button class="btn btn-sm btn-primary" data-upd-notes>${icon("download")} ${onAndroid() ? "Установить" : "Обновить"} ${esc(u.latest)}</button>`
    : `<button class="btn btn-sm" data-upd-check ${busy ? "disabled" : ""}>${icon("reset")} Проверить сейчас</button>`;
  return `<section class="set-card"><h3>${icon("download")} Обновления</h3>
    ${setRow("Обновляться автоматически", a.phone || onAndroid()
      ? "Clepsy сам скачает новую версию с GitHub и предложит установить"
      : "Clepsy сам скачает новую версию с GitHub и перезапустится, когда окно закрыто и таймер не идёт",
      `<label class="switch"><input type="checkbox" data-set-autoupdate ${a.autoUpdate !== false ? "checked" : ""}><i></i></label>`)}
    ${setRow(`Версия ${esc(u.current)}`, updateStatus(u) +
      (u.notes ? ` · <button class="link-btn" data-upd-notes>что нового в ${esc(u.latest)}</button>` : ""), btn)}
  </section>`;
}

function onUpdateClick(e) {
  if (e.target.closest("[data-upd-check]")) { checkUpdate(); return true; }
  if (e.target.closest("[data-upd-notes]")) { showNotes(true); return true; }
  return false;
}

// при открытии и потом раз в 30 минут — чтобы окно узнало о скачанной версии
function initUpdate() {
  $("#notesModal").addEventListener("click", e => {
    if (e.target.id === "notesModal" || e.target.closest("[data-notes-close]")) $("#notesModal").hidden = true;
    else if (e.target.closest("[data-upd-install]")) installUpdate();
  });
  loadUpdate();
  setInterval(() => { if (!document.hidden) loadUpdate(); }, 30 * 60 * 1000);
}
