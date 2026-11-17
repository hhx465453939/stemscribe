/** @type {import('next').NextConfig} */
const nextConfig = {
  // The desktop shell loads the UI from disk via WebView2, so the build must be a
  // plain static bundle. No SSR, no image optimizer, no server runtime.
  output: "export",
  images: { unoptimized: true },
  // Relative asset paths: the app is served from a file:// style origin inside
  // WebView2, where a leading "/" resolves to the drive root.
  assetPrefix: "",
  trailingSlash: true,
  reactStrictMode: true,
};

export default nextConfig;
