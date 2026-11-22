// denma: rich text in Text and Heading blocks: the editor's extensions, and converting Markdown and plain text to it.
import { Editor, Extension, InputRule, Extensions, wrappingInputRule } from '@tiptap/core';
import Blockquote from '@tiptap/extension-blockquote';
import Document from '@tiptap/extension-document';
import Strike from '@tiptap/extension-strike';
import { Color, TextStyle } from '@tiptap/extension-text-style';
import { Plugin } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import StarterKit from '@tiptap/starter-kit';

import { renderMarkdownString, sanitizer } from '@usewaypoint/block-text';

// Template tags ({{ UnsubscribeURL }}) and listmonk's @TrackLink are kept as link addresses.
const TEMPLATE_TAG = /\{\{.*\}\}/;

// Links made in the editor open in a new tab, as Markdown's do; others keep their own target.
export const NEW_LINK = { target: '_blank' };

const LINK_RULE = /\[([^\]]+)\]\(([^)\s]+)\)$/;

// LinkRule makes [text](address), typed, a link.
const LinkRule = Extension.create({
  name: 'denmaLinkRule',
  addInputRules() {
    return [
      new InputRule({
        find: LINK_RULE,
        handler: ({ state, range, match }) => {
          const link = state.schema.marks.link.create({ href: match[2], ...NEW_LINK });
          state.tr.replaceWith(range.from, range.to, state.schema.text(match[1], [link]));
        },
      }),
    ];
  },
});

// Quotes made in the editor have a line down their left.
export const QUOTE_STYLE = 'margin:0;padding-left:16px;border-left:3px solid #cccccc';

// Quote is a blockquote, started by typing "> ".
const Quote = Blockquote.extend({
  addInputRules() {
    return [wrappingInputRule({ find: /^\s*>\s$/, type: this.type, getAttributes: { style: QUOTE_STYLE } })];
  },
});

// Del is strikethrough, saved as <del> as Markdown's ~~ is.
const Del = Strike.extend({
  renderHTML({ HTMLAttributes }) {
    return ['del', HTMLAttributes, 0];
  },
});

const TAG_TEXT = /\{\{.*?\}\}/g;

// TagMarks shades template tags ({{ ... }}) in the editor; the text is unchanged.
const TagMarks = Extension.create({
  name: 'denmaTagMarks',
  addProseMirrorPlugins() {
    return [new Plugin({
      props: {
        decorations(state) {
          const marks: Decoration[] = [];
          state.doc.descendants((node, pos) => {
            if (!node.isText) return;
            for (const m of (node.text || '').matchAll(TAG_TEXT)) {
              marks.push(Decoration.inline(pos + m.index!, pos + m.index! + m[0].length, { class: 'denma-rich-tag' }));
            }
          });
          return DecorationSet.create(state.doc, marks);
        },
      },
    })];
  },
});

// BlockStyle keeps the style of paragraphs, headings, lists and links, as Markdown's HTML and plain text converted have them.
const BlockStyle = Extension.create({
  name: 'denmaBlockStyle',
  addGlobalAttributes() {
    return [{
      types: ['paragraph', 'heading', 'bulletList', 'orderedList', 'listItem', 'blockquote', 'link'],
      attributes: {
        style: {
          default: null,
          keepOnSplit: true,
          parseHTML: (el) => el.getAttribute('style'),
          renderHTML: (attrs) => (attrs.style ? { style: attrs.style } : {}),
        },
      },
    }];
  },
});

// pastedHtml takes the styles out of pasted HTML.
export const pastedHtml = (html: string) => html.replace(/\sstyle="[^"]*"/gi, '');

const LINK = {
  openOnClick: false,
  autolink: true,
  HTMLAttributes: { target: null, rel: null, class: null },
  isAllowedUri: (url: string, ctx: { defaultValidate: (u: string) => boolean }) => TEMPLATE_TAG.test(url) || ctx.defaultValidate(url),
};

