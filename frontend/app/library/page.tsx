export default function LibraryPage() {
  const items = [
    { id: "BV1xx411c7mD", title: "示例：一场 42 分钟的访谈", segs: 240, dur: "42:10", state: "已完成" },
    { id: "BV1yy411c7mE", title: "示例：产品拆解", segs: 96, dur: "18:02", state: "已完成" },
  ];

  return (
    <>
      <h1 className="page-title">资料库</h1>
      <p className="page-sub">每集一个自包含目录，可直接备份或用 grep 检索</p>

      <table className="grid">
        <thead>
          <tr>
            <th>标题</th>
            <th>ID</th>
            <th className="num">段数</th>
            <th className="num">时长</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          {items.map((it) => (
            <tr key={it.id}>
              <td>{it.title}</td>
              <td className="mono">{it.id}</td>
              <td className="num">{it.segs}</td>
              <td className="num">{it.dur}</td>
              <td>
                <span className="dot dot--done" aria-hidden /> {it.state}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}
