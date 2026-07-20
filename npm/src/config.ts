/**
 * The shape of `config/zenith.ts` in the owner's project.
 *
 * This file is read server-side only. Two of its fields are secrets that must
 * never reach a browser: `apiKey` and `jwtSecret`.
 */
export interface ZenithConfig {
  /** URL of the central Zenith service that receives events and serves stats. */
  backendUrl: string

  /**
   * Public site key. It ships inside the tracking snippet, so treat it as
   * readable by anyone. It authorizes writing events to this site and nothing
   * else.
   */
  siteKey: string

  /**
   * Secret api key. It authorizes reading this site's analytics, and the proxy
   * sends it server-side only. It must never be rendered into a page.
   */
  apiKey: string

  /** Where the domain-native dashboard mounts on the owner's site. */
  dashboardPath: string

  /** Password-gate the owner view. */
  protected: boolean

  /** bcrypt hash of the dashboard password. Never a plaintext password. */
  passwordHash?: string

  /** Signs the dashboard's session cookie. */
  jwtSecret?: string

  /** The owner's domain, e.g. "example.com". */
  siteDomain: string

  /** How long a dashboard session lasts, in seconds. */
  sessionTtl?: number
}

/** Applied when `config/zenith.ts` leaves a field out. */
export const defaultConfig = {
  dashboardPath: '/analytics-dashboard',
  protected: true,
  sessionTtl: 60 * 60 * 12, // 12 hours
} satisfies Partial<ZenithConfig>

/** The minimum length of a signing secret, in characters. */
export const MIN_SECRET_LENGTH = 32

export class ConfigError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'ConfigError'
  }
}

/**
 * Validates a config and fills in defaults.
 *
 * Every failure here is a deployment mistake that would otherwise surface as a
 * confusing 500 at request time, or -- worse -- as an unprotected dashboard.
 * They are caught at startup, and each message says how to fix it.
 */
export function resolveConfig(input: Partial<ZenithConfig>): ZenithConfig {
  const config = { ...defaultConfig, ...input }

  if (!config.backendUrl) {
    throw new ConfigError(
      'Zenith config: set backendUrl to your Zenith service, e.g. https://zenith.example.com',
    )
  }
  if (!isHttpUrl(config.backendUrl)) {
    throw new ConfigError(`Zenith config: backendUrl must be an http(s) URL, got "${config.backendUrl}"`)
  }
  if (!config.siteKey) {
    throw new ConfigError('Zenith config: set siteKey to your site\'s public key (zk_...)')
  }
  if (!config.apiKey) {
    throw new ConfigError('Zenith config: set apiKey to your site\'s secret key (zk_...)')
  }

  // A swap here would put the secret key in every visitor's page source and
  // leave the dashboard unable to read anything. Both are zk_-prefixed, so
  // they are easy to transpose and impossible to tell apart by eye.
  if (config.siteKey === config.apiKey) {
    throw new ConfigError(
      'Zenith config: siteKey and apiKey are the same value. The site key is public ' +
        '(it ships in the tracking snippet); the api key is secret. They must differ.',
    )
  }

  if (!config.siteDomain) {
    throw new ConfigError('Zenith config: set siteDomain to your domain, e.g. example.com')
  }

  if (!config.dashboardPath.startsWith('/')) {
    throw new ConfigError(
      `Zenith config: dashboardPath must start with "/", got "${config.dashboardPath}"`,
    )
  }

  // A protected dashboard with no password is an unprotected dashboard. Fail
  // rather than silently serve a client's analytics to the internet.
  if (config.protected) {
    if (!config.passwordHash) {
      throw new ConfigError(
        'Zenith config: protected is true but passwordHash is not set. ' +
          'Generate one with `npx zenith hash` , or set protected: false to publish the dashboard.',
      )
    }
    if (!looksLikeBcrypt(config.passwordHash)) {
      throw new ConfigError(
        'Zenith config: passwordHash is not a bcrypt hash. It must never be a plaintext ' +
          'password. Generate one with `npx zenith hash`.',
      )
    }
    if (!config.jwtSecret) {
      throw new ConfigError(
        'Zenith config: protected is true but jwtSecret is not set. ' +
          'Generate one with `openssl rand -base64 32`.',
      )
    }
    if (config.jwtSecret.length < MIN_SECRET_LENGTH) {
      throw new ConfigError(
        `Zenith config: jwtSecret is ${config.jwtSecret.length} characters; it must be at ` +
          `least ${MIN_SECRET_LENGTH}. Generate one with \`openssl rand -base64 32\`.`,
      )
    }
  }

  if (config.sessionTtl <= 0) {
    throw new ConfigError(`Zenith config: sessionTtl must be positive, got ${config.sessionTtl}`)
  }

  // Spelled out field by field rather than spread: the checks above prove
  // these are present, but only an explicit object convinces the compiler --
  // and it means a field added to ZenithConfig cannot silently arrive here
  // unvalidated.
  return {
    // Trailing slashes would produce "https://host//api/collect".
    backendUrl: config.backendUrl.replace(/\/+$/, ''),
    siteKey: config.siteKey,
    apiKey: config.apiKey,
    dashboardPath: config.dashboardPath.replace(/\/+$/, '') || '/',
    protected: config.protected,
    passwordHash: config.passwordHash,
    jwtSecret: config.jwtSecret,
    siteDomain: config.siteDomain,
    sessionTtl: config.sessionTtl,
  }
}

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

function looksLikeBcrypt(value: string): boolean {
  return /^\$2[aby]\$\d{2}\$/.test(value)
}
