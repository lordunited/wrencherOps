// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import { StringStream } from "@codemirror/language";
import { describe, expect, it } from "vitest";
import { hcl } from "@/lib/hclMode";

// tokens runs the highlighter over lines, keeping its state between them,
// and returns each token's text and style.
function tokens(...lines: string[]): [string, string | null][] {
  const state = hcl.startState!(2);
  const out: [string, string | null][] = [];
  for (const line of lines) {
    const stream = new StringStream(line, 2, 2);
    while (!stream.eol()) {
      const style = hcl.token(stream, state);
      const text = stream.current();
      if (text.trim()) out.push([text, style ?? null]);
      stream.start = stream.pos;
    }
  }
  return out;
}

const styleOf = (toks: [string, string | null][], text: string) => toks.find(([t]) => t === text)?.[1];

describe("hcl highlighter", () => {
  it("marks blocks, strings, numbers and literals", () => {
    const t = tokens('resource "aws_instance" "web" {', "  count = 2", "  enabled = true", "}");
    expect(styleOf(t, "resource")).toBeTruthy();
    expect(t.some(([text, s]) => text.includes("aws_instance") && s === "string")).toBe(true);
    expect(styleOf(t, "2")).toBe("number");
    expect(styleOf(t, "true")).toBeTruthy();
  });
  it("carries a block comment across lines", () => {
    const t = tokens("/* a", "still a comment */", "x = 1");
    expect(t[0][1]).toBe("comment");
    expect(t[1][1]).toBe("comment");
    expect(styleOf(t, "1")).toBe("number");
  });
  it("carries a heredoc to its end marker", () => {
    const t = tokens("x = <<EOT", "anything = here", "EOT", "y = 1");
    expect(t.find(([text]) => text.includes("anything"))?.[1]).toBe("string");
    expect(styleOf(t, "1")).toBe("number");
  });
});
