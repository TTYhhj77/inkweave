function escapeHTML(str: string): string {
  return str
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function formatInline(escaped: string): string {
  return escaped
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>')
    .replace(/(^|[^*])\*([^*\n]+)\*(?!\*)/g, '$1<em>$2</em>');
}

/**
 * Converts lightweight Markdown from AI replies into safe HTML after escaping raw HTML entities.
 */
export function renderSafeMarkdown(src: string): string {
  if (!src) return '';
  const lines = src.replaceAll('\r\n', '\n').split('\n');
  const out: string[] = [];
  let inUl = false;
  let inOl = false;

  const closeLists = () => {
    if (inUl) {
      out.push('</ul>');
      inUl = false;
    }
    if (inOl) {
      out.push('</ol>');
      inOl = false;
    }
  };

  for (const rawLine of lines) {
    const line = rawLine.trimEnd();
    const trimmed = line.trim();

    if (trimmed === '') {
      closeLists();
      continue;
    }
    if (trimmed === '---' || trimmed === '***') {
      closeLists();
      out.push('<hr />');
      continue;
    }

    const headingMatch = trimmed.match(/^(#{1,4})\s+(.+)$/);
    if (headingMatch) {
      closeLists();
      const level = Math.min(4, Math.max(2, headingMatch[1].length));
      out.push(`<h${level}>${formatInline(escapeHTML(headingMatch[2]))}</h${level}>`);
      continue;
    }

    const ulMatch = trimmed.match(/^[-*•]\s+(.+)$/);
    if (ulMatch) {
      if (inOl) {
        out.push('</ol>');
        inOl = false;
      }
      if (!inUl) {
        out.push('<ul>');
        inUl = true;
      }
      out.push(`<li>${formatInline(escapeHTML(ulMatch[1]))}</li>`);
      continue;
    }

    const olMatch = trimmed.match(/^\d+\.\s+(.+)$/);
    if (olMatch) {
      if (inUl) {
        out.push('</ul>');
        inUl = false;
      }
      if (!inOl) {
        out.push('<ol>');
        inOl = true;
      }
      out.push(`<li>${formatInline(escapeHTML(olMatch[1]))}</li>`);
      continue;
    }

    if (trimmed.startsWith('> ')) {
      closeLists();
      out.push(`<blockquote>${formatInline(escapeHTML(trimmed.slice(2)))}</blockquote>`);
      continue;
    }

    closeLists();
    out.push(`<p>${formatInline(escapeHTML(trimmed))}</p>`);
  }

  closeLists();
  return out.join('');
}
