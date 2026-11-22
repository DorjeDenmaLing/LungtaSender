// denma: a Text block on the canvas: rich text is typed in place once the block is selected.
import React, { useRef } from 'react';

import { EditorContent, useEditor } from '@tiptap/react';
import { getFontFamily, Text, TextProps } from '@usewaypoint/block-text';

import { useCurrentBlockId } from '../../editor/EditorBlock';
import { setDocument, useDocument, useSelectedBlockId } from '../../editor/EditorContext';

import RichBar, { Tool } from './RichBar';
import { editorHtml, looksLikeMarkdown, markdownHtml, pastedHtml, richTextExtensions, sameHtml } from './richText';

const TOOLS: Tool[] = ['block', 'bold', 'italic', 'strike', 'link', 'bullets', 'numbers', 'colour', 'name', 'clear'];

// Where the click that selected a block was, for the editor's first caret.
let lastClick: { left: number; top: number } | null = null;
export const setLastClick = (ev: React.MouseEvent) => { lastClick = { left: ev.clientX, top: ev.clientY }; };
export const takeLastClick = () => { const c = lastClick; lastClick = null; return c; };

export default function TextEditor(props: TextProps) {
  const id = useCurrentBlockId();
  const selected = useSelectedBlockId() === id;
  if (props.props?.format !== 'html' || !selected) {
    return (
      <div onMouseDown={setLastClick}>
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
      const click = takeLastClick();
      const pos = click && ed.view.posAtCoords(click);
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
      <RichBar editor={editor} tools={TOOLS} />
      <div style={wStyle}>
        <EditorContent editor={editor} />
      </div>
    </div>
  );
}
