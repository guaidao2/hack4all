'use strict';

// The coverage map page.
//
// Everything drawn here comes from /api/tactical-map, which parses the same
// Markdown file that `hack4all -x id:red-team-tactical-map` prints. There is no
// second copy of the content to keep in sync: the picture and the text are one
// file.
//
// Ticking a box stores the state in localStorage. Nothing leaves the browser —
// there is no server-side state at all.

const STORE = 'hack4all.map.v1';
const LANG_KEY = 'hack4all.lang';

const params = new URLSearchParams(location.search);
let lang = params.get('lang') || localStorage.getItem(LANG_KEY) || 'en';

const T = {
  en: {
    title: 'Red team tactical map',
    back: 'search',
    confirmed: 'confirmed',
    reset: 'reset',
    hint: 'Tick what your engagement has covered. Progress is stored in this browser only; nothing is sent anywhere.',
    surfaces: 'Surfaces that cut across every stage',
    stages: 'The stages',
    missed: 'Most often missed',
    oftenMissed: 'Often missed',
    related: 'related',
    ofItems: (n, t) => n + ' / ' + t + ' items',
  },
  zh: {
    title: '红队战术地图',
    back: '搜索',
    confirmed: '已确认',
    reset: '重置',
    hint: '勾选这次项目已经覆盖到的项。进度只保存在这个浏览器里，不会发送到任何地方。',
    surfaces: '横穿所有阶段的面',
    stages: '各个阶段',
    missed: '最常被漏掉的',
    oftenMissed: '常被漏掉',
    related: '相关篇目',
    ofItems: (n, t) => n + ' / ' + t + ' 项',
  },
};
const t = () => T[lang] || T.en;

let data = null;
let done = loadDone();

function loadDone() {
  try {
    return JSON.parse(localStorage.getItem(STORE)) || {};
  } catch (e) {
    return {};
  }
}

function saveDone() {
  try {
    localStorage.setItem(STORE, JSON.stringify(done));
  } catch (e) {
    /* private mode: the page still works, it just forgets */
  }
}

function el(tag, cls, text) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
}

