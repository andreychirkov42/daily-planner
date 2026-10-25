"use strict";
// Общие функции для основной страницы и мини-панели в трее.

function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

/* ================= даты ================= */
function ymd(d) {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
function parseYmd(s) { const [y, m, d] = s.split("-").map(Number); return new Date(y, m - 1, d); }
function today() { return ymd(new Date()); }
function addDays(s, n) { const d = parseYmd(s); d.setDate(d.getDate() + n); return ymd(d); }
function diffDays(a, b) { return Math.round((parseYmd(b) - parseYmd(a)) / 864e5); }

/* ================= быстрый ввод ================= */
const WEEKDAYS = [
  [/^(воскресенье|вс)$/, 0], [/^(понедельник|пн)$/, 1], [/^(вторник|вт)$/, 2], [/^(среду|среда|ср)$/, 3],
  [/^(четверг|чт)$/, 4], [/^(пятницу|пятница|пт)$/, 5], [/^(субботу|суббота|сб)$/, 6],
];

function parseQuick(raw, lists = []) {
  const out = { title: "", due: "", dueTime: "", priority: 0, tags: [], listId: "", repeat: "" };
  let s = " " + raw + " ";
  const take = (re, fn) => { s = s.replace(re, (...m) => (fn(m) === false ? m[0] : " ")); };
  const end = "(?=\\s)";

  take(/\s#([\p{L}\p{N}_-]+)/gu, m => { out.tags.push(m[1].toLowerCase()); });
  take(/\s!([1-3])(?=\s)/g, m => { out.priority = +m[1]; });
  take(/\s@([\p{L}\p{N}_-]+)/gu, m => {
    const l = lists.find(l => l.name.toLowerCase().startsWith(m[1].toLowerCase()));
    if (!l) return false;
    out.listId = l.id;
  });

  const repeats = [
    ["(каждый\\s+день|ежедневно)", "daily"], ["(по\\s+будням)", "weekdays"],
    ["(каждую\\s+неделю|еженедельно)", "weekly"], ["(каждый\\s+месяц|ежемесячно)", "monthly"],
    ["(каждый\\s+год|ежегодно)", "yearly"],
  ];
  for (const [re, v] of repeats) take(new RegExp("\\s" + re + end, "iu"), () => { out.repeat = v; });

  take(/\s(?:в\s+)?([01]?\d|2[0-3]):([0-5]\d)(?=\s)/u, m => { out.dueTime = `${m[1].padStart(2, "0")}:${m[2]}`; });

  const td = today();
  take(new RegExp("\\s(сегодня)" + end, "iu"), () => { out.due = td; });
  take(new RegExp("\\s(послезавтра)" + end, "iu"), () => { out.due = addDays(td, 2); });
  take(new RegExp("\\s(завтра)" + end, "iu"), () => { out.due = addDays(td, 1); });
  take(new RegExp("\\sчерез\\s+(\\d+)\\s+(дн[яейь]+|день)" + end, "iu"), m => { out.due = addDays(td, +m[1]); });
  take(new RegExp("\\sчерез\\s+неделю" + end, "iu"), () => { out.due = addDays(td, 7); });
  take(/\s(\d{1,2})\.(\d{1,2})(?:\.(\d{2,4}))?(?=\s)/u, m => {
    const now = new Date();
    let y = m[3] ? +m[3] : now.getFullYear();
    if (y < 100) y += 2000;
    const d = new Date(y, +m[2] - 1, +m[1]);
    if (d.getMonth() !== +m[2] - 1) return false;
    if (!m[3] && ymd(d) < td) d.setFullYear(y + 1);
    out.due = ymd(d);
  });
  take(/\s(?:во?\s+)?([\p{L}]+)(?=\s)/gu, m => {
    if (out.due) return false;
    const w = WEEKDAYS.find(([re]) => re.test(m[1].toLowerCase()));
    if (!w) return false;
    let n = (w[1] - new Date().getDay() + 7) % 7;
    if (n === 0) n = 7;
    out.due = addDays(td, n);
  });

  // время без даты — значит сегодня
  if (out.dueTime && !out.due) out.due = td;
  out.title = s.replace(/\s+/g, " ").trim();
  return out;
}


/* ================= помодоро: состояние живёт на сервере, окна только показывают его ================= */
const POMO_NAMES = { work: "Фокус", short: "Перерыв", long: "Длинный перерыв" };

function fmtMMSS(sec) {
  return `${String(Math.floor(sec / 60)).padStart(2, "0")}:${String(sec % 60).padStart(2, "0")}`;
}

// pomoClient опрашивает сервер и зовёт onChange каждые полсекунды, onFinish — когда сессия закончилась.
function pomoClient(onChange, onFinish) {
  let st = null, lastFinished = null;
  async function call(body) {
    try {
      const r = await fetch("/api/pomodoro", body ? {
        method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
      } : undefined);
      if (r.ok) apply(await r.json());
    } catch { /* сервер перезапускается — попробуем позже */ }
  }
  function apply(n) {
    if (lastFinished !== null && n.finishedAt && n.finishedAt !== lastFinished && onFinish) onFinish(n);
    lastFinished = n.finishedAt || 0;
    st = n;
    onChange(st);
  }
  const left = () => (!st ? 0 : st.running ? Math.max(0, Math.ceil((st.endAt - Date.now()) / 1000)) : st.left);
  setInterval(() => call(), 2000);
  setInterval(() => st && onChange(st), 500);
  call();
  return {
    state: () => st,
    left,
    progress: () => (st ? 1 - left() / st.durations[st.mode] : 0),
    toggle: label => call({ action: st && st.running ? "pause" : "start", label: label || "" }),
    start: label => call({ action: "start", label: label || "" }),
    reset: () => call({ action: "reset" }),
    skip: () => call({ action: "skip" }),
    mode: (mode, label) => call({ action: "mode", mode, label: label || "" }),
    set: minutes => call({ action: "set", minutes }),
    add: minutes => call({ action: "add", minutes }),
    async saveSettings(set) {
      const r = await fetch("/api/pomodoro/settings", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(set) });
      const body = await r.json();
      if (!r.ok) throw new Error(body.error || "не удалось сохранить");
      apply(body);
    },
    refresh: () => call(),
  };
}

const TRASH_SVG = icon("trash"); // из общего набора icons.js

// parseClock понимает «25», «12:30», «1:05:00», «7,5» и возвращает минуты (или NaN).
function parseClock(str) {
  const v = String(str).trim().replace(",", ".");
  if (/^\d+(\.\d+)?$/.test(v)) return parseFloat(v);
  const parts = v.split(":").map(Number);
  if (parts.length < 2 || parts.length > 3 || parts.some(n => !Number.isFinite(n) || n < 0)) return NaN;
  const [h, m, sec] = parts.length === 3 ? parts : [0, ...parts];
  return h * 60 + m + sec / 60;
}

const POMO_PRESETS = [[25, 5, 15], [50, 10, 20], [90, 15, 30], [15, 3, 10]];

// форма настроек таймера; одинаковая для окна программы и мини-панели
function pomoSettingsHTML(set) {
  const num = (k, label, max, unit) => `<label class="pset-row"><span>${label}</span>
    <input type="number" min="1" max="${max}" step="1" data-pset="${k}" value="${set[k]}"><em>${unit}</em></label>`;
  return `<div class="pset">
    <div class="pset-presets">${POMO_PRESETS.map(([w, s, l]) => `<button type="button" class="pset-chip ${set.work === w && set.short === s && set.long === l ? "on" : ""}" data-preset="${w},${s},${l}">${w}/${s}</button>`).join("")}</div>
    ${num("work", "Фокус", 180, "мин")}${num("short", "Перерыв", 60, "мин")}${num("long", "Длинный перерыв", 120, "мин")}
    ${num("longEvery", "Длинный после", 12, "фокусов")}
    <label class="pset-row pset-check"><input type="checkbox" data-pset="autoBreak" ${set.autoBreak ? "checked" : ""}><span>Перерыв запускается сам</span></label>
    <div class="pset-err" data-pset-err></div>
    <div class="pset-actions"><button type="button" class="pset-cancel" data-pset-cancel>Отмена</button><button type="button" class="pset-save" data-pset-save>Сохранить</button></div>
  </div>`;
}

// readPomoSettings собирает значения из формы
function readPomoSettings(root) {
  const v = k => root.querySelector(`[data-pset="${k}"]`);
  return { work: +v("work").value, short: +v("short").value, long: +v("long").value, longEvery: +v("longEvery").value, autoBreak: v("autoBreak").checked };
}

// bindPomoSettings вешает обработчики на форму: пресеты, сохранение, отмена
function bindPomoSettings(root, client, close) {
  root.addEventListener("click", async e => {
    const preset = e.target.closest("[data-preset]");
    if (preset) {
      const [w, s, l] = preset.dataset.preset.split(",");
      root.querySelector(`[data-pset="work"]`).value = w;
      root.querySelector(`[data-pset="short"]`).value = s;
      root.querySelector(`[data-pset="long"]`).value = l;
      root.querySelectorAll("[data-preset]").forEach(b => b.classList.toggle("on", b === preset));
      return;
    }
    if (e.target.closest("[data-pset-cancel]")) { close(); return; }
    if (e.target.closest("[data-pset-save]")) {
      try { await client.saveSettings(readPomoSettings(root)); close(); }
      catch (err) { root.querySelector("[data-pset-err]").textContent = err.message; }
    }
  });
  root.addEventListener("keydown", e => {
    if (e.key === "Enter") root.querySelector("[data-pset-save]").click();
    if (e.key === "Escape") { e.stopPropagation(); close(); }
  });
}

// editClock превращает цифры таймера в поле ввода; Enter сохраняет, Esc отменяет
function editClock(el, client) {
  if (el.querySelector("input")) return;
  const old = el.textContent;
  el.innerHTML = `<input class="clock-input" value="${old}" title="Минуты или мм:сс, Enter — сохранить">`;
  const inp = el.querySelector("input");
  inp.focus(); inp.select();
  let done = false;
  const finish = async save => {
    if (done) return;
    done = true;
    const m = parseClock(inp.value);
    el.textContent = old;
    if (save && m > 0) await client.set(m);
  };
  inp.addEventListener("keydown", e => {
    if (e.key === "Enter") finish(true);
    if (e.key === "Escape") { e.stopPropagation(); finish(false); }
  });
  inp.addEventListener("blur", () => finish(true));
}

// bindClockArrows: стрелки ▲▼ у часов (±1 минута, зажатие повторяет) и колёсико мыши над часами
function bindClockArrows(root, clockEl, client) {
  let holdTimer = null, repeatTimer = null;
  const stop = () => { clearTimeout(holdTimer); clearInterval(repeatTimer); holdTimer = repeatTimer = null; };
  root.querySelectorAll("[data-adj]").forEach(btn => {
    const step = +btn.dataset.adj;
    btn.addEventListener("pointerdown", e => {
      e.preventDefault();
      client.add(step);
      holdTimer = setTimeout(() => { repeatTimer = setInterval(() => client.add(step), 130); }, 450);
    });
    ["pointerup", "pointerleave", "pointercancel"].forEach(ev => btn.addEventListener(ev, stop));
  });
  clockEl.addEventListener("wheel", e => {
    if (clockEl.querySelector("input")) return;
    e.preventDefault();
    client.add(e.deltaY < 0 ? 1 : -1);
  }, { passive: false });
}
