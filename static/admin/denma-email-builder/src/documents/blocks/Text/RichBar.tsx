// denma: the formatting bar over a rich text block on the canvas, with Ctrl/⌘+K for links.
import React, { useEffect, useState } from 'react';

import {
  FormatBold,
  FormatClear,
  FormatColorText,
  FormatItalic,
  FormatListBulleted,
  FormatListNumbered,
  FormatStrikethrough,
  InsertLink,
  PersonOutline,
} from '@mui/icons-material';
import { Box, Button, IconButton, Menu, MenuItem, Paper, Popover, Tooltip } from '@mui/material';
import { Editor, useEditorState } from '@tiptap/react';

import { useDocument } from '../../editor/EditorContext';

import { NEW_LINK, QUOTE_STYLE } from './richText';

const MAC = /Mac|iPhone|iPad/.test(navigator.platform);
export const KEY = MAC ? '⌘' : 'Ctrl+';

export type Tool = 'block' | 'bold' | 'italic' | 'strike' | 'link' | 'bullets' | 'numbers' | 'colour' | 'name' | 'clear';

const BLOCKS = [
  { name: 'Paragraph', level: 0 },
  { name: 'Heading 1', level: 1 },
  { name: 'Heading 2', level: 2 },
  { name: 'Heading 3', level: 3 },
  { name: 'Quote', level: -1 },
] as const;

const NAMES = [
  { name: 'First name', tag: '{{ .Subscriber.FirstName }}' },
  { name: 'Full name', tag: '{{ .Subscriber.Name }}' },
];

const HEX = /#[0-9a-f]{6}\b/gi;

