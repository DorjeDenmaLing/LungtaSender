// The builder as window.EmailBuilder (visual-editor.js), with renderHtml for the checks.
import * as builder from './main';
import { renderHtmlWithMeta } from './utils';

(window as any).EmailBuilder = { ...builder, renderHtml: renderHtmlWithMeta };
