export const GATEWAY_URL = "http://localhost:8080";

// adminFetch calls the admin API with the given token attached. Throws
// with the response body's text on any non-2xx status, so callers can
// show the gateway's actual error message instead of guessing one.
export async function adminFetch(token, path, options = {}) {
  const res = await fetch(`${GATEWAY_URL}${path}`, {
    ...options,
    headers: {
      "X-Admin-Token": token,
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...options.headers,
    },
  });

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(text.trim() || `request failed: ${res.status}`);
  }

  if (res.status === 204) return null;
  return res.json();
}