// emailColours are the colours of the e-mail's text and buttons, most used first, for text colour.
function emailColours(doc: Record<string, any>): string[] {
  const count = new Map<string, number>();
  const add = (c?: string | null) => {
    if (c) count.set(c.toLowerCase(), (count.get(c.toLowerCase()) ?? 0) + 1);
  };
  const root = doc.root?.data ?? {};
  add(root.textColor);
  Object.values(doc).forEach((b: any) => {
    add(b?.data?.style?.color);
    add(b?.data?.props?.buttonBackgroundColor);
    if (b?.type === 'Text' || b?.type === 'Heading') {
      (String(b.data?.props?.text ?? '').match(/color:\s*#[0-9a-f]{6}/gi) ?? []).forEach((m) => add(m.match(HEX)?.[0]));
    }
  });
  count.delete(String(root.canvasColor ?? '#ffffff').toLowerCase());
  return [...count.entries()].sort((a, b) => b[1] - a[1]).map(([c]) => c).slice(0, 8);
}

// setLink asks for an address and makes the selection a link, or adds the address as one.
function setLink(editor: Editor) {
  const prev = editor.getAttributes('link').href ?? 'https://';
  const url = (window.prompt('Link address (empty to remove the link)', prev) ?? prev).trim();
  if (url === prev) return;
  const chain = editor.chain().focus().extendMarkRange('link');
  if (!url || url === 'https://') {
    chain.unsetLink().run();
  } else if (editor.state.selection.empty && !editor.isActive('link')) {
    chain.insertContent({ type: 'text', text: url, marks: [{ type: 'link', attrs: { href: url, ...NEW_LINK } }] }).run();
  } else {
    chain.setLink({ href: url, ...NEW_LINK }).run();
  }
}

// clearFormat takes bold, italic, strikethrough and colour off the selection; links stay.
function clearFormat(editor: Editor) {
  const chain = editor.chain().focus();
  ['bold', 'italic', 'strike'].filter((m) => editor.schema.marks[m]).forEach((m) => chain.unsetMark(m));
  chain.unsetColor().removeEmptyTextStyle().run();
}

function setBlock(editor: Editor, level: number) {
  const chain = editor.chain().focus();
  if (level === -1) {
    if (!editor.isActive('blockquote')) chain.setParagraph().wrapIn('blockquote', { style: QUOTE_STYLE });
  } else {
    if (editor.isActive('blockquote')) chain.lift('blockquote');
    if (level === 0) chain.setParagraph();
    else chain.setHeading({ level: level as 1 | 2 | 3 });
  }
  chain.run();
}

export default function RichBar({ editor, tools }: { editor: Editor | null; tools: Tool[] }) {
  const doc = useDocument();
  const [colourAt, setColourAt] = useState<HTMLElement | null>(null);
  const [nameAt, setNameAt] = useState<HTMLElement | null>(null);
  const [blockAt, setBlockAt] = useState<HTMLElement | null>(null);

  useEffect(() => {
    const el = editor?.view.dom;
    const onKey = (ev: KeyboardEvent) => {
      if (editor && (MAC ? ev.metaKey : ev.ctrlKey) && !ev.altKey && !ev.shiftKey && ev.key.toLowerCase() === 'k') {
        ev.preventDefault();
        setLink(editor);
      }
    };
    el?.addEventListener('keydown', onKey);
    return () => el?.removeEventListener('keydown', onKey);
  }, [editor]);

  const active = useEditorState({
    editor,
    selector: ({ editor: ed }) => ({
      bold: ed?.isActive('bold') ?? false,
      italic: ed?.isActive('italic') ?? false,
      strike: ed?.isActive('strike') ?? false,
      link: ed?.isActive('link') ?? false,
      bullets: ed?.isActive('bulletList') ?? false,
      numbers: ed?.isActive('orderedList') ?? false,
      colour: (ed?.getAttributes('textStyle').color as string | undefined) ?? '',
      block: ed?.isActive('blockquote') ? -1 : ([1, 2, 3].find((l) => ed?.isActive('heading', { level: l })) ?? 0),
    }),
  });
  if (!editor) return null;

  const run = (f: (c: ReturnType<Editor['chain']>) => ReturnType<Editor['chain']>) => () => f(editor.chain().focus()).run();
  const buttons: Record<Exclude<Tool, 'block'>, { label: string; icon: JSX.Element; onClick: (ev: React.MouseEvent<HTMLElement>) => void; pressed?: boolean }> = {
    bold: { label: `Bold (${KEY}B)`, icon: <FormatBold fontSize="small" />, onClick: run((c) => c.toggleBold()), pressed: active?.bold },
    italic: { label: `Italic (${KEY}I)`, icon: <FormatItalic fontSize="small" />, onClick: run((c) => c.toggleItalic()), pressed: active?.italic },
    strike: { label: `Strikethrough (${KEY}${MAC ? '⇧' : 'Shift+'}S)`, icon: <FormatStrikethrough fontSize="small" />, onClick: run((c) => c.toggleStrike()), pressed: active?.strike },
    link: { label: `Link (${KEY}K)`, icon: <InsertLink fontSize="small" />, onClick: () => setLink(editor), pressed: active?.link },
    bullets: { label: 'Bulleted list', icon: <FormatListBulleted fontSize="small" />, onClick: run((c) => c.toggleBulletList()), pressed: active?.bullets },
    numbers: { label: 'Numbered list', icon: <FormatListNumbered fontSize="small" />, onClick: run((c) => c.toggleOrderedList()), pressed: active?.numbers },
    colour: {
      label: 'Text colour',
      icon: (
        <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <FormatColorText fontSize="small" />
          <Box sx={{ width: 16, height: 3, mt: '-3px', bgcolor: active?.colour || 'transparent' }} />
        </Box>
      ),
      onClick: (ev) => setColourAt(ev.currentTarget),
    },
    name: { label: 'Insert their name', icon: <PersonOutline fontSize="small" />, onClick: (ev) => setNameAt(ev.currentTarget) },
    clear: { label: 'Clear formatting', icon: <FormatClear fontSize="small" />, onClick: () => clearFormat(editor) },
  };
  const colours = emailColours(doc);
  const block = BLOCKS.find((b) => b.level === active?.block) ?? BLOCKS[0];

  return (
    <Paper
      elevation={2}
      role="toolbar"
      aria-label="Text formatting"
      className="denma-rich-bar"
      sx={{ position: 'absolute', bottom: '100%', left: 0, zIndex: 1, display: 'flex', flexWrap: 'wrap', alignItems: 'center', maxWidth: '100%', mb: '4px' }}
      onMouseDown={(ev) => ev.preventDefault()}
    >
      {tools.map((t) => {
        if (t === 'block') {
          return (
            <Button
              key={t}
              size="small"
              color="inherit"
              aria-label="Paragraph, heading or quote"
              aria-haspopup="menu"
              sx={{ textTransform: 'none', minWidth: 104, justifyContent: 'space-between' }}
              onClick={(ev) => setBlockAt(ev.currentTarget)}
            >
              {block.name} ▾
            </Button>
          );
        }
        const b = buttons[t];
        return (
          <Tooltip key={t} title={b.label}>
            <IconButton
              size="small"
              aria-label={b.label}
              aria-pressed={b.pressed === undefined ? undefined : b.pressed}
              color={b.pressed ? 'primary' : 'default'}
              onClick={b.onClick}
            >
              {b.icon}
            </IconButton>
          </Tooltip>
        );
      })}
      <Menu anchorEl={blockAt} open={!!blockAt} onClose={() => setBlockAt(null)} disableAutoFocusItem disableRestoreFocus>
        {BLOCKS.map((b) => (
          <MenuItem key={b.name} selected={b.level === block.level} onClick={() => { setBlockAt(null); setBlock(editor, b.level); }}>
            {b.name}
          </MenuItem>
        ))}
      </Menu>
      <Menu anchorEl={nameAt} open={!!nameAt} onClose={() => setNameAt(null)} disableAutoFocusItem disableRestoreFocus>
        {NAMES.map((n) => (
          <MenuItem key={n.name} onClick={() => { setNameAt(null); editor.chain().focus().insertContent(n.tag).run(); }}>
            {n.name}
          </MenuItem>
        ))}
      </Menu>
      <Popover
        open={!!colourAt}
        anchorEl={colourAt}
        onClose={() => setColourAt(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        disableAutoFocus
        disableRestoreFocus
      >
        <Box className="denma-rich-colours" sx={{ display: 'flex', flexWrap: 'wrap', gap: '6px', p: 1, maxWidth: 200 }}>
          <Button size="small" color="inherit" sx={{ textTransform: 'none', width: '100%' }} onClick={() => { setColourAt(null); editor.chain().focus().unsetColor().removeEmptyTextStyle().run(); }}>
            The block's colour
          </Button>
          {colours.map((c) => (
            <Tooltip key={c} title={c}>
              <button
                type="button"
                aria-label={c}
                aria-pressed={active?.colour.toLowerCase() === c}
                style={{ width: 24, height: 24, borderRadius: 4, border: '1px solid rgba(0,0,0,0.2)', background: c, cursor: 'pointer' }}
                onClick={() => { setColourAt(null); editor.chain().focus().setColor(c).run(); }}
              />
            </Tooltip>
          ))}
        </Box>
      </Popover>
    </Paper>
  );
}
