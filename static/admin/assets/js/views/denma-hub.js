// denma: the hub's dashboard (partials/denma/hub.html): every center's sending from /api/denma/hub/stats.
import Alpine from 'alpinejs';
import { urls } from '../main.js';
import * as s from '../denma-stats.js';
import { centerPicker, hubFigures, hubStats, savedCenter, saveCenter } from '../denma-hub-ui.js';

// [key, label, format, higher is good, description]
const METRICS = [
  ['sends', 'Total sends', 'count', true, 'Emails sent by campaigns that started in this period.'],
  ['delivery', 'Delivery rate', 'pct1', true, "Sent emails that didn't bounce."],
  ['open', 'Open rate', 'pct1', true, 'Unique people who opened, as a share of delivered emails (per campaign).'],
  ['click', 'Click rate', 'pct2', true, 'Unique people who clicked a link, as a share of delivered emails (per campaign).'],
  ['unsub', 'Unsubscribed rate', 'pct2', false, 'People who unsubscribed in this period (not bounce removals), as a share of delivered emails.'],
  ['newSubs', 'New contacts', 'count', true, 'Contacts added in this period.'],
];

const KEYS = ['campaigns', 'sends', 'camp_bounces', 'tracked_delivered', 'opens', 'clicks', 'unlinked', 'unsubs', 'hard', 'soft', 'complaint', 'feedback_sends', 'feedback_complaint', 'new_subscribers'];

// The SES account's figure, shown for all centers (cmd/denma_ses_reputation.go).
const SES_HELP = "Amazon SES's own figure for all centers together, as it stands now (not for this period): complaints over recent emails to providers that report them (Gmail doesn't). SES reviews accounts at 0.1% and can pause sending at 0.5%.";
const SES_STATUS = { HEALTHY: 'Healthy', PROBATION: 'Under review', SHUTDOWN: 'Sending paused' };

// sesNote says how the SES account stands, and whether it's sending.
function sesNote(r) {
  if (!r) {
    return '';
  }
  if (!r.available) {
    return `Amazon SES's own figures aren't shown${r.reason ? ` (${r.reason.replace(/\.$/, '')})` : ''}; the complaint rate is worked out from the centers' emails, as SES counts them.`;
  }
  let t = `Amazon SES account: ${SES_STATUS[r.status] || r.status || 'status not given'}${r.sending_enabled ? '' : ', sending paused'}.`;
  if (r.at) {
    const at = new Date(r.at);
    t += ` Its rates were last updated ${s.fmtDate(at)}, ${s.fmtTime(at)}.`;
  }
  return t;
}

const placeholders = (list) => list.map((m) => ({ key: m[0], label: m[1], help: m[4] || m[3], value: '…', change: null, health: null }));

// Adds up the periods of some centers.
function total(list, k) {
  const out = Object.fromEntries(KEYS.map((x) => [x, 0]));
  list.forEach((c) => {
    if (c[k]) {
      KEYS.forEach((x) => { out[x] += c[k][x] || 0; });
    }
  });
  return out;
}

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

