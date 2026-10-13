import {syntaxHighlighting} from '@codemirror/language';
import {tagHighlighter, tags} from '@lezer/highlight';

/** Colour themes of the editor, each with a light and a dark variant defined in editor.css. */
export const palettes = {
  github: 'GitHub',
  vscode: 'Visual Studio Code (Light+ and Dark+)',
  solarized: 'Solarized',
  catppuccin: 'Catppuccin (Latte and Mocha)',
  gruvbox: 'Gruvbox',
  monochrome: 'Monochrome (Radix Gray)',
};

export type Palette = keyof typeof palettes;

/** Appearance of the editor: the browser's preference, light or dark, shared with the pages of the server. */
export const appearances = {
  auto: 'Browser preference',
  light: 'Light',
  dark: 'Dark',
};

export type Appearance = keyof typeof appearances;

/**
 * Highlights the code blocks with the cm-t-* classes coloured by the theme in editor.css, over the light colours of
 * the MDXEditor code block.
 */
export const codeHighlighting = syntaxHighlighting(
  tagHighlighter([
    {tag: tags.keyword, class: 'cm-t-keyword'},
    {tag: [tags.string, tags.special(tags.string), tags.character, tags.regexp, tags.escape], class: 'cm-t-string'},
    {tag: [tags.literal, tags.number, tags.bool, tags.atom, tags.color, tags.constant(tags.name), tags.standard(tags.name), tags.changed], class: 'cm-t-number'},
    {tag: tags.comment, class: 'cm-t-comment'},
    {tag: [tags.function(tags.variableName), tags.function(tags.propertyName), tags.macroName, tags.labelName], class: 'cm-t-function'},
    {tag: [tags.typeName, tags.className, tags.namespace, tags.annotation, tags.self], class: 'cm-t-type'},
    {tag: [tags.name, tags.variableName], class: 'cm-t-variable'},
    {tag: tags.propertyName, class: 'cm-t-property'},
    {tag: [tags.tagName, tags.angleBracket], class: 'cm-t-tag'},
    {tag: tags.attributeName, class: 'cm-t-attribute'},
    {tag: tags.operator, class: 'cm-t-operator'},
    {tag: [tags.punctuation, tags.separator, tags.bracket], class: 'cm-t-punctuation'},
    {tag: [tags.meta, tags.processingInstruction, tags.documentMeta], class: 'cm-t-meta'},
    {tag: tags.heading, class: 'cm-t-heading'},
    {tag: tags.emphasis, class: 'cm-t-emphasis'},
    {tag: tags.strong, class: 'cm-t-strong'},
    {tag: tags.strikethrough, class: 'cm-t-strikethrough'},
    {tag: [tags.link, tags.url], class: 'cm-t-link'},
    {tag: tags.inserted, class: 'cm-t-inserted'},
    {tag: tags.deleted, class: 'cm-t-deleted'},
    {tag: tags.invalid, class: 'cm-t-invalid'},
  ]),
);
