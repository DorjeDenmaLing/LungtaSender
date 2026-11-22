// denma: Analytics -> Complaints (partials/denma/complaints.html, cmd/denma_complaint_days.go).
import Alpine from 'alpinejs';
import * as s from '../denma-stats.js';

const SES_BANDS = s.BOUNCE_METRICS.find((m) => m[0] === 'complaintRate')[4];
const GMAIL_BANDS = s.BOUNCE_METRICS.find((m) => m[0] === 'gmailSpam')[4];
const SES_MIN = 100; // SES's deliveries in a day before its daily rate is drawn
const LIMITS = [0.001];
const HEALTH_CLASS = { warning: 'denma-rate-watch', danger: 'denma-rate-risk' };

// [key, label, format, description, bands]
const CELLS = [
  ['sent', 'Emails sent', 'count', 'Emails the center sent in this period, by UTC day.'],
  ['complaints', 'Complaints', 'count', "People who marked the center's mail as spam, as reported to Amazon SES. Gmail doesn't report them."],
  ['sesRate', 'Complaint rate (Amazon SES)', 'pct2', "Amazon SES's own complaint rate for the center's domain over this period. It should be under 0.1%.", SES_BANDS],
  ['gmailAvg', 'Complaint rate (Gmail)', 'pct2', "Gmail users who reported the center's mail as spam, as a share of its mail in their inboxes, averaged over this period's days. Postmaster Tools can be 2 to 4 days behind. It should be under 0.1%.", GMAIL_BANDS],
  ['gmailWorst', "Gmail's worst day", 'pct2', "Gmail's highest daily complaint rate in this period. Postmaster Tools can be 2 to 4 days behind.", GMAIL_BANDS],
];

const dayDate = (day) => new Date(`${day}T12:00:00`);
const sesRate = (d) => (d.ses_deliveries >= SES_MIN ? d.ses_complaints / d.ses_deliveries : null);
const cls = (v, bands) => {
  const h = s.health(v, bands);
  return (h && HEALTH_CLASS[h.variant]) || '';
};

// The daily rates as lines, with the limits, at the container's width.
function chart(days, width) {
  const NS = 'http://www.w3.org/2000/svg';
  const W = Math.max(300, Math.round(width || 900));
  const H = W < 500 ? 200 : 240;
  const top = 12;
  const bottom = 26;
  const left = 46;
  const n = days.length;
  const ses = days.map(sesRate);
  const gmail = days.map((d) => d.gmail_spam_rate);
  const max = Math.max(0.002, ...ses.filter((v) => v != null), ...gmail.filter((v) => v != null)) * 1.1;
  const y = (v) => top + (H - top - bottom) * (1 - v / max);
  const slot = (W - left) / Math.max(1, n);
  const x = (i) => left + slot * i + slot / 2;

  const svg = document.createElementNS(NS, 'svg');
  svg.setAttribute('viewBox', `0 0 ${W} ${H}`);
  svg.setAttribute('class', 'denma-chart');
  svg.setAttribute('role', 'img');
  svg.setAttribute('aria-label', 'Complaint rates per day, Amazon SES and Gmail, against 0.1%');
  const node = (parent, tag, attrs, text) => {
    const e = document.createElementNS(NS, tag);
    Object.entries(attrs).forEach(([k, v]) => e.setAttribute(k, v));
    if (text != null) e.textContent = text;
    parent.appendChild(e);
    return e;
  };

  const step = max > 0.02 ? 0.005 : (max > 0.008 ? 0.002 : 0.001);
  for (let v = 0; v <= max; v += step) {
    node(svg, 'line', { x1: left, x2: W, y1: y(v), y2: y(v), class: v === 0 ? 'axis' : 'grid' });
    node(svg, 'text', { x: left - 6, y: y(v) + 4, 'text-anchor': 'end', class: 'tick' }, `${(v * 100).toFixed(1)}%`);
  }
  LIMITS.forEach((v) => node(svg, 'line', { x1: left, x2: W, y1: y(v), y2: y(v), class: 'limit' }));

  // Hover targets, with the day's figures.
  const every = Math.ceil(n / Math.max(3, Math.floor((W - left) / 80)));
  days.forEach((d, i) => {
    const g = node(svg, 'g', {});
    const parts = [`${s.fmtDate(dayDate(d.day))}: ${d.sent.toLocaleString()} sent, ${d.complaints} complaint${d.complaints === 1 ? '' : 's'}`];
    if (d.ses_deliveries) parts.push(`Amazon SES ${d.ses_complaints} of ${d.ses_deliveries.toLocaleString()}${ses[i] != null ? ` (${s.fmt(ses[i], 'pct2')})` : ''}`);
    if (d.gmail_spam_rate != null) parts.push(`Gmail ${s.fmt(d.gmail_spam_rate, 'pct2')}`);
    node(g, 'title', {}, parts.join('; '));
    node(g, 'rect', { x: left + slot * i, width: slot, y: top, height: H - top - bottom, class: 'hit' });
    if (i % every === 0) {
      node(svg, 'text', { x: x(i), y: H - 8, 'text-anchor': 'middle', class: 'tick' }, dayDate(d.day).toLocaleDateString([], { month: 'short', day: 'numeric' }));
    }
  });

  // A line through the days with a figure, dashed across days without one, and a dot on each.
  [[ses, 'ses'], [gmail, 'gmail']].forEach(([vals, c]) => {
    let prev = null;
    vals.forEach((v, i) => {
      if (v == null) return;
      const pt = [x(i).toFixed(1), y(v).toFixed(1)];
      if (prev) {
        const gap = i - prev.i > 1 ? ' gap' : '';
        node(svg, 'line', { x1: prev.pt[0], y1: prev.pt[1], x2: pt[0], y2: pt[1], class: `line ${c}${gap}` });
      }
      prev = { i, pt };
    });
    vals.forEach((v, i) => {
      if (v != null) node(svg, 'circle', { cx: x(i), cy: y(v), r: 3, class: `dot ${c}` });
    });
  });
  return svg;
}

