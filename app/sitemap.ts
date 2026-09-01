import type { MetadataRoute } from "next"

// Only the two routes that are pages. Every command route is a live lookup
// whose answer is true for as long as its ttl and different for every target,
// so there is nothing stable there to index.
export default function sitemap(): MetadataRoute.Sitemap {
  return [
    { url: "https://dug.sh", changeFrequency: "weekly", priority: 1 },
    { url: "https://dug.sh/developers", changeFrequency: "monthly", priority: 0.9 },
    { url: "https://dug.sh/about", changeFrequency: "monthly", priority: 0.8 },
    { url: "https://dug.sh/deprecation", changeFrequency: "yearly", priority: 0.6 },
    { url: "https://dug.sh/contact", changeFrequency: "yearly", priority: 0.5 },
    { url: "https://dug.sh/privacy", changeFrequency: "yearly", priority: 0.3 },
  ]
}
