import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { MessageContent } from "./MessageContent";

// MessageContent renders user/assistant chat messages via react-markdown.
// react-markdown sanitizes by default (no raw HTML), but we verify the
// invariant here so a future config change (e.g. enabling rehype-raw)
// would fail this test loudly.
describe("MessageContent — XSS / injection safety", () => {
  it("does not render raw <script> tags as executable elements", () => {
    const { container } = render(
      <MessageContent content={'hello\n\n<script>alert("xss")</script>'} />
    );
    expect(container.querySelector("script")).toBeNull();
  });

  it("does not render raw <iframe> tags", () => {
    const { container } = render(
      <MessageContent content={'<iframe src="https://evil.com"></iframe>'} />
    );
    expect(container.querySelector("iframe")).toBeNull();
  });

  it("does not honor onerror on injected <img>", () => {
    const { container } = render(
      <MessageContent content={'<img src=x onerror="alert(1)">'} />
    );
    // react-markdown without rehype-raw renders the literal text, not an img element.
    const imgs = container.querySelectorAll("img");
    for (const img of Array.from(imgs)) {
      expect(img.getAttribute("onerror")).toBeNull();
    }
  });

  it("renders safe markdown elements (h1, p, strong)", () => {
    const { container } = render(
      <MessageContent content={"# Title\n\nBody **bold**"} />
    );
    expect(container.querySelector("h1")?.textContent).toContain("Title");
    expect(container.querySelector("strong")?.textContent).toBe("bold");
  });

  it("normalizes zero-width characters out of content", () => {
    const { container } = render(
      <MessageContent content={"vis​ible"} />
    );
    expect(container.textContent).toContain("visible");
    expect(container.textContent).not.toContain("​");
  });

  it("collapses 3+ blank lines to a single blank between paragraphs", () => {
    const { container } = render(
      <MessageContent content={"one\n\n\n\n\ntwo"} />
    );
    // Two paragraphs, not five empty ones.
    expect(container.querySelectorAll("p")).toHaveLength(2);
  });

  it("highlights case-insensitive search matches without raw HTML", () => {
    const { container } = render(
      <MessageContent content={"Игорь пишет, потом игорь уточняет"} highlight="игорь" />
    );
    const marks = container.querySelectorAll('mark[data-chat-search-mark="true"]');
    expect(marks).toHaveLength(2);
    expect(marks[0].textContent).toBe("Игорь");
    expect(marks[1].textContent).toBe("игорь");
  });
});

// Complaint "the message is laid out for desktop and does not fit the phone" (2026-07-10):
// a GFM table/code block without an overflow-x wrapper pushed the message bubble
// beyond the screen. We check that wide content is now wrapped in
// its own scroll container, not in a plain <table>/<pre>.
describe("MessageContent — mobile overflow containment", () => {
  it("wraps GFM tables in a horizontally-scrollable container", () => {
    const { container } = render(
      <MessageContent content={"| A | B |\n| - | - |\n| 1 | 2 |"} />
    );
    const table = container.querySelector("table");
    expect(table).not.toBeNull();
    const wrapper = table?.parentElement as HTMLElement;
    expect(wrapper.style.overflowX).toBe("auto");
  });

  it("wraps fenced code blocks in a horizontally-scrollable <pre>", () => {
    const { container } = render(
      <MessageContent content={"```\nvery long unbroken line of code that would otherwise overflow\n```"} />
    );
    const pre = container.querySelector("pre");
    expect(pre).not.toBeNull();
    expect((pre as HTMLElement).style.overflowX).toBe("auto");
  });
});
