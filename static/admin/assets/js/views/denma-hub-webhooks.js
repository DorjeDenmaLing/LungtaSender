// denma: the hub's Webhooks page: shared Server webhooks (cmd/denma_hubsignup.go).
import Alpine from 'alpinejs';
import { api } from '../main.js';
import * as u from '../utils.js';
import { denmaAgo } from '../denma-ago.js';

const newForm = () => ({ id: 0, name: '', create_tags: false, allowed_from: '', notify_url: '' });

function denmaHubHooks() {
  return {
    hooks: window._denmaHubHooks || [],
    edit: newForm(),
    rotate: { id: 0, name: '', overlap_hours: 0 },
    shown: [],
    ago: denmaAgo,

    centerNames(h) {
      const names = h.centers.map((c) => c.name);
      return names.length > 5 ? `${names.slice(0, 5).join(', ')} and ${names.length - 5} more` : names.join(', ');
    },

    copy(text) {
      u.copyToClipboard(text);
    },

    put(h) {
      const i = this.hooks.findIndex((x) => x.id === h.id);
      if (i < 0) {
        this.hooks.push(h);
      } else {
        this.hooks.splice(i, 1, h);
      }
    },

    // New secrets, shown once.
    show(h) {
      this.shown = [];
      if (h.secret) {
        this.shown.push({ label: 'Secret', value: h.secret });
      }
      if (h.notify_secret) {
        this.shown.push({ label: 'Signing secret for the notices', value: h.notify_secret });
      }
      delete h.secret;
      delete h.notify_secret;
      if (this.shown.length) {
        this.$refs.secretDialog.showModal();
      }
    },

    onAdd() {
      this.edit = newForm();
      this.$refs.hookDialog.showModal();
      this.$nextTick(() => this.$refs.hookName.focus());
    },

    onEdit(h) {
      this.edit = {
        id: h.id, name: h.name, create_tags: h.create_tags, allowed_from: h.allowed_from.join('\n'), notify_url: h.notify_url,
      };
      this.$refs.hookDialog.showModal();
    },

    async onSubmit() {
      const e = this.edit;
      const body = {
        name: e.name,
        create_tags: e.create_tags,
        allowed_from: e.allowed_from.split(/[\s,;]+/).map((x) => x.trim()).filter(Boolean),
        notify_url: e.notify_url.trim(),
      };
      const h = e.id
        ? await api('hubhooks', `/denma/hub/signup-hooks/${e.id}`, 'PUT', body)
        : await api('hubhooks', '/denma/hub/signup-hooks', 'POST', body);
      this.$refs.hookDialog.close();
      if (e.id && !h.notify_secret) {
        u.toast(`Saved ${h.name}.`, 'success');
      }
      this.show(h);
      this.put(h);
    },

    onRotateOpen(h) {
      this.rotate = { id: h.id, name: h.name, overlap_hours: 0 };
      this.$refs.rotateDialog.showModal();
    },

    async onRotate() {
      const h = await api('hubhooks', `/denma/hub/signup-hooks/${this.rotate.id}/key`, 'POST', { overlap_hours: this.rotate.overlap_hours });
      this.$refs.rotateDialog.close();
      this.show(h);
      this.put(h);
    },

    async onNotifySecret(h) {
      if (!(await u.confirm(`Give ${h.name}'s notices a new signing secret? The old one stops working at once, so the system has to be given the new one.`))) {
        return;
      }
      const nh = await api('hubhooks', `/denma/hub/signup-hooks/${h.id}/notify-secret`, 'POST');
      this.show(nh);
      this.put(nh);
    },

    async onTest(h) {
      await api('hubhooks', `/denma/hub/signup-hooks/${h.id}/notify-test`, 'POST');
      u.toast('The notices address took the test event.', 'success');
    },

    async onDelete(h) {
      const where = h.centers.length ? ` It's turned off in the ${h.centers.length} centers it's on in.` : '';
      if (!(await u.confirm(`Delete ${h.name}? Its secret stops working.${where}`))) {
        return;
      }
      await api('hubhooks', `/denma/hub/signup-hooks/${h.id}`, 'DELETE');
      this.hooks = this.hooks.filter((x) => x.id !== h.id);
      u.toast(`Deleted ${h.name}.`, 'success');
    },
  };
}

document.addEventListener('alpine:init', () => {
  Alpine.data('denmaHubHooks', denmaHubHooks);
}, { once: true });
