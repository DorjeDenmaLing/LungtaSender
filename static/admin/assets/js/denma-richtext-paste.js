// denma: images pasted into the rich-text editor (a screenshot, or HTML with data: images) are uploaded to the
// center's Media and inserted by their URL.
import { urls } from './main.js';
import { toast } from './utils.js';

const isImage = (file) => file.type.startsWith('image/');

// dataBlob is the file in a data: URL.
function dataBlob(src) {
  const [head, data = ''] = src.split(/,(.*)/s);
  const type = head.slice(5).split(';')[0];
  const raw = head.endsWith(';base64') ? atob(data) : decodeURIComponent(data);
  return new Blob([Uint8Array.from(raw, (c) => c.charCodeAt(0))], { type });
}

// denmaPasteImages makes the <richtext-editor> el upload pasted images.
export function denmaPasteImages(el) {
  el.editor.addEventListener('pasteImage', (e) => {
    const files = [...(e.detail.clipboardData?.files || [])].filter(isImage);
    if (files.length) {
      upload(el, document.createDocumentFragment(), files);
    }
  });

  el.editor.addEventListener('willPaste', (e) => {
    const { fragment } = e.detail;
    if (fragment?.querySelector('img[src^="data:image/"]')) {
      e.preventDefault();
      upload(el, fragment, []);
    }
  });
}

// upload puts the images (files, or the data: images in fragment) into Media, then inserts the fragment.
async function upload(el, fragment, files) {
  if (el.inert) {
    return;
  }

  const ed = el.editor;
  const range = ed.getSelection().cloneRange();
  const uploads = files.length
    ? files.map((file) => {
      const img = document.createElement('img');
      fragment.append(img);
      return [img, file];
    })
    : [...fragment.querySelectorAll('img[src^="data:image/"]')].map((img) => [img, null]);

  const controller = new AbortController();
  const { signal } = controller;
  const cancel = (e) => {
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      controller.abort();
    }
  };

  // The editor is locked while uploading, so Escape is heard outside it.
  document.addEventListener('keydown', cancel, true);
  el.setAttribute('data-spinner', 'overlay');
  el.setAttribute('aria-busy', 'true');
  el.inert = true;

  try {
    for (const [img, file] of uploads) {
      const blob = file || dataBlob(img.src);
      const body = new FormData();
      body.set('file', blob, blob.name || `image.${blob.type.split('/')[1].split('+')[0]}`);
      const resp = await fetch(`${urls.api}/media`, { method: 'POST', body, signal, headers: { Accept: 'application/json' } });
      if (!resp.ok) {
        const out = await resp.json().catch(() => ({}));
        throw new Error(out.message || resp.statusText);
      }
      const out = await resp.json();
      signal.throwIfAborted();

      img.setAttribute('src', out.data.url);
      img.removeAttribute('srcset');
    }

    if (el.isConnected && el.editor === ed && fragment.hasChildNodes()) {
      const holder = document.createElement('div');
      holder.append(fragment);
      ed.setSelection(range);
      ed.insertHTML(holder.innerHTML);
    }
  } catch (err) {
    if (!signal.aborted) {
      toast(err.message, 'danger');
    }
  } finally {
    document.removeEventListener('keydown', cancel, true);
    el.inert = false;
    el.removeAttribute('aria-busy');
    if (el.isConnected && el.editor === ed) {
      ed.focus();
    }
  }
}
