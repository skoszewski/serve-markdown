import {
  addExportVisitor$,
  addImportVisitor$,
  addLexicalNode$,
  realmPlugin,
  type LexicalExportVisitor,
  type MdastImportVisitor,
} from '@mdxeditor/editor';
import {DecoratorNode, type ElementNode, type LexicalNode, type NodeKey, type SerializedLexicalNode, type Spread} from 'lexical';
import type * as Mdast from 'mdast';
import type {ReactNode} from 'react';

type SerializedRawHtmlNode = Spread<{html: string; inline: boolean}, SerializedLexicalNode>;

/**
 * HTML of a plain Markdown document kept as it is written: a block of its own, or a tag within a paragraph. It is
 * shown as its source and edited in source mode.
 */
export class RawHtmlNode extends DecoratorNode<ReactNode> {
  __html: string;
  __inline: boolean;

  static getType(): string {
    return 'raw-html';
  }

  static clone(node: RawHtmlNode): RawHtmlNode {
    return new RawHtmlNode(node.__html, node.__inline, node.__key);
  }

  static importJSON(json: SerializedRawHtmlNode): RawHtmlNode {
    return new RawHtmlNode(json.html, json.inline);
  }

  constructor(html: string, inline: boolean, key?: NodeKey) {
    super(key);
    this.__html = html;
    this.__inline = inline;
  }

  exportJSON(): SerializedRawHtmlNode {
    return {...super.exportJSON(), type: 'raw-html', version: 1, html: this.__html, inline: this.__inline};
  }

  createDOM(): HTMLElement {
    const element = document.createElement(this.__inline ? 'span' : 'div');
    element.className = this.__inline ? 'raw-html raw-html-inline' : 'raw-html';
    return element;
  }

  updateDOM(): false {
    return false;
  }

  isInline(): boolean {
    return this.__inline;
  }

  getHtml(): string {
    return this.getLatest().__html;
  }

  decorate(): ReactNode {
    return <code title="HTML, edited in source mode">{this.__html}</code>;
  }
}

/**
 * Tells whether a Lexical node is raw HTML.
 *
 * @param node Lexical node.
 * @returns True for a RawHtmlNode.
 */
function $isRawHtmlNode(node: LexicalNode | null | undefined): node is RawHtmlNode {
  return node instanceof RawHtmlNode;
}

/** Reads the HTML no other visitor takes, a block at the top level and a tag within its parent elsewhere. */
const importVisitor: MdastImportVisitor<Mdast.Html> = {
  testNode: 'html',
  priority: -100,
  visitNode({mdastNode, lexicalParent}) {
    (lexicalParent as ElementNode).append(new RawHtmlNode(mdastNode.value, lexicalParent.getType() !== 'root'));
  },
};

/** Writes the HTML back as it was read. */
const exportVisitor: LexicalExportVisitor<RawHtmlNode, Mdast.Html> = {
  testLexicalNode: $isRawHtmlNode,
  visitLexicalNode({mdastParent, lexicalNode, actions}) {
    actions.appendToParent(mdastParent, {type: 'html', value: lexicalNode.getHtml()});
  },
};

/** Keeps the HTML of a plain Markdown document, read without the MDX processing of HTML tags. */
export const rawHtmlPlugin = realmPlugin({
  init(realm) {
    realm.pubIn({
      [addImportVisitor$]: importVisitor,
      [addLexicalNode$]: RawHtmlNode,
      [addExportVisitor$]: exportVisitor,
    });
  },
});
