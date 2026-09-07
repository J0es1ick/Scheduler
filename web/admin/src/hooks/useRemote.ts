import { useCallback, useEffect, useRef, useState, useId } from "react";

export function useRemote<T>(
  loader: () => Promise<T>,
  dependencies: unknown[] = [],
  options: { enabled?: boolean } = {},
) {
  const remoteID = useId();
  const updatedAt = useRef<number | null>(null);
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
    window.dispatchEvent(
      new CustomEvent("scheduler:refresh-status", { detail: { id: remoteID } }),
    );
    try {
      const result = await loader();
      if (request === requestSequence.current) {
        setData(result);
        updatedAt.current = Date.now();
      }
    } catch (caught) {
      if (request === requestSequence.current) {
        if (updatedAt.current)
          window.dispatchEvent(
            new CustomEvent("scheduler:refresh-status", {
              detail: {
                id: remoteID,
                updatedAt: updatedAt.current,
                error:
                  caught instanceof Error
                    ? caught.message
                    : "Ошибка обновления",
              },
            }),
          );
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
    setData(null);
    updatedAt.current = null;
    if (enabled) void reload();
    else {
      requestSequence.current += 1;
      setLoading(false);
    }
    return () => {
      requestSequence.current += 1;
      window.dispatchEvent(
        new CustomEvent("scheduler:refresh-status", {
          detail: { id: remoteID },
        }),
      );
    };
  }, [enabled, reload, remoteID]);

  return { data, error, loading, reload, setData };
}
