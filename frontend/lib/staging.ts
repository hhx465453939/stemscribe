// Staging is a list of paths, not a copy. Dropping or browsing only records the
// absolute path and its size for display; the media file is never moved, copied or
// read here. The pipeline touches it only after the user presses Start.
export interface StagedFile {
  path: string;
  /** Bytes, or -1 when the platform cannot stat (browser shell). */
  size: number;
}

/** Windows paths are case-insensitive; comparing them exactly lets the same file
 * appear twice when it was dropped once with a different drive-letter case. */
function key(path: string): string {
  return path.trim().toLowerCase();
}

export function addStaged(list: StagedFile[], incoming: StagedFile[]): StagedFile[] {
  const seen = new Set(list.map((f) => key(f.path)));
  const next = [...list];
  for (const f of incoming) {
    const k = key(f.path);
    if (f.path.trim() === "" || seen.has(k)) continue;
    seen.add(k);
    next.push({ path: f.path, size: f.size });
  }
  return next;
}

export function removeStaged(list: StagedFile[], path: string): StagedFile[] {
  const k = key(path);
  return list.filter((f) => key(f.path) !== k);
}

export function clearStaged(): StagedFile[] {
  return [];
}

/** Human size for the row. -1 means "unknown", which is the browser shell. */
export function formatSize(bytes: number): string {
  if (bytes < 0) return "—";
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
  if (bytes >= 1024 ** 2) return `${Math.round(bytes / 1024 ** 2)} MB`;
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${bytes} B`;
}

/** Basename for both separators; the UI never renders a full path in a table. */
export function baseName(path: string): string {
  const parts = path.split(/[\\/]/);
  return parts[parts.length - 1] || path;
}
