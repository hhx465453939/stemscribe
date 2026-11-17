export default function ModelsPage() {
  const installed = [
    { name: "Whisper Medium (q5_0)", kind: "转写", size: "540 MB", vram: "2.6 GB", license: "MIT", state: "已校验" },
    { name: "MDX-Net · Kim Vocal 2", kind: "分离", size: "63 MB", vram: "1.5 GB", license: "MIT", state: "已校验" },
  ];

  const available = [
    { name: "Whisper Small (q5_0)", kind: "转写", size: "190 MB", vram: "1.2 GB", license: "MIT", tier: "轻量" },
    { name: "Whisper Large v3 (q5_0)", kind: "转写", size: "1.08 GB", vram: "6.2 GB", license: "MIT", tier: "极致" },
    { name: "whisper.cpp CUDA 12", kind: "运行时", size: "48 MB", vram: "0.3 GB", license: "MIT", tier: null },
    { name: "FFmpeg 7.1 (LGPL)", kind: "运行时", size: "68 MB", vram: "—", license: "LGPL", tier: null },
  ];

  return (
    <>
      <h1 className="page-title">模型中心</h1>
      <p className="page-sub">
        模型与运行时安装到工作目录，可整目录迁移；校验值为空表示尚未实测
      </p>

      <h2 className="section-title">已安装</h2>
      <table className="grid">
        <thead>
          <tr>
            <th>名称</th>
            <th>类型</th>
            <th className="num">体积</th>
            <th className="num">显存</th>
            <th>许可</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          {installed.map((m) => (
            <tr key={m.name}>
              <td>{m.name}</td>
              <td className="mono">{m.kind}</td>
              <td className="num">{m.size}</td>
              <td className="num">{m.vram}</td>
              <td className="mono">{m.license}</td>
              <td>
                <span className="dot dot--done" aria-hidden /> {m.state}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <h2 className="section-title">可安装</h2>
      <table className="grid">
        <thead>
          <tr>
            <th>名称</th>
            <th>类型</th>
            <th className="num">体积</th>
            <th className="num">显存</th>
            <th>许可</th>
            <th>档位</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {available.map((m) => (
            <tr key={m.name}>
              <td>{m.name}</td>
              <td className="mono">{m.kind}</td>
              <td className="num">{m.size}</td>
              <td className="num">{m.vram}</td>
              <td className="mono">{m.license}</td>
              <td>{m.tier ? <span className="tag">{m.tier}</span> : <span className="muted">—</span>}</td>
              <td style={{ textAlign: "right" }}>
                <button className="btn" type="button">
                  安装
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <p className="mono muted" style={{ marginTop: "var(--sp-6)" }}>
        权重不随安装包分发，仅记录官方来源与校验值。安装前请确认对应许可。
      </p>
    </>
  );
}