// Inline markdown is stripped to plain text: the source lines carry `code` and
// **emphasis** that would otherwise show up literally in the cards.
function plain(s) {
  return String(s)
    .replace(/`([^`]*)`/g, '$1')
    .replace(/\*\*([^*]*)\*\*/g, '$1')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1');
}

function entryLink(id) {
  const a = el('a', null, id);
  a.href = '/?q=' + encodeURIComponent('id:' + id);
  a.title = lang === 'zh' ? '打开这一篇' : 'Open this entry';
  return a;
}

function relatedList(ids) {
  if (!ids || !ids.length) return null;
  const box = el('div', 'rel');
  ids.forEach((id) => box.appendChild(entryLink(id)));
  return box;
}

function buildHeader() {
  document.title = 'Hack4all — ' + t().title;
  document.getElementById('mapTitle').textContent = t().title;
  const back = document.getElementById('back');
  back.textContent = t().back;
  back.href = '/';

  const hint = document.getElementById('hint');
  hint.textContent = t().hint;
  const rb = document.getElementById('reset');
  rb.textContent = t().reset;
  rb.addEventListener('click', () => {
    if (!confirm(lang === 'zh' ? '清空所有勾选？' : 'Clear every tick?')) return;
    done = {};
    saveDone();
    document.querySelectorAll('.item input').forEach((i) => {
      i.checked = false;
      i.closest('.item').classList.remove('done');
    });
    updateProgress();
  });

  document.querySelectorAll('.lang-btn[data-lang]').forEach((b) => {
    b.classList.toggle('active', b.dataset.lang === lang);
    b.addEventListener('click', () => {
      lang = b.dataset.lang;
      localStorage.setItem(LANG_KEY, lang);
      location.search = '?lang=' + lang;
    });
  });
}

function buildSurfaces() {
  const wrap = document.getElementById('surfaces');
  if (!data.surfaces || !data.surfaces.length) {
    wrap.hidden = true;
    return;
  }
  document.getElementById('surfTitle').textContent = t().surfaces;
  const grid = document.getElementById('surfgrid');
  data.surfaces.forEach((s) => {
    const card = el('div', 'surf');
    card.appendChild(el('h3', null, plain(s.name)));
    if (s.why) card.appendChild(el('p', null, plain(s.why)));
    const rel = relatedList(s.related);
    if (rel) card.appendChild(rel);
    grid.appendChild(card);
  });
}

function buildStages() {
  document.getElementById('stageTitle').textContent = t().stages;
  const list = document.getElementById('stagelist');

  data.stages.forEach((st) => {
    const li = el('li', 'stage');
    li.dataset.stage = String(st.number);

    const num = el('div', 'num', String(st.number));
    num.title = st.title;
    li.appendChild(num);

    const card = el('div', 'card');
    const h = el('h3');
    h.appendChild(el('span', 'n', String(st.number).padStart(2, '0')));
    h.appendChild(document.createTextNode(plain(st.title)));
    card.appendChild(h);

    st.items.forEach((item, i) => {
      const key = st.number + '.' + i;
      const label = el('label', 'item');
      const cb = document.createElement('input');
      cb.type = 'checkbox';
      cb.checked = !!done[key];
      if (cb.checked) label.classList.add('done');

      const span = el('span', null, plain(item));
      label.appendChild(cb);
      label.appendChild(span);

      cb.addEventListener('change', () => {
        if (cb.checked) done[key] = true;
        else delete done[key];
        label.classList.toggle('done', cb.checked);
        saveDone();
        updateProgress();
      });

      card.appendChild(label);
    });

    if (st.missed) {
      const m = el('div', 'missed');
      m.appendChild(el('b', null, t().oftenMissed + ': '));
      m.appendChild(document.createTextNode(plain(st.missed)));
      card.appendChild(m);
    }

    const rel = relatedList(st.related);
    if (rel) card.appendChild(rel);

    li.appendChild(card);
    list.appendChild(li);
  });
}

function buildTopMissed() {
  const sect = document.getElementById('missSect');
  if (!data.topMissed || !data.topMissed.length) {
    sect.hidden = true;
    return;
  }
  document.getElementById('missTitle').textContent = t().missed;
  const ol = document.getElementById('misslist');
  data.topMissed.forEach((raw) => {
    const li = document.createElement('li');
    // The source bolds the short label at the start of each item; keep that and
    // drop the rest of the markdown.
    const m = String(raw).match(/^([^*]+?)\*\*(.*)$/);
    if (m) {
      const strong = document.createElement('strong');
      strong.textContent = m[1] + m[2].split(',')[0] + (m[2].includes(',') ? ',' : '');
      li.appendChild(strong);
      const rest = m[2].includes(',') ? m[2].slice(m[2].indexOf(',') + 1) : '';
      if (rest.trim()) li.appendChild(document.createTextNode(' ' + plain(rest)));
    } else {
      li.textContent = plain(raw);
    }
    ol.appendChild(li);
  });
}

function updateProgress() {
  let total = 0;
  let n = 0;
  data.stages.forEach((st) => {
    total += st.items.length;
    const seen = st.items.filter((_, i) => done[st.number + '.' + i]).length;
    n += seen;
    const li = document.querySelector('.stage[data-stage="' + st.number + '"]');
    if (li) li.classList.toggle('full', seen === st.items.length && st.items.length > 0);
  });
  document.getElementById('done').textContent = String(n);
  document.getElementById('total').textContent = String(total);
  document.getElementById('ptext').textContent = t().confirmed;
  document.getElementById('bar').style.width = total ? (100 * n / total).toFixed(1) + '%' : '0%';
  document.getElementById('reset').style.visibility = n ? 'visible' : 'hidden';
}

async function main() {
  buildHeader();
  try {
    const r = await fetch('/api/tactical-map?lang=' + encodeURIComponent(lang));
    if (!r.ok) throw new Error(await r.text());
    data = await r.json();
  } catch (e) {
    const p = el('p', 'err', lang === 'zh' ? '无法加载地图：' + e.message : 'Could not load the map: ' + e.message);
    document.querySelector('.mapwrap').appendChild(p);
    return;
  }
  buildSurfaces();
  buildStages();
  buildTopMissed();
  updateProgress();
}

main();
