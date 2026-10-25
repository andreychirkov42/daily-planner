"use strict";
// Холст для быстрых набросков: ручка, маркер, ластик, отмена, автосохранение и галерея.

const sketch = (() => {
  const W = 1920, H = 1200; // постоянное разрешение листа, на экране масштабируется
  const PAPER = "#fdfcf9";
  const COLORS = ["#1f2430", "#2f6bff", "#ef4d5a", "#22a06b", "#f29a2e", "#8b5cf6", "#8a94a6"];
  const TOOLS = { pen: "✏️ Ручка", marker: "🖍 Маркер", eraser: "🧽 Ластик" };

  let root = null, canvas, ctx, overlay, octx;
  let tool = safeGet("sk:tool") || "pen";
  let color = safeGet("sk:color") || COLORS[0];
  let size = +(safeGet("sk:size") || 4);
  let strokes = [], redo = [], base = null, current = null, list = [];
  let drawing = null, saveTimer = null, dirty = false;

  function build() {
    root = document.createElement("div");
    root.className = "sk";
    root.innerHTML = `
      <div class="sk-bar">
        <div class="sk-group">${Object.entries(TOOLS).map(([k, n]) => `<button class="sk-btn" data-tool="${k}" title="${n} (${{ pen: "B", marker: "M", eraser: "E" }[k]})">${n}</button>`).join("")}</div>
        <div class="sk-group">${COLORS.map(c => `<button class="sk-color" data-color="${c}" style="--c:${c}" title="${c}"></button>`).join("")}
          <label class="sk-color sk-custom" title="Свой цвет"><input type="color" id="skPick"></label></div>
        <label class="sk-size" title="Толщина ( [ и ] )"><input type="range" id="skSize" min="1" max="48"><b id="skSizeVal"></b></label>
        <div class="sk-group">
          <button class="sk-btn" data-act="undo" title="Отменить (Ctrl+Z)">↶</button>
          <button class="sk-btn" data-act="redo" title="Повторить (Ctrl+Y)">↷</button>
          <button class="sk-btn" data-act="clear" title="Очистить лист">${TRASH_SVG}</button>
        </div>
        <span class="grow"></span>
        <input class="input sk-title" id="skTitle" placeholder="Название наброска">
        <span class="sk-status" id="skStatus"></span>
        <a class="sk-btn" id="skDownload" title="Скачать PNG" download>⬇ PNG</a>
        <button class="btn btn-sm btn-primary" data-act="new" title="Начать новый лист">＋ Новый лист</button>
      </div>
      <div class="sk-stage"><div class="sk-paper">
        <canvas width="${W}" height="${H}"></canvas><canvas class="sk-overlay" width="${W}" height="${H}"></canvas>
      </div></div>
      <div class="sk-gallery" id="skGallery"></div>`;
    [canvas, overlay] = root.querySelectorAll("canvas");
    ctx = canvas.getContext("2d");
    octx = overlay.getContext("2d");
    paint();

    overlay.addEventListener("pointerdown", start);
    overlay.addEventListener("pointermove", move);
    overlay.addEventListener("pointerup", end);
    overlay.addEventListener("pointercancel", end);
    root.addEventListener("click", onClick);
    root.querySelector("#skSize").addEventListener("input", e => setSize(+e.target.value));
    root.querySelector("#skPick").addEventListener("input", e => setColor(e.target.value));
    root.querySelector("#skTitle").addEventListener("change", e => rename(e.target.value.trim()));
    root.querySelector("#skDownload").addEventListener("click", e => {
      e.currentTarget.href = canvas.toDataURL("image/png");
      e.currentTarget.download = (root.querySelector("#skTitle").value || "набросок") + ".png";
    });
    document.addEventListener("keydown", onKey);
    syncBar();
    loadList();
  }

  // ---------- рисование ----------
  function pos(e) {
    const r = overlay.getBoundingClientRect();
    const p = e.pointerType === "pen" && e.pressure > 0 ? e.pressure : 0.5;
    return [(e.clientX - r.left) * W / r.width, (e.clientY - r.top) * H / r.height, p];
  }

  function start(e) {
    if (e.button > 0) return;
    try { overlay.setPointerCapture(e.pointerId); } catch { /* синтетическое событие без указателя */ }
    drawing = { tool, color, size, pts: [pos(e)] };
    drawStroke(octx, drawing, true);
  }

  function move(e) {
    if (!drawing) return;
    // getCoalescedEvents даёт все точки между кадрами — линия не рвётся при быстром движении
    const evs = e.getCoalescedEvents ? e.getCoalescedEvents() : [e];
    for (const ev of evs.length ? evs : [e]) drawing.pts.push(pos(ev));
    octx.clearRect(0, 0, W, H);
    drawStroke(octx, drawing, true);
  }

  function end() {
    if (!drawing) return;
    octx.clearRect(0, 0, W, H);
    strokes.push(drawing);
    redo = [];
    drawStroke(ctx, drawing);
    drawing = null;
    changed();
  }

  // drawStroke рисует штрих сглаженной линией; маркер полупрозрачный, ластик — цветом бумаги
  function drawStroke(c, s, preview) {
    if (s.clear) { c.fillStyle = PAPER; c.fillRect(0, 0, W, H); return; }
    c.save();
    c.lineCap = c.lineJoin = "round";
    c.strokeStyle = c.fillStyle = s.tool === "eraser" ? PAPER : s.color;
    c.globalAlpha = s.tool === "marker" ? 0.35 : 1;
    if (preview && s.tool === "eraser") { c.strokeStyle = "rgba(120,130,150,.35)"; }
    const k = s.tool === "marker" ? 3 : s.tool === "eraser" ? 4 : 1;
    const w = p => s.size * k * (0.4 + p[2] * 1.2);
    const pts = s.pts;
    if (pts.length === 1) {
      c.beginPath(); c.arc(pts[0][0], pts[0][1], w(pts[0]) / 2, 0, Math.PI * 2); c.fill();
    } else if (s.tool === "marker" || pts.every(p => p[2] === 0.5)) {
      // одна непрерывная линия — без «бусин» на прозрачном маркере
      c.lineWidth = w(pts[0]);
      c.beginPath();
      c.moveTo(pts[0][0], pts[0][1]);
      for (let i = 1; i < pts.length - 1; i++) {
        const mx = (pts[i][0] + pts[i + 1][0]) / 2, my = (pts[i][1] + pts[i + 1][1]) / 2;
        c.quadraticCurveTo(pts[i][0], pts[i][1], mx, my);
      }
      c.lineTo(pts.at(-1)[0], pts.at(-1)[1]);
      c.stroke();
    } else {
      // перо с нажимом: отрезки разной толщины
      for (let i = 1; i < pts.length; i++) {
        c.lineWidth = w(pts[i]);
        c.beginPath(); c.moveTo(pts[i - 1][0], pts[i - 1][1]); c.lineTo(pts[i][0], pts[i][1]); c.stroke();
      }
    }
    c.restore();
  }

  function paint() {
    ctx.globalAlpha = 1;
    ctx.fillStyle = PAPER;
    ctx.fillRect(0, 0, W, H);
    if (base) ctx.drawImage(base, 0, 0, W, H);
    for (const s of strokes) drawStroke(ctx, s);
  }

  function undo() { if (!strokes.length) return; redo.push(strokes.pop()); paint(); changed(); }
  function redoStroke() { if (!redo.length) return; strokes.push(redo.pop()); paint(); changed(); }
  function clear() { strokes.push({ clear: true }); redo = []; paint(); changed(); }

  // ---------- сохранение ----------
  function changed() {
    dirty = true;
    status("…");
    clearTimeout(saveTimer);
    saveTimer = setTimeout(save, 1200);
    syncBar();
  }

  async function save() {
    clearTimeout(saveTimer);
    if (!dirty) return;
    dirty = false;
    const png = canvas.toDataURL("image/png");
    const title = root.querySelector("#skTitle").value.trim() || defaultTitle();
    try {
      const sk = await api(current ? "PUT" : "POST", "/sketches" + (current ? "/" + current : ""), { title, png });
      current = sk.id;
      root.querySelector("#skTitle").value = sk.title;
      status("сохранено ✓");
      loadList();
    } catch {
      dirty = true;
      status("не сохранилось");
    }
  }

  async function rename(title) {
    if (!current) { if (title) changed(); return; }
    await api("PUT", "/sketches/" + current, { title: title || defaultTitle() });
    loadList();
  }

  function defaultTitle() {
    const d = new Date();
    return "Набросок " + fmtShort.format(d).replace(".", "") + " " + d.toTimeString().slice(0, 5);
  }

  async function newSheet() {
    await save();
    current = null; base = null; strokes = []; redo = [];
    root.querySelector("#skTitle").value = "";
    paint(); status(""); syncBar(); renderGallery();
  }

  async function open(id) {
    if (id === current) return;
    await save();
    const img = new Image();
    img.src = `/api/sketches/${id}/image?v=${Date.now()}`;
    await img.decode().catch(() => {});
    current = id; base = img; strokes = []; redo = [];
    root.querySelector("#skTitle").value = (list.find(x => x.id === id) || {}).title || "";
    paint(); status(""); syncBar(); renderGallery();
  }

  async function remove(id) {
    const sk = list.find(x => x.id === id);
    if (!confirm(`Удалить набросок «${sk ? sk.title : ""}»?`)) return;
    await api("DELETE", "/sketches/" + id);
    if (id === current) { current = null; base = null; strokes = []; root.querySelector("#skTitle").value = ""; paint(); }
    loadList();
  }

  async function loadList() {
    try { list = (await api("GET", "/sketches")) || []; } catch { list = []; }
    renderGallery();
  }

  function renderGallery() {
    const g = root && root.querySelector("#skGallery");
    if (!g) return;
    g.innerHTML = list.length ? list.map(sk => `<div class="sk-thumb ${sk.id === current ? "on" : ""}" data-open="${sk.id}" title="${esc(sk.title)}">
        <img src="/api/sketches/${sk.id}/image?v=${new Date(sk.updatedAt).getTime()}" alt="" loading="lazy">
        <span>${esc(sk.title)}</span>
        <button class="icon-btn" data-del-sketch="${sk.id}" title="Удалить">${TRASH_SVG}</button>
      </div>`).join("")
      : `<p class="hint">Здесь появятся сохранённые наброски. Рисунок сохраняется сам через секунду после последнего штриха.</p>`;
  }

  // ---------- панель инструментов ----------
  function setTool(t) { tool = t; safeSet("sk:tool", t); syncBar(); }
  function setColor(c) { color = c; safeSet("sk:color", c); if (tool === "eraser") tool = "pen"; syncBar(); }
  function setSize(v) { size = Math.max(1, Math.min(48, v)); safeSet("sk:size", size); syncBar(); }

  function syncBar() {
    root.querySelectorAll("[data-tool]").forEach(b => b.classList.toggle("on", b.dataset.tool === tool));
    root.querySelectorAll("[data-color]").forEach(b => b.classList.toggle("on", b.dataset.color === color));
    root.querySelector("#skPick").value = color;
    root.querySelector(".sk-custom").classList.toggle("on", !COLORS.includes(color));
    root.querySelector("#skSize").value = size;
    root.querySelector("#skSizeVal").textContent = size;
    root.querySelector('[data-act="undo"]').disabled = !strokes.length;
    root.querySelector('[data-act="redo"]').disabled = !redo.length;
    overlay.style.cursor = tool === "eraser" ? "cell" : "crosshair";
  }

  function status(t) { const el = root.querySelector("#skStatus"); if (el) el.textContent = t; }

  function onClick(e) {
    const b = e.target.closest("[data-tool], [data-color], [data-act], [data-open], [data-del-sketch]");
    if (!b) return;
    const d = b.dataset;
    if (d.delSketch) { e.stopPropagation(); remove(d.delSketch); return; }
    if (d.tool) setTool(d.tool);
    else if (d.color) setColor(d.color);
    else if (d.open) open(d.open);
    else if (d.act === "undo") undo();
    else if (d.act === "redo") redoStroke();
    else if (d.act === "clear") clear();
    else if (d.act === "new") newSheet();
  }

  function onKey(e) {
    if (S.view !== "sketch" || e.target.matches("input, textarea, select")) return;
    const k = e.key.toLowerCase();
    if ((e.ctrlKey || e.metaKey) && (k === "z" || k === "я")) { e.preventDefault(); e.shiftKey ? redoStroke() : undo(); return; }
    if ((e.ctrlKey || e.metaKey) && (k === "y" || k === "н")) { e.preventDefault(); redoStroke(); return; }
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (k === "b" || k === "и") setTool("pen");
    else if (k === "m" || k === "ь") setTool("marker");
    else if (k === "e" || k === "у") setTool("eraser");
    else if (k === "[" || k === "х") setSize(size - 2);
    else if (k === "]" || k === "ъ") setSize(size + 2);
    else return;
    e.stopPropagation();
  }

  return {
    // mount вставляет холст в раздел; сам холст живёт постоянно, поэтому перерисовка страницы рисунок не теряет
    mount(el) {
      if (!root) build();
      el.appendChild(root);
      renderGallery();
    },
    flush: () => root && save(),
  };
})();
