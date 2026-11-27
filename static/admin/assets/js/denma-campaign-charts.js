// denma: Analytics -> Explore's views, clicks and bounces charts, in place of listmonk's.
import Chart from 'chart.js/auto';

const SERIES = 8;
const HOUR = 3600e3;
const DAY = 24 * HOUR;
const MAX_BUCKETS = 400;

// The page's theme colours, resolved (they're light-dark() tokens).
function themeColors() {
  const probe = document.createElement('span');
  probe.hidden = true;
  document.querySelector('section.analytics').appendChild(probe);
  const get = (v) => {
    probe.style.color = `var(${v})`;
    return getComputedStyle(probe).color;
  };
  const c = {
    text: get('--foreground'),
    muted: get('--muted-foreground'),
    grid: get('--border'),
    card: get('--card'),
    hard: get('--danger'),
    soft: get('--warning'),
    series: Array.from({ length: SERIES }, (_, i) => get(`--denma-series-${i + 1}`)),
  };
  probe.remove();
  return c;
}

// An instant's wall clock in the time zone, as milliseconds read as UTC.
function wall(t, zone) {
  const p = {};
  new Intl.DateTimeFormat('en-US', {
    timeZone: zone, hourCycle: 'h23', year: 'numeric', month: 'numeric', day: 'numeric', hour: 'numeric', minute: 'numeric', second: 'numeric',
  }).formatToParts(t).forEach((x) => { p[x.type] = +x.value; });
  return Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second);
}

// The instant a wall clock (milliseconds read as UTC) shows in the time zone.
function instant(w, zone) {
  const off = (t) => wall(t, zone) - Math.floor(t / 1000) * 1000;
  return w - off(w - off(w));
}

// The bucket a count is in: hours for a range under 7 days, else days (as the server's query does),
// both the center's own (its time zone, which the inputs are in too).
function bucketing(d, zone) {
  const from = document.querySelector('input[name=from]')?.value;
  const to = document.querySelector('input[name=to]')?.value;
  const rows = [...(d.views || []), ...(d.clicks || []), ...(d.bounces || [])];
  const start = instant(Date.parse(`${from}:00Z`), zone);
  const end = instant(Date.parse(`${to}:00Z`), zone);
  const unit = end - start >= 7 * DAY ? DAY : HOUR;
  const floor = (t) => instant(Math.floor(wall(t, zone) / unit) * unit, zone);
  // The next bucket's start: a day is 23 to 25 hours.
  const next = (t) => floor(t + 1.5 * unit);
  let keys = [];
  if (Number.isFinite(start) && Number.isFinite(end)) {
    for (let t = floor(start); t <= end && keys.length <= MAX_BUCKETS; t = next(t)) keys.push(t);
  }
  // A long range: only from the first count to the last.
  if (keys.length > MAX_BUCKETS || !keys.length) {
    const ts = rows.map((r) => floor(Date.parse(r.timestamp)));
    keys = [];
    for (let t = Math.min(...ts); t <= Math.max(...ts); t = next(t)) keys.push(t);
  }
  return { unit, keys, zone, floor };
}

