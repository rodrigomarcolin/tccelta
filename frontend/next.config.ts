import type { NextConfig } from "next";

const config: NextConfig = {
  basePath: "/dashboard",
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
  // The host root is outside the basePath, so it needs basePath: false.
  async redirects() {
    return [{ source: "/", destination: "/dashboard", basePath: false, permanent: false }];
  },
};

export default config;
