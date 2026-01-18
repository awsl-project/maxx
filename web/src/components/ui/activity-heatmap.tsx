import { useMemo } from 'react';
import { Tooltip, TooltipContent, TooltipTrigger } from './tooltip';
import { cn } from '@/lib/utils';

interface HeatmapDataPoint {
  date: string; // YYYY-MM-DD
  count: number;
}

interface ActivityHeatmapProps {
  data: HeatmapDataPoint[];
  className?: string;
  colorScheme?: 'green' | 'blue' | 'purple' | 'orange';
  maxWeeks?: number; // 最大显示周数，超出部分从前面截取
  timezone?: string; // 后端配置的时区，如 "Asia/Shanghai"
}

// 颜色方案
const colorSchemes = {
  green: {
    empty: 'bg-muted',
    level1: 'bg-emerald-200 dark:bg-emerald-900',
    level2: 'bg-emerald-400 dark:bg-emerald-700',
    level3: 'bg-emerald-500 dark:bg-emerald-500',
    level4: 'bg-emerald-600 dark:bg-emerald-400',
  },
  blue: {
    empty: 'bg-muted',
    level1: 'bg-blue-200 dark:bg-blue-900',
    level2: 'bg-blue-400 dark:bg-blue-700',
    level3: 'bg-blue-500 dark:bg-blue-500',
    level4: 'bg-blue-600 dark:bg-blue-400',
  },
  purple: {
    empty: 'bg-muted',
    level1: 'bg-violet-200 dark:bg-violet-900',
    level2: 'bg-violet-400 dark:bg-violet-700',
    level3: 'bg-violet-500 dark:bg-violet-500',
    level4: 'bg-violet-600 dark:bg-violet-400',
  },
  orange: {
    empty: 'bg-muted',
    level1: 'bg-orange-200 dark:bg-orange-900',
    level2: 'bg-orange-400 dark:bg-orange-700',
    level3: 'bg-orange-500 dark:bg-orange-500',
    level4: 'bg-orange-600 dark:bg-orange-400',
  },
};

function getColorLevel(
  count: number,
  maxCount: number,
  scheme: keyof typeof colorSchemes
): string {
  const colors = colorSchemes[scheme];
  if (count === 0) return colors.empty;

  const ratio = count / maxCount;
  if (ratio <= 0.25) return colors.level1;
  if (ratio <= 0.5) return colors.level2;
  if (ratio <= 0.75) return colors.level3;
  return colors.level4;
}

function formatDate(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  });
}