export function richTextExtensions(): Extensions {
  return [
    StarterKit.configure({
      blockquote: false,
      code: false,
      codeBlock: false,
      strike: false,
      underline: false,
      trailingNode: false,
      heading: { levels: [1, 2, 3] },
      link: LINK,
    }),
    Quote,
    Del,
    TextStyle,
    Color,
    LinkRule,
    BlockStyle,
    TagMarks,
  ];
}

// headingExtensions are a heading's: one line of italic, strikethrough, colour and links.
export function headingExtensions(): Extensions {
  return [
    Document.extend({ content: 'paragraph' }),
    StarterKit.configure({
      document: false,
      blockquote: false,
      bold: false,
      bulletList: false,
      code: false,
      codeBlock: false,
      hardBreak: false,
      heading: false,
      horizontalRule: false,
      listItem: false,
      listKeymap: false,
      orderedList: false,
      strike: false,
      underline: false,
      trailingNode: false,
      link: LINK,
    }),
    Del,
    TextStyle,
    Color,
    LinkRule,
    TagMarks,
  ];
}

// headingText is a heading's text as saved: plain text if it has no formatting, else its HTML.
export function headingText(editor: Editor): { text: string; format?: 'html' } {
  const html = hexColours(editor.getHTML()).replace(/^<p>|<\/p>$/g, '');
  if (!/</.test(html)) {
    return { text: new DOMParser().parseFromString(`<body>${html}</body>`, 'text/html').body.textContent || '' };
  }
  return { text: html, format: 'html' };
}

// headingContent is a heading's text for the editor.
export const headingContent = (text: string, format?: string | null) => `<p>${format === 'html' ? text : escapeText(text)}</p>`;

const hex = (n: string) => Number(n).toString(16).padStart(2, '0');

// hexColours writes the browser's rgb() colours in styles as #rrggbb.
const hexColours = (html: string) => html.replace(/ style="[^"]*"/g, (st) => st.replace(/rgb\((\d+), ?(\d+), ?(\d+)\)/g, (_m, r, g, b) => `#${hex(r)}${hex(g)}${hex(b)}`));

