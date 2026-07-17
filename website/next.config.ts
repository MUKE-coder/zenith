import type { NextConfig } from "next";
import createMDX from "@next/mdx";

const nextConfig: NextConfig = {
  pageExtensions: ["ts", "tsx", "md", "mdx"],
  // Emit a minimal, self-contained server at .next/standalone for Docker —
  // deployable without node_modules. See node_modules/next/dist/docs output.md.
  output: "standalone",
};

const withMDX = createMDX({
  options: {
    remarkPlugins: [],
    // Turbopack requires plugins by string name — function instances can't be
    // passed to the Rust worker. Our options are plain serializable objects.
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
