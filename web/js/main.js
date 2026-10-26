"use strict";
// Запуск: обработчики событий и первая загрузка.

/* ================= инициализация ================= */
function init() {
  hydrateIcons(); // <i data-i> в разметке → SVG
  applyTheme();
  $("#quickAdd").addEventListener("submit", quickAdd);
  $("#qaInput").addEventListener("input", renderPreview);
  // клик в любом месте строки ставит курсор в поле ввода
  $("#quickAdd").addEventListener("mousedown", e => {
    if (e.target.closest("button, input")) return;
    e.preventDefault();
    $("#qaInput").focus();
  });

  $("#sidebar").addEventListener("click", onSidebarClick);
  $("#addList").addEventListener("click", e => { e.stopPropagation(); showNewList(); });
  $("#search").addEventListener("input", e => { S.search = e.target.value.trim(); renderNav(); renderView(); });

  const view = $("#view");
  view.addEventListener("click", onViewClick);
  view.addEventListener("input", onNoteInput);
  view.addEventListener("dragover", onDragOver);
  view.addEventListener("drop", onDrop);
  view.addEventListener("submit", async e => {
    if (e.target.id !== "addHabit") return;
    e.preventDefault();
    const name = e.target.name.value.trim();
    const h = await api("POST", "/habits", { name, color: PALETTE[S.habits.length % PALETTE.length] });
    S.habits.push(h);
    renderView();
    $("#addHabit input").focus();
  });

  $("#headRight").addEventListener("change", e => {
    if (e.target.id === "boardListSel") { S.boardList = e.target.value; renderView(); return; }
    if (e.target.id !== "sortSel") return;
    S.sort = e.target.value; safeSet("sort", S.sort); renderView();
  });
  $("#headRight").addEventListener("click", onViewClick);
  $("#planModal").addEventListener("click", onPlanClick);
  $("#planModal").addEventListener("change", onPlanChange);

  // живые таймеры без полной перерисовки
  setInterval(() => {
    for (const el of $$("[data-live]")) {
      const t = S.tasks.find(x => x.id === el.dataset.live);
      if (t) el.textContent = fmtClock(spentNow(t));
    }
  }, 1000);

  const detail = $("#detail");
  detail.addEventListener("input", onDetailInput);
  detail.addEventListener("change", e => { if (e.target.matches("select, input[type=date], input[type=time]")) { onDetailInput(e); renderDetail(); } });
  detail.addEventListener("click", onDetailClick);
  detail.addEventListener("keydown", onDetailKey);
  detail.addEventListener("toggle", e => { if (e.target.matches("[data-more]")) safeSet("detailMore", e.target.open ? "1" : "0"); }, true);

  $("#burger").onclick = () => $("#sidebar").classList.toggle("open");
  $("#helpModal").addEventListener("click", e => { if (e.target.id === "helpModal" || e.target.dataset.close !== undefined) $("#helpModal").hidden = true; });
  loadSettings(); // настройки программы (конец дня, крестик, автозапуск)

  document.addEventListener("keydown", e => {
    const typing = e.target.matches("input, textarea, select");
    if (e.key === "Escape") {
      if (!$("#helpModal").hidden) $("#helpModal").hidden = true;
      else if (!$("#planModal").hidden) { $("#planModal").hidden = true; render(); }
      else if (S.selected) { S.selected = null; render(); }
      else if (typing) e.target.blur();
      $("#sidebar").classList.remove("open");
      return;
    }
    if (typing || e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === "n" || e.key === "т") { e.preventDefault(); $("#qaInput").focus(); }
    else if (e.key === "/" || e.key === ".") { e.preventDefault(); $("#search").focus(); }
    else if (e.key === "t" || e.key === "е") toggleTheme();
    else if (e.key === "?") $("#helpModal").hidden = false;
    else if (e.key === "p" || e.key === "з") openPlan();
    else if (e.key === "d" || e.key === "в") setView("sketch");
    else if (/^[1-9]$/.test(e.key)) setView(VIEW_KEYS[+e.key - 1]);
  });

  pomo.init();
  // ссылка вида /#schedule открывает нужный раздел (так делает мини-панель в трее)
  const fromHash = () => {
    const v = decodeURIComponent(location.hash.slice(1));
    if (v && (TITLES[v] || v.startsWith("list:") || v.startsWith("tag:"))) setView(v);
  };
  window.addEventListener("hashchange", fromHash);
  window.openView = v => { if (v) setView(v); };
  load().then(async () => {
    fromHash();
    checkDeadlines();
    setInterval(checkDeadlines, 30000);
    if (location.search.includes("smoke")) await smokeTour();
    document.documentElement.dataset.ready = "1";
  });
  // перерисовка после полуночи, чтобы «Сегодня» не устаревало
  let day = today();
  setInterval(() => { if (today() !== day) { day = today(); load(); } }, 60000);
  // переносы задач делает сервер — подхватываем их, когда окно снова в фокусе
  window.addEventListener("focus", () => load());
}

init();

// smokeTour (страница открыта с ?smoke) обходит все разделы и основные окна —
// так автотест ловит ошибки, которые проявляются только при отрисовке
async function smokeTour() {
  const pause = () => new Promise(r => setTimeout(r, 30));
  for (const v of [...Object.keys(TITLES), ...S.lists.map(l => "list:" + l.id)]) {
    setView(v);
    await pause();
  }
  S.search = "а"; renderView(); S.search = "";
  const t = S.tasks.find(x => !x.done) || S.tasks[0];
  if (t) {
    S.selected = t.id;
    safeSet("detailMore", "0"); render(); await pause();
    safeSet("detailMore", "1"); render(); await pause();
    S.selected = null; render();
  }
  // холст: проводим линию и сохраняем
  setView("sketch"); await pause();
  const ov = $(".sk-overlay"), r = ov.getBoundingClientRect();
  const ev = (type, k) => ov.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerId: 1, clientX: r.left + r.width * k, clientY: r.top + r.height * k, button: 0 }));
  ev("pointerdown", 0.2); ev("pointermove", 0.4); ev("pointermove", 0.6); ev("pointerup", 0.6);
  await sketch.flush();
    openPlan(); await pause(); $("#planModal").hidden = true;
  $("#pomoGear").click(); await pause(); $("#pomoGear").click();
  setView("today");
}
