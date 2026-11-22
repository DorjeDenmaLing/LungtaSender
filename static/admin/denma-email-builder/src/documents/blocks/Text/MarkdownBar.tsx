// denma: the Markdown bar over a Text block's Content box: bold, italic, link and lists, with Ctrl/⌘+B, I and K.
import React from 'react';

// GitHub's guide to Markdown.
const MARKDOWN_HELP = 'https://docs.github.com/en/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax';

const MAC = /Mac|iPhone|iPad/.test(navigator.platform);
const KEY = MAC ? '⌘' : 'Ctrl+';

const svg = (body: string) => `<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;

type Format = 'bold' | 'italic' | 'link' | 'bullets' | 'numbers';

const BUTTONS: { f: Format; label: string; key?: string; icon: string }[] = [
  { f: 'bold', label: 'Bold', key: 'b', icon: svg('<path d="M6 4h8a4 4 0 0 1 0 8H6z"/><path d="M6 12h9a4 4 0 0 1 0 8H6z"/>') },
  { f: 'italic', label: 'Italic', key: 'i', icon: svg('<line x1="19" y1="4" x2="10" y2="4"/><line x1="14" y1="20" x2="5" y2="20"/><line x1="15" y1="4" x2="9" y2="20"/>') },
  { f: 'link', label: 'Link', key: 'k', icon: svg('<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>') },
  { f: 'bullets', label: 'Bulleted list', icon: svg('<line x1="9" y1="6" x2="20" y2="6"/><line x1="9" y1="12" x2="20" y2="12"/><line x1="9" y1="18" x2="20" y2="18"/><circle cx="4.5" cy="6" r="1"/><circle cx="4.5" cy="12" r="1"/><circle cx="4.5" cy="18" r="1"/>') },
  { f: 'numbers', label: 'Numbered list', icon: svg('<line x1="10" y1="6" x2="21" y2="6"/><line x1="10" y1="12" x2="21" y2="12"/><line x1="10" y1="18" x2="21" y2="18"/><path d="M4 6h1v4"/><path d="M4 10h2"/><path d="M6 18H4c0-1 2-2 2-3s-1-1.5-2-1"/>') },
];

const HELP_ICON = svg('<circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/>');

// replace puts text over the textarea's start-end as typing does, then selects selStart-selEnd.
function replace(ta: HTMLTextAreaElement, start: number, end: number, text: string, selStart: number, selEnd: number) {
  const doc = ta.ownerDocument;
  ta.focus();
  ta.setSelectionRange(start, end);
  if (!doc.execCommand('insertText', false, text)) {
    const set = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!;
    set.call(ta, ta.value.slice(0, start) + text + ta.value.slice(end));
    ta.dispatchEvent(new Event('input', { bubbles: true }));
  }
  ta.setSelectionRange(selStart, selEnd);
}

// stars counts the *s running back from i (dir -1) or on from i (dir 1).
function stars(v: string, i: number, dir: number) {
  let n = 0;
  for (let j = i; j >= 0 && j < v.length && v[j] === '*'; j += dir) n += 1;
  return n;
}

// has tells whether runs of l and r *s on either side hold bold (n 2) or italic (n 1).
const has = (l: number, r: number, n: number) => (n === 2 ? Math.min(l, r) >= 2 : Math.min(l, r) % 2 === 1);

// emphasis makes the selection bold (n 2) or italic (n 1) with *s, or takes that away if it has it.
function emphasis(ta: HTMLTextAreaElement, n: number) {
  const v = ta.value;
  let s = ta.selectionStart;
  let e = ta.selectionEnd;
  while (s < e && /\s/.test(v[s])) s += 1;
  while (e > s && /\s/.test(v[e - 1])) e -= 1;
  const sel = v.slice(s, e);
  const mark = '*'.repeat(n);
  const inL = stars(sel, 0, 1);
  const inR = stars(sel, sel.length - 1, -1);
  if (inL < sel.length && has(inL, inR, n)) {
    const inner = sel.slice(n, -n);
    replace(ta, s, e, inner, s, s + inner.length);
  } else if (has(stars(v, s - 1, -1), stars(v, e, 1), n)) {
    replace(ta, s - n, e + n, sel, s - n, e - n);
  } else {
    replace(ta, s, e, mark + sel + mark, s + n, e + n);
  }
}

// list makes the selected lines a list, or plain lines again if they're all already that list.
function list(ta: HTMLTextAreaElement, numbered: boolean) {
  const v = ta.value;
  const s = v.lastIndexOf('\n', ta.selectionStart - 1) + 1;
  // A selection ending at a line's start doesn't take that line.
  let end = ta.selectionEnd;
  if (end > ta.selectionStart && v[end - 1] === '\n') end -= 1;
  let e = v.indexOf('\n', end);
  if (e < 0) e = v.length;
  const lines = v.slice(s, e).split('\n');
  const item = numbered ? /^\s*\d+[.)]\s+/ : /^\s*[-*+]\s+/;
  const any = /^\s*(?:[-*+]|\d+[.)])\s+/;
  const filled = lines.filter((l) => l.trim());
  let out;
  if (filled.length && filled.every((l) => item.test(l))) {
    out = lines.map((l) => l.replace(item, ''));
  } else {
    let i = 0;
    out = lines.map((l) => (l.trim() ? `${numbered ? `${i += 1}.` : '-'} ${l.replace(any, '')}` : l));
  }
  const text = out.join('\n');
  replace(ta, s, e, text, s, s + text.length);
}

// link makes the selection a link, asking for its address.
function link(ta: HTMLTextAreaElement) {
  const win = ta.ownerDocument.defaultView!;
  const s = ta.selectionStart;
  const e = ta.selectionEnd;
  const url = (win.prompt('Link address', 'https://') || '').trim();
  if (!url || url === 'https://') {
    ta.focus();
    ta.setSelectionRange(s, e);
    return;
  }
  const text = ta.value.slice(s, e) || 'link text';
  replace(ta, s, e, `[${text}](${url})`, s + 1, s + 1 + text.length);
}

const FORMATS: Record<Format, (ta: HTMLTextAreaElement) => void> = {
  bold: (ta) => emphasis(ta, 2),
  italic: (ta) => emphasis(ta, 1),
  link,
  bullets: (ta) => list(ta, false),
  numbers: (ta) => list(ta, true),
};

const textarea = (el: HTMLElement | null) => el?.querySelector<HTMLTextAreaElement>('textarea:not([aria-hidden])') ?? null;

// MarkdownBar is the bar and its Content box (children), whose textarea it formats.
export default function MarkdownBar({ children }: { children: JSX.Element }) {
  const ref = React.useRef<HTMLDivElement>(null);
  const onKeyDown = (ev: React.KeyboardEvent) => {
    if (!(MAC ? ev.metaKey : ev.ctrlKey) || ev.altKey || ev.shiftKey) {
      return;
    }
    const b = BUTTONS.find((x) => x.key === ev.key.toLowerCase());
    const ta = textarea(ref.current);
    if (b && ta && ev.target === ta) {
      ev.preventDefault();
      FORMATS[b.f](ta);
    }
  };
  return (
    <div ref={ref} onKeyDownCapture={onKeyDown}>
      <div className="denma-md-bar" role="toolbar" aria-label="Markdown formatting">
        {BUTTONS.map((b) => (
          <button
            key={b.f}
            type="button"
            data-format={b.f}
            title={b.key ? `${b.label} (${KEY}${b.key.toUpperCase()})` : b.label}
            aria-label={b.label}
            dangerouslySetInnerHTML={{ __html: b.icon }}
            // The textarea keeps the focus, and its selection.
            onMouseDown={(ev) => ev.preventDefault()}
            onClick={() => {
              const ta = textarea(ref.current);
              if (ta) FORMATS[b.f](ta);
            }}
          />
        ))}
        <a
          className="denma-md-help"
          href={MARKDOWN_HELP}
          target="_blank"
          rel="noopener noreferrer"
          title="How to write Markdown (GitHub)"
          aria-label="How to write Markdown (GitHub)"
          dangerouslySetInnerHTML={{ __html: HELP_ICON }}
        />
      </div>
      {children}
    </div>
  );
}
