import MarkdownIt from 'markdown-it';
import hljs from 'highlight.js/lib/core';
import asm from 'highlight.js/lib/languages/x86asm';
import c from 'highlight.js/lib/languages/c';
import cpp from 'highlight.js/lib/languages/cpp';
import go from 'highlight.js/lib/languages/go';
import javascript from 'highlight.js/lib/languages/javascript';
import python from 'highlight.js/lib/languages/python';
import bash from 'highlight.js/lib/languages/bash';
import plaintext from 'highlight.js/lib/languages/plaintext';

// Register a tight subset so the bundle stays small. Anything outside this
// list falls back to plaintext via the highlight() callback below.
hljs.registerLanguage('asm', asm);
hljs.registerLanguage('x86asm', asm);
hljs.registerLanguage('c', c);
hljs.registerLanguage('cpp', cpp);
hljs.registerLanguage('go', go);
hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('js', javascript);
hljs.registerLanguage('python', python);
hljs.registerLanguage('py', python);
hljs.registerLanguage('bash', bash);
hljs.registerLanguage('sh', bash);
hljs.registerLanguage('plaintext', plaintext);

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// html:false makes the renderer drop any raw HTML in the source. The tutor
// text comes from an LLM and could contain hostile markup, so we never trust
// it. Without raw HTML the output is XSS-safe without DOMPurify.
const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
  highlight(str: string, lang: string): string {
    const safeLang = lang && hljs.getLanguage(lang) ? lang : 'plaintext';
    let body: string;
    try {
      body = hljs.highlight(str, { language: safeLang, ignoreIllegals: true }).value;
    } catch {
      body = escapeHtml(str);
    }
    // The wrapper carries data-code with the raw source so a delegated click
    // handler on the message can copy it without touching the DOM tree of
    // the highlighted spans.
    const rawAttr = escapeHtml(str);
    return (
      `<div class="md-codeblock" data-code="${rawAttr}">`
      + `<span class="md-codeblock-lang">${safeLang}</span>`
      + `<button type="button" class="md-codeblock-copy" data-copy aria-label="Copiar código">Copiar</button>`
      + `<pre class="hljs"><code class="language-${safeLang}">${body}</code></pre>`
      + `</div>`
    );
  },
});

// Open external links in a new tab without sending the referrer.
const defaultLinkRender = md.renderer.rules.link_open
  || ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));
md.renderer.rules.link_open = function (tokens, idx, options, env, self) {
  const token = tokens[idx];
  const href = String(token.attrGet('href') ?? '');
  if (/^https?:\/\//i.test(href)) {
    token.attrSet('target', '_blank');
    token.attrSet('rel', 'noopener noreferrer');
  }
  return defaultLinkRender(tokens, idx, options, env, self);
};

function sanitize(text: string): string {
  // Drop empty fence pairs the LLM sometimes emits before a real code block.
  let s = text.replace(/^\s*```\s*\n\s*```\s*(?:\n|$)/gm, '');
  // During streaming a chunk may end mid-fence; close it so md-it doesn't
  // render an empty <pre> while the next chunk is in flight.
  const fences = (s.match(/^```/gm) || []).length;
  if (fences % 2 === 1) s += '\n```';
  return s;
}

export function renderMarkdown(text: string): string {
  if (!text) return '';
  return md.render(sanitize(text));
}
