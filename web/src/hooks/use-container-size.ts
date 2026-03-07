import { useState, useEffect, useCallback } from 'react';

/**
 * Measures container dimensions via ResizeObserver, filtering out
 * 0/negative values to avoid Recharts "width(-1) and height(-1)" warnings.
 * Uses callback ref so the observer re-attaches when the DOM node changes
 * (e.g., after conditional rendering unmount/remount).
 * Fixes Issue #220.
 */
export function useContainerSize() {
  const [node, setNode] = useState<HTMLDivElement | null>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });

  const ref = useCallback((el: HTMLDivElement | null) => {
    setNode(el);
  }, []);

  const handleResize = useCallback((entries: ResizeObserverEntry[]) => {
    for (const entry of entries) {
      const { width, height } = entry.contentRect;
      const w = Math.floor(width);
      const h = Math.floor(height);
      if (w > 0 && h > 0) {
        setSize((prev) => (prev.width === w && prev.height === h ? prev : { width: w, height: h }));
      }
    }
  }, []);

  useEffect(() => {
    if (!node) return;
    const observer = new ResizeObserver(handleResize);
    observer.observe(node);
    const rect = node.getBoundingClientRect();
    const w = Math.floor(rect.width);
    const h = Math.floor(rect.height);
    if (w > 0 && h > 0) setSize({ width: w, height: h });
    return () => observer.disconnect();
  }, [handleResize, node]);

  return { ref, ...size };
}
