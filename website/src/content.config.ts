import { docsLoader } from "@astrojs/starlight/loaders";
import { docsSchema } from "@astrojs/starlight/schema";
import { defineCollection } from "astro:content";

function docsPath({ entry }: { entry: string }) {
  const slug = entry.replace(/\.(md|mdx|markdown)$/i, "").replace(/\/index$/, "");
  return `docs${slug && slug !== "index" ? `/${slug}` : ""}`;
}

export const collections = {
  docs: defineCollection({ loader: docsLoader({ generateId: docsPath }), schema: docsSchema() }),
};
