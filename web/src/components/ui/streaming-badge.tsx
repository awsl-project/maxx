/**
 * Streaming Badge 组件
 * 显示实时活动请求数
 */

import { cn } from '@/lib/utils';

interface StreamingBadgeProps {
  /** 当前计数 */
  count: number;
  /** 徽章颜色 (用于边框和发光效果) */
  color?: string;
  /** 自定义类名 */
  className?: string;
  /** 显示变体: 'corner' 用于卡片右上角, 'inline' 用于行内显示 */
  variant?: 'corner' | 'inline';
}

/**
 * Streaming Badge
 * 特性：
 * - 计数 > 0 时显示
 * - 带脉冲动画和彩色发光效果
 */
export function StreamingBadge({
  count,
  color = '#0078D4',
  className,
  variant = 'inline',
}: StreamingBadgeProps) {
  if (count === 0) {
    return null;
  }

  const isCorner = variant === 'corner';

  return (
    <span
      className={cn(
        'font-extrabold animate-pulse-soft text-center bg-secondary/95 backdrop-blur-sm',
        isCorner ? 'px-2.5 py-1 text-xs border-l-2 border-b-2' : 'w-5 h-5 text-[10px] border rounded-sm flex items-center justify-center',
        className,
      )}
      style={{
        borderColor: color,
        boxShadow: isCorner ? `0 0 12px ${color}50` : `0 0 8px ${color}40`,
      }}
    >
      {count}
    </span>
  );
}
