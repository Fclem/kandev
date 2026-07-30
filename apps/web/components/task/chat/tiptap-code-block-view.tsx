"use client";

import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from "@tiptap/react";

const CODE_LANGUAGES = [
  { value: "", labelKey: "task:plain" },
  { value: "javascript", labelKey: "task:javascript" },
  { value: "typescript", labelKey: "task:typescript" },
  { value: "python", labelKey: "task:python" },
  { value: "go", labelKey: "task:go" },
  { value: "rust", labelKey: "task:rust" },
  { value: "java", labelKey: "task:java" },
  { value: "cpp", label: "C++" },
  { value: "c", label: "C" },
  { value: "css", label: "CSS" },
  { value: "html", label: "HTML" },
  { value: "json", label: "JSON" },
  { value: "yaml", label: "YAML" },
  { value: "markdown", labelKey: "task:markdown" },
  { value: "bash", labelKey: "task:bash" },
  { value: "sql", label: "SQL" },
  { value: "xml", label: "XML" },
];

export function CodeBlockView({ node, updateAttributes }: NodeViewProps) {
  const language = (node.attrs.language as string) || "";

  return (
    <NodeViewWrapper as="pre">
      <select
        contentEditable={false}
        className="code-block-language"
        value={language}
        onChange={(e) => updateAttributes({ language: e.target.value })}
      >
        {CODE_LANGUAGES.map((lang) => (
          <option key={lang.value} value={lang.value}>
            {lang.label}
          </option>
        ))}
      </select>
      {/* @ts-expect-error -- NodeViewContent 'as' prop accepts any HTML tag but types only allow 'div' */}
      <NodeViewContent as="code" className={language ? `language-${language} hljs` : ""} />
    </NodeViewWrapper>
  );
}
