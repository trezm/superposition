import { createHighlighter, type Highlighter } from "shiki";

let highlighterPromise: Promise<Highlighter> | null = null;

const EXT_TO_LANG: Record<string, string> = {
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  go: "go",
  py: "python",
  rs: "rust",
  rb: "ruby",
  java: "java",
  kt: "kotlin",
  swift: "swift",
  c: "c",
  h: "c",
  cpp: "cpp",
  hpp: "cpp",
  cs: "csharp",
  css: "css",
  scss: "scss",
  html: "html",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  toml: "toml",
  xml: "xml",
  md: "markdown",
  mdx: "mdx",
  sql: "sql",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  dockerfile: "dockerfile",
  graphql: "graphql",
  vue: "vue",
  svelte: "svelte",
  lua: "lua",
  zig: "zig",
  php: "php",
};

export function extToLang(filePath: string): string | undefined {
  // Handle special filenames
  const name = filePath.split("/").pop()?.toLowerCase() ?? "";
  if (name === "dockerfile") return "dockerfile";
  if (name === "makefile") return "makefile";

  const ext = name.split(".").pop()?.toLowerCase();
  if (!ext) return undefined;
  return EXT_TO_LANG[ext];
}

export async function getHighlighter(): Promise<Highlighter> {
  if (!highlighterPromise) {
    highlighterPromise = createHighlighter({
      themes: ["github-dark"],
      langs: [],
    });
  }
  return highlighterPromise;
}

export async function tokenizeLines(
  code: string,
  lang: string,
): Promise<{ content: string; color?: string }[][]> {
  const highlighter = await getHighlighter();

  // Load language on demand
  const loadedLangs = highlighter.getLoadedLanguages();
  if (!loadedLangs.includes(lang)) {
    try {
      await highlighter.loadLanguage(lang as any);
    } catch {
      // Language not available, return plain text
      return code.split("\n").map((line) => [{ content: line }]);
    }
  }

  const { tokens } = highlighter.codeToTokens(code, {
    lang: lang as any,
    theme: "github-dark",
  });

  return tokens.map((line) =>
    line.map((token) => ({
      content: token.content,
      color: token.color,
    })),
  );
}
