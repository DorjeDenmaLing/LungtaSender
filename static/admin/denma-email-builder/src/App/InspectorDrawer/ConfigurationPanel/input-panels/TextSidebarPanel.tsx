import React, { useState } from 'react';

import { Alert, InputLabel, Stack, ToggleButton, ToggleButtonGroup, Typography } from '@mui/material';

import { TextProps, TextPropsSchema } from '@usewaypoint/block-text';

import MarkdownBar from '../../../../documents/blocks/Text/MarkdownBar';
import { toMarkdown, toRichText } from '../../../../documents/blocks/Text/richText';

import BaseSidebarPanel from './helpers/BaseSidebarPanel';
import TextInput from './helpers/inputs/TextInput';
import MultiStylePropertyPanel from './helpers/style-inputs/MultiStylePropertyPanel';

type TextSidebarPanelProps = {
  data: TextProps;
  setData: (v: TextProps) => void;
};

type Format = 'html' | 'markdown' | 'plain';

const formatOf = (p: TextProps['props']): Format => (p?.format === 'html' ? 'html' : p?.markdown ? 'markdown' : 'plain');

export default function TextSidebarPanel({ data, setData }: TextSidebarPanelProps) {
  const [, setErrors] = useState<Zod.ZodError | null>(null);
  // denma: rich text, Markdown, or plain text (only for blocks that have it).
  const [format, setFormat] = useState<Format>(formatOf(data.props));
  const [hasPlain] = useState(format === 'plain');
  const [note, setNote] = useState('');
  const [contentKey, setContentKey] = useState(0);

  const updateData = (d: unknown) => {
    const res = TextPropsSchema.safeParse(d);
    if (res.success) {
      setData(res.data);
      setErrors(null);
    } else {
      setErrors(res.error);
    }
  };

  const changeFormat = (to: Format) => {
    const text = data.props?.text ?? '';
    const props = { ...data.props };
    setNote('');
    if (to === 'html') {
      const r = toRichText(text, format === 'markdown');
      if ('reason' in r) {
        setNote(r.reason);
        return;
      }
      Object.assign(props, { format: 'html', markdown: undefined, text: r.html });
    } else if (format === 'html') {
      const r = toMarkdown(text);
      if (!r.exact) {
        setNote('Some of its formatting has no Markdown, so it\'s kept as HTML, which Markdown shows as it is.');
      }
      Object.assign(props, { format: undefined, markdown: true, text: r.text });
    } else {
      Object.assign(props, { format: undefined, markdown: to === 'markdown' });
    }
    setFormat(to);
    setContentKey((k) => k + 1);
    updateData({ ...data, props });
  };

  const content = (
    <TextInput
      key={contentKey}
      label="Content"
      rows={5}
      defaultValue={data.props?.text ?? ''}
      onChange={(text) => updateData({ ...data, props: { ...data.props, text } })}
    />
  );

  return (
    <BaseSidebarPanel title="Text block">
      <Stack alignItems="flex-start">
        <InputLabel shrink>Format</InputLabel>
        <ToggleButtonGroup
          exclusive
          fullWidth
          size="small"
          value={format}
          onChange={(_, v: Format | null) => v && v !== format && changeFormat(v)}
        >
          <ToggleButton value="html">Rich text</ToggleButton>
          <ToggleButton value="markdown">Markdown</ToggleButton>
          {hasPlain ? <ToggleButton value="plain">Plain text</ToggleButton> : <></>}
        </ToggleButtonGroup>
      </Stack>
      {note ? <Alert severity="info" onClose={() => setNote('')}>{note}</Alert> : <></>}
      {format === 'html' ? (
        <Typography variant="body2" color="text.secondary">
          Type in the e-mail. **bold**, *italic*, # heading and - list format as you type.
        </Typography>
      ) : format === 'markdown' ? (
        <MarkdownBar>{content}</MarkdownBar>
      ) : (
        content
      )}

      <MultiStylePropertyPanel
        names={['color', 'backgroundColor', 'fontFamily', 'fontSize', 'fontWeight', 'textAlign', 'padding']}
        value={data.style}
        onChange={(style) => updateData({ ...data, style })}
      />
    </BaseSidebarPanel>
  );
}