// 获取指定时区的今天日期 (YYYY-MM-DD)
function getTodayInTimezone(timezone?: string): string {
  try {
    const formatter = new Intl.DateTimeFormat('en-CA', {
      timeZone: timezone || 'Asia/Shanghai',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
    return formatter.format(new Date());
  } catch {
    // 如果时区无效，使用本地时间
    const today = new Date();
    const year = today.getFullYear();
    const month = String(today.getMonth() + 1).padStart(2, '0');
    const day = String(today.getDate()).padStart(2, '0');
    return `${year}-${month}-${day}`;
  }
}

export function ActivityHeatmap({
  data,
  className,
  colorScheme = 'green',
  maxWeeks,
  timezone,
}: ActivityHeatmapProps) {
  // 创建日期到数据的映射
  const dataMap = useMemo(() => {
    const map = new Map<string, number>();
    data.forEach((d) => map.set(d.date, d.count));
    return map;
  }, [data]);

  // 计算最大值用于颜色分级
  const maxCount = useMemo(() => {
    if (data.length === 0) return 1;
    return Math.max(...data.map((d) => d.count), 1);
  }, [data]);

  // 生成网格数据（按周组织，类似 GitHub）
  const gridData = useMemo(() => {
    if (data.length === 0) return [];

    // 获取日期范围
    const dates = data.map((d) => new Date(d.date)).sort((a, b) => a.getTime() - b.getTime());
    const startDate = dates[0];

    // 使用配置的时区确定"今天"
    const todayStr = getTodayInTimezone(timezone);
    const today = new Date(todayStr + 'T00:00:00');

    // 使用本地日期格式化，避免时区问题
    const formatLocalDate = (d: Date) => {
      const year = d.getFullYear();
      const month = String(d.getMonth() + 1).padStart(2, '0');
      const day = String(d.getDate()).padStart(2, '0');
      return `${year}-${month}-${day}`;
    };

    // 调整到周日开始
    const adjustedStart = new Date(startDate);
    adjustedStart.setDate(adjustedStart.getDate() - adjustedStart.getDay());

    // 补全到本周六（确保显示完整的一周）
    const adjustedEnd = new Date(today);
    const daysUntilSaturday = 6 - today.getDay();
    adjustedEnd.setDate(adjustedEnd.getDate() + daysUntilSaturday);

    const weeks: { date: string; count: number; dayOfWeek: number; isFuture: boolean }[][] = [];
    let currentWeek: { date: string; count: number; dayOfWeek: number; isFuture: boolean }[] = [];

    const current = new Date(adjustedStart);
    while (current <= adjustedEnd) {
      const dateStr = formatLocalDate(current);
      const count = dataMap.get(dateStr) || 0;
      const dayOfWeek = current.getDay();
      const isFuture = dateStr > todayStr;

      currentWeek.push({ date: dateStr, count, dayOfWeek, isFuture });

      if (dayOfWeek === 6) {
        weeks.push(currentWeek);
        currentWeek = [];
      }

      current.setDate(current.getDate() + 1);
    }

    // 添加最后一周（如果有）
    if (currentWeek.length > 0) {
      weeks.push(currentWeek);
    }

    // 如果设置了 maxWeeks，只保留最后 maxWeeks 周（优先显示今天）
    if (maxWeeks && weeks.length > maxWeeks) {
      return weeks.slice(-maxWeeks);
    }

    return weeks;
  }, [data, dataMap, maxWeeks, timezone]);

  if (data.length === 0) {
    return (
      <div className={cn('text-sm text-muted-foreground', className)}>
        暂无活动数据
      </div>
    );
  }

  return (
    <div className={cn('flex flex-col gap-1', className)}>
      {/* 热力图网格 - overflow-hidden + justify-end 确保今天的数据始终可见 */}
      <div className="flex gap-[3px] pb-1 overflow-hidden justify-end">
        {gridData.map((week, weekIndex) => (
          <div key={weekIndex} className="flex flex-col gap-[3px]">
            {week.map((day) =>
              day.isFuture ? (
                // 未来日期：显示为空白/禁用状态，不可交互
                <div
                  key={day.date}
                  className="w-3 h-3 rounded-sm bg-muted/30 border border-dashed border-muted-foreground/20"
                />
              ) : (
                <Tooltip key={day.date}>
                  <TooltipTrigger>
                    <div
                      className={cn(
                        'w-3 h-3 rounded-sm cursor-default transition-colors',
                        getColorLevel(day.count, maxCount, colorScheme)
                      )}
                    />
                  </TooltipTrigger>
                  <TooltipContent side="top" className="text-xs">
                    <p className="font-medium">{formatDate(day.date)}</p>
                    <p className="text-muted-foreground">
                      {day.count.toLocaleString()} 请求
                    </p>
                  </TooltipContent>
                </Tooltip>
              )
            )}
          </div>
        ))}
      </div>

      {/* 图例 */}
      <div className="flex items-center gap-2 text-xs text-muted-foreground mt-1">
        <span>少</span>
        <div className="flex gap-[2px]">
          {['empty', 'level1', 'level2', 'level3', 'level4'].map((level) => (
            <div
              key={level}
              className={cn(
                'w-3 h-3 rounded-sm',
                colorSchemes[colorScheme][level as keyof (typeof colorSchemes)['green']]
              )}
            />
          ))}
        </div>
        <span>多</span>
      </div>
    </div>
  );
}
