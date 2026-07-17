import type { NextConfig } from "next";
import createMDX from "@next/mdx";

const nextConfig: NextConfig = {
  pageExtensions: ["ts", "tsx", "md", "mdx"],
};

const withMDX = createMDX({
  options: {
    remarkPlugins: [],
    // Turbopack requires plugins by string name — function instances can't be
    // passed to the Rust worker (see node_modules/next/dist/docs mdx guide).
    // Our options here are plain serializable objects, so this is allowed.
    rehypePlugins: [
      "rehype-slug",
      [
        "@shikijs/rehype",
        {
          themes: { light: "github-light", dark: "github-dark-default" },
          defaultColor: "dark",
        },
      ],
    ],
  },
});

export default withMDX(nextConfig);
