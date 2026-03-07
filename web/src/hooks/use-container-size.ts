import { useRef, useState, useEffect, useCallback } from 'react';

/**
 * Measures container dimensions via ResizeObserver, filtering out
 * 0/negative values to avoid Recharts "width(-1) and height(-1)" warnings.
 * Fixes Issue #220.
 */
export function useContainerSize() {
  const ref = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ width: 0, height: 0 });

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
    const el = ref.current;
    if (!el) return;
    const observer = new ResizeObserver(handleResize);
    observer.observe(el);
    const rect = el.getBoundingClientRect();
    const w = Math.floor(rect.width);
    const h = Math.floor(rect.height);
    if (w > 0 && h > 0) setSize({ width: w, height: h });
    return () => observer.disconnect();
  }, [handleResize]);

  return { ref, ...size };
}
