// Typed bridge to the Go backend.
//
// In the shell, window.go.main.App exposes the bindings. In a plain browser (and
// during `next build`) there is no bridge, so we fall back to demo data. The
// fallback is explicit and the UI labels it — never a silent fake.
//
// Localisation rule: this module never returns user-visible prose. It returns
// i18n keys and the components translate them (CHG-0007 AC-04).

import type { Key } from "./i18n";

export type TaskStage =
  | "queued"
  | "downloading"
  | "separating"
  | "transcribing"
  | "aligning"
  | "polishing"
  | "exporting"
  | "done"
  | "failed";

export interface TaskView {
  id: string;
  /** Present for real tasks coming from the backend. */
  title?: string;
  /** Present for demo tasks, so their text localises with the UI. */
  titleKey?: Key;
  stage: TaskStage;
  current: number;
  total: number;
  detail: string;
  errorHintKey?: Key;
  errorHint?: string;
}

export interface ModelCaps {
  context_window: number;
  max_output_tokens: number;
  vision: boolean;
  tools: boolean;
  streaming: boolean;
  reasoning_supported: boolean;
  reasoning_mode?: string;
  caps_source: "registry" | "probe" | "manual";
}

export interface ModelParams {
  temperature: number;
  top_p: number;
  max_tokens: number;
  reasoning_effort?: string;
  stream: boolean;
}

export interface ProviderModel {
  id: string;
  caps: ModelCaps;
  params: ModelParams;
}

interface Bridge {
  ListTasks(): Promise<TaskView[]>;
  ProbeHardware(): Promise<Record<string, unknown>>;
  RefreshModels(providerId: string): Promise<string[]>;
}

declare global {
  interface Window {
    go?: { main?: { App?: Bridge } };
  }
}

export function bridge(): Bridge | null {
  if (typeof window === "undefined") return null;
  return window.go?.main?.App ?? null;
}

export const isShell = (): boolean => bridge() !== null;

const demoTasks: TaskView[] = [
  {
    id: "demo-1",
    titleKey: "demo.interview",
    stage: "transcribing",
    current: 128,
    total: 240,
    detail: "medium-q5_0 · cuda12",
  },
  {
    id: "demo-2",
    titleKey: "demo.teardown",
    stage: "done",
    current: 96,
    total: 96,
    detail: "medium-q5_0 · 4m12s",
  },
  {
    id: "demo-3",
    titleKey: "demo.failed",
    stage: "failed",
    current: 0,
    total: 0,
    detail: "DL_403",
    errorHintKey: "demo.failedHint",
  },
];

export async function listTasks(): Promise<TaskView[]> {
  const b = bridge();
  if (!b) return demoTasks;
  try {
    return await b.ListTasks();
  } catch {
    return [];
  }
}

export const demoModels: ProviderModel[] = [
  {
    id: "qwen2.5:14b",
    caps: {
      context_window: 131072,
      max_output_tokens: 8192,
      vision: false,
      tools: true,
      streaming: true,
      reasoning_supported: false,
      caps_source: "registry",
    },
    params: { temperature: 0.3, top_p: 0.9, max_tokens: 6000, stream: true },
  },
  {
    id: "gpt-4.1",
    caps: {
      context_window: 1047576,
      max_output_tokens: 32768,
      vision: true,
      tools: true,
      streaming: true,
      reasoning_supported: false,
      caps_source: "manual",
    },
    params: { temperature: 0.2, top_p: 0.9, max_tokens: 16000, stream: true },
  },
  {
    id: "o4-mini",
    caps: {
      context_window: 200000,
      max_output_tokens: 100000,
      vision: true,
      tools: true,
      streaming: true,
      reasoning_supported: true,
      reasoning_mode: "effort",
      caps_source: "registry",
    },
    params: {
      temperature: 1,
      top_p: 1,
      max_tokens: 32000,
      reasoning_effort: "medium",
      stream: true,
    },
  },
];

export function formatContextWindow(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 1)}M`;
  if (n >= 1000) return `${Math.round(n / 1000)}k`;
  return String(n);
}

/** Maps a pipeline stage to its dictionary key; translation stays in the UI. */
export function stageKey(stage: TaskStage): Key {
  return `stage.${stage}` as Key;
}
