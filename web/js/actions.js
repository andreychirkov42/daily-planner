"use strict";
// Действия с задачами, перетаскивание, списки.

/* ================= действия с задачами ================= */
const wait = ms => new Promise(r => setTimeout(r, ms));

// схлопывает карточку перед тем, как она переедет в другую группу или исчезнет
function collapse(el) {
  const h = el.offsetHeight;
  const anim = el.animate([
    { height: h + "px", opacity: 1, transform: "none" },
    { height: "0px", opacity: 0, transform: "translateX(14px)", paddingTop: "0px", paddingBottom: "0px", marginBottom: "-6px", borderWidth: "0px" },
  ], { duration: 260, easing: "ease-in", fill: "forwards" }).finished.catch(() => {});
  // в фоновой вкладке кадры тормозят — не держим интерфейс дольше нужного
  return Promise.race([anim, wait(320)]);
}

async function toggleTask(id) {
  const el = $(`.task[data-id="${id}"]`);
  const t = S.tasks.find(x => x.id === id);
  const willBeDone = !t.done;
  // сразу показываем результат, не дожидаясь сервера
  if (el) {
    el.classList.toggle("done", willBeDone);
    if (willBeDone) $(".check", el).classList.add("burst");
  }
  if (id in saveTimers) await flushSave(id);
  const started = Date.now();
  const { task, next } = await api("POST", `/tasks/${id}/toggle`);
  replaceTask(task);
  if (next) { S.tasks.push(next); S.enter.add(next.id); }
  if (task.done) {
    chime();
    toast(next ? `Готово! Следующий повтор: ${humanDate(next.due)}` : "Задача выполнена ✓");
    if (el) maybeCelebrate(el);
  }
  if (el && el.isConnected && S.selected !== id) {
    await wait(Math.max(0, (willBeDone ? 450 : 120) - (Date.now() - started)));
    await collapse(el);
    S.enter.add(id);
  }
  render();
}

// конфетти, когда закрыта последняя задача на сегодня
function maybeCelebrate(el) {
  const td = today();
  if (active().some(t => t.due && t.due <= td)) return;
  if (!S.tasks.some(t => doneOn(t, td))) return;
  const r = $(".check", el).getBoundingClientRect();
  const colors = ["#2f6bff", "#19b5ff", "#7aa7ff", "#9fe0ff", "#ffffff", "#ffd166"];
  for (let i = 0; i < 36; i++) {
    const c = document.createElement("i");
    c.className = "confetti";
    c.style.background = colors[i % colors.length];
    c.style.left = r.left + r.width / 2 + "px";
    c.style.top = r.top + r.height / 2 + "px";
    document.body.appendChild(c);
    const ang = Math.random() * Math.PI * 2, dist = 80 + Math.random() * 160;
    c.animate([
      { transform: "translate(0,0) rotate(0)", opacity: 1 },
      { transform: `translate(${Math.cos(ang) * dist}px, ${Math.sin(ang) * dist + 120}px) rotate(${Math.random() * 720}deg)`, opacity: 0 },
    ], { duration: 900 + Math.random() * 500, easing: "cubic-bezier(.2,.7,.4,1)" }).finished.then(() => c.remove());
  }
  setTimeout(() => toast("Все задачи на сегодня выполнены! 🎉"), 500);
}

function greeting() {
  const h = new Date().getHours();
  return h < 5 ? "Доброй ночи" : h < 12 ? "Доброе утро" : h < 18 ? "Добрый день" : "Добрый вечер";
}

async function deleteTask(id) {
  const t = S.tasks.find(t => t.id === id);
  await api("DELETE", "/tasks/" + id);
  S.tasks = S.tasks.filter(x => x.id !== id);
  if (S.selected === id) S.selected = null;
  render();
  toast(`Удалено «${t.title}»`, "Вернуть", async () => {
    const back = await api("POST", "/tasks", t);
    S.tasks.push(back);
    render();
  });
}

function flash(id) {
  const el = $(`.task[data-id="${id}"]`);
  if (el) el.scrollIntoView({ block: "nearest", behavior: "smooth" });
}

