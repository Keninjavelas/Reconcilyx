import { useEffect, useState } from "react";

export function useApi<T, A extends unknown[]>(
  loader: (...args: A) => Promise<T>,
  ...args: A
): { data: T | undefined; error?: string; loading: boolean; refetch: () => void } {
  const [data, setData] = useState<T | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const [epoch, setEpoch] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(undefined);
    loader(...args)
      .then((d) => {
        if (!cancelled) {
          setData(d);
          setLoading(false);
        }
      })
      .catch((e: unknown) => {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : String(e));
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [epoch, ...args]);

  return {
    data,
    error,
    loading,
    refetch: () => setEpoch((n) => n + 1),
  };
}
