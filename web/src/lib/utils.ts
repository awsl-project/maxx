import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

/**
 * 合并 Tailwind CSS 类名
 * 使用 clsx 处理条件类名，tailwind-merge 处理冲突类名
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * 格式化时长（从纳秒转换为人类可读格式）
 * @param ns - 纳秒数（Go time.Duration 格式）
 * @returns 格式化后的时长字符串（如 "123ms", "4.56s", "2m 30s"）
 */
export function formatDuration(ns: number): string {
  // Convert nanoseconds to milliseconds
  const ms = ns / 1_000_000
  if (ms < 1000) return `${ms.toFixed(0)}ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(2)}s`
  const minutes = Math.floor(seconds / 60)
  const remainingSeconds = Math.floor(seconds % 60)
  return `${minutes}m ${remainingSeconds}s`
}
