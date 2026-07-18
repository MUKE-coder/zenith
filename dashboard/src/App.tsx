import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'

import styles from './components/AppShell.module.css'
import {
  IconEvents,
  IconGlobe,
  IconOverview,
  IconPlus,
  IconSeo,
  IconSettings,
  IconSetup,
  IconSignOut,
  ZenithMark,
} from './components/icons'
import { Login } from './components/Login'
import { NewSiteDialog } from './components/NewSiteDialog'
import { EmptyState, ErrorState, Skeleton } from './components/Panel'
import { RangePicker } from './components/RangePicker'
import { Realtime } from './components/Realtime'
import { SiteSwitcher } from './components/SiteSwitcher'
import { useAsync } from './hooks/useAsync'
import { api } from './lib/api'
import { useAuth } from './lib/auth'
import { embedConfig } from './lib/embed'
import { DEFAULT_RANGE } from './lib/range'
import type { RangeKey } from './lib/range'
import type { Site } from './lib/types'
import { Dashboard } from './views/Dashboard'
import { Events } from './views/Events'
import { Audits } from './views/Audits'
import { Settings } from './views/Settings'
import { Setup } from './views/Setup'

type Tab = 'overview' | 'events' | 'audits' | 'setup' | 'settings'

/** Remembers the site across reloads, so a refresh does not lose your place. */
const SITE_KEY = 'zenith.site'

type NavItem = { tab: Tab; label: string; icon: ReactNode; developerOnly?: boolean }

const NAV: NavItem[] = [
  { tab: 'overview', label: 'Overview', icon: <IconOverview /> },
  { tab: 'events', label: 'Events', icon: <IconEvents /> },
  { tab: 'audits', label: 'SEO', icon: <IconSeo /> },
  // Setup shows the secret api key, so it is developer-only.
  { tab: 'setup', label: 'Setup', icon: <IconSetup />, developerOnly: true },
  // Settings is the deployment's, and it lists every client. An owner must
  // never see it.
  { tab: 'settings', label: 'Settings', icon: <IconSettings />, developerOnly: true },
]

export default function App() {
  const { session, checking, signOut, expire } = useAuth()

  if (checking) {
    return (
      <div style={{ display: 'grid', placeItems: 'center', minHeight: '100%' }}>
        <Skeleton width="220px" height={16} />
      </div>
    )
  }

  if (!session) return <Login />

  return <Console onSignOut={signOut} onUnauthorized={expire} isOwner={session.role === 'owner'} ownSiteId={session.site_id} />
}

type ConsoleProps = {
  isOwner: boolean
  ownSiteId: string | undefined
  onSignOut: () => Promise<void>
  onUnauthorized: () => void
}

