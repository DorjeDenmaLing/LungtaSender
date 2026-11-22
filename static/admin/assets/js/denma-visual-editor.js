// denma: the visual e-mail builder's stylesheet, phone layout and file version (called by visual-editor.js).

// The width up to which the settings panel opens over the e-mail (stylesheet media query).
const PHONE = 600;

// denmaVersioned adds the build's version (visual-editor.js's ?v=) to a frame file's address.
export function denmaVersioned(url) {
  const v = new URL(document.querySelector('script[src*="/js/visual-editor.js"]')?.src || location.href).searchParams.get('v');
  return v ? `${url}?v=${v}` : url;
}

// denmaEditorFrame gives the builder's frame our stylesheet, and its phone layout.
export function denmaEditorFrame(iframe, base) {
  const doc = iframe.contentDocument;
  const css = doc.createElement('link');
  css.rel = 'stylesheet';
  css.href = new URL(denmaVersioned(`${base}/../denma-email-builder.css`), location.href).href;
  doc.head.appendChild(css);

  if (iframe.contentWindow.innerWidth > PHONE) {
    return;
  }
  // Its button shows "close" (LastPage) while the panel is open.
  const close = () => {
    const btn = doc.querySelector('[data-testid="LastPageOutlinedIcon"]')?.closest('button');
    btn?.click();
    return !!btn;
  };
  const obs = new MutationObserver(() => {
    if (close()) {
      obs.disconnect();
    }
  });
  obs.observe(doc.body, { childList: true, subtree: true });
  setTimeout(() => obs.disconnect(), 10000);
}
