'use strict';

// Hack4all web UI. Plain JS, no build step, no CDN: the whole thing has to work
// from a single binary on an air-gapped machine.

const state = {
  lang: (navigator.language || 'en').toLowerCase().startsWith('zh') ? 'zh' : 'en',
  query: '',
  category: '',
  items: [],
  selected: null,
};

const $ = (sel) => document.querySelector(sel);

async function api(path, params) {
  const url = new URL(path, location.origin);
  for (const [k, v] of Object.entries(params || {})) {
    if (v !== '' && v !== null && v !== undefined) url.searchParams.set(k, v);
  }
  const res = await fetch(url, { headers: { Accept: 'application/json' } });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

// ---------------------------------------------------------------- markdown
// A deliberately small renderer: headings, fenced code, tables, lists, quotes,
// rules, bold, inline code and links. That is exactly the subset the content
// files use, and a dependency-free page stays deployable offline.

function escapeHtml(s) {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function inline(s) {
  let out = escapeHtml(s);
  out = out.replace(/`([^`]+)`/g, '<code>$1</code>');
  out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  out = out.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer noopener">$1</a>');
  return out;
}

function splitRow(line) {
  return line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim());
}

function renderMarkdown(md) {
  const lines = (md || '').split('\n');
  const out = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    if (/^\s*```/.test(line)) {
      const lang = line.trim().slice(3).trim();
      const buf = [];
      i++;
      while (i < lines.length && !/^\s*```/.test(lines[i])) buf.push(lines[i++]);
      i++;
      const cls = lang ? ` class="language-${escapeHtml(lang)}"` : '';
      out.push(`<pre><code${cls}>${escapeHtml(buf.join('\n'))}</code></pre>`);
      continue;
    }

    if (/^\s*\|/.test(line) && i + 1 < lines.length && /^\s*\|[\s:|-]+\|\s*$/.test(lines[i + 1])) {
      const head = splitRow(line);
      i += 2;
      const rows = [];
      while (i < lines.length && /^\s*\|/.test(lines[i])) rows.push(splitRow(lines[i++]));
      out.push(
        '<table><thead><tr>' + head.map((c) => `<th>${inline(c)}</th>`).join('') + '</tr></thead><tbody>' +
        rows.map((r) => '<tr>' + r.map((c) => `<td>${inline(c)}</td>`).join('') + '</tr>').join('') +
        '</tbody></table>'
      );
      continue;
    }

    const h = /^(#{1,6})\s+(.*)$/.exec(line);
    if (h) {
      const level = Math.min(h[1].length + 1, 6); // body ## is a page-level h3
      out.push(`<h${level}>${inline(h[2])}</h${level}>`);
      i++;
      continue;
    }

    if (/^\s*(-{3,}|_{3,}|\*{3,})\s*$/.test(line)) {
      out.push('<hr>');
      i++;
      continue;
    }

    if (/^\s*>/.test(line)) {
      const buf = [];
      while (i < lines.length && /^\s*>/.test(lines[i])) buf.push(lines[i++].replace(/^\s*>\s?/, ''));
      out.push(`<blockquote>${inline(buf.join(' '))}</blockquote>`);
      continue;
    }

    if (/^\s*[-*+]\s+/.test(line)) {
      const buf = [];
      while (i < lines.length && /^\s*[-*+]\s+/.test(lines[i])) buf.push(lines[i++].replace(/^\s*[-*+]\s+/, ''));
      out.push('<ul>' + buf.map((x) => `<li>${inline(x)}</li>`).join('') + '</ul>');
      continue;
    }

    if (/^\s*\d+\.\s+/.test(line)) {
      const buf = [];
      while (i < lines.length && /^\s*\d+\.\s+/.test(lines[i])) buf.push(lines[i++].replace(/^\s*\d+\.\s+/, ''));
      out.push('<ol>' + buf.map((x) => `<li>${inline(x)}</li>`).join('') + '</ol>');
      continue;
    }

    if (!line.trim()) {
      i++;
      continue;
    }

    const buf = [];
    while (
      i < lines.length &&
      lines[i].trim() &&
      !/^\s*(#{1,6}\s|```|[-*+]\s|\d+\.\s|>|\|)/.test(lines[i])
    ) {
      buf.push(lines[i++]);
    }
    out.push(`<p>${inline(buf.join(' '))}</p>`);
  }

  return out.join('\n');
}

// ---------------------------------------------------------------- rendering

function titleOf(t) {
  return (state.lang === 'zh' ? t.title.zh : t.title.en) || t.title.en || t.title.zh || t.id;
}

