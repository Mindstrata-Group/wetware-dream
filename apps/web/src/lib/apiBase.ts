// Where the web app sends API requests.
//
// NEXT_PUBLIC_API_BASE_URL is inlined at build time:
//   - unset         -> local development default (http://localhost:18080);
//   - empty string  -> same origin: the browser calls /api/... on the site
//                      itself and a reverse proxy routes it to the API. This
//                      is what makes one prebuilt image work for any domain;
//   - a URL         -> that URL.
//
// Server-side code (route handlers, server components) prefers API_BASE_URL,
// the API address inside the container network, and must always get an
// absolute URL.

export const LOCAL_API_BASE_URL = "http://localhost:18080";

export function resolvePublicApiBase(raw: string | undefined): string {
  return raw ?? LOCAL_API_BASE_URL;
}

export function resolveServerApiBase(server: string | undefined, publicRaw: string | undefined): string {
  return server || publicRaw || LOCAL_API_BASE_URL;
}

export const publicApiBase = resolvePublicApiBase(process.env.NEXT_PUBLIC_API_BASE_URL);

export function serverApiBase(): string {
  return resolveServerApiBase(process.env.API_BASE_URL, process.env.NEXT_PUBLIC_API_BASE_URL);
}
