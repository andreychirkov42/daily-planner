"use strict";
// Помодоро в боковой панели (общий с мини-панелью и треем).

/* ================= помодоро (общий с мини-панелью и треем) ================= */
const pomo = (() => {
  let c;
  function draw(st) {
    if (!$("#pomoTime").querySelector("input")) $("#pomoTime").textContent = fmtMMSS(c.left());
    $("#pomoStart").textContent = st.running ? "Пауза" : st.left < st.durations[st.mode] ? "Дальше" : "Старт";
    $("#pomoLabel").textContent = st.label || { work: "Время сосредоточиться", short: "Короткий перерыв", long: "Длинный перерыв" }[st.mode];
    $$(".pomo-modes button").forEach(b => {
      b.classList.toggle("on", b.dataset.mode === st.mode);
      b.innerHTML = { work: "Фокус", short: "Перерыв", long: "Длинный" }[b.dataset.mode] + "<small>" + st.settings[b.dataset.mode] + " мин</small>";
    });
    $("#pomoDone").innerHTML = '<i class="tomato"></i>'.repeat(Math.min(st.count, 12));
    document.title = st.running ? `${fmtMMSS(c.left())} · ${POMO_NAMES[st.mode]}` : $("#viewTitle").textContent + " · Clepsy";
  }
  function finished(st) {
    // звук играет мини-панель в трее, здесь только подсказка
    toast(st.finished === "work" ? "Помидор готов! Перерыв уже идёт" : "Перерыв окончен — за работу!");
    S.pomos[today()] = st.count;
  }
  return {
    client: () => c,
    init() {
      c = pomoClient(draw, finished);
      $("#pomoStart").onclick = () => c.toggle();
      $("#pomoReset").onclick = () => c.reset();
      $$(".pomo-modes button").forEach(b => (b.onclick = () => c.mode(b.dataset.mode)));
      $("#pomoTime").onclick = () => editClock($("#pomoTime"), c);
      bindClockArrows($("#pomoClock"), $("#pomoTime"), c);
      const box = $("#pomoSettings");
      const close = () => { box.hidden = true; $("#pomoGear").classList.remove("on"); };
      $("#pomoGear").onclick = () => {
        if (!box.hidden) return close();
        box.innerHTML = pomoSettingsHTML(c.state().settings);
        box.hidden = false;
        $("#pomoGear").classList.add("on");
        box.querySelector("[data-pset=work]").focus();
      };
      bindPomoSettings(box, c, close);
    },
    focus(text) { c.mode("work", text).then(() => c.start()); },
  };
})();
