"use strict";
// Обновления: карточка в «Настройках» и подсказка, когда новая версия готова.
// Windows — сервер сам скачивает и ставит новую версию (/api/update); Android — оболочка на Java (ClepsyAndroid).

let updateInfo = null; // current, latest, available, state, error, checked, updatedFrom, canInstall
const onAndroid = () => !!(window.ClepsyAndroid && ClepsyAndroid.updateState);
let updateLoading = false;

async function loadUpdate() {
  if (updateLoading) return; // карточка просит данные при каждой отрисовке — не плодим запросы
  updateLoading = true;
  try {
    updateInfo = onAndroid() ? JSON.parse(ClepsyAndroid.updateState()) : await (await fetch("/api/update")).json();
  } catch { /* сервер перезапускается */ }
  updateLoading = false;
  onUpdateInfo();
}

// новое состояние: перерисовать настройки и, если версия скачана, один раз предложить поставить
function onUpdateInfo() {
  const u = updateInfo;
  if (!u) return;
  if (S.view === "settings") renderView();
  if (u.updatedFrom && safeGet("updatedNotice") !== u.current) {
    safeSet("updatedNotice", u.current);
    toast(`Clepsy обновлён до версии ${u.current}`);
  }
  if (u.state === "ready" && safeGet("updateOffered") !== u.latest) {
    safeSet("updateOffered", u.latest);
    toast(`Готова новая версия ${u.latest}`, onAndroid() ? "Установить" : "Обновить", installUpdate, 15000);
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
  if (onAndroid()) { ClepsyAndroid.installUpdate(); return; }
  updateInfo = { ...updateInfo, state: "installing" };
  if (S.view === "settings") renderView();
  toast("Ставлю обновление — Clepsy перезапустится");
  api("POST", "/update/install").catch(() => {});
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
    ? `<button class="btn btn-sm btn-primary" data-upd-install>${icon("download")} ${onAndroid() ? "Установить" : "Обновить"} ${esc(u.latest)}</button>`
    : `<button class="btn btn-sm" data-upd-check ${busy ? "disabled" : ""}>${icon("reset")} Проверить сейчас</button>`;
  return `<section class="set-card"><h3>${icon("download")} Обновления</h3>
    ${setRow("Обновляться автоматически", a.phone || onAndroid()
      ? "Clepsy сам скачает новую версию с GitHub и предложит установить"
      : "Clepsy сам скачает новую версию с GitHub и перезапустится, когда окно закрыто и таймер не идёт",
      `<label class="switch"><input type="checkbox" data-set-autoupdate ${a.autoUpdate !== false ? "checked" : ""}><i></i></label>`)}
    ${setRow(`Версия ${esc(u.current)}`, updateStatus(u), btn)}
  </section>`;
}

function onUpdateClick(e) {
  if (e.target.closest("[data-upd-check]")) { checkUpdate(); return true; }
  if (e.target.closest("[data-upd-install]")) { installUpdate(); return true; }
  return false;
}

// при открытии и потом раз в 30 минут — чтобы окно узнало о скачанной версии
function initUpdate() {
  loadUpdate();
  setInterval(() => { if (!document.hidden) loadUpdate(); }, 30 * 60 * 1000);
}