async function onViewClick(e) {
  const nav = e.target.closest("[data-view]");
  if (nav) { setView(nav.dataset.view); return; }

  const cal = e.target.closest("[data-cal]");
  if (cal) {
    const n = +cal.dataset.cal;
    S.calMonth = n ? new Date(S.calMonth.getFullYear(), S.calMonth.getMonth() + n, 1) : firstOfMonth(new Date());
    if (!n) S.calPick = today();
    renderView(); return;
  }
  const gact = e.target.closest("[data-act]")?.dataset.act;
  if (gact === "plan") { openPlan(); return; }
  if (gact === "dayEnd") { askDayEnd(); renderView(); return; }

  const sched = e.target.closest("[data-sched]");
  if (sched) {
    const n = +sched.dataset.sched;
    S.schedDay = n ? addDays(S.schedDay || today(), n) : today();
    renderView(); return;
  }
  const uns = e.target.closest("[data-unsched-id]");
  if (uns) {
    const t = S.tasks.find(t => t.id === uns.dataset.unschedId);
    t.dueTime = ""; saveTask(t, 0); render(); return;
  }
  const block = e.target.closest("[data-block]");
  if (block) { S.selected = block.dataset.block; render(); return; }

  const calTask = e.target.closest("[data-cal-task]");
  if (calTask) { S.selected = calTask.dataset.calTask; render(); return; }
  const day = e.target.closest("[data-day]");
  if (day) { S.calPick = S.calPick === day.dataset.day ? null : day.dataset.day; renderView(); return; }

  const hb = e.target.closest("[data-habit]");
  if (hb) {
    const h = await api("POST", `/habits/${hb.dataset.habit}/toggle/${hb.dataset.date}`);
    S.habits[S.habits.findIndex(x => x.id === h.id)] = h;
    if (h.done[hb.dataset.date]) chime();
    renderView(); return;
  }
  const hd = e.target.closest("[data-del-habit]");
  if (hd) {
    if (!confirm("Удалить привычку вместе с историей?")) return;
    await api("DELETE", "/habits/" + hd.dataset.delHabit);
    S.habits = S.habits.filter(h => h.id !== hd.dataset.delHabit);
    renderView(); return;
  }
  if (e.target.id === "clearDone") {
    if (!confirm("Удалить все выполненные задачи навсегда?")) return;
    const { removed } = await api("POST", "/tasks/clear-completed");
    S.tasks = S.tasks.filter(t => !t.done);
    render(); toast(`Удалено задач: ${removed}`); return;
  }

  const taskEl = e.target.closest(".task");
  if (!taskEl) return;
  const id = taskEl.dataset.id;
  const act = e.target.closest("[data-act]")?.dataset.act;
  if (act === "toggle") return toggleTask(id);
  if (act === "del") return deleteTask(id);
  if (act === "timer") return timerToggle(id);
  if (act === "pin") { const t = S.tasks.find(t => t.id === id); t.pinned = !t.pinned; saveTask(t, 0); render(); return; }
  S.selected = S.selected === id ? null : id;
  render();
}

/* ================= drag & drop ================= */
let dragId = null;
function bindDrag() {
  for (const el of $$(".task[draggable], .cal-task[draggable], .block[draggable]")) {
    el.addEventListener("dragstart", e => {
      e.stopPropagation();
      dragId = el.dataset.id || el.dataset.calTask || el.dataset.block;
      e.dataTransfer.effectAllowed = "move";
      e.dataTransfer.setData("text/plain", dragId);
      requestAnimationFrame(() => el.classList.add("dragging"));
    });
    el.addEventListener("dragend", () => { el.classList.remove("dragging"); $$(".drop-above,.drop").forEach(x => x.classList.remove("drop-above", "drop")); });
  }
}

// слот шкалы времени под курсором (блоки перекрывают слоты, поэтому считаем по координате)
function slotAt(e) {
  const tl = e.target.closest("#timeline");
  if (!tl) return null;
  const idx = Math.floor((e.clientY - tl.getBoundingClientRect().top) / (HOUR_PX / 2));
  return $$(".slot", tl)[Math.max(0, idx)] || null;
}