function Console({ isOwner, ownSiteId, onSignOut, onUnauthorized }: ConsoleProps) {
  const embed = embedConfig()
  const [tab, setTab] = useState<Tab>('overview')
  const [range, setRange] = useState<RangeKey>(DEFAULT_RANGE)
  const [siteId, setSiteId] = useState<string | undefined>(
    () => ownSiteId ?? localStorage.getItem(SITE_KEY) ?? undefined,
  )
  const [addingSite, setAddingSite] = useState(false)
  // A site that has been created but is not in the fetched list yet.
  const [pendingSiteId, setPendingSiteId] = useState<string>()

  // An owner never lists sites: their credential names the only one they may
  // read, and the endpoint would refuse them anyway.
  const sites = useAsync((signal) => api.sites(signal), [], {
    enabled: !isOwner,
    onUnauthorized,
  })

  const list = useMemo(() => sites.data?.sites ?? [], [sites.data])

  // Fall back to the first site when nothing is chosen, or when the remembered
  // one has since been deleted.
  useEffect(() => {
    if (isOwner || list.length === 0) return

    // A just-created site is not in the list until the refetch lands. Falling
    // back to the first one in the meantime would steal the selection from
    // under the developer who just added it.
    if (pendingSiteId) {
      if (!list.some((s) => s.id === pendingSiteId)) return
      setPendingSiteId(undefined)
    }

    if (!siteId || !list.some((s) => s.id === siteId)) {
      setSiteId(list[0].id)
    }
  }, [isOwner, list, siteId, pendingSiteId])

  useEffect(() => {
    if (siteId && !isOwner) localStorage.setItem(SITE_KEY, siteId)
  }, [siteId, isOwner])

  const selected: Site | undefined = list.find((s) => s.id === siteId)

  // An owner's scope is settled the moment they are authenticated: their
  // credential names the site. A developer's is settled once they have picked
  // one from the list.
  const ready = isOwner || Boolean(siteId)

  const visibleNav = NAV.filter((item) => !item.developerOnly || !isOwner)

  function onSiteCreated(site: Site) {
    setAddingSite(false)
    setPendingSiteId(site.id)
    setSiteId(site.id)
    sites.reload()
    // Land on Setup: a site with no snippet installed collects nothing, and
    // that is the very next thing to do.
    setTab('setup')
  }

  return (
    <div className={styles.shell}>
      <aside className={styles.sidebar}>
        {/* Embedded, this is a page of the owner's site and it says so. The
            console is Zenith's, and says that. */}
        <div className={styles.brand}>
          {embed ? (
            <span className={styles.mark} aria-hidden="true" />
          ) : (
            <ZenithMark size={24} />
          )}
          <span className={styles.brandName}>{embed?.siteDomain ?? 'Zenith'}</span>
        </div>

        <nav className={styles.nav} aria-label="Sections">
          {visibleNav.map((item) => (
            <button
              key={item.tab}
              type="button"
              className={`${styles.navItem} ${tab === item.tab ? styles.navItemActive : ''}`}
              aria-current={tab === item.tab ? 'page' : undefined}
              onClick={() => setTab(item.tab)}
            >
              <span className={styles.navIcon}>{item.icon}</span>
              {item.label}
            </button>
          ))}
        </nav>

        <div className={styles.sidebarFoot}>
          <button type="button" className={styles.signOut} onClick={() => void onSignOut()}>
            <span className={styles.navIcon}>
              <IconSignOut />
            </span>
            Sign out
          </button>
        </div>
      </aside>

      <div className={styles.main}>
        {/* Settings is global and timeless: a site switcher, a live count, and
            a date range would all be answering questions it does not ask. */}
        {tab !== 'settings' && (
          <header className={styles.topbar}>
            <div className={styles.topbarLeft}>
              {isOwner ? (
                <strong>{selected?.name ?? embed?.siteDomain ?? 'Analytics'}</strong>
              ) : (
                <>
                  <SiteSwitcher
                    sites={list}
                    selected={selected}
                    onSelect={(site) => setSiteId(site.id)}
                  />
                  {list.length > 0 && (
                    <button
                      type="button"
                      className={`button-ghost ${styles.addSite}`}
                      onClick={() => setAddingSite(true)}
                    >
                      <IconPlus />
                      New site
                    </button>
                  )}
                </>
              )}
            </div>

            <div className={styles.topbarRight}>
              <Realtime siteId={siteId} ready={ready} onUnauthorized={onUnauthorized} />
              <RangePicker value={range} onChange={setRange} />
            </div>
          </header>
        )}

        <main className={styles.content}>
          <div className={styles.pageTitle}>
            <h1>{pageTitle(tab)}</h1>
            {/* Settings is global: naming a site under it would be wrong. */}
            {tab !== 'settings' && (selected?.domain ?? embed?.siteDomain) && (
              <p className={styles.pageSubtitle}>{selected?.domain ?? embed?.siteDomain}</p>
            )}
          </div>

          <Body
            isOwner={isOwner}
            sitesLoading={sites.loading}
            sitesError={sites.error}
            onRetrySites={sites.reload}
            hasSites={isOwner || list.length > 0}
            site={selected}
            siteId={siteId}
            ready={ready}
            tab={tab}
            range={range}
            onUnauthorized={onUnauthorized}
            onAddSite={() => setAddingSite(true)}
            onSiteDeleted={() => {
              setSiteId(undefined)
              setTab('overview')
              sites.reload()
            }}
          />
        </main>
      </div>

      {addingSite && (
        <NewSiteDialog onClose={() => setAddingSite(false)} onCreated={onSiteCreated} />
      )}
    </div>
  )
}

function pageTitle(tab: Tab): string {
  switch (tab) {
    case 'events':
      return 'Events'
    case 'audits':
      return 'SEO audit'
    case 'setup':
      return 'Setup'
    case 'settings':
      return 'Settings'
    default:
      return 'Overview'
  }
}

type BodyProps = {
  isOwner: boolean
  sitesLoading: boolean
  sitesError: string | undefined
  onRetrySites: () => void
  hasSites: boolean
  site: Site | undefined
  siteId: string | undefined
  ready: boolean
  tab: Tab
  range: RangeKey
  onUnauthorized: () => void
  onAddSite: () => void
  onSiteDeleted: () => void
}

function Body({
  isOwner,
  sitesLoading,
  sitesError,
  onRetrySites,
  hasSites,
  site,
  siteId,
  ready,
  tab,
  range,
  onUnauthorized,
  onAddSite,
  onSiteDeleted,
}: BodyProps) {
  // Settings needs no site: it is the deployment's, and it loads its own data.
  if (tab === 'settings') {
    return <Settings onUnauthorized={onUnauthorized} />
  }

  if (!isOwner && sitesError) {
    return <ErrorState message={sitesError} onRetry={onRetrySites} />
  }

  if (!isOwner && sitesLoading) {
    return <Skeleton width="100%" height={120} />
  }

  // The first thing a new deployment shows, so it has to say what to do next
  // and hand over the control that does it.
  if (!hasSites) {
    return (
      <EmptyState
        icon={<IconGlobe />}
        title="Add your first site"
        hint="Zenith measures one site per set of keys. Add one and you'll get a snippet to drop into its pages — pageviews start arriving the moment it's live."
        action={
          <button type="button" className="button-primary" onClick={onAddSite}>
            <IconPlus />
            Add site
          </button>
        }
      />
    )
  }

  if (tab === 'setup') {
    return <Setup site={site} onDeleted={onSiteDeleted} />
  }
  if (tab === 'audits') {
    return <Audits siteId={siteId} ready={ready} onUnauthorized={onUnauthorized} />
  }
  if (tab === 'events') {
    return <Events siteId={siteId} ready={ready} range={range} onUnauthorized={onUnauthorized} />
  }
  return <Dashboard siteId={siteId} ready={ready} range={range} onUnauthorized={onUnauthorized} />
}
