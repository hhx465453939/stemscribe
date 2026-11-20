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

interface LibraryItem {
  id: string;
  title: string;
  status: string;
  segments: number;
  stages_done: number;
  stages_total: number;
  engine: string;
  engine_version: string;
}

// No fabricated fallback. When the backend is unreachable the board stays empty and
// says so; invented rows would make a broken system indistinguishable from a
// working one, which is exactly the failure this project refuses.
export async function listTasks(): Promise<TaskView[]> {
  try {
    const res = await fetch("/api/library", { cache: "no-store" });
    if (!res.ok) return [];
    const body = (await res.json()) as { items?: LibraryItem[] };
    return (body.items ?? []).map((it) => ({
      id: it.id,
      title: it.title || it.id,
      stage: (it.status === "done" ? "done" : it.status === "failed" ? "failed" : "queued") as TaskStage,
      current: it.stages_done,
      total: it.stages_total,
      detail: [it.engine, it.engine_version].filter(Boolean).join(" "),
    }));
  } catch {
    return [];
  }
}

// Empty on purpose. The LLM provider layer is not implemented yet (CHG-0004), and
// listing invented models with invented context windows is indistinguishable from a
// working provider to anyone reading the screen. An empty list plus an explicit
// "not implemented" note is the honest state.
export const demoModels: ProviderModel[] = [];

export function formatContextWindow(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(n % 1_000_000 === 0 ? 0 : 1)}M`;
  if (n >= 1000) return `${Math.round(n / 1000)}k`;
  return String(n);
}

/** Maps a pipeline stage to its dictionary key; translation stays in the UI. */
export function stageKey(stage: TaskStage): Key {
  return `stage.${stage}` as Key;
}
