"use strict";
// Единый набор линейных иконок Clepsy: сетка 24×24, линия 1.8, скруглённые концы, цвет — currentColor.
// Часть форм по мотивам Lucide (https://lucide.dev, лицензия ISC).

const ICONS = {
  // разделы
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  upcoming: '<rect x="3" y="4.5" width="18" height="16.5" rx="3"/><path d="M8 2.5v4M16 2.5v4M3 10h18M8 14h.01M12 14h.01M16 14h.01M8 17.5h.01M12 17.5h.01"/>',
  inbox: '<path d="M21.5 12.5h-5.5l-2 3h-4l-2-3H2.5"/><path d="M5.5 5.2 2.5 12.5v5a2 2 0 0 0 2 2h15a2 2 0 0 0 2-2v-5l-3-7.3a2 2 0 0 0-1.8-1.2H7.3a2 2 0 0 0-1.8 1.2z"/>',
  list: '<path d="M9 6h12M9 12h12M9 18h12M4 6h.01M4 12h.01M4 18h.01"/>',
  menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
  dots: '<path d="M12 5h.01M12 12h.01M12 19h.01"/>',
  alert: '<circle cx="12" cy="12" r="9"/><path d="M12 7.5v5M12 16.5h.01"/>',
  board: '<rect x="3" y="3" width="18" height="18" rx="3"/><path d="M8 7.5v8M12 7.5v4.5M16 7.5v6"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7.5V12l3 2"/>',
  calendar: '<rect x="3" y="4.5" width="18" height="16.5" rx="3"/><path d="M8 2.5v4M16 2.5v4M3 10h18"/>',
  pen: '<path d="M16.9 3.6a2.1 2.1 0 0 1 3 3L8 18.5 3.5 20.5l2-4.5z"/><path d="M14.5 6l3.5 3.5"/>',
  matrix: '<rect x="3.5" y="3.5" width="7" height="7" rx="1.8"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.8"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.8"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.8"/>',
  flame: '<path d="M12 21.5a6.5 6.5 0 0 0 6.5-6.5c0-3.5-2.5-5.5-3.5-8.5-1.5 2-2 3-3.5 3.5C11.5 7 10 4.5 8.5 2.5 8 6 5.5 8.5 5.5 13a6.5 6.5 0 0 0 6.5 8.5z"/><path d="M12 21.5a2.8 2.8 0 0 1-2.8-2.8c0-1.7 1.4-2.6 2.8-4.2 1.4 1.6 2.8 2.5 2.8 4.2a2.8 2.8 0 0 1-2.8 2.8z"/>',
  chart: '<path d="M3.5 3.5v15a2 2 0 0 0 2 2h15"/><path d="M8.5 16.5v-4M13 16.5V8M17.5 16.5v-6.5"/>',
  done: '<circle cx="12" cy="12" r="9"/><path d="m8.5 12.3 2.5 2.5 4.6-5"/>',
  // действия
  plus: '<path d="M12 5v14M5 12h14"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20.5 20.5-4.5-4.5"/>',
  theme: '<circle cx="12" cy="12" r="9"/><path d="M12 3a9 9 0 0 1 0 18z" fill="currentColor" stroke="none"/>',
  bell: '<path d="M6 8.5a6 6 0 0 1 12 0c0 6.5 2.5 8.5 2.5 8.5h-17S6 15 6 8.5"/><path d="M10.3 20.5a2 2 0 0 0 3.4 0"/>',
  download: '<path d="M12 3.5v11M7.5 10l4.5 4.5 4.5-4.5M4.5 20.5h15"/>',
  upload: '<path d="M12 15V4M7.5 8.5 12 4l4.5 4.5M4.5 20.5h15"/>',
  keyboard: '<rect x="2.5" y="6" width="19" height="12" rx="2.5"/><path d="M6.5 10h.01M10 10h.01M14 10h.01M17.5 10h.01M7.5 14h9"/>',
  pin: '<path d="M12 16.5v5"/><path d="M9 3.5h6l-.8 6.3 3.3 3a1 1 0 0 1-.7 1.7H7.2a1 1 0 0 1-.7-1.7l3.3-3z"/>',
  play: '<path d="M7.5 4.8v14.4a.8.8 0 0 0 1.2.7l11.2-7.2a.8.8 0 0 0 0-1.4L8.7 4.1a.8.8 0 0 0-1.2.7z"/>',
  pause: '<path d="M8.5 5v14M15.5 5v14"/>',
  x: '<path d="M6.5 6.5l11 11M17.5 6.5l-11 11"/>',
  check: '<path d="m5 12.5 4.5 4.5L19 7.5"/>',
  trash: '<path d="M4 7h16M10 11v6M14 11v6M5.5 7l1 12a2 2 0 0 0 2 1.8h7a2 2 0 0 0 2-1.8l1-12M9 7V4.8a.8.8 0 0 1 .8-.8h4.4a.8.8 0 0 1 .8.8V7"/>',
  settings: '<path d="M4 20.5v-6.5M4 10V3.5M12 20.5V12M12 8V3.5M20 20.5v-4.5M20 12V3.5M1.5 14h5M9.5 8h5M17.5 16h5"/>',
  reset: '<path d="M3.5 12a8.5 8.5 0 1 0 2.5-6"/><path d="M3.5 3.5V8H8"/>',
  skip: '<path d="M5.5 5.3v13.4a.8.8 0 0 0 1.3.6l9-6.7a.8.8 0 0 0 0-1.2l-9-6.7a.8.8 0 0 0-1.3.6zM19 5v14"/>',
  monitor: '<rect x="2.5" y="4" width="19" height="13" rx="2.5"/><path d="M8.5 21h7M12 17v4"/>',
  globe: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c2.5 2.5 3.8 5.5 3.8 9s-1.3 6.5-3.8 9c-2.5-2.5-3.8-5.5-3.8-9S9.5 5.5 12 3z"/>',
  power: '<path d="M12 3v8.5"/><path d="M17.7 6.8a8 8 0 1 1-11.4 0"/>',
  sunrise: '<path d="M12 3v5M7.5 6.5l1.4 1.4M16.5 6.5l-1.4 1.4M2.5 17.5h2M19.5 17.5h2M3 21h18"/><path d="M7.5 17.5a4.5 4.5 0 0 1 9 0"/>',
  // свойства задачи
  date: '<rect x="3.5" y="5" width="17" height="15.5" rx="2.5"/><path d="M8 3v4M16 3v4M3.5 10h17"/>',
  repeat: '<path d="m17 2.5 3.5 3.5-3.5 3.5"/><path d="M3.5 11.5v-1a4.5 4.5 0 0 1 4.5-4.5h12.5"/><path d="m7 21.5-3.5-3.5L7 14.5"/><path d="M20.5 12.5v1a4.5 4.5 0 0 1-4.5 4.5H3.5"/>',
  move: '<path d="M4.5 12h14M13 6.5l5.5 5.5-5.5 5.5"/>',
  shift: '<path d="m5.5 17 5-5-5-5M12.5 17l5-5-5-5"/>',
  subtasks: '<path d="m3.5 6.5 2 2 3.5-3.5M3.5 15.5l2 2 3.5-3.5M12.5 7h8M12.5 16h8"/>',
  note: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5M9 13h6M9 17h4"/>',
  timer: '<circle cx="12" cy="13.5" r="7.5"/><path d="M12 10v3.5l2.2 1.8M9.5 2.5h5"/>',
  hourglass: '<path d="M6.5 3h11M6.5 21h11"/><path d="M7.5 3v1.5c0 3 4.5 4.8 4.5 7.5s-4.5 4.5-4.5 7.5V21M16.5 3v1.5c0 3-4.5 4.8-4.5 7.5s4.5 4.5 4.5 7.5V21"/>',
  target: '<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1" fill="currentColor"/>',
  zap: '<path d="M13 2.5 4.5 14h7l-1 7.5L19 10h-7z"/>',
  flag: '<path d="M5 21.5V4"/><path d="M5 4.5c4-2 7 2 11 0 1.5-.7 2.5-.8 3-.5v10c-.5-.3-1.5-.2-3 .5-4 2-7-2-11 0"/>',
  progress: '<circle cx="12" cy="12" r="9"/><path d="M12 3a9 9 0 0 1 9 9h-9z" fill="currentColor" stroke="none"/>',
  circle: '<circle cx="12" cy="12" r="9"/>',
  share: '<path d="M4.5 12.5v6a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-6M16 6.5l-4-4-4 4M12 2.5v13"/>',
  archive: '<rect x="2.5" y="3.5" width="19" height="5" rx="1.5"/><path d="M4.5 8.5v10a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-10M10 12.5h4"/>',
  // холст
  marker: '<path d="m9.5 11.5-6 6V20.5h3l6-6"/><path d="m21 9-6.5 6.5a1.5 1.5 0 0 1-2.1 0l-3.4-3.4a1.5 1.5 0 0 1 0-2.1L15.5 3.5"/>',
  eraser: '<path d="M7.5 20.5 3.2 16.2a1.5 1.5 0 0 1 0-2.1l9.9-9.9a1.5 1.5 0 0 1 2.1 0l5.6 5.6a1.5 1.5 0 0 1 0 2.1l-8.7 8.6M21 20.5H7.5M5.5 11.5l7 7"/>',
  undo: '<path d="M9 14.5 4 9.5l5-5"/><path d="M4 9.5h10.5a5.5 5.5 0 0 1 0 11H11"/>',
  redo: '<path d="m15 14.5 5-5-5-5"/><path d="M20 9.5H9.5a5.5 5.5 0 0 0 0 11H13"/>',
  image: '<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.5-3.5a1.5 1.5 0 0 0-2.1 0L6 21"/>',
};

// icon возвращает SVG-разметку иконки для вставки в HTML-строки
function icon(name, cls = "") {
  return `<svg class="i ${cls}" viewBox="0 0 24 24" aria-hidden="true">${ICONS[name] || ""}</svg>`;
}

// hydrateIcons заменяет <i data-i="имя"></i> в статичной разметке на SVG
function hydrateIcons(root = document) {
  root.querySelectorAll("i[data-i]").forEach(el => { el.outerHTML = icon(el.dataset.i, el.className); });
}
