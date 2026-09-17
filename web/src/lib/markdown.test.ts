import { describe, expect, it } from 'vitest';
import { renderMarkdownBlocks } from './markdown';

describe('renderMarkdownBlocks', () => {
  it('renders common markdown blocks without raw html injection', () => {
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
    expect(String(JSON.stringify(blocks))).toContain('Hello');
    expect(String(JSON.stringify(blocks))).toContain('https://example.com');
    expect(String(JSON.stringify(blocks))).toContain('<script>alert(1)</script>');
  });
});
