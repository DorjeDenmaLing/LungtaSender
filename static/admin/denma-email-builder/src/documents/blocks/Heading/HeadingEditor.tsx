// denma: a Heading block on the canvas, typed in place once it's selected; plain text until it's formatted.
import React, { useRef } from 'react';

import { EditorContent, useEditor } from '@tiptap/react';
import { getFontFamily, getFontSize, getPadding, Heading, HeadingProps, HeadingPropsDefaults } from '@usewaypoint/block-heading';

import { useCurrentBlockId } from '../../editor/EditorBlock';
import { setDocument, useDocument, useSelectedBlockId } from '../../editor/EditorContext';
import RichBar, { Tool } from '../Text/RichBar';
import { setLastClick, takeLastClick } from '../Text/TextEditor';
import { headingContent, headingExtensions, headingText, pastedHtml, sameHtml } from '../Text/richText';

const TOOLS: Tool[] = ['italic', 'strike', 'link', 'colour', 'name', 'clear'];

export default function HeadingEditor(props: HeadingProps) {
  const id = useCurrentBlockId();
  if (useSelectedBlockId() !== id) {
    return (
      <div onMouseDown={setLastClick}>
        <Heading {...props} />
      </div>
    );
  }
  return <RichHeading id={id} {...props} />;
}

function RichHeading({ id, style, props }: HeadingProps & { id: string }) {
  const document = useDocument();
  const docRef = useRef(document);
  docRef.current = document;
  const level = props?.level ?? HeadingPropsDefaults.level;

  const editor = useEditor({
    extensions: headingExtensions(),
    content: headingContent(props?.text ?? '', props?.format),
    editorProps: {
      attributes: { 'aria-label': 'Heading', role: 'heading', 'aria-level': level.slice(1), class: 'denma-rich-text denma-rich-heading' },
      transformPastedHTML: pastedHtml,
      // Enter ends the heading.
      handleKeyDown: (_view, ev) => ev.key === 'Enter',
    },
    onCreate: ({ editor: ed }) => {
      const click = takeLastClick();
      const pos = click && ed.view.posAtCoords(click);
      ed.commands.focus(pos ? pos.pos : 'end');
    },
    onUpdate: ({ editor: ed }) => {
      const block = docRef.current[id];
      if (block?.type !== 'Heading') return;
      const was = block.data.props ?? {};
      const { text, format } = headingText(ed);
      // Unchanged as the e-mail shows it: kept as it was saved.
      if ((format ?? null) === (was.format ?? null) && (format ? sameHtml(text, was.text ?? '') : text === (was.text ?? ''))) return;
      setDocument({ [id]: { ...block, data: { ...block.data, props: { ...was, text, format: format ?? undefined } } } });
    },
  });

  const hStyle = {
    color: style?.color ?? undefined,
    backgroundColor: style?.backgroundColor ?? undefined,
    fontWeight: style?.fontWeight ?? 'bold',
    textAlign: style?.textAlign ?? undefined,
    margin: 0,
    fontFamily: getFontFamily(style?.fontFamily),
    fontSize: getFontSize(level),
    padding: getPadding(style?.padding),
  };

  return (
    <div style={{ position: 'relative' }} onClick={(ev) => ev.stopPropagation()}>
      <RichBar editor={editor} tools={TOOLS} block={style ?? undefined} />
      <div style={hStyle}>
        <EditorContent editor={editor} />
      </div>
    </div>
  );
}
