"use client";

import { api, ApiError } from "@/lib/api-client";
import {
  clearStoredUser,
  clearTenantId,
  clearTokens,
  getRefreshToken,
  getStoredUser,
  getTenantId,
  setStoredUser,
  setTenantId,
  setTokens,
} from "@/lib/auth-storage";
import type { Permission, User } from "@/types/api";
import { useRouter } from "next/navigation";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

type AuthContextValue = {
  user: User | null;
  permissions: Permission[];
  isLoading: boolean;
  isAuthenticated: boolean;
  selectedTenantId: string | null;
  login: (email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshMe: () => Promise<User | null>;
  hasPermission: (permission?: string | string[] | null) => boolean;
  setSelectedTenantId: (tenantId: string | null) => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<User | null>(() => getStoredUser<User>());
  const [isLoading, setIsLoading] = useState(true);
  const [selectedTenantId, updateSelectedTenantId] = useState<string | null>(
    () => getTenantId(),
  );

  const refreshMe = useCallback(async () => {
    try {
      const me = await api.me();
      setUser(me);
      setStoredUser(me);
      return me;
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setUser(null);
        clearStoredUser();
        clearTokens();
      }
      return null;
    }
  }, []);

  useEffect(() => {
    let mounted = true;

    const boot = async () => {
      if (!getRefreshToken() && !getStoredUser<User>()) {
        if (mounted) setIsLoading(false);
        return;
      }

      await refreshMe();

      if (mounted) setIsLoading(false);
    };

    void boot();

    return () => {
      mounted = false;
    };
  }, [refreshMe]);

  const login = useCallback(async (email: string, password: string) => {
    const pair = await api.login(email, password);
    setTokens(pair.access_token, pair.refresh_token);
    const me = await api.me();
    setUser(me);
    setStoredUser(me);
    router.push("/dashboard");
    router.refresh();
  }, [router]);

  const logout = useCallback(async () => {
    try {
      await api.logout(getRefreshToken());
    } finally {
      clearTokens();
      clearStoredUser();
      clearTenantId();
      setUser(null);
      updateSelectedTenantId(null);
      router.push("/login");
      router.refresh();
    }
  }, [router]);

  const setSelectedTenant = useCallback((tenantId: string | null) => {
    updateSelectedTenantId(tenantId);
    if (tenantId) {
      setTenantId(tenantId);
    } else {
      clearTenantId();
    }
  }, []);

  const permissions = useMemo(() => user?.permissions ?? [], [user]);

  const hasPermission = useCallback((permission?: string | string[] | null) => {
    if (!permission) return true;
    if (Array.isArray(permission)) {
      return permission.some((entry) => permissions.includes(entry));
    }
    return permissions.includes(permission);
  }, [permissions]);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      permissions,
      isLoading,
      isAuthenticated: Boolean(user),
      selectedTenantId,
      login,
      logout,
      refreshMe,
      hasPermission,
      setSelectedTenantId: setSelectedTenant,
    }),
    [
      user,
      permissions,
      isLoading,
      selectedTenantId,
      login,
      logout,
      refreshMe,
      hasPermission,
      setSelectedTenant,
    ],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth must be used inside AuthProvider");
  }

  return context;
}
