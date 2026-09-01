import { Analytics } from "@vercel/analytics/next"
import { SpeedInsights } from "@vercel/speed-insights/next"
import type { Metadata } from "next"
import { Geist_Mono } from "next/font/google"

import "./globals.css"
import { ThemeProvider } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

const fontMono = Geist_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
})

const SITE = "https://dug.sh"

const DESCRIPTION =
  "A command driven terminal for domain and network diagnostics. Every answer is a live query, labelled with how old it is."

export const metadata: Metadata = {
  // Without this every relative URL below stays relative, and a crawler reading
  // og:image off a scraped copy of the page has nothing to resolve it against.
  metadataBase: new URL(SITE),

  // The bare name on the landing, "<page> · dug" everywhere else. A child page
  // setting title to a plain string opts into the template automatically.
  title: { default: "dug", template: "%s · dug" },
  description: DESCRIPTION,
  applicationName: "dug",
  authors: [{ name: "Zaid Mukaddam" }],
  creator: "Zaid Mukaddam",

  // Terms someone would actually search, not the whole command list. The
  // command names are already in the page text and in llms.txt.
  keywords: [
    "dns lookup",
    "dig online",
    "dns propagation checker",
    "tls certificate checker",
    "whois rdap lookup",
    "spf dkim dmarc checker",
    "asn lookup",
    "reverse dns",
    "network diagnostics",
  ],

  alternates: { canonical: "/" },

  openGraph: {
    type: "website",
    url: SITE,
    siteName: "dug",
    title: "dug",
    description: DESCRIPTION,
    locale: "en_US",
  },

  // No images key on either card: app/opengraph-image.tsx is picked up by file
  // convention and Next fills in both og:image and twitter:image, at the right
  // absolute URL and with the dimensions already measured. Naming them here as
  // well would only be a second copy to keep in step.
  twitter: {
    card: "summary_large_image",
    title: "dug",
    description: DESCRIPTION,
  },

  robots: {
    index: true,
    follow: true,
    googleBot: { index: true, follow: true, "max-image-preview": "large" },
  },

  // The command routes answer in text or json rather than html, so a browser
  // pointed at one is reading output, not a page. Say so.
  formatDetection: { telephone: false, address: false, email: false },
}

// Two nodes in one graph: what the thing is, and who publishes it.
//
// SoftwareApplication rather than Organization as the primary, because this is
// a tool and not a company. Every field here is checkable from the deployment
// itself. Nothing that would need a fact this project does not have is
// present: no postal address, no telephone, no contactPoint, because inventing
// any of them to satisfy a schema is how structured data stops being worth
// trusting. See the note in the README about what completing them would take.
const STRUCTURED_DATA = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "SoftwareApplication",
      "@id": `${SITE}/#app`,
      name: "dug",
      url: SITE,
      description: DESCRIPTION,
      applicationCategory: "DeveloperApplication",
      applicationSubCategory: "Network diagnostics",
      operatingSystem: "Any",
      browserRequirements: "Works without JavaScript over curl; the browser app needs a modern browser.",
      isAccessibleForFree: true,
      // Free and keyless, said in the vocabulary a parser reads rather than
      // only in the prose.
      offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
      featureList: [
        "DNS record lookup across a fixed resolver list",
        "DNS propagation comparison",
        "TLS certificate chain, validity and protocol inspection",
        "RDAP registration lookup with status codes decoded",
        "SPF, DKIM, DMARC and alignment policy checks",
        "ASN, prefix and reverse DNS lookup",
        "HTTP header, redirect chain and timing inspection",
      ],
      author: { "@id": `${SITE}/#author` },
      publisher: { "@id": `${SITE}/#author` },
      potentialAction: {
        "@type": "SearchAction",
        target: {
          "@type": "EntryPoint",
          urlTemplate: `${SITE}/dig/{domain}`,
        },
        "query-input": "required name=domain",
      },
    },
    {
      "@type": "Person",
      "@id": `${SITE}/#author`,
      name: "Zaid Mukaddam",
      url: SITE,
    },
    {
      "@type": "WebSite",
      "@id": `${SITE}/#website`,
      url: SITE,
      name: "dug",
      description: DESCRIPTION,
      publisher: { "@id": `${SITE}/#author` },
      inLanguage: "en",
    },
  ],
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={cn("antialiased", fontMono.variable)}
    >
      <body>
        {/* In the server html rather than injected, so a client that never runs
            javascript still reads it. */}
        <script
          type="application/ld+json"
          // biome-ignore lint/security/noDangerouslySetInnerHtml: the only way to emit a ld+json body, and the content is a local constant rather than anything a request can reach
          dangerouslySetInnerHTML={{ __html: JSON.stringify(STRUCTURED_DATA) }}
        />

        <ThemeProvider>{children}</ThemeProvider>
        <Analytics />
        <SpeedInsights />
      </body>
    </html>
  )
}
