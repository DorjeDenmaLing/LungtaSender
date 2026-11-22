// denma: a Text block on the canvas: rich text is typed in place once the block is selected.
import React, { useEffect, useRef } from 'react';

import { FormatBold, FormatItalic, FormatListBulleted, FormatListNumbered, InsertLink } from '@mui/icons-material';
import { IconButton, Paper, Tooltip } from '@mui/material';
import { EditorContent, useEditor, useEditorState } from '@tiptap/react';
import { getFontFamily, Text, TextProps } from '@usewaypoint/block-text';

import { useCurrentBlockId } from '../../editor/EditorBlock';
import { setDocument, useDocument, useSelectedBlockId } from '../../editor/EditorContext';

import { editorHtml, looksLikeMarkdown, markdownHtml, NEW_LINK, pastedHtml, richTextExtensions, sameHtml } from './richText';

const MAC = /Mac|iPhone|iPad/.test(navigator.platform);
const KEY = MAC ? '⌘' : 'Ctrl+';

// Where the click that selected a block was, for the editor's first caret.
let lastClick: { left: number; top: number } | null = null;

export default function TextEditor(props: TextProps) {
  const id = useCurrentBlockId();
  const selected = useSelectedBlockId() === id;
  if (props.props?.format !== 'html' || !selected) {
    return (
      <div onMouseDown={(ev) => { lastClick = { left: ev.clientX, top: ev.clientY }; }}>
        <Text {...props} />
      </div>
    );
  }
  return <RichText id={id} {...props} />;
}

function RichText({ id, style, props }: TextProps & { id: string }) {
  const document = useDocument();
  const docRef = useRef(document);
  docRef.current = document;

  const editor = useEditor({
    extensions: richTextExtensions(),
    content: props?.text ?? '',
    editorProps: {
      attributes: { 'aria-label': 'Text', class: 'denma-rich-text' },
      transformPastedHTML: pastedHtml,
      handlePaste: (_view, ev) => {
        const text = ev.clipboardData?.getData('text/plain') ?? '';
        if (ev.clipboardData?.getData('text/html') || !looksLikeMarkdown(text)) {
          return false;
        }
        editorRef.current?.commands.insertContent(markdownHtml(text));
        return true;
      },
    },
    onCreate: ({ editor: ed }) => {
      const pos = lastClick && ed.view.posAtCoords(lastClick);
      lastClick = null;
      ed.commands.focus(pos ? pos.pos : 'end');
    },
    onUpdate: ({ editor: ed }) => {
      const block = docRef.current[id];
      const text = editorHtml(ed);
      // Unchanged as the e-mail shows it: kept as it was saved.
      if (block?.type !== 'Text' || sameHtml(text, block.data.props?.text ?? '')) return;
      setDocument({ [id]: { ...block, data: { ...block.data, props: { ...block.data.props, text } } } });
    },
  });
  const editorRef = useRef(editor);
  editorRef.current = editor;

  // Ctrl/⌘+K: link.
  useEffect(() => {
    const el = editor?.view.dom;
    const onKey = (ev: KeyboardEvent) => {
      if ((MAC ? ev.metaKey : ev.ctrlKey) && !ev.altKey && !ev.shiftKey && ev.key.toLowerCase() === 'k') {
        ev.preventDefault();
        setLink();
      }
    };
    el?.addEventListener('keydown', onKey);
    return () => el?.removeEventListener('keydown', onKey);
  });

  const active = useEditorState({
    editor,
    selector: ({ editor: ed }) => ({
      bold: ed?.isActive('bold') ?? false,
      italic: ed?.isActive('italic') ?? false,
      link: ed?.isActive('link') ?? false,
      bullets: ed?.isActive('bulletList') ?? false,
      numbers: ed?.isActive('orderedList') ?? false,
    }),
  });

  const setLink = () => {
    if (!editor) return;
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
  };

  const buttons = [
    { name: 'bold', label: `Bold (${KEY}B)`, icon: <FormatBold fontSize="small" />, run: () => editor?.chain().focus().toggleBold().run() },
    { name: 'italic', label: `Italic (${KEY}I)`, icon: <FormatItalic fontSize="small" />, run: () => editor?.chain().focus().toggleItalic().run() },
    { name: 'link', label: `Link (${KEY}K)`, icon: <InsertLink fontSize="small" />, run: setLink },
    { name: 'bullets', label: 'Bulleted list', icon: <FormatListBulleted fontSize="small" />, run: () => editor?.chain().focus().toggleBulletList().run() },
    { name: 'numbers', label: 'Numbered list', icon: <FormatListNumbered fontSize="small" />, run: () => editor?.chain().focus().toggleOrderedList().run() },
  ] as const;

  const wStyle = {
    color: style?.color ?? undefined,
    backgroundColor: style?.backgroundColor ?? undefined,
    fontSize: style?.fontSize ?? undefined,
    fontFamily: getFontFamily(style?.fontFamily),
    fontWeight: style?.fontWeight ?? undefined,
    textAlign: style?.textAlign ?? undefined,
    padding: style?.padding ? `${style.padding.top}px ${style.padding.right}px ${style.padding.bottom}px ${style.padding.left}px` : undefined,
  };

  return (
    <div style={{ position: 'relative' }} onClick={(ev) => ev.stopPropagation()}>
      <Paper
        elevation={2}
        role="toolbar"
        aria-label="Text formatting"
        className="denma-rich-bar"
        sx={{ position: 'absolute', bottom: '100%', left: 0, zIndex: 1, display: 'flex', mb: '4px' }}
        onMouseDown={(ev) => ev.preventDefault()}
      >
        {buttons.map((b) => (
          <Tooltip key={b.name} title={b.label}>
            <IconButton
              size="small"
              aria-label={b.label}
              aria-pressed={active?.[b.name] ?? false}
              color={active?.[b.name] ? 'primary' : 'default'}
              onClick={b.run}
            >
              {b.icon}
            </IconButton>
          </Tooltip>
        ))}
      </Paper>
      <div style={wStyle}>
        <EditorContent editor={editor} />
      </div>
    </div>
  );
}
