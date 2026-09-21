import { Fragment, type ReactNode } from 'react';

const htmlTagNamePattern = '[A-Za-z][A-Za-z0-9:-]*';
const allowedHtmlTags = new Set([
  'a',
  'b',
  'blockquote',
  'br',
  'code',
  'del',
  'div',
  'em',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'hr',
  'i',
  'img',
  'li',
  'mark',
  'ol',
  'p',
  'pre',
  's',
  'small',
  'span',
  'strong',
  'sub',
  'sup',
  'table',
  'tbody',
  'td',
  'th',
  'thead',
  'tr',
  'u',
  'ul',
]);
const uriAttributes = new Set(['href', 'src']);
const allowedStyleProperties = new Set([
  'background-color',
  'color',
  'font-size',
  'font-style',
  'font-weight',
  'text-align',
  'text-decoration',
]);

function sanitizeHtml(markup: string): string {
  return markup
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/<\s*(script|style|iframe|object|embed|svg|math)\b[\s\S]*?<\s*\/\s*\1\s*>/gi, '')
    .replace(/<\s*\/?\s*([A-Za-z][A-Za-z0-9:-]*)([^>]*)>/g, (tag, rawName, rawAttrs) => {
      const tagName = String(rawName).toLowerCase();
      if (!allowedHtmlTags.has(tagName)) return '';
      const isClosing = /^<\s*\//.test(tag);
      if (isClosing) return `</${tagName}>`;
      const isSelfClosing =
        /\/\s*>$/.test(tag) || tagName === 'br' || tagName === 'hr' || tagName === 'img';
      const attrs = sanitizeHtmlAttributes(tagName, String(rawAttrs || ''));
      return `<${tagName}${attrs}${isSelfClosing ? ' /' : ''}>`;
    });
}

