import { useEffect, useState } from "react";
import { getPublicInfo } from "./api";
import type { PublicInfo } from "./model";

export function usePublicInfo() {
  const [info, setInfo] = useState<PublicInfo | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const controller = new AbortController();

    getPublicInfo(controller.signal)
      .then(setInfo)
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }

        // Статический контент остаётся доступным при сбое живой статистики.
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => controller.abort();
  }, []);

  return { info, loading };
}
