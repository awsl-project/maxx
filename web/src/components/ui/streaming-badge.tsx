/**
 * Streaming Badge 组件
 * 显示实时活动请求数，带延迟消失效果
 */

import { useState, useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

interface StreamingBadgeProps {
  /** 当前计数 */
  count: number
  /** 徽章颜色 (用于边框和发光效果) */
  color?: string
  /** 自定义类名 */
  className?: string
  /** 延迟消失时间 (ms)，默认 1000 */
  hideDelay?: number
}

/**
 * Streaming Badge
 * 特性：
 * - 计数 > 0 时立即显示
 * - 计数 = 0 时延迟隐藏，避免频繁闪烁
 * - 带脉冲动画和彩色发光效果
 */
export function StreamingBadge({
  count,
  color = '#0078D4',
  className,
  hideDelay = 1000,
}: StreamingBadgeProps) {
  const [displayCount, setDisplayCount] = useState(count)
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    // 计数 > 0: 立即显示，清除任何待执行的隐藏定时器
    if (count > 0) {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
        timeoutRef.current = null
      }
      setDisplayCount(count)
    } else {
      // 计数 = 0: 延迟后隐藏（防止闪烁）
      if (!timeoutRef.current && displayCount > 0) {
        timeoutRef.current = setTimeout(() => {
          setDisplayCount(0)
          timeoutRef.current = null
        }, hideDelay)
      }
    }

    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current)
      }
    }
  }, [count, displayCount, hideDelay])

  // 不显示时返回 null
  if (displayCount === 0) {
    return null
  }

  return (
    <span
      className={cn(
        'px-2 py-0.5 rounded text-xs font-extrabold animate-pulse-soft shadow-md min-w-[1.5rem] text-center bg-surface-secondary border',
        className
      )}
      style={{
        borderColor: color,
        color: 'var(--color-primary-foreground)',
        boxShadow: `0 0 10px ${color}60`,
      }}
    >
      {displayCount}
    </span>
  )
}
