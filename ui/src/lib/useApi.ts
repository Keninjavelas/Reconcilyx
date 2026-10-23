import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/store/app";

interface ApiState<T> {
  data?: T;
  error?: string;
  loading: boolean;
  fetchedAt?: string;
}

export function useApi<T, A extends unknown[]>(
  loader: (...args: A) => Promise<T>,
  ...args: A
): ApiState<T> & { refetch: () => void } {
  const argsKey = JSON.stringify(args);
  const loaderRef = useRef(loader);
  const argsRef = useRef(args);
  loaderRef.current = loader;
  argsRef.current = args;

  const refreshEpoch = useAppStore((s) => s.refreshEpoch);
  const [localEpoch, setLocalEpoch] = useState(0);
  const [state, setState] = useState<ApiState<T>>({ loading: true });

  useEffect(() => {
    let cancelled = false;
    setState((prev) => ({ ...prev, loading: true, error: undefined }));

    loaderRef
      .current(...argsRef.current)
      .then((data) => {
        if (cancelled) return;
        setState({ data, loading: false, fetchedAt: new Date().toISOString() });
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setState((prev) => ({
          data: prev.data,
          loading: false,
          error: error instanceof Error ? error.message : String(error),
          fetchedAt: new Date().toISOString(),
        }));
      });

    return () => {
      cancelled = true;
    };
  }, [argsKey, refreshEpoch, localEpoch]);

  const refetch = useCallback(() => setLocalEpoch((n) => n + 1), []);

  return { ...state, refetch };
}
