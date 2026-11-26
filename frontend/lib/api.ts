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

export interface LibraryItemView {
  id: string;
  title: string;
  status: string;
  /** Absolute bundle directory. Used by "open folder" and by the API assertion. */
  dir: string;
  segments: number;
  duration_ms: number;
  engine: string;
  engine_version: string;
  model_id?: string;
  transcribe_backend?: string;
}

/** Real bundles only. An empty array is the truth when nothing has run yet. */
export async function listLibrary(): Promise<LibraryItemView[]> {
  try {
    const res = await fetch("/api/library", { cache: "no-store" });
    if (!res.ok) return [];
    const body = (await res.json()) as { items?: LibraryItemView[] };
    return body.items ?? [];
  } catch {
    return [];
  }
}

export interface CatalogItemView {
  id: string;
  name: string;
  kind: string;
  license: string;
  tier?: string;
  size_mb: number;
  vram_mb: number;
  installed: boolean;
  /** Registry version, or the installed audio-separator version. */
  version?: string;
  /** Absolute install directory; only present when installed. */
  path?: string;
}

export interface CatalogView {
  installed: CatalogItemView[];
  available: CatalogItemView[];
}

/** Installed versus downloadable, as reported by the Go registry. */
export async function listCatalog(): Promise<CatalogView> {
  try {
    const res = await fetch("/api/models", { cache: "no-store" });
    if (!res.ok) return { installed: [], available: [] };
    return (await res.json()) as CatalogView;
  } catch {
    return { installed: [], available: [] };
  }
}

export interface InstallJobView {
  id: string;
  target: string;
  state: "idle" | "running" | "done" | "failed";
  percent: number;
  message: string;
}

/** Starts an install. Returns an error string instead of throwing, so the UI can show
 * the refusal verbatim (unverified entries are refused by design, not by accident). */
export async function startInstall(id: string): Promise<{ error?: string }> {
  try {
    const res = await fetch(`/api/models/${encodeURIComponent(id)}/install`, { method: "POST" });
    const body = (await res.json().catch(() => ({}))) as { id?: string; error?: string };
    if (!res.ok) return { error: body.error || `HTTP ${res.status}` };
    return {};
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) };
  }
}

/** Current install job. Percent is only meaningful when > 0. */
export async function getInstallStatus(): Promise<InstallJobView | null> {
  try {
    const res = await fetch("/api/models/install", { cache: "no-store" });
    if (!res.ok) return null;
    return (await res.json()) as InstallJobView;
  } catch {
    return null;
  }
}

export interface QueueItemView {
  id: string;
  input: string;
  state: "waiting" | "running" | "done" | "failed" | "canceled";
  percent: number;
  segments: number;
  artifact_dir: string;
  message: string;
  /** Pipeline stage id while running (separate, transcribe, ...). */
  stage?: string;
  /** Milliseconds since the run started; 0 when not running. */
  elapsed_ms?: number;
  /** True after a stop was requested but before the process tree is gone. */
  stopping?: boolean;
  /** Machine codes the row must surface, e.g. MODEL_SUBSTITUTED. */
  warnings?: string[];
}

export interface QueueView {
  items: QueueItemView[];
  current: string;
  done: number;
  total: number;
}

/** Queues several inputs. Rejected paths come back verbatim so the UI can explain
 * why (e.g. a .txt is refused at the door rather than failing three stages later). */
export async function enqueue(
  inputs: string[],
  opts: { outputDir?: string; model?: string; params?: Record<string, string> } = {},
): Promise<{ added?: number; rejected?: string[]; error?: string }> {
  try {
    const res = await fetch("/api/queue", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        inputs,
        output_dir: opts.outputDir ?? "",
        model: opts.model ?? "",
        params: opts.params ?? {},
      }),
    });
    const body = (await res.json().catch(() => ({}))) as {
      added?: number;
      rejected?: string[];
      error?: string;
    };
    if (!res.ok) return { error: body.error || `HTTP ${res.status}` };
    return body;
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) };
  }
}

export async function getQueue(): Promise<QueueView | null> {
  try {
    const res = await fetch("/api/queue", { cache: "no-store" });
    if (!res.ok) return null;
    return (await res.json()) as QueueView;
  } catch {
    return null;
  }
}

export async function removeQueued(id: string): Promise<boolean> {
  try {
    const res = await fetch(`/api/queue/${encodeURIComponent(id)}`, { method: "DELETE" });
    return res.ok || res.status === 204;
  } catch {
    return false;
  }
}

/** Stops the running item. The row stays visible with state "canceled". */
export async function stopQueued(id: string): Promise<{ error?: string }> {
  try {
    const res = await fetch(`/api/queue/${encodeURIComponent(id)}/stop`, { method: "POST" });
    if (res.ok) return {};
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    return { error: body.error || `HTTP ${res.status}` };
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) };
  }
}

export interface SettingsView {
  output_dir: string;
  model: string;
  /** UVR5 separation model. Separate from the transcription model on purpose. */
  separator?: string;
  workdir: string;
}

export async function getSettings(): Promise<SettingsView | null> {
  try {
    const res = await fetch("/api/settings", { cache: "no-store" });
    if (!res.ok) return null;
    return (await res.json()) as SettingsView;
  } catch {
    return null;
  }
}

export async function saveSettings(patch: {
  output_dir?: string;
  model?: string;
  separator?: string;
}): Promise<{ error?: string }> {
  try {
    const res = await fetch("/api/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    });
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    if (!res.ok) return { error: body.error || `HTTP ${res.status}` };
    return {};
  } catch (e) {
    return { error: e instanceof Error ? e.message : String(e) };
  }
}

/** Interface a native shell exposes; absent in a plain browser. */
export interface ShellBridge {
  PickFiles?: () => Promise<string[]>;
  PickFolder?: (initial?: string) => Promise<string>;
  StatFiles?: (paths: string[]) => Promise<{ path: string; size: number; error?: string }[]>;
  OpenFolder?: (path: string) => Promise<void>;
}

export function shell(): ShellBridge | null {
  if (typeof window === "undefined") return null;
  return (window as unknown as { go?: { main?: { App?: ShellBridge } } }).go?.main?.App ?? null;
}

/** The bits of the Wails runtime this UI uses. Absent in a browser. */
interface WailsRuntime {
  OnFileDrop?: (
    cb: (x: number, y: number, paths: string[]) => void,
    useDropTarget?: boolean,
  ) => void;
  OnFileDropOff?: () => void;
}

/**
 * Subscribes to files dropped on the window. Returns an unsubscribe function.
 *
 * `useDropTarget=false` accepts a drop anywhere in the window instead of only on an
 * element tagged via a CSS variable; for a single-window tool that is the behaviour
 * people expect (ADR-040).
 */
export function onFilesDropped(cb: (paths: string[]) => void): () => void {
  if (typeof window === "undefined") return () => {};
  const rt = (window as unknown as { runtime?: WailsRuntime }).runtime;
  if (!rt?.OnFileDrop) return () => {};
  rt.OnFileDrop((_x, _y, paths) => {
    if (Array.isArray(paths) && paths.length > 0) cb(paths);
  }, false);
  return () => rt.OnFileDropOff?.();
}

/** Formats mm:ss from milliseconds; used by the library table. */
export function formatDuration(ms: number): string {
  if (!ms || ms <= 0) return "—";
  const total = Math.round(ms / 1000);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

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