// editorHtml is the editor's HTML as saved: a list item's only paragraph unwrapped, as Markdown writes it, and no empty paragraphs.
export function editorHtml(editor: Editor): string {
  return hexColours(editor.getHTML())
    .replace(/<li([^>]*)><p>((?:(?!<\/?p[ >]).)*)<\/p><\/li>/g, '<li$1>$2</li>')
    .replace(/<p(?: style="[^"]*")?><\/p>/g, '');
}

function escapeText(s: string) { return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'); }

// Whitespace around these tags doesn't show.
const BLOCK_SPACE = /\s*(<\/?(?:body|p|ul|ol|li|h[1-6]|blockquote|hr|br|div|table|thead|tbody|tr|td|th)\b[^>]*>)\s*/g;

// canonical writes HTML's elements, sorted attributes (styles as the browser writes them) and text with whitespace collapsed.
function canonical(html: string): string {
  const doc = new DOMParser().parseFromString(`<body>${html}</body>`, 'text/html');
  doc.querySelectorAll('li > p:only-child').forEach((p) => p.replaceWith(...p.childNodes));
  const walk = (n: Node): string => {
    if (n.nodeType === Node.TEXT_NODE) {
      return (n.textContent || '').replace(/\s+/g, ' ');
    }
    if (n.nodeType !== Node.ELEMENT_NODE) {
      return '';
    }
    const el = n as Element;
    const attrs = [...el.attributes].map((a) => `${a.name}="${a.name === 'style' ? (el as HTMLElement).style.cssText : a.value}"`).sort().join(' ');
    const inner = [...el.childNodes].map(walk).join('');
    return `<${el.tagName.toLowerCase()}${attrs ? ` ${attrs}` : ''}>${inner}</${el.tagName.toLowerCase()}>`;
  };
  return walk(doc.body).replace(BLOCK_SPACE, '$1').replace(/<\/?body>/g, '');
}

// toRichText gives a Markdown or plain block's text as rich text, or why the editor can't keep it exactly.
export function toRichText(text: string, markdown: boolean): { html: string } | { reason: string } {
  const html = markdown ? renderMarkdownString(text) : sanitizer(`<p style="margin:0">${escapeText(text)}</p>`);
  const editor = new Editor({ extensions: richTextExtensions(), content: html });
  const out = editorHtml(editor);
  editor.destroy();
  if (canonical(out) !== canonical(html)) {
    return { reason: 'It has formatting the rich text editor can\'t keep (such as images, tables or styled text), so it stays Markdown.' };
  }
  return { html };
}

// looksLikeMarkdown tells whether pasted text uses Markdown's emphasis, headings, lists, quotes or links.
export function looksLikeMarkdown(s: string): boolean {
  return /(\*\*|__)\S|(^|\n)(#{1,3} |[-*+] |\d+\. |> )|\[[^\]]+\]\([^)\s]+\)/.test(s);
}

// markdownHtml is pasted Markdown as HTML for the editor.
export const markdownHtml = (s: string) => renderMarkdownString(s);

// Characters Markdown would read as formatting, in text.
const escapeMarkdown = (s: string) => s.replace(/([\\`*_[\]<])/g, '\\$1');

// mdInline writes an element's inline content as Markdown.
function mdInline(n: Node): string {
  if (n.nodeType === Node.TEXT_NODE) {
    return escapeMarkdown((n.textContent || '').replace(/\s+/g, ' '));
  }
  if (n.nodeType !== Node.ELEMENT_NODE) return '';
  const el = n as Element;
  const inner = [...el.childNodes].map(mdInline).join('');
  switch (el.tagName.toLowerCase()) {
    case 'strong': case 'b': return `**${inner}**`;
    case 'em': case 'i': return `*${inner}*`;
    case 'u': return `<u>${inner}</u>`;
    case 'del': case 's': return `~~${inner}~~`;
    case 'br': return '\n';
    case 'a': return `[${inner}](${el.getAttribute('href') || ''})`;
    default: return inner;
  }
}

// mdBlock writes a block element as Markdown lines.
function mdBlock(el: Element): string {
  const tag = el.tagName.toLowerCase();
  const kids = [...el.children];
  switch (tag) {
    case 'p': return mdInline(el).replace(/^([#>+-])/gm, '\\$1').replace(/^(\d+)\./gm, '$1\\.');
    case 'h1': case 'h2': case 'h3': return `${'#'.repeat(Number(tag[1]))} ${mdInline(el)}`;
    case 'hr': return '---';
    case 'blockquote': return kids.map(mdBlock).join('\n\n').split('\n').map((l) => `> ${l}`).join('\n');
    case 'ul': case 'ol': return kids.map((li, i) => {
      const mark = tag === 'ol' ? `${i + 1}. ` : '- ';
      const body = li.children.length && [...li.children].every((c) => /^(P|UL|OL)$/.test(c.tagName))
        ? [...li.children].map(mdBlock).join('\n')
        : mdInline(li);
      return mark + body.split('\n').join(`\n${' '.repeat(mark.length)}`);
    }).join('\n');
    default: return el.outerHTML;
  }
}

// toMarkdown gives a rich text block's HTML as Markdown that renders the same, or the HTML itself, which Markdown keeps as it is.
export function toMarkdown(html: string): { text: string; exact: boolean } {
  const body = new DOMParser().parseFromString(`<body>${html}</body>`, 'text/html').body;
  const text = [...body.children].map(mdBlock).join('\n\n');
  if (canonical(renderMarkdownString(text)) === canonical(sanitizer(html))) {
    return { text, exact: true };
  }
  return { text: html, exact: false };
}

// sameHtml tells whether two HTMLs have the same elements, attributes and text.
export const sameHtml = (a: string, b: string) => canonical(a) === canonical(b);
