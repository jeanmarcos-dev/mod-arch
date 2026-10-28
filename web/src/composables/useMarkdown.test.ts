import { describe, expect, it } from 'vitest';
import { renderMarkdown } from './useMarkdown';

describe('renderMarkdown', () => {
  it('drops raw HTML coming from the tutor', () => {
    const html = renderMarkdown('hola <img src=x onerror=alert(1)> <script>alert(1)</script>');
    expect(html).not.toContain('<img');
    expect(html).not.toContain('<script');
    expect(html).toContain('&lt;script&gt;');
  });

  it('never turns javascript: URLs into links', () => {
    const html = renderMarkdown('[click](javascript:alert(1))');
    expect(html).not.toContain('href="javascript:');
  });

  it('opens external links in a new tab without referrer', () => {
    const html = renderMarkdown('see https://example.com/docs');
    expect(html).toContain('href="https://example.com/docs"');
    expect(html).toContain('target="_blank"');
    expect(html).toContain('rel="noopener noreferrer"');
  });

  it('wraps code blocks and escapes their raw source', () => {
    const html = renderMarkdown('```c\nint a = 1 < 2;\n```');
    expect(html).toContain('class="md-codeblock"');
    expect(html).toContain('data-code="int a = 1 &lt; 2;');
    expect(html).toContain('language-c');
  });

  it('falls back to plaintext for unknown languages', () => {
    expect(renderMarkdown('```brainfuck\n+++\n```')).toContain('language-plaintext');
  });

  it('closes a fence left open by streaming', () => {
    const html = renderMarkdown('texto\n```c\nint x;');
    expect(html).toContain('<pre class="hljs">');
  });
});