function renderList() {
  const ul = $('#list');
  ul.innerHTML = '';
  $('#listmeta').textContent = `${state.items.length} result${state.items.length === 1 ? '' : 's'}${state.category ? ' in ' + state.category : ''}`;

  for (const t of state.items) {
    const li = document.createElement('li');
    if (t.id === state.selected) li.classList.add('active');
    const meta = [t.category.join('/'), (t.attck || []).join(' ')].filter(Boolean).join('  ·  ');
    li.innerHTML = `<span class="t"></span><span class="m"></span>`;
    li.querySelector('.t').textContent = titleOf(t);
    li.querySelector('.m').textContent = meta;
    li.addEventListener('click', () => select(t.id));
    ul.appendChild(li);
  }

  if (!state.items.length) {
    ul.innerHTML = '<li class="dim" style="cursor:default">no technique matches this search</li>';
  }
}

function renderCategories(cats) {
  const nav = $('#cats');
  nav.innerHTML = '';
  const all = document.createElement('a');
  all.href = '#';
  all.textContent = 'all';
  if (!state.category) all.classList.add('active');
  all.addEventListener('click', (e) => {
    e.preventDefault();
    state.category = '';
    search();
  });
  nav.appendChild(all);

  for (const c of cats) {
    const a = document.createElement('a');
    a.href = '#';
    a.className = c.depth >= 2 ? 'd2' : c.depth === 1 ? 'd1' : '';
    if (c.path === state.category) a.classList.add('active');
    a.innerHTML = `${escapeHtml(c.name)}<span class="n">${c.count}</span>`;
    a.title = c.path;
    a.addEventListener('click', (e) => {
      e.preventDefault();
      state.category = c.path === state.category ? '' : c.path;
      search();
    });
    nav.appendChild(a);
  }
}

function renderDetail(t) {
  const el = $('#detail');
  const chips = []
    .concat((t.attck || []).map((a) => `<span class="chip attck">${escapeHtml(a)}</span>`))
    .concat((t.tags || []).map((x) => `<span class="chip">${escapeHtml(x)}</span>`))
    .concat((t.tools || []).map((x) => `<span class="chip">tool:${escapeHtml(x)}</span>`))
    .join('');

  const summary = (state.lang === 'zh' ? t.summary.zh : t.summary.en) || t.summary.en || '';

  el.innerHTML =
    `<h1>${escapeHtml(titleOf(t))}</h1>` +
    `<div class="meta">${escapeHtml(t.category.join(' / '))}${t.difficulty ? '  ·  ' + escapeHtml(t.difficulty) : ''}${t.updated ? '  ·  ' + escapeHtml(t.updated) : ''}  ·  ${escapeHtml(t.id)}</div>` +
    `<div class="chips">${chips}</div>` +
    (summary ? `<p class="summary">${inline(summary)}</p>` : '') +
    renderMarkdown(t.body);

  el.scrollTop = 0;
}

// ---------------------------------------------------------------- actions

async function search() {
  const data = await api('/api/search', { q: state.query, lang: state.lang, category: state.category });
  state.items = data.matches || [];
  if (!state.items.some((t) => t.id === state.selected)) state.selected = null;
  renderList();
  if (state.selected) {
    select(state.selected, true);
  }
}

async function select(id, keepHash) {
  state.selected = id;
  renderList();
  const t = await api('/api/technique', { id, lang: state.lang });
  renderDetail(t);
  if (!keepHash) location.hash = '#/' + id;
}

async function loadStats() {
  const s = await api('/api/stats');
  $('#stats').textContent = `${s.techniques} techniques · ${s.categories} categories · ${s.lang}`;
}

async function loadCategories() {
  const d = await api('/api/categories');
  renderCategories(d.categories || []);
}

function setLang(lang) {
  state.lang = lang === 'zh' ? 'zh' : 'en';
  document.documentElement.lang = state.lang === 'zh' ? 'zh-CN' : 'en';
  document.querySelectorAll('.lang-btn').forEach((b) => {
    b.classList.toggle('active', b.dataset.lang === state.lang);
  });
}

function onHash() {
  const id = decodeURIComponent((location.hash || '').replace(/^#\/?/, ''));
  if (id && id !== state.selected) select(id, true).catch(() => {});
}

// ---------------------------------------------------------------- wiring

let timer = null;
$('#q').addEventListener('input', (e) => {
  state.query = e.target.value;
  clearTimeout(timer);
  timer = setTimeout(() => search().catch(console.error), 180);
});

document.querySelectorAll('.lang-btn').forEach((b) => {
  b.addEventListener('click', async () => {
    setLang(b.dataset.lang);
    await loadStats();
    renderList();
    if (state.selected) {
      const t = await api('/api/technique', { id: state.selected, lang: state.lang });
      renderDetail(t);
    }
  });
});

window.addEventListener('hashchange', onHash);

(async function init() {
  setLang(state.lang);
  try {
    await loadStats();
    await loadCategories();
    if (location.hash) onHash();
    await search();
  } catch (err) {
    $('#listmeta').textContent = 'failed to load: ' + err.message;
  }
})();
