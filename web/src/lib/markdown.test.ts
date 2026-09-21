import { createElement, Fragment } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { MarkdownContent, renderMarkdownBlocks } from './markdown';

describe('renderMarkdownBlocks', () => {
  it('renders common markdown blocks and keeps code html inert', () => {
    const blocks = renderMarkdownBlocks(
      [
        '# Hello **world**',
        '',
        '- one',
        '- [link](https://example.com)',
        '',
        '> quote',
        '',
        '```',
        '<script>alert(1)</script>',
        '```',
      ].join('\n'),
    );

    expect(blocks).toHaveLength(4);
    const html = renderToStaticMarkup(createElement(Fragment, null, ...blocks));
    expect(html).toContain('Hello');
    expect(html).toContain('https://example.com');
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;');
  });

  it('keeps multi-line html blocks together', () => {
    const html = renderToStaticMarkup(
      createElement(MarkdownContent, {
        markdown: [
          '<table>',
          '<tbody>',
          '<tr><td>One</td><td><strong>Two</strong></td></tr>',
          '</tbody>',
          '</table>',
        ].join('\n'),
      }),
    );

    expect(html).toContain('<table>');
    expect(html).toContain('<tbody>');
    expect(html).toContain('<tr><td>One</td><td><strong>Two</strong></td></tr>');
  });

  it('renders sanitized html inside user announcement markdown', () => {
    const html = renderToStaticMarkup(
      createElement(MarkdownContent, {
        markdown: [
          '# Notice',
          '',
          '<div style="color: red; position: fixed" onclick="alert(1)">HTML <strong>enabled</strong><script>alert(1)</script></div>',
          '',
          'Inline <span style="color: #16a34a">green</span> and <a href="javascript:alert(1)">bad</a>.',
        ].join('\n'),
      }),
    );

    expect(html).toContain('<div style="color: red">HTML <strong>enabled</strong></div>');
    expect(html).toContain('<span style="color: #16a34a">green</span>');
    expect(html).not.toContain('onclick');
    expect(html).not.toContain('<script>');
    expect(html).not.toContain('javascript:');
  });
});
