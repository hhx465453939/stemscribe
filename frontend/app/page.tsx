import { NewTask } from "./components/NewTask";
import { TaskBoard } from "./components/TaskBoard";

export default function HomePage() {
  return (
    <>
      <h1 className="page-title">新建任务</h1>
      <p className="page-sub">
        本地运行 · 音视频不出本机 · 仅润色会外发文本
      </p>

      <NewTask />

      <h2 className="section-title">任务</h2>
      <TaskBoard />
    </>
  );
}