function dropTarget(e) {
  const task = e.target.closest(".task");
  const src = $(`.task[data-id="${dragId}"]`);
  if (task && task.dataset.id !== dragId && src && task.parentElement === src.parentElement) return [task, "drop-above"];
  const t = e.target.closest(".cal-day") || slotAt(e) || e.target.closest(".col") || e.target.closest("[data-unsched-zone]");
  return t ? [t, "drop"] : [null];
}

function onDragOver(e) {
  if (!dragId) return;
  const [el, cls] = dropTarget(e);
  if (!el) return;
  e.preventDefault();
  $$(".drop-above,.drop").forEach(x => x.classList.remove("drop-above", "drop"));
  el.classList.add(cls);
}

async function onDrop(e) {
  if (!dragId) return;
  e.preventDefault();
  const [el, cls] = dropTarget(e);
  const id = dragId; dragId = null;
  const t = S.tasks.find(t => t.id === id);
  if (!el) return;
  if (cls === "drop") {
    if (el.dataset.day) {
      t.due = el.dataset.day;
      toast(`Перенесено на ${humanDate(t.due)}`);
    } else if (el.dataset.time) {
      t.due = S.schedDay || today();
      t.dueTime = el.dataset.time;
    } else if (el.dataset.unschedZone) {
      t.dueTime = "";
      if (!t.due) t.due = S.schedDay || today();
    } else if (el.dataset.status) {
      const st = el.dataset.status;
      if (st === "done") { if (!t.done) await toggleTask(id); return; }
      if (t.done) await toggleTask(id);
      const cur = S.tasks.find(x => x.id === id);
      if (cur.timerStart && st === "todo") await timerToggle(id);
      cur.status = st === "doing" ? "doing" : "";
      saveTask(cur, 0); render(); return;
    }
    saveTask(t, 0);
    render();
    return;
  }
  const target = el;
  const box = target.parentElement;
  const ids = $$(".task", box).map(x => x.dataset.id).filter(x => x !== id);
  ids.splice(ids.indexOf(target.dataset.id), 0, id);
  // общий порядок: переставленные задачи встают на место, которое занимали
  const orders = ids.map(i => S.tasks.find(t => t.id === i).order).sort((a, b) => a - b);
  ids.forEach((i, k) => { S.tasks.find(t => t.id === i).order = orders[k]; });
  const all = [...S.tasks].sort((a, b) => a.order - b.order).map(t => t.id);
  all.forEach((i, k) => { S.tasks.find(t => t.id === i).order = k; });
  render();
  await api("POST", "/tasks/reorder", all);
}

/* ================= списки ================= */
function showNewList() {
  const slot = $("#newListSlot");
  slot.innerHTML = `<input class="input" id="newListName" placeholder="Название и Enter" style="margin:4px 0">`;
  const inp = $("#newListName");
  inp.focus();
  inp.addEventListener("keydown", async e => {
    if (e.key === "Escape") { slot.innerHTML = ""; }
    if (e.key !== "Enter" || !inp.value.trim()) return;
    const l = await api("POST", "/lists", { name: inp.value.trim(), color: PALETTE[S.lists.length % PALETTE.length] });
    S.lists.push(l);
    setView("list:" + l.id);
  });
  inp.addEventListener("blur", () => setTimeout(() => { if (slot.isConnected) slot.innerHTML = ""; }, 150));
}

async function onSidebarClick(e) {
  const del = e.target.closest("[data-del-list]");
  if (del) {
    e.stopPropagation();
    const l = listById(del.dataset.delList);
    if (!confirm(`Удалить список «${l.name}»? Задачи переедут во «Входящие».`)) return;
    await api("DELETE", "/lists/" + l.id);
    S.lists = S.lists.filter(x => x.id !== l.id);
    S.tasks.forEach(t => { if (t.listId === l.id) t.listId = ""; });
    if (S.view === "list:" + l.id) S.view = "inbox";
    render(); return;
  }
  const nav = e.target.closest("[data-view]");
  if (nav) setView(nav.dataset.view);
}
