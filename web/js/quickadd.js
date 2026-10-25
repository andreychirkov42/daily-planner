"use strict";
// Строка быстрого добавления задачи.

/* ================= быстрый ввод ================= */
function viewDefaults() {
  const v = S.view;
  if (v === "today") return { due: today() };
  if (v === "calendar" && S.calPick) return { due: S.calPick };
  if (v === "schedule") return { due: S.schedDay || today() };
  if (v.startsWith("list:")) return { listId: v.slice(5) };
  if (v.startsWith("tag:")) return { tags: [v.slice(4)] };
  return {};
}

function renderPreview() {
  const raw = $("#qaInput").value;
  const box = $("#qaPreview");
  if (!raw.trim()) { box.innerHTML = ""; return; }
  const p = parseQuick(raw, S.lists);
  const chips = [];
  if (p.due) chips.push(`<span class="chip today">📅 ${humanDate(p.due)}${p.dueTime ? " " + p.dueTime : ""}</span>`);
  if (p.priority) chips.push(`<span class="chip p${p.priority}" style="color:var(--pc)">⚑ ${PRI_NAMES[p.priority]}</span>`);
  if (p.repeat) chips.push(`<span class="chip">🔁 ${REPEAT_NAMES[p.repeat]}</span>`);
  if (p.listId) { const l = listById(p.listId); chips.push(`<span class="chip"><i class="dot" style="background:${l.color};margin:0"></i>${esc(l.name)}</span>`); }
  for (const t of p.tags) chips.push(`<span class="chip tag">#${esc(t)}</span>`);
  box.innerHTML = chips.join("");
}

async function quickAdd(e) {
  e.preventDefault();
  const input = $("#qaInput");
  const p = parseQuick(input.value, S.lists);
  if (!p.title) return;
  const def = viewDefaults();
  const task = {
    title: p.title, notes: "", done: false, subtasks: [], pinned: false,
    priority: p.priority,
    due: p.due || def.due || "",
    dueTime: p.dueTime,
    listId: p.listId || def.listId || "",
    tags: [...new Set([...(def.tags || []), ...p.tags])],
    repeat: p.repeat,
  };
  const created = await api("POST", "/tasks", task);
  S.tasks.push(created);
  S.enter.add(created.id);
  input.value = "";
  renderPreview();
  render();
  flash(created.id);
}
