// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

import type { StreamParser, StringStream } from "@codemirror/language";

// A small HCL highlighter for Terraform files (CodeMirror has none built
// in): comments (#, //, /* */), strings with ${…} interpolation, heredocs,
// numbers, block keywords and literals.
const BLOCKS = new Set(["resource", "data", "variable", "output", "module", "provider", "terraform", "locals", "moved", "import", "check", "removed", "backend", "required_providers", "dynamic", "content", "lifecycle", "provisioner", "connection"]);
const LITERALS = new Set(["true", "false", "null"]);
const KEYWORDS = new Set(["for", "in", "if", "else", "endif", "endfor"]);

interface State {
  inComment: boolean;
  inString: boolean;
  heredoc: string | null;
}

function string(stream: StringStream, state: State) {
  while (!stream.eol()) {
    if (stream.match("${", false)) return "string";
    const ch = stream.next();
    if (ch === "\\") stream.next();
    else if (ch === '"') {
      state.inString = false;
      return "string";
    }
  }
  return "string";
}

export const hcl: StreamParser<State> = {
  name: "hcl",
  startState: () => ({ inComment: false, inString: false, heredoc: null }),
  token(stream, state) {
    if (state.heredoc) {
      if (stream.sol() && stream.match(new RegExp(`^\\s*${state.heredoc}\\s*$`))) {
        state.heredoc = null;
        return "string";
      }
      stream.skipToEnd();
      return "string";
    }
    if (state.inComment) {
      if (stream.skipTo("*/")) {
        stream.match("*/");
        state.inComment = false;
      } else stream.skipToEnd();
      return "comment";
    }
    if (state.inString) {
      if (stream.match("${")) {
        while (!stream.eol() && !stream.match("}")) stream.next();
        return "variableName";
      }
      return string(stream, state);
    }
    if (stream.eatSpace()) return null;
    if (stream.match("#") || stream.match("//")) {
      stream.skipToEnd();
      return "comment";
    }
    if (stream.match("/*")) {
      state.inComment = true;
      return "comment";
    }
    const heredoc = stream.match(/^<<-?([A-Za-z_]+)/) as RegExpMatchArray | null;
    if (heredoc) {
      state.heredoc = heredoc[1];
      return "string";
    }
    if (stream.peek() === '"') {
      stream.next();
      state.inString = true;
      return string(stream, state);
    }
    if (stream.match(/^-?\d+(\.\d+)?/)) return "number";
    const word = stream.match(/^[A-Za-z_][\w-]*/) as RegExpMatchArray | null;
    if (word) {
      const w = word[0];
      if (stream.sol() || BLOCKS.has(w)) {
        if (BLOCKS.has(w)) return "keyword";
      }
      if (LITERALS.has(w)) return "atom";
      if (KEYWORDS.has(w)) return "keyword";
      // An attribute name: followed by "=".
      if (stream.match(/^\s*=(?!=)/, false)) return "propertyName";
      return "variableName";
    }
    stream.next();
    return null;
  },
  languageData: { commentTokens: { line: "#", block: { open: "/*", close: "*/" } } },
};
