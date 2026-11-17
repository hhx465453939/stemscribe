"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const items = [
  { href: "/", label: "新建任务" },
  { href: "/library", label: "资料库" },
  { href: "/models", label: "模型中心" },
  { href: "/settings", label: "设置" },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="sidebar">
      <div className="brand">STEMSCRIBE</div>

      <nav className="nav">
        {items.map((it) => (
          <Link key={it.href} href={it.href} data-active={pathname === it.href}>
            {it.label}
          </Link>
        ))}
      </nav>

      {/* The resource bar is always visible: knowing what the machine is doing is
          the core promise of the product (PRD §4.3). */}
      <div className="resource-bar">
        <span>GPU 8.1 / 12 GB</span>
        <span>cuda12 · ready</span>
      </div>
    </aside>
  );
}
