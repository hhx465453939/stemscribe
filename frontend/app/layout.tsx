import type { Metadata } from "next";
import "./globals.css";
import { Providers } from "./components/Providers";
import { Sidebar } from "./components/Sidebar";

export const metadata: Metadata = {
  title: "Stemscribe",
  description: "Local-first video to transcript pipeline",
};

// Resolved before first paint so the correct theme is applied without a flash.
// Order of precedence: explicit URL override (used by screenshots and deep
// links) -> stored preference -> OS preference.
const bootScript = `
(function(){try{
  var q=new URLSearchParams(location.search);
  var t=q.get('theme')||localStorage.getItem('stemscribe.theme');
  if(t!=='light'&&t!=='dark'){t=window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light';}
  document.documentElement.dataset.theme=t;
  var l=q.get('lang')||localStorage.getItem('stemscribe.lang');
  if(l!=='zh'&&l!=='en'){l=((navigator.language||'zh').toLowerCase().indexOf('zh')===0)?'zh':'en';}
  document.documentElement.lang=(l==='zh')?'zh-CN':'en';
}catch(e){document.documentElement.dataset.theme='light';document.documentElement.lang='zh-CN';}})();
`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN" data-theme="light" suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: bootScript }} />
      </head>
      <body>
        <Providers>
          <div className="shell">
            <Sidebar />
            <main className="main">{children}</main>
          </div>
        </Providers>
      </body>
    </html>
  );
}
