import { useEffect, useState, useCallback } from 'react';

interface UseCountdownOptions {
  initialSeconds: number;
  onComplete?: () => void;
  autoStart?: boolean;
}

interface UseCountdownReturn {
  remainingTime: number;
  isRunning: boolean;
  reset: (seconds?: number) => void;
  start: () => void;
  stop: () => void;
}

export function useCountdown({
  initialSeconds,
  onComplete,
  autoStart = true,
}: UseCountdownOptions): UseCountdownReturn {
  const [remainingTime, setRemainingTime] = useState(initialSeconds);
  const [isRunning, setIsRunning] = useState(autoStart);

  const reset = useCallback(
    (seconds?: number) => {
      setRemainingTime(seconds ?? initialSeconds);
      setIsRunning(autoStart);
    },
    [initialSeconds, autoStart],
  );

  const start = useCallback(() => {
    setIsRunning(true);
  }, []);

  const stop = useCallback(() => {
    setIsRunning(false);
  }, []);

  useEffect(() => {
    if (!isRunning || remainingTime <= 0) return;

    const interval = setInterval(() => {
      setRemainingTime((prev) => {
        if (prev <= 1) {
          clearInterval(interval);
          setIsRunning(false);
          return 0;
        }
        return prev - 1;
      });
    }, 1000);

    return () => clearInterval(interval);
  }, [isRunning, remainingTime]);

  useEffect(() => {
    if (remainingTime === 0 && onComplete) {
      onComplete();
    }
  }, [remainingTime, onComplete]);

  return { remainingTime, isRunning, reset, start, stop };
}
