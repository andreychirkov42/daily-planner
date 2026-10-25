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
    notify("⏰ Дедлайн: " + t.title, `Срок — сегодня в ${t.dueTime}`);
    toast(`⏰ Дедлайн: ${t.title}`);
  }
  safeSet("notified", JSON.stringify(seen));
  renderNav();
}

let toastTimer;
function toast(text, actionText, action) {
  const el = $("#toast");
  el.innerHTML = `<span>${esc(text)}</span>${actionText ? `<button>${esc(actionText)}</button>` : ""}`;
  el.hidden = false;
  if (actionText) el.querySelector("button").onclick = () => { el.hidden = true; action(); };
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (el.hidden = true), actionText ? 5000 : 2600);
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
    if (s) s.textContent = "сохранено ✓";
  }, 600);
}
