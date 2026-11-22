// denma: a campaign's From set to the center's contact e-mail (cmd/denma_center_from.go).
import { config } from './main.js';

// The address of a From ("Name" <a@b> or a@b), lowercase.
const addrOf = (from) => {
  const m = /<([^>]*)>\s*$/.exec(from || '');
  return (m ? m[1] : from || '').trim().toLowerCase();
};

// The name of a From, or ''.
const nameOf = (from) => {
  const m = /^\s*"?([^"<]*?)"?\s*</.exec(from || '');
  return m ? m[1].trim() : '';
};

export function denmaCenterFrom(isNew, from) {
  const center = window._denmaCenterFrom || {};
  const email = center.email || '';
  return {
    denmaFromCenter: !isNew && !!email && addrOf(from) === email.toLowerCase(),
    _denmaFromPrev: '',

    // Unticks the switch when the From changes to another address (a merged save).
    denmaCenterFromInit() {
      this.$watch('form.from_email', (v) => {
        if (this.denmaFromCenter && addrOf(v) !== email.toLowerCase()) {
          this.denmaFromCenter = false;
        }
      });
    },

    // Sets the From to the center's e-mail with the From's name, or back to the address it had.
    onDenmaFromCenter(on) {
      this.denmaFromCenter = on;
      if (on) {
        this._denmaFromPrev = this.form.from_email;
        const name = (nameOf(this.form.from_email) || center.name || '').replace(/["\\]/g, '');
        this.form.from_email = name ? `"${name}" <${email}>` : email;
        return;
      }
      this.form.from_email = this._denmaFromPrev || config.from_email || '';
    },
  };
}
