// denma: e-mails someone a link to set a new password (cmd/denma_password_reset.go).
import { api } from './main.js';
import * as u from './utils.js';

export function denmaPasswordReset() {
  return {
    // Asks, then sends the link through path (after /api), for the person named who at address email.
    async denmaSendReset(path, who, email) {
      const to = email ? `${who} (${email})` : who;
      if (!(await u.confirm(`E-mail ${to} a link to set a new password? It works for 30 minutes. Their password stays as it is until they use it.`))) {
        return;
      }
      await api('denma.reset', path, 'POST');
      u.toast(`Sent ${who} a link to set a new password.`);
    },
  };
}
