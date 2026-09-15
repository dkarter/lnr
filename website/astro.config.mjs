import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";
import { existsSync, readFileSync } from "node:fs";

const manifest = JSON.parse(
  readFileSync(new URL("../.release-please-manifest.json", import.meta.url), "utf8"),
);
const docsVersion = process.env.LNR_DOCS_VERSION || "dev";
const stableVersion = process.env.LNR_STABLE_VERSION || manifest["."];
const base = process.env.LNR_SITE_BASE || (docsVersion === "dev" ? "/dev" : "/");
const site = "https://lnr.doriankarter.com";

process.env.PUBLIC_LNR_DOCS_VERSION = docsVersion;
process.env.PUBLIC_LNR_STABLE_VERSION = stableVersion;
process.env.PUBLIC_LNR_STABLE_ROUTES = process.env.LNR_STABLE_ROUTES || "";
process.env.PUBLIC_LNR_HAS_STABLE_DOCS = process.env.LNR_HAS_STABLE_DOCS || "true";

const docsRoot = new URL("./src/content/docs/", import.meta.url);
const doc = (label, slug, file = `${slug.replace(/^docs\//, "")}.md`) =>
  existsSync(new URL(file, docsRoot)) ? { label, slug } : null;
const available = (items) => items.filter(Boolean);

export default defineConfig({
  site,
  base,
  prefetch: false,
  integrations: [
    starlight({
      title: "lnr",
      description: "Linear issues at terminal speed.",
      favicon: "/favicon.svg",
      head: [
        { tag: "meta", attrs: { property: "og:image", content: `${site}/og-image.svg` } },
        { tag: "meta", attrs: { name: "twitter:card", content: "summary_large_image" } },
        { tag: "meta", attrs: { name: "twitter:image", content: `${site}/og-image.svg` } },
      ],
      social: [{ icon: "github", label: "GitHub", href: "https://github.com/dkarter/lnr" }],
      customCss: ["./src/styles/starlight.css"],
      components: {
        SiteTitle: "./src/components/SiteTitle.astro",
        LanguageSelect: "./src/components/VersionSelect.astro",
      },
      editLink: {
        baseUrl: `https://github.com/dkarter/lnr/edit/${
          docsVersion === "dev" ? "main" : `v${stableVersion}`
        }/website/`,
      },
      lastUpdated: true,
      disable404Route: true,
      sidebar: [
        {
          label: "Start",
          items: available([
            doc("Overview", "docs", "index.mdx"),
            doc("Install", "docs/install"),
            doc("Quick start", "docs/quick-start"),
            doc("Quick command", "docs/quick"),
            doc("Authentication", "docs/authentication"),
          ]),
        },
        {
          label: "Workflows",
          items: available([
            doc("Create issues", "docs/create-issues"),
            doc("Find issues", "docs/find-issues"),
            doc("Update and delete", "docs/manage-issues"),
            doc("Configuration", "docs/configuration"),
            doc("Terminal integrations", "docs/integrations"),
          ]),
        },
        {
          label: "Reference",
          items: available([
            doc("CLI reference", "docs/cli-reference"),
            doc("Automation and JSON", "docs/automation"),
            doc("Agent skill", "docs/agent-skill"),
          ]),
        },
      ],
    }),
  ],
});
