import { useState, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { Cooldown } from '@/lib/transport';

interface CooldownTimerProps {
  cooldown: Cooldown;
  className?: string;
}

/**
 * 实时倒计时组件，每秒更新显示
 * 过期时自动触发 cooldowns 刷新
 */
export function CooldownTimer({ cooldown, className }: CooldownTimerProps) {
  const queryClient = useQueryClient();
  const [remainingSeconds, setRemainingSeconds] = useState(() => calculateRemaining(cooldown));

  useEffect(() => {
    // 每秒更新一次
    const interval = setInterval(() => {
      const remaining = calculateRemaining(cooldown);
      setRemainingSeconds(remaining);

      // 过期时刷新 cooldowns
      if (remaining <= 0) {
        queryClient.invalidateQueries({ queryKey: ['cooldowns'] });
        clearInterval(interval);
      }
    }, 1000);

    return () => clearInterval(interval);
  }, [cooldown, queryClient]);

  // 已过期，不显示
  if (remainingSeconds <= 0) {
    return null;
  }

  return <span className={className}>{formatSeconds(remainingSeconds)}</span>;
}

function calculateRemaining(cooldown: Cooldown): number {
  const untilTime =
    cooldown.untilTime || ((cooldown as unknown as Record<string, unknown>).until as string);
  if (!untilTime) return 0;

  const until = new Date(untilTime).getTime();
  const now = Date.now();
  return Math.max(0, Math.floor((until - now) / 1000));
}

function formatSeconds(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;

  if (hours > 0) {
    return `${String(hours).padStart(2, '0')}h ${String(minutes).padStart(2, '0')}m ${String(secs).padStart(2, '0')}s`;
  } else if (minutes > 0) {
    return `${String(minutes).padStart(2, '0')}m ${String(secs).padStart(2, '0')}s`;
  } else {
    return `${String(secs).padStart(2, '0')}s`;
  }
}
