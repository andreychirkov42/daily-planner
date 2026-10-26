"use strict";
// Раздел «Настройки»: тема, автозапуск, поведение крестика, уведомления, рабочий день, Помодоро, данные.

let appInfo = null; // ответ /api/settings: closeAction, dayEnd, autostart, version, dataPath…

async function loadSettings() {
  try { appInfo = await api("GET", "/settings"); } catch { /* сервер перезапускается */ }
  if (appInfo) S.app = { ...S.app, closeAction: appInfo.closeAction, dayEnd: appInfo.dayEnd };
  if (S.view === "settings") renderView();
}

async function saveSettings(patch) {
  appInfo = await api("PUT", "/settings", patch);
  S.app = { ...S.app, closeAction: appInfo.closeAction, dayEnd: appInfo.dayEnd };
  renderView();
  toast("Сохранено");
}

function themeChoice() { return safeGet("theme") || ""; }

function notifyState() {
  if (!("Notification" in window)) return ["unsupported", "Этот браузер не умеет показывать уведомления"];
  if (Notification.permission === "granted") return ["on", "Включены — придут, когда наступит срок задачи"];
  if (Notification.permission === "denied") return ["denied", "Запрещены в настройках браузера"];
  return ["off", "Выключены"];
}

function setRow(title, hint, control) {
  return `<div class="set-row"><div class="set-text"><b>${title}</b>${hint ? `<span>${hint}</span>` : ""}</div><div class="set-ctl">${control}</div></div>`;
}

function seg(attr, current, options) {
  return `<div class="seg set-seg">${options.map(([v, label]) => `<button class="${current === v ? "on" : ""}" ${attr}="${v}">${label}</button>`).join("")}</div>`;
}

function viewSettings() {
  if (!appInfo) { loadSettings(); return `<p class="hint">Загружаю настройки…</p>`; }
  const a = appInfo;
  const [nState, nText] = notifyState();
  const st = pomo.client() && pomo.client().state();
  const keys = [["N", "новая задача"], ["/", "поиск"], ["1–9", "разделы"], ["P", "спланировать день"], ["D", "холст"], ["T", "сменить тему"], ["Esc", "закрыть панель"], ["?", "подсказка по быстрому вводу"]];

  return `<div class="settings">
    <section class="set-card"><h3>${icon("theme")} Внешний вид</h3>
      ${setRow("Тема", "«Как в системе» подстраивается под тему Windows", seg("data-theme-set", themeChoice(), [["", "Как в системе"], ["light", "Светлая"], ["dark", "Тёмная"]]))}
    </section>

    ${a.desktop ? `<section class="set-card"><h3>${icon("monitor")} Программа</h3>
      ${setRow("Запускать вместе с Windows", "Clepsy тихо появится в трее при входе в систему",
        `<label class="switch"><input type="checkbox" data-set-autostart ${a.autostart ? "checked" : ""}><i></i></label>`)}
      ${setRow("Крестик окна", "Что делать, когда закрываешь окно программы",
        seg("data-close-set", a.closeAction, [["tray", "Свернуть в трей"], ["quit", "Закрыть программу"]]))}
    </section>` : ""}

    <section class="set-card"><h3>${icon("bell")} Уведомления</h3>
      ${setRow("Напоминания о дедлайнах", nText,
        nState === "off" ? `<button class="btn btn-sm btn-primary" data-set-notify>Включить</button>`
          : `<span class="set-badge ${nState}">${{ on: "Включены", denied: "Запрещены", unsupported: "Недоступны" }[nState]}</span>`)}
    </section>

    <section class="set-card"><h3>${icon("clock")} Рабочий день</h3>
      ${setRow("Конец дня", "От него считается свободное время в полосе загрузки и в «Спланировать день»",
        `<input class="input set-time" type="time" data-set-dayend value="${esc(a.dayEnd)}">`)}
    </section>

    <section class="set-card"><h3>${icon("timer")} Помодоро</h3>
      ${st ? `<div class="set-pomo" id="setPomo">${pomoSettingsHTML(st.settings)}</div>` : `<p class="hint">Таймер ещё загружается…</p>`}
    </section>

    <section class="set-card"><h3>${icon("archive")} Данные</h3>
      ${setRow("Где хранятся", `<code class="set-path">${esc(a.dataPath)}</code>`,
        a.canReveal ? `<button class="btn btn-sm" data-set-reveal>${icon("monitor")} Показать в папке</button>` : "")}
      ${setRow("Резервная копия", "Все задачи, списки, привычки и заметки одним JSON-файлом",
        `<a class="btn btn-sm" href="/api/export">${icon("download")} Экспорт</a>
         <label class="btn btn-sm">${icon("upload")} Импорт<input type="file" data-set-import accept="application/json" hidden></label>`)}
    </section>

    <section class="set-card"><h3>${icon("keyboard")} Горячие клавиши</h3>
      <div class="set-keys">${keys.map(([k, d]) => `<kbd>${k}</kbd><span>${d}</span>`).join("")}</div>
      <button class="btn btn-sm" data-set-help style="margin-top:12px">Как писать задачи в строке добавления</button>
    </section>

    <p class="set-about">Clepsy ${esc(a.version)} · <button class="link-btn" data-set-github>исходный код на GitHub</button></p>
  </div>`;
}

// bindSettings вешает обработчики после каждой отрисовки раздела
function bindSettings(root) {
  const box = root.querySelector("#setPomo");
  if (box) bindPomoSettings(box, pomo.client(), () => { toast("Длительности сохранены"); renderView(); });

  root.querySelector("[data-set-dayend]")?.addEventListener("change", e => {
    if (e.target.value) saveSettings({ dayEnd: e.target.value });
  });
  root.querySelector("[data-set-autostart]")?.addEventListener("change", e => saveSettings({ autostart: e.target.checked }));
  root.querySelector("[data-set-import]")?.addEventListener("change", async e => {
    const f = e.target.files[0];
    if (!f) return;
    try {
      const data = JSON.parse(await f.text());
      if (!confirm("Заменить все текущие данные данными из файла?")) return;
      await api("POST", "/import", data);
      await load();
      toast("Данные импортированы");
    } catch { toast("Не удалось прочитать файл"); }
  });
}

function onSettingsClick(e) {
  const b = e.target.closest("[data-theme-set], [data-close-set], [data-set-notify], [data-set-reveal], [data-set-help], [data-set-github]");
  if (!b) return false;
  const d = b.dataset;
  if (d.themeSet !== undefined) {
    if (d.themeSet) safeSet("theme", d.themeSet); else { try { localStorage.removeItem("theme"); } catch { /* приватный режим */ } }
    applyTheme();
    renderView();
  } else if (d.closeSet) saveSettings({ closeAction: d.closeSet });
  else if (d.setNotify !== undefined) enableNotifications().then(renderView);
  else if (d.setReveal !== undefined) api("POST", "/app/reveal");
  else if (d.setHelp !== undefined) $("#helpModal").hidden = false;
  else if (d.setGithub !== undefined) {
    // в приложении Android ссылку открывает сама оболочка: серверу на телефоне браузер не запустить
    if (window.ClepsyAndroid && appInfo) ClepsyAndroid.openUrl(appInfo.repo);
    else api("POST", "/app/github");
  }
  return true;
}

async function enableNotifications() {
  if (!("Notification" in window)) return toast("Браузер не поддерживает уведомления");
  const p = await Notification.requestPermission();
  toast(p === "granted" ? "Напоминания о дедлайнах включены" : "Уведомления запрещены в браузере");
}
