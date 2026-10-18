import { useEffect, useRef } from "react";

export function useReveal() {
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const elements = Array.from(
      root.current?.querySelectorAll<HTMLElement>("[data-reveal]") ?? [],
    );
    const preference = window.matchMedia("(prefers-reduced-motion: reduce)");
    const animations = new Set<Animation>();
    const observer: IntersectionObserver | null =
      !preference.matches && "IntersectionObserver" in window
        ? new IntersectionObserver((entries) => {
          entries.forEach((entry) => {
            if (!entry.isIntersecting) return;
            const element = entry.target as HTMLElement;
            observer?.unobserve(element);
            element.dataset.revealed = "true";
            if (
              preference.matches ||
              !element.animate ||
              element.contains(document.activeElement)
            ) return;
            const animation = element.animate(
              [
                { opacity: 0, transform: "translateY(16px)" },
                { opacity: 1, transform: "none" },
              ],
              {
                duration: 560,
                delay: Math.min(Number(element.dataset.revealDelay) || 0, 150),
                easing: "cubic-bezier(0.22, 1, 0.36, 1)",
                fill: "backwards",
              },
            );
            animations.add(animation);
            animation.finished.then(
              () => animations.delete(animation),
              () => animations.delete(animation),
            );
          });
        }, { threshold: 0.08, rootMargin: "0px 0px -24px 0px" })
        : null;

    const showAll = () => {
      observer?.disconnect();
      animations.forEach((animation) => animation.cancel());
      animations.clear();
      elements.forEach((element) => { element.dataset.revealed = "true"; });
    };

    if (!observer) {
      showAll();
      return;
    }

    elements.forEach((element) => observer.observe(element));
    const onPreference = () => {
      if (preference.matches) showAll();
    };
    const onFocus = (event: FocusEvent) => {
      const element = (event.target as HTMLElement).closest<HTMLElement>("[data-reveal]");
      if (!element) return;
      observer?.unobserve(element);
      element.dataset.revealed = "true";
      element.getAnimations().forEach((animation) => animation.cancel());
    };
    preference.addEventListener("change", onPreference);
    const container = root.current;
    container?.addEventListener("focusin", onFocus);
    return () => {
      showAll();
      preference.removeEventListener("change", onPreference);
      container?.removeEventListener("focusin", onFocus);
    };
  }, []);

  return root;
}