function sanitizeHtmlAttributes(tagName: string, attrs: string): string {
  const sanitized: string[] = [];
  attrs.replace(
    /([A-Za-z_:][A-Za-z0-9_:.-]*)(?:\s*=\s*("[^"]*"|'[^']*'|[^\s"'=<>`]+))?/g,
    (_match, rawName: string, rawValue: string | undefined) => {
      const name = rawName.toLowerCase();
      if (name.startsWith('on') || name === 'srcdoc' || name === 'formaction') return '';
      if (name === 'target' || name === 'rel') return '';
      if (tagName !== 'a' && tagName !== 'img' && uriAttributes.has(name)) return '';
      if (
        !['alt', 'colspan', 'height', 'href', 'rowspan', 'src', 'style', 'title', 'width'].includes(
          name,
        )
      ) {
        return '';
      }
      const value = unquoteHtmlAttr(rawValue || '');
      if (uriAttributes.has(name) && !isSafeHtmlUri(value, tagName === 'img')) return '';
      if (name === 'style') {
        const style = sanitizeInlineStyle(value);
        if (!style) return '';
        sanitized.push(` ${name}="${escapeHtmlAttr(style)}"`);
        return '';
      }
      sanitized.push(` ${name}="${escapeHtmlAttr(value)}"`);
      if (tagName === 'a' && name === 'href') sanitized.push(' target="_blank" rel="noreferrer"');
      return '';
    },
  );
  return sanitized.join('');
}

function unquoteHtmlAttr(value: string): string {
  if (
    (value.startsWith('"') && value.endsWith('"')) ||
    (value.startsWith("'") && value.endsWith("'"))
  ) {
    return value.slice(1, -1);
  }
  return value;
}

function escapeHtmlAttr(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

function isSafeHtmlUri(value: string, allowDataImage: boolean): boolean {
  const trimmed = [...value.trim()].filter((char) => char > ' ' && char !== '\u007f').join('');
  if (!trimmed) return false;
  if (
    trimmed.startsWith('#') ||
    trimmed.startsWith('/') ||
    trimmed.startsWith('./') ||
    trimmed.startsWith('../')
  ) {
    return true;
  }
  if (/^(https?:|mailto:|tel:)/i.test(trimmed)) return true;
  return (
    allowDataImage && /^data:image\/(png|gif|jpe?g|webp);base64,[A-Za-z0-9+/=]+$/i.test(trimmed)
  );
}

function sanitizeInlineStyle(value: string): string {
  const declarations: string[] = [];
  value.split(';').forEach((part) => {
    const [rawProperty, ...rawValueParts] = part.split(':');
    if (!rawProperty || rawValueParts.length === 0) return;
    const property = rawProperty.trim().toLowerCase();
    const styleValue = rawValueParts.join(':').trim();
    if (!allowedStyleProperties.has(property)) return;
    if (!styleValue || /url\s*\(|expression\s*\(|javascript:/i.test(styleValue)) return;
    declarations.push(`${property}: ${styleValue}`);
  });
  return declarations.join('; ');
}

function renderHtml(markup: string, key: string, block = false): ReactNode {
  const sanitized = sanitizeHtml(markup);
  if (!sanitized) return null;
  const className = block
    ? 'prose prose-sm max-w-none text-foreground [&_a]:text-primary [&_a]:underline [&_a]:underline-offset-2 [&_table]:w-full [&_table]:border-collapse [&_td]:border [&_td]:border-border [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-border [&_th]:px-2 [&_th]:py-1'
    : undefined;
  if (block) {
    return <div key={key} className={className} dangerouslySetInnerHTML={{ __html: sanitized }} />;
  }
  return <span key={key} dangerouslySetInnerHTML={{ __html: sanitized }} />;
}

function looksLikeHtmlLine(line: string): boolean {
  return new RegExp(`^<\\/?${htmlTagNamePattern}(?:\\s[^>]*)?>`).test(line);
}

function getOpenHtmlBlockTag(line: string): string | null {
  const match = new RegExp(`^<(${htmlTagNamePattern})(?:\\s[^>]*)?>`, 'i').exec(line);
  if (!match) return null;
  const tagName = match[1].toLowerCase();
  if (['br', 'hr', 'img'].includes(tagName) || /\/\s*>$/.test(line)) return null;
  if (new RegExp(`</\\s*${tagName}\\s*>`, 'i').test(line)) return null;
  return tagName;
}

function closesHtmlBlock(line: string, tagName: string): boolean {
  return new RegExp(`</\\s*${tagName}\\s*>`, 'i').test(line);
}

function splitInline(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  const pattern =
    /(\[[^\]]+\]\(https?:\/\/[^\s)]+\)|`[^`]+`|\*\*[^*]+\*\*|__[^_]+__|\*[^*]+\*|_[^_]+_|<([A-Za-z][A-Za-z0-9:-]*)(?:\s[^>]*)?>[\s\S]*?<\/\2>|<[A-Za-z][A-Za-z0-9:-]*(?:\s[^>]*)?\/?>)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) nodes.push(text.slice(lastIndex, match.index));
    const token = match[0];
    const key = `${match.index}-${token}`;
    if (token.startsWith('<')) {
      nodes.push(renderHtml(token, key));
    } else if (token.startsWith('[')) {
      const link = /^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)$/.exec(token);
      if (link) {
        nodes.push(
          <a
            key={key}
            href={link[2]}
            target="_blank"
            rel="noreferrer"
            className="text-primary underline underline-offset-2 hover:text-primary/80"
          >
            {link[1]}
          </a>,
        );
      } else {
        nodes.push(token);
      }
    } else if (token.startsWith('`')) {
      nodes.push(
        <code key={key} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.9em]">
          {token.slice(1, -1)}
        </code>,
      );
    } else if (token.startsWith('**') || token.startsWith('__')) {
      nodes.push(
        <strong key={key} className="font-semibold text-foreground">
          {token.slice(2, -2)}
        </strong>,
      );
    } else {
      nodes.push(
        <em key={key} className="italic">
          {token.slice(1, -1)}
        </em>,
      );
    }
    lastIndex = match.index + token.length;
  }
  if (lastIndex < text.length) nodes.push(text.slice(lastIndex));
  return nodes;
}

