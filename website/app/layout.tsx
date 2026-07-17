import type { Metadata } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";

const geistSans = Geist({ variable: "--font-geist-sans", subsets: ["latin"] });
const geistMono = Geist_Mono({ variable: "--font-geist-mono", subsets: ["latin"] });

const title = "Zenith — Privacy-first analytics for every site you build";
const description =
  "Cookieless, multi-site web analytics and SEO auditing you self-host once and share with clients — each dashboard native to their own domain. No consent banner.";

export const metadata: Metadata = {
  metadataBase: new URL("https://zenith.dev"),
  title: { default: title, template: "%s — Zenith" },
  description,
  keywords: ["analytics", "privacy", "cookieless", "seo", "nextjs", "self-hosted"],
  openGraph: {
    title,
    description,
    type: "website",
    siteName: "Zenith",
    images: [{ url: "/og.png", width: 1200, height: 630, alt: "Zenith" }],
  },
  twitter: { card: "summary_large_image", title, description, images: ["/og.png"] },
  icons: { icon: "/favicon.svg" },
};

// Set the theme before first paint so there is no flash. Dark is the default.
const themeScript = `
try {
  var t = localStorage.getItem('zenith-theme');
  if (t === 'light') document.documentElement.setAttribute('data-theme', 'light');
} catch (e) {}
`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body className="min-h-full">{children}</body>
    </html>
  );
}
