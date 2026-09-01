import type { MetadataRoute } from "next"

// Two html pages and a large number of command routes that are not pages at
// all. A crawler that walks /tls/<anything> is spending someone else's dns and
// tls budget to index an answer that is true for an hour, so those paths are
// disallowed by prefix and the sitemap names only what is worth indexing.
//
// llms.txt and openapi.json stay crawlable on purpose: they are the two
// documents that describe the whole surface without querying it.
export default function robots(): MetadataRoute.Robots {
  return {
    rules: {
      userAgent: "*",
      allow: ["/", "/about", "/developers", "/contact", "/privacy", "/llms.txt", "/openapi.json"],
      disallow: [
        "/dig/",
        "/prop/",
        "/ttl/",
        "/ns/",
        "/dnssec/",
        "/rdap/",
        "/watch/",
        "/tls/",
        "/http/",
        "/trace/",
        "/mail/",
        "/spf/",
        "/ip/",
        "/asn/",
        "/net/",
        "/ping/",
        "/route/",
        "/ports/",
        "/vs/",
        "/api/",
      ],
    },
    sitemap: "https://dug.sh/sitemap.xml",
  }
}