export function renderMarkdownBlocks(markdown: string): ReactNode[] {
  const lines = markdown.replace(/\r\n/g, '\n').split('\n');
  const blocks: ReactNode[] = [];
  let paragraph: string[] = [];
  let list: string[] = [];
  let quote: string[] = [];
  let code: string[] | null = null;
  let htmlBlock: string[] | null = null;
  let htmlBlockTag: string | null = null;

  const flushParagraph = () => {
    if (paragraph.length === 0) return;
    const text = paragraph.join(' ').trim();
    if (text) {
      blocks.push(
        <p key={`p-${blocks.length}`} className="leading-7 text-foreground">
          {splitInline(text)}
        </p>,
      );
    }
    paragraph = [];
  };

  const flushList = () => {
    if (list.length === 0) return;
    blocks.push(
      <ul key={`ul-${blocks.length}`} className="list-disc space-y-1 pl-5 text-foreground">
        {list.map((item, index) => (
          <li key={index}>{splitInline(item)}</li>
        ))}
      </ul>,
    );
    list = [];
  };

  const flushQuote = () => {
    if (quote.length === 0) return;
    blocks.push(
      <blockquote
        key={`quote-${blocks.length}`}
        className="border-l-2 border-primary/40 pl-3 text-sm leading-7 text-muted-foreground"
      >
        {quote.map((item, index) => (
          <Fragment key={index}>
            {index > 0 ? <br /> : null}
            {splitInline(item)}
          </Fragment>
        ))}
      </blockquote>,
    );
    quote = [];
  };

  const flushFlow = () => {
    flushParagraph();
    flushList();
    flushQuote();
  };

  const flushHtmlBlock = () => {
    if (!htmlBlock) return;
    blocks.push(renderHtml(htmlBlock.join('\n'), `html-${blocks.length}`, true));
    htmlBlock = null;
    htmlBlockTag = null;
  };

  lines.forEach((rawLine) => {
    const line = rawLine.trimEnd();
    const trimmedLine = line.trim();

    if (htmlBlock) {
      htmlBlock.push(line);
      if (htmlBlockTag && closesHtmlBlock(trimmedLine, htmlBlockTag)) {
        flushHtmlBlock();
      }
      return;
    }

    if (code) {
      if (line.trim().startsWith('```')) {
        blocks.push(
          <pre
            key={`code-${blocks.length}`}
            className="overflow-x-auto rounded-lg border border-border bg-muted/40 p-3 text-xs text-foreground"
          >
            <code>{code.join('\n')}</code>
          </pre>,
        );
        code = null;
      } else {
        code.push(rawLine);
      }
      return;
    }
    if (line.trim().startsWith('```')) {
      flushFlow();
      code = [];
      return;
    }
    if (!line.trim()) {
      flushFlow();
      return;
    }
    if (looksLikeHtmlLine(trimmedLine)) {
      flushFlow();
      const openHtmlBlockTag = getOpenHtmlBlockTag(trimmedLine);
      if (openHtmlBlockTag) {
        htmlBlock = [trimmedLine];
        htmlBlockTag = openHtmlBlockTag;
      } else {
        blocks.push(renderHtml(trimmedLine, `html-${blocks.length}`, true));
      }
      return;
    }
    const heading = /^(#{1,3})\s+(.+)$/.exec(line.trim());
    if (heading) {
      flushFlow();
      const level = heading[1].length;
      const className =
        level === 1
          ? 'text-base font-semibold leading-7 text-foreground'
          : 'font-semibold leading-7 text-foreground';
      blocks.push(
        level === 1 ? (
          <h3 key={`h-${blocks.length}`} className={className}>
            {splitInline(heading[2])}
          </h3>
        ) : level === 2 ? (
          <h4 key={`h-${blocks.length}`} className={className}>
            {splitInline(heading[2])}
          </h4>
        ) : (
          <h5 key={`h-${blocks.length}`} className={className}>
            {splitInline(heading[2])}
          </h5>
        ),
      );
      return;
    }
    const bullet = /^[-*+]\s+(.+)$/.exec(line.trim());
    if (bullet) {
      flushParagraph();
      flushQuote();
      list.push(bullet[1]);
      return;
    }
    const blockquote = /^>\s?(.+)$/.exec(line.trim());
    if (blockquote) {
      flushParagraph();
      flushList();
      quote.push(blockquote[1]);
      return;
    }
    flushList();
    flushQuote();
    paragraph.push(line.trim());
  });

  flushHtmlBlock();

  if (code !== null) {
    const unclosedCode = code as string[];
    blocks.push(
      <pre
        key={`code-${blocks.length}`}
        className="overflow-x-auto rounded-lg border border-border bg-muted/40 p-3 text-xs text-foreground"
      >
        <code>{unclosedCode.join('\n')}</code>
      </pre>,
    );
  }
  flushFlow();
  return blocks;
}

export function MarkdownContent({ markdown }: { markdown: string }) {
  const blocks = renderMarkdownBlocks(markdown.trim());
  if (blocks.length === 0) return null;
  return <div className="space-y-3 text-sm">{blocks}</div>;
}
