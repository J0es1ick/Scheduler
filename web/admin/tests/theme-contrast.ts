import { expect, type Page } from "@playwright/test";

export async function expectReadableTheme(page: Page) {
  const failures = await page
    .locator(
      [
        ".desktop-nav button",
        ".status-pill",
        ".button-primary:not(:disabled)",
        ".source-transport-warning",
        ".snapshot-quarantine",
        ".identity-conflict",
        ".snapshot-lesson-card",
        ".snapshot-lesson-card h4",
        ".snapshot-group-item.is-changed span",
        ".board-lesson",
        ".login-submit:not(:disabled)",
        ".theme-control-caption strong",
      ].join(","),
    )
    .evaluateAll((elements) => {
      const rgb = (color: string) => {
        const numbers = color.match(/[\d.]+/g)?.map(Number);
        return numbers && color.startsWith("rgb") ? numbers : null;
      };
      const luminance = (values: number[]) =>
        values.slice(0, 3).reduce((sum, value, i) => {
          const normalized = value / 255;
          return (
            sum +
            (normalized <= 0.04045
              ? normalized / 12.92
              : ((normalized + 0.055) / 1.055) ** 2.4) *
              [0.2126, 0.7152, 0.0722][i]
          );
        }, 0);
      return elements.flatMap((element) => {
        if (!element.getClientRects().length || !element.textContent?.trim())
          return [];
        const foreground = rgb(getComputedStyle(element).color);
        let background: number[] | null = null;
        for (
          let parent: Element | null = element;
          parent;
          parent = parent.parentElement
        ) {
          const candidate = rgb(getComputedStyle(parent).backgroundColor);
          if (candidate && (candidate[3] === undefined || candidate[3] === 1)) {
            background = candidate;
            break;
          }
        }
        if (!foreground || !background) return [];
        const a = luminance(foreground),
          b = luminance(background);
        const contrast = (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
        return contrast < 4.5
          ? [
              {
                element: element.className,
                text: element.textContent?.slice(0, 50),
                contrast,
              },
            ]
          : [];
      });
    });
  expect(failures, "text contrast in rendered theme states").toEqual([]);
}
