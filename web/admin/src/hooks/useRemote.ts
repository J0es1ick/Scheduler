import { useCallback, useEffect, useRef, useState } from "react";

export function useRemote<T>(
  loader: () => Promise<T>,
  dependencies: unknown[] = [],
  options: { enabled?: boolean } = {},
) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(options.enabled !== false);
  const requestSequence = useRef(0);
  const enabled = options.enabled !== false;

  const reload = useCallback(async () => {
    if (!enabled) return;
    const request = ++requestSequence.current;
    setLoading(true);
    setError("");
    try {
      const result = await loader();
      if (request === requestSequence.current) setData(result);
    } catch (caught) {
      if (request === requestSequence.current) {
        setError(
          caught instanceof Error ? caught.message : "Неизвестная ошибка",
        );
      }
    } finally {
      if (request === requestSequence.current) setLoading(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...dependencies, enabled]);

  useEffect(() => {
    if (enabled) {
      void reload();
      return;
    }
    requestSequence.current += 1;
    setLoading(false);
  }, [enabled, reload]);

  return { data, error, loading, reload, setData };
}
