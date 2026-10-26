"use strict";
// Звук, уведомления, тосты, тема, заметка дня.

/* ================= звук, уведомления, тосты ================= */
let audioCtx;
function chime(long) {
  try {
    audioCtx = audioCtx || new AudioContext();
    const notes = long ? [660, 880, 990] : [880, 1320];
    notes.forEach((f, i) => {
      const o = audioCtx.createOscillator(), g = audioCtx.createGain();
      o.frequency.value = f; o.type = "sine";
      const t0 = audioCtx.currentTime + i * 0.09;
      g.gain.setValueAtTime(0.0001, t0);
      g.gain.exponentialRampToValueAtTime(0.12, t0 + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, t0 + 0.35);
      o.connect(g).connect(audioCtx.destination);
      o.start(t0); o.stop(t0 + 0.4);
    });
  } catch { /* без звука */ }
}

function notify(title, body) {
  if ("Notification" in window && Notification.permission === "granted") new Notification(title, { body, icon: "" });
}

function checkDeadlines() {
  const td = today();
  const now = new Date();
  const hm = `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
  const seen = JSON.parse(safeGet("notified") || "{}");
  for (const t of active()) {
    if (t.due !== td || !t.dueTime || t.dueTime > hm) continue;
    const key = t.id + t.due + t.dueTime;
    if (seen[key]) continue;
    seen[key] = 1;
    notify("Дедлайн: " + t.title, `Срок — сегодня в ${t.dueTime}`);
    toast(`Дедлайн: ${t.title}`, { kind: "deadline", ms: 8000 });
  }
  safeSet("notified", JSON.stringify(seen));
  renderNav();
}

// Вид подсказки: от него цвет полосы и фона (стили .t-*) и иконка.
const TOAST_ICONS = { info: "check", done: "done", error: "alert", delete: "trash", repeat: "repeat", date: "date", update: "download", deadline: "bell", pomo: "timer" };

let toastTimer;
// toast(текст, { kind, action: [подпись, функция], ms }); полоска снизу показывает, сколько подсказка ещё провисит
function toast(text, opts = {}) {
  const el = $("#toast");
  const kind = TOAST_ICONS[opts.kind] ? opts.kind : "info";
  const ms = opts.ms || (opts.action ? 5000 : 2600);
  el.className = "toast t-" + kind;
  el.style.setProperty("--ms", ms + "ms");
  el.innerHTML = `<span class="t-ic">${icon(TOAST_ICONS[kind])}</span><span class="t-tx">${esc(text)}</span>` +
    (opts.action ? `<button>${esc(opts.action[0])}</button>` : "") + `<i class="t-bar"></i>`;
  el.hidden = false;
  el.style.animation = "none"; void el.offsetWidth; el.style.animation = ""; // новая поверх старой — появиться заново
  if (opts.action) el.querySelector("button").onclick = () => { el.hidden = true; opts.action[1](); };
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), ms);
}

/* ================= тема ================= */
function applyTheme() {
  const t = safeGet("theme");
  if (t) document.documentElement.dataset.theme = t;
  else delete document.documentElement.dataset.theme;
  reportTheme();
}

// reportTheme сообщает программе тему, чтобы заголовок окна был в цвет приложения, а не белым
function reportTheme() {
  const dark = document.documentElement.dataset.theme === "dark" ||
    (!document.documentElement.dataset.theme && matchMedia("(prefers-color-scheme: dark)").matches);
  // в Android оболочка красит строку состояния и полосу навигации
  if (window.ClepsyAndroid) return ClepsyAndroid.setDark(dark, getComputedStyle(document.documentElement).getPropertyValue("--bg").trim());
  // у браузера своё хранилище и своя тема — красить заголовок окна программы должно только оно само
  if (!(window.chrome && window.chrome.webview)) return;
  fetch("/api/app/theme", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ dark }) }).catch(() => {});
}
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => applyTheme());
function toggleTheme() {
  const dark = document.documentElement.dataset.theme === "dark" ||
    (!document.documentElement.dataset.theme && matchMedia("(prefers-color-scheme: dark)").matches);
  safeSet("theme", dark ? "light" : "dark");
  applyTheme();
}

/* ================= заметка дня ================= */
let noteTimer;
function onNoteInput(e) {
  if (e.target.id !== "dayNote") return;
  const td = today(), text = e.target.value;
  S.notes[td] = text;
  $("#noteSaved").textContent = "сохраняю…";
  clearTimeout(noteTimer);
  noteTimer = setTimeout(async () => {
    await api("PUT", "/notes/" + td, { text });
    const s = $("#noteSaved");
    if (s) s.textContent = "сохранено";
  }, 600);
}
