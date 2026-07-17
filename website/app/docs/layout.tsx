import { Nav } from "@/components/nav";
import { Footer } from "@/components/footer";
import { DocsSidebar } from "@/components/docs/sidebar";
import { Toc } from "@/components/docs/toc";
import { DocPager } from "@/components/docs/pager";
import { Container } from "@/components/ui";

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  return (
    <>
      <Nav />
      <Container className="grid gap-10 py-10 lg:grid-cols-[220px_minmax(0,1fr)] lg:gap-12 lg:py-14 xl:grid-cols-[220px_minmax(0,1fr)_200px]">
        {/* Sidebar — sticky on desktop, inline on mobile. */}
        <aside className="lg:sticky lg:top-24 lg:h-[calc(100vh-8rem)] lg:overflow-y-auto lg:pr-2">
          <DocsSidebar />
        </aside>

        {/* Content. */}
        <div className="min-w-0">
          <article className="prose">{children}</article>
          <DocPager />
        </div>

        {/* On this page — desktop only. */}
        <aside className="hidden xl:sticky xl:top-24 xl:block xl:h-[calc(100vh-8rem)]">
          <Toc />
        </aside>
      </Container>
      <Footer />
    </>
  );
}
