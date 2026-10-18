export function installSurfaceLight(root: HTMLElement) {
  const preference = window.matchMedia(
    "(hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference)",
  );
  let active: HTMLElement | null = null;
  let frame = 0;
  let x = 0;
  let y = 0;
  const clear = () => {
    cancelAnimationFrame(frame);
    frame = 0;
    if (active) {
      delete active.dataset.lightActive;
      active.style.removeProperty("--light-x");
      active.style.removeProperty("--light-y");
    }
    active = null;
  };
  const move = (event: PointerEvent) => {
    if (!preference.matches || event.pointerType !== "mouse") return;
    const next = event.target instanceof Element
      ? event.target.closest<HTMLElement>("[data-surface-light]")
      : null;
    if (next !== active) {
      clear();
      active = next;
    }
    if (!active) return;
    x = event.clientX;
    y = event.clientY;
    if (frame) return;
    frame = requestAnimationFrame(() => {
      frame = 0;
      if (!active) return;
      const rect = active.getBoundingClientRect();
      active.style.setProperty("--light-x", `${x - rect.left}px`);
      active.style.setProperty("--light-y", `${y - rect.top}px`);
      active.dataset.lightActive = "true";
    });
  };
  root.addEventListener("pointermove", move, { passive: true });
  root.addEventListener("pointerleave", clear);
  preference.addEventListener("change", clear);
  return () => {
    clear();
    root.removeEventListener("pointermove", move);
    root.removeEventListener("pointerleave", clear);
    preference.removeEventListener("change", clear);
  };
}
