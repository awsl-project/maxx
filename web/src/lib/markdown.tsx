import { Fragment, type ReactNode } from 'react';

function splitInline(text: string): ReactNode[] {
  const nodes: ReactNode[] = [];
  const pattern =
    /(\[[^\]]+\]\(https?:\/\/[^\s)]+\)|`[^`]+`|\*\*[^*]+\*\*|__[^_]+__|\*[^*]+\*|_[^_]+_)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(text)) !== null) {
    if (match.index > lastIndex) nodes.push(text.slice(lastIndex, match.index));
    const token = match[0];
    const key = `${match.index}-${token}`;
    if (token.startsWith('[')) {
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

  lines.forEach((rawLine) => {
    const line = rawLine.trimEnd();
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
