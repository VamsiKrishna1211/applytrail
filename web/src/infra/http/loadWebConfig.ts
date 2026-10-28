import type { components } from '@/api/schema'

export interface WebConfig {
  /** Set when this server signs people in with Clerk. */
  clerkPublishableKey: string | undefined
}

const KEY_ONLY: WebConfig = { clerkPublishableKey: undefined }

/**
 * Asks the server how it signs people in, before anything renders.
 *
 * Read at runtime rather than baked into the bundle, so one image serves any
 * deployment. A server that cannot answer — an older build without the
 * endpoint, or a network failure — falls back to API keys, which every build
 * accepts, rather than leaving the page blank.
 */
export async function loadWebConfig(
  baseUrl = '',
  fetchImpl: typeof globalThis.fetch = globalThis.fetch.bind(globalThis),
): Promise<WebConfig> {
  try {
    const response = await fetchImpl(`${baseUrl.replace(/\/$/, '')}/auth/config`, {
      headers: { Accept: 'application/json' },
    })
    if (!response.ok) return KEY_ONLY
    const body = (await response.json()) as components['schemas']['WebConfig']
    return { clerkPublishableKey: body.clerk_publishable_key || undefined }
  } catch {
    return KEY_ONLY
  }
}
