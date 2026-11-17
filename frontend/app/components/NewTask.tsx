"use client";

import { useState } from "react";

export function NewTask() {
  const [value, setValue] = useState("");
  const [separate, setSeparate] = useState(true);
  const [polish, setPolish] = useState(false);

  return (
    <div className="panel stack">
      <div className="row">
        <input
          className="input"
          placeholder="粘贴链接，或把本地文件路径拖进来…"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          aria-label="任务输入"
        />
        <button className="btn btn--primary" type="button" disabled={value.trim() === ""}>
          开始
        </button>
      </div>

      {/* Defaults must let 80% of users press Start immediately (PRD §4.2);
          advanced knobs stay folded. */}
      <div className="row" style={{ gap: "var(--sp-6)" }}>
        <label className="row" style={{ gap: "var(--sp-2)" }}>
          <input
            type="checkbox"
            checked={separate}
            onChange={(e) => setSeparate(e.target.checked)}
          />
          <span>人声分离</span>
        </label>

        <label className="row" style={{ gap: "var(--sp-2)" }}>
          <input type="checkbox" checked={polish} onChange={(e) => setPolish(e.target.checked)} />
          <span>LLM 润色</span>
          <span className="tag">会外发文本</span>
        </label>

        <span className="mono muted">
          转写 medium · cuda12 · 分段 15s
        </span>
      </div>

      {polish ? (
        <p className="mono muted" style={{ margin: 0 }}>
          润色会把逐字稿发送到你配置的端点（设置 → Provider）。音视频文件不会离开本机。
        </p>
      ) : null}
    </div>
  );
}
