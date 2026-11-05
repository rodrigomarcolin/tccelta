import type { NextConfig } from "next";

const config: NextConfig = {
  basePath: "/dashboard",
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
};

export default config;