function component() {
  return {
    ...s.loadRange(),
    ranges: s.RANGES,
    summary: { cells: CELLS.map((m) => ({ key: m[0], label: m[1], help: m[3], value: '…', change: null, health: null })) },
    rows: [],
    days: [],
    note: '',
    busy: true,
    seq: 0,

    init() {
      this.load();
      let t;
      window.addEventListener('resize', () => {
        clearTimeout(t);
        t = setTimeout(() => this.draw(), 150);
      });
    },

    onRange() {
      if (this.range !== 'custom') {
        this.load();
      } else if (!this.from) {
        this.from = s.isoDay(s.addDays(new Date(), -30));
        this.to = s.isoDay(new Date());
      }
    },

    async load() {
      const p = s.periods(this);
      if (!p) {
        this.note = 'Choose a start and end date.';
        return;
      }
      s.saveRange(this);
      this.seq += 1;
      const { seq } = this;
      this.busy = true;
      try {
        const q = new URLSearchParams({ from: p.cur.start.toISOString(), to: p.cur.end.toISOString() });
        const data = await s.getJSON(`/denma/stats/complaint-days?${q}`);
        if (seq !== this.seq) return;
        this.render(data);
      } catch (err) {
        if (seq !== this.seq) return;
        this.note = `Couldn't load the figures (${err.message}). Try reloading the page.`;
      } finally {
        if (seq === this.seq) this.busy = false;
      }
    },

    render(data) {
      const days = data.days || [];
      this.days = days;
      const sum = (k) => days.reduce((n, d) => n + (d[k] || 0), 0);
      const gm = days.filter((d) => d.gmail_spam_rate != null);
      const worst = gm.reduce((w, d) => (!w || d.gmail_spam_rate > w.gmail_spam_rate ? d : w), null);
      const deliveries = sum('ses_deliveries');
      const f = {
        sent: sum('sent'),
        complaints: sum('complaints'),
        sesRate: deliveries > 0 ? sum('ses_complaints') / deliveries : null,
        gmailAvg: gm.length ? gm.reduce((n, d) => n + d.gmail_spam_rate, 0) / gm.length : null,
        gmailWorst: worst ? worst.gmail_spam_rate : null,
      };
      this.summary.cells = CELLS.map((m) => ({
        key: m[0],
        label: m[1],
        help: m[3],
        value: s.fmt(f[m[0]], m[2]),
        change: m[0] === 'gmailWorst' && worst ? { cls: '', arrow: '', text: s.fmtDate(dayDate(worst.day)), title: '' } : null,
        health: s.health(f[m[0]], m[4]),
      }));

      this.rows = days.filter((d) => d.sent || d.complaints || d.ses_deliveries || d.gmail_spam_rate != null)
        .reverse()
        .map((d) => {
          const r = sesRate(d);
          return {
            day: d.day,
            dayText: s.fmtDate(dayDate(d.day)),
            sentText: d.sent.toLocaleString(),
            complaintsText: String(d.complaints),
            sesText: d.ses_deliveries != null ? `${d.ses_complaints} of ${d.ses_deliveries.toLocaleString()}` : '—',
            sesRateText: r != null ? s.fmt(r, 'pct2') : '—',
            sesClass: cls(r, SES_BANDS),
            gmailText: d.gmail_spam_rate != null ? s.fmt(d.gmail_spam_rate, 'pct2') : '—',
            gmailClass: cls(d.gmail_spam_rate, GMAIL_BANDS),
          };
        });

      const notes = [];
      if (!data.domains) {
        notes.push("Amazon SES's and Gmail's own figures are by sending domain, and this center has no domain of its own yet, so only its complaints are shown.");
      } else {
        notes.push(`Days are UTC days, as Amazon SES and Gmail count them. SES's daily rate is drawn for days with at least ${SES_MIN} of its deliveries; Gmail gives a rate only for days with enough mail to Gmail users, and its Postmaster Tools can be 2 to 4 days behind.`);
      }
      const since = data.sent_since ? new Date(data.sent_since) : null;
      if (days.length && since && since > new Date(`${days[0].day}T00:00:00Z`)) {
        notes.push(`Emails sent are counted from ${s.fmtDate(since)}.`);
      }
      this.note = notes.join(' ');
      this.$nextTick(() => this.draw());
    },

    draw() {
      const box = this.$refs.chart;
      if (box && this.days.length) box.replaceChildren(chart(this.days, box.clientWidth));
    },
  };
}

document.addEventListener('alpine:init', () => {
  Alpine.data('denmaComplaints', component);
});
