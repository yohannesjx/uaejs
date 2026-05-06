const ACCESS_TOKEN_KEY = "dubai-admin-access-token";
const REFRESH_TOKEN_KEY = "dubai-admin-refresh-token";
const USER_KEY = "dubai-admin-user";
const TENANT_ID_KEY = "dubai-admin-tenant-id";

function inBrowser() {
  return typeof window !== "undefined";
}

export function getAccessToken() {
  if (!inBrowser()) return null;
  return window.localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getRefreshToken() {
  if (!inBrowser()) return null;
  return window.localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function setTokens(accessToken: string, refreshToken: string) {
  if (!inBrowser()) return;
  window.localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
  window.localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
  document.cookie = `access_token=${accessToken}; path=/; max-age=${60 * 60 * 24 * 7}; samesite=lax`;
}

export function clearTokens() {
  if (!inBrowser()) return;
  window.localStorage.removeItem(ACCESS_TOKEN_KEY);
  window.localStorage.removeItem(REFRESH_TOKEN_KEY);
  document.cookie = "access_token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
}

export function getStoredUser<T>() {
  if (!inBrowser()) return null as T | null;

  const raw = window.localStorage.getItem(USER_KEY);
  if (!raw) return null as T | null;

  try {
    return JSON.parse(raw) as T;
  } catch {
    return null as T | null;
  }
}

export function setStoredUser(value: unknown) {
  if (!inBrowser()) return;
  window.localStorage.setItem(USER_KEY, JSON.stringify(value));
}

export function clearStoredUser() {
  if (!inBrowser()) return;
  window.localStorage.removeItem(USER_KEY);
}

export function getTenantId() {
  if (!inBrowser()) return null;
  return window.localStorage.getItem(TENANT_ID_KEY);
}

export function setTenantId(tenantId: string) {
  if (!inBrowser()) return;
  window.localStorage.setItem(TENANT_ID_KEY, tenantId);
}

export function clearTenantId() {
  if (!inBrowser()) return;
  window.localStorage.removeItem(TENANT_ID_KEY);
}