// A bucket's label, in the center's time zone.
function labeler(b, years) {
  const opts = (long) => (b.unit === DAY
    ? { day: 'numeric', month: 'short', year: years || long ? 'numeric' : undefined, weekday: long ? 'short' : undefined, timeZone: b.zone }
    : { day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit', weekday: long ? 'short' : undefined, timeZone: b.zone });
  const at = (t) => new Date(t);
  return {
    short: (t) => at(t).toLocaleString([], opts(false)),
    long: (t) => at(t).toLocaleString([], opts(true)),
    // Hours: the date at midnight, and every 6 hours when there are 2 days or less.
    tick: (t) => {
      if (b.unit === DAY) return at(t).toLocaleString([], opts(false));
      const h = new Date(wall(t, b.zone)).getUTCHours();
      if (h === 0) return at(t).toLocaleDateString([], { day: 'numeric', month: 'short', timeZone: b.zone });
      if (b.keys.length <= 48 && h % 6 === 0) return at(t).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', timeZone: b.zone });
      return null;
    },
  };
}

function tooltip(c) {
  return {
    backgroundColor: c.card,
    borderColor: c.grid,
    borderWidth: 1,
    titleColor: c.text,
    bodyColor: c.text,
    footerColor: c.muted,
    padding: 10,
    boxPadding: 4,
    usePointStyle: true,
  };
}

const charts = [];

function draw(canvas, cfg) {
  if (canvas) charts.push(new Chart(canvas, cfg));
}

// A row's campaign or automation, as the series' keys.
const rowKey = (r) => (r.automation_id ? `a${r.automation_id}` : `c${r.campaign_id}`);

// Bars of one figure per bucket, stacked by camp (camps: { key, label }) or by stack ({ label, color, has(row) }),
// and its totals donut by camp.
function renderCounts(key, camps, rows, b, c, stack) {
  const canvas = document.getElementById(`chart-${key}`);
  if (!canvas) return;
  const index = new Map(b.keys.map((t, i) => [t, i]));
  const years = new Set(b.keys.map((t) => new Date(wall(t, b.zone)).getUTCFullYear())).size > 1;
  const lbl = labeler(b, years);
  const many = camps.length > 1;
  const bars = (label, color, has) => {
    const data = new Array(b.keys.length).fill(0);
    rows.forEach((r) => {
      const i = index.get(b.floor(Date.parse(r.timestamp)));
      if (has(r) && i !== undefined) data[i] += r.count;
    });
    return {
      label, data, backgroundColor: color, borderRadius: 2, maxBarThickness: 32,
    };
  };
  const datasets = camps.map((cm, n) => bars(cm.label, c.series[n % SERIES], (r) => rowKey(r) === cm.key));
  const stacked = stack ? stack.map((st) => bars(st.label, st.color, st.has)) : datasets;
  const legend = many || !!stack;

  draw(canvas, {
    type: 'bar',
    data: { labels: b.keys, datasets: stacked },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: { duration: 300 },
      interaction: { mode: 'index', intersect: false },
      plugins: {
        legend: { display: legend, position: 'bottom', labels: { color: c.text, usePointStyle: true, pointStyle: 'rectRounded' } },
        tooltip: {
          ...tooltip(c),
          callbacks: {
            title: (items) => lbl.long(b.keys[items[0].dataIndex]),
            label: (item) => (legend ? ` ${item.dataset.label}: ${item.raw.toLocaleString()}` : ` ${item.raw.toLocaleString()}`),
          },
        },
      },
      scales: {
        x: {
          stacked: true,
          grid: { display: false },
          border: { color: c.grid },
          ticks: { color: c.muted, maxRotation: 0, autoSkip: b.unit === DAY, autoSkipPadding: 16, callback: (v) => lbl.tick(b.keys[v]) },
        },
        y: {
          stacked: true,
          beginAtZero: true,
          grid: { color: c.grid },
          border: { display: false },
          ticks: { color: c.muted, precision: 0 },
        },
      },
    },
  });

  // The totals donut compares campaigns: only for more than one.
  const donut = document.getElementById(`chart-${key}-donut`);
  const col = donut?.closest('.donut-container');
  if (!many) {
    if (col) col.hidden = true;
    canvas.closest('.col-9')?.classList.replace('col-9', 'col-12');
    return;
  }
  draw(donut, {
    type: 'doughnut',
    data: {
      labels: datasets.map((ds) => ds.label),
      datasets: [{
        data: datasets.map((ds) => ds.data.reduce((a, v) => a + v, 0)),
        backgroundColor: datasets.map((ds) => ds.backgroundColor),
        borderColor: c.card,
        borderWidth: 2,
      }],
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      cutout: '65%',
      animation: { duration: 300 },
      plugins: {
        legend: { display: false },
        tooltip: {
          ...tooltip(c),
          callbacks: {
            label: (item) => {
              const total = item.dataset.data.reduce((a, v) => a + v, 0);
              return ` ${item.raw.toLocaleString()} (${total ? ((item.raw / total) * 100).toFixed(1) : 0}%)`;
            },
          },
        },
      },
    },
  });
}

function render(d) {
  charts.splice(0).forEach((ch) => ch.destroy());
  const c = themeColors();
  Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;
  Chart.defaults.color = c.muted;
  const a = window._denmaExplore || {};
  const camps = (d.campaigns || []).map((cm) => ({ key: `c${cm.id}`, label: `#${cm.id}: ${cm.name}` }))
    .concat((a.automations || []).map((au) => ({ key: `a${au.id}`, label: `Automation: ${au.name}` })));
  const all = {};
  ['views', 'clicks'].forEach((k) => { all[k] = [...(d[k] || []), ...(a[k] || [])]; });
  all.bounces = a.typed_bounces || []; // hard and soft, without complaints (cmd/denma_explore_bounces.go)
  const b = bucketing(all, a.time_zone || 'UTC');
  renderCounts('views', camps, all.views, b, c);
  renderCounts('clicks', camps, all.clicks, b, c);
  renderCounts('bounces', camps, all.bounces, b, c, [
    { label: 'Hard', color: c.hard, has: (r) => r.type === 'hard' },
    { label: 'Soft', color: c.soft, has: (r) => r.type === 'soft' },
  ]);
}

// Draws the charts; returns true, so listmonk's aren't drawn.
export function denmaRenderCharts(d) {
  render(d);
  // The theme: the system's, or the admin's switch (main.js sets color-scheme).
  const redraw = () => requestAnimationFrame(() => render(d));
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', redraw);
  new MutationObserver(redraw).observe(document.documentElement, { attributes: true, attributeFilter: ['style'] });
  return true;
}
