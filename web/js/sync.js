"use strict";
// Синхронизация телефона с ПК: карточка в «Настройках» и автосинхронизация на телефоне.
// На ПК — включить приём и показать адрес с кодом; на телефоне — ввести их и синхронизироваться.

let syncInfo = null; // ответ /api/sync: listen, code, addrs, port, peer, hasPeerCode, last, lastErr
let syncBusy = false, syncLoading = false;

async function loadSync() {
  if (syncLoading) return; // карточка просит данные при каждой отрисовке — не плодим запросы
  syncLoading = true;
  try { syncInfo = await (await fetch("/api/sync")).json(); } catch { /* сервер перезапускается */ }
  syncLoading = false;
  if (S.view === "settings" && syncInfo) renderView();
}

// Пока человек что-то вводит, рисует или правка ещё не ушла на сервер, данные не перезагружаем:
// load() перерисует страницу, и правка потеряется.
function busyEditing() {
  return !!S.selected || Object.keys(saveTimers).length > 0 || S.view === "sketch" ||
    !$("#planModal").hidden || !$("#mBackdrop").hidden ||
    !!(document.activeElement && document.activeElement.matches("input, textarea, select"));
}

function syncWhen() {
  if (!syncInfo || !syncInfo.last) return "ещё не было";
  const d = new Date(syncInfo.last);
  const hm = d.toLocaleTimeString("ru", { hour: "2-digit", minute: "2-digit" });
  return ymd(d) === today() ? "сегодня в " + hm : humanDate(ymd(d)).toLowerCase() + " в " + hm;
}

function syncCard(a) {
  const s = syncInfo;
  if (!s) { loadSync(); return ""; }
  if (a.phone) {
    return `<section class="set-card"><h3>${icon("share")} Синхронизация с компьютером</h3>
      ${setRow("Адрес компьютера", "Есть в настройках Clepsy на ПК, раздел «Телефон»",
        `<input class="input sync-input" data-sync-peer value="${esc(s.peer)}" placeholder="192.168.1.5" inputmode="url" autocomplete="off">`)}
      ${setRow("Код", s.hasPeerCode ? "Сохранён. Введи новый, только если на ПК его сменили" : "Восемь символов из настроек на ПК",
        `<input class="input sync-input" data-sync-code placeholder="${s.hasPeerCode ? "••••-••••" : "ABCD-EFGH"}" autocomplete="off" autocapitalize="characters">`)}
      <div class="sync-actions">
        <button class="btn btn-sm btn-primary" data-sync-save>${syncBusy ? "Синхронизирую…" : icon("reset") + " Синхронизировать"}</button>
        <span class="sync-status ${s.lastErr ? "err" : ""}">${s.lastErr ? esc(s.lastErr) : s.peer ? "Последняя: " + syncWhen() : ""}</span>
      </div>
      <p class="hint">Синхронизация идёт сама при открытии приложения и раз в несколько минут, когда компьютер доступен — дома по Wi-Fi или через Tailscale.</p>
    </section>`;
  }
  return `<section class="set-card"><h3>${icon("share")} Телефон</h3>
    ${setRow("Синхронизация с телефоном", "Clepsy на телефоне сможет забирать и отправлять задачи, когда он в одной сети с компьютером",
      `<label class="switch"><input type="checkbox" data-sync-listen ${s.listen ? "checked" : ""}><i></i></label>`)}
    ${s.listen ? `
      ${setRow("Адрес компьютера", "Введи на телефоне адрес из той же сети, что и телефон (обычно 192.168…)",
        `<span class="sync-addrs">${(s.addrs || []).map(x => `<code>${esc(x)}</code>`).join("") || "нет сети"}</span>`)}
      ${setRow("Код", "Его вводят на телефоне один раз",
        `<code class="sync-code">${esc(s.code)}</code><button class="btn btn-sm" data-sync-newcode>Новый код</button>`)}
      ${setRow("Последняя синхронизация", "", `<span class="sync-status">${syncWhen()}</span>`)}
      <p class="hint">Если телефон не подключается: Windows могла спросить, пускать ли Clepsy в сеть, — нужно разрешить для частных сетей.</p>` : ""}
  </section>`;
}

function bindSync(root) {
  root.querySelector("[data-sync-listen]")?.addEventListener("change", async e => {
    syncInfo = await api("PUT", "/sync", { listen: e.target.checked });
    renderView();
  });
}

function onSyncClick(e) {
  const b = e.target.closest("[data-sync-newcode], [data-sync-save]");
  if (!b) return false;
  if (b.dataset.syncNewcode !== undefined) {
    if (confirm("Сменить код? Телефон, подключённый со старым кодом, перестанет синхронизироваться, пока не введёшь новый.")) {
      api("PUT", "/sync", { newCode: true }).then(s => { syncInfo = s; renderView(); });
    }
  } else saveAndSync();
  return true;
}

// телефон: сохраняет адрес и код (если их меняли) и сразу синхронизируется
async function saveAndSync() {
  const peer = $("[data-sync-peer]").value.trim(), code = $("[data-sync-code]").value.trim();
  if (!peer) return toast("Введи адрес компьютера");
  if (!code && !syncInfo.hasPeerCode) return toast("Введи код с компьютера");
  const patch = {};
  if (peer !== syncInfo.peer) patch.peer = peer;
  if (code) patch.peerCode = code;
  if (Object.keys(patch).length) syncInfo = await api("PUT", "/sync", patch);
  runSync(true);
}

async function runSync(manual) {
  if (syncBusy) return;
  syncBusy = true;
  if (S.view === "settings") renderView();
  try {
    const res = await fetch("/api/sync/run", { method: "POST" });
    const body = await res.json();
    if (res.ok) {
      syncInfo = body.sync;
      if (body.changed) await load();
      if (manual) toast(body.changed ? "Синхронизировано, данные обновлены" : "Синхронизировано");
    } else {
      if (manual) toast(body.error || "Не удалось синхронизировать");
      await loadSync();
    }
  } catch { /* сервер перезапускается */ }
  syncBusy = false;
  if (S.view === "settings") renderView();
}

// Телефон: тихая синхронизация при открытии, при возвращении в приложение и раз в 3 минуты.
// ПК с включённым приёмом: раз в минуту подхватывает то, что прислал телефон, — иначе открытое окно
// показывало бы старую задачу и при правке затёрло бы ею свежую версию с телефона.
function initAutoSync() {
  const auto = () => {
    if (document.hidden || busyEditing() || !appInfo || !syncInfo) return;
    if (appInfo.phone && syncInfo.peer) runSync(false);
  };
  loadSync().then(auto);
  document.addEventListener("visibilitychange", auto);
  setInterval(auto, 3 * 60 * 1000);
  setInterval(() => {
    if (!document.hidden && !busyEditing() && appInfo && !appInfo.phone && syncInfo && syncInfo.listen) load();
  }, 60 * 1000);
}
