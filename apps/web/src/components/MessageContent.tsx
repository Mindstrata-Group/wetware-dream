"use client";

import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Children, cloneElement, isValidElement, type ReactNode } from "react";

function cleanMarkdown(raw: string) {
  return raw
    .replace(/[\u200B-\u200D\uFEFF]/g, '')
    .split('\n')
    .map(line => line.replace(/^\s*[•·]+\s*/g, '- ').replace(/[ \t]+$/g, '').trimStart())
    .join('\n')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

function splitHighlightedText(text: string, needle: string): ReactNode {
  const query = needle.trim();
  if (!query) return text;
  const lowerText = text.toLowerCase();
  const lowerQuery = query.toLowerCase();
  const parts: ReactNode[] = [];
  let cursor = 0;
  let index = lowerText.indexOf(lowerQuery);
  while (index >= 0) {
    if (index > cursor) parts.push(text.slice(cursor, index));
    const match = text.slice(index, index + query.length);
    parts.push(
      <mark
        key={`${index}-${match}`}
        data-chat-search-mark="true"
        style={{ borderRadius: 4, padding: "0 2px", background: "#FDE68A", color: "#1F2937" }}
      >
        {match}
      </mark>,
    );
    cursor = index + query.length;
    index = lowerText.indexOf(lowerQuery, cursor);
  }
  if (cursor < text.length) parts.push(text.slice(cursor));
  return parts;
}

function highlightChildren(children: ReactNode, needle: string): ReactNode {
  if (!needle.trim()) return children;
  return Children.map(children, (child) => {
    if (typeof child === "string") return splitHighlightedText(child, needle);
    if (Array.isArray(child)) return highlightChildren(child, needle);
    if (isValidElement<{ children?: ReactNode }>(child)) {
      return cloneElement(child, {
        children: highlightChildren(child.props.children, needle),
      });
    }
    return child;
  });
}

export function MessageContent({ content, highlight = "" }: { content: string; highlight?: string }) {
  const highlightNode = (children: ReactNode) => highlightChildren(children, highlight);
  return (
    <div className="prose prose-sm max-w-none" style={{ maxWidth: "100%" }}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          h1: ({ children }) => <h1 className="text-lg font-bold mb-2">{highlightNode(children)}</h1>,
          h2: ({ children }) => <h2 className="text-base font-semibold mb-2">{highlightNode(children)}</h2>,
          h3: ({ children }) => <h3 className="text-sm font-medium mb-1">{highlightNode(children)}</h3>,
          p: ({ children }) => <p className="mb-2 whitespace-pre-wrap">{highlightNode(children)}</p>,
          ul: ({ children }) => <ul className="list-disc list-inside mb-2">{highlightNode(children)}</ul>,
          ol: ({ children }) => <ol className="list-decimal list-inside mb-2">{highlightNode(children)}</ol>,
          li: ({ children }) => <li className="mb-1">{highlightNode(children)}</li>,
          strong: ({ children }) => <strong className="font-semibold">{highlightNode(children)}</strong>,
          em: ({ children }) => <em className="italic">{highlightNode(children)}</em>,
          code: ({ children }) => (
            <code className="bg-slate-100 px-1 py-0.5 rounded text-xs font-mono" style={{ overflowWrap: "break-word", wordBreak: "break-word" }}>{highlightNode(children)}</code>
          ),
          // Long code blocks/tables from the AI used to push the message bubble
          // beyond the screen on mobile (did not fit even in landscape):
          // without its own overflow-x the content overflowed the parent's maxWidth.
          pre: ({ children }) => (
            <pre className="mb-2" style={{ overflowX: "auto", maxWidth: "100%" }}>{children}</pre>
          ),
          table: ({ children }) => (
            <div style={{ overflowX: "auto", maxWidth: "100%" }}>
              <table className="mb-2">{highlightNode(children)}</table>
            </div>
          ),
          blockquote: ({ children }) => (
            <blockquote className="border-l-4 border-slate-300 pl-4 italic mb-2">
              {highlightNode(children)}
            </blockquote>
          ),
        }}
      >
        {cleanMarkdown(content)}
      </ReactMarkdown>
    </div>
  );
}