function component() {
  return {
    ...s.loadRange(),
    ...centerPicker([{ slug: '', name: 'All centers' }]),
    ranges: s.RANGES,
    center: savedCenter(),
    centers: [],
    perf: { cells: placeholders(METRICS) },
    bounce: { cells: placeholders(s.BOUNCE_METRICS) },
    ses: null,
    sesNote: '',
    sesAlert: false,
    problems: { text: '', url: '' },
    note: '',
    busy: true,
    seq: 0,

    get selected() {
      return this.centers.find((c) => c.slug === this.center) || null;
    },

    init() {
      this.load();
    },

    onRange() {
      if (this.range !== 'custom') {
        this.load();
      } else if (!this.from) {
        this.from = s.isoDay(s.addDays(new Date(), -30));
        this.to = s.isoDay(new Date());
      }
    },

    onCenter() {
      saveCenter(this.center);
      this.render();
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
      this.p = p;

      // SES's figures come in on their own, so a slow answer doesn't hold up the rest.
      s.getJSON('/denma/hub/ses').catch(() => null).then((r) => {
        if (seq !== this.seq) return;
        this.ses = r;
        if (this.centers.length) this.render(); // otherwise the figures' render shows it
      });
      try {
        const data = await hubStats(p);
        if (seq !== this.seq) return;
        this.centers = data || [];
        if (this.center && !this.selected) {
          this.center = '';
        }
        this.render();
      } catch (err) {
        if (seq !== this.seq) return;
        this.note = `Couldn't load the figures (${err.message}). Try reloading the page.`;
      } finally {
        if (seq === this.seq) this.busy = false;
      }
    },

    render() {
      const list = this.selected ? [this.selected] : this.centers;
      const cur = hubFigures(total(list, 'cur'));
      const prev = hubFigures(total(list, 'prev'));

      this.perf.cells = METRICS.map((m) => ({
        key: m[0],
        label: m[1],
        help: m[4],
        value: s.fmt(cur[m[0]], m[2]),
        change: s.change(cur[m[0]], prev[m[0]], m[3], m[2]),
        health: null,
      }));
      this.bounce.cells = s.BOUNCE_METRICS.map((m) => ({
        key: m[0],
        label: m[1],
        help: m[3],
        value: s.fmt(cur[m[0]], m[2]),
        change: s.change(cur[m[0]], prev[m[0]], false, m[2], true),
        health: s.health(cur[m[0]], m[4]),
      }));
      const r = this.ses;
      if (!this.center && r && r.available && r.complaint_rate != null) {
        const cell = this.bounce.cells.find((x) => x.key === 'complaintRate');
        Object.assign(cell, {
          label: 'Complaint rate (Amazon SES)',
          help: SES_HELP,
          value: s.fmt(r.complaint_rate, 'pct2'),
          change: { cls: '', arrow: '', text: 'now, all centers', title: SES_HELP },
          health: s.health(r.complaint_rate, s.BOUNCE_METRICS[4][4]),
        });
      }
      this.sesNote = sesNote(r);
      this.sesAlert = !!(r && r.available && ((r.status && r.status !== 'HEALTHY') || !r.sending_enabled));

      const days = this.p ? s.periodDays(this.p) : 0;
      const subs = list.reduce((n, c) => n + (c.subscribers || 0), 0);
      const sending = list.filter((c) => c.cur && c.cur.campaigns > 0).length;
      const who = this.selected ? '' : ` from ${sending} of ${plural(list.length, 'center', 'centers')}`;
      this.note = `${plural(cur.campaigns, 'campaign', 'campaigns')}${who} in this period, compared with the previous ${plural(days, 'day', 'days')}. ${subs.toLocaleString()} contacts in all.`;
      const unlinked = total(list, 'cur').unlinked;
      if (unlinked) {
        this.note += ` ${plural(unlinked, "open isn't", "opens aren't")} linked to anyone (people deleted since, or opened while individual tracking was off), so the open rate leaves ${unlinked === 1 ? 'it' : 'them'} out.`;
      }
      if (window._LM_CONFIG?.privacy?.individual_tracking === false) {
        this.note += ' ⚠ Individual subscriber tracking is off (Settings → Privacy): new opens and clicks aren\'t linked to people, so campaigns sent now get no open or click rates.';
      }

      // Centers not running (other than disabled ones), to look into.
      const failed = this.centers.filter((c) => c.error && c.status !== 'disabled' && !c.starting).length;
      const starting = this.centers.filter((c) => c.starting).length;
      const text = [];
      if (failed) text.push(`${plural(failed, 'center', 'centers')} didn't start.`);
      if (starting) text.push(`${plural(starting, 'center is', 'centers are')} still starting.`);
      this.problems = {
        text: text.join(' '),
        url: `${urls.admin}/centers?status=${failed ? 'error' : 'starting'}`,
      };
    },
  };
}

document.addEventListener('alpine:init', () => {
  Alpine.data('denmaHub', component);
}, { once: true });
