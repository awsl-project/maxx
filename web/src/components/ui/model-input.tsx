import { useState, useMemo, useEffect, useRef } from 'react';
import { ChevronDown, Search, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';

// 常见模型列表
const COMMON_MODELS = [
  // Claude wildcards (for source patterns)
  { id: '*claude*', name: 'All Claude models', provider: 'Claude' },
  { id: '*sonnet*', name: 'All Sonnet models', provider: 'Claude' },
  { id: '*opus*', name: 'All Opus models', provider: 'Claude' },
  { id: '*haiku*', name: 'All Haiku models', provider: 'Claude' },
  // Claude models
  {
    id: 'claude-sonnet-4-20250514',
    name: 'Claude Sonnet 4',
    provider: 'Claude',
  },
  { id: 'claude-opus-4-20250514', name: 'Claude Opus 4', provider: 'Claude' },
  {
    id: 'claude-3-5-sonnet-20241022',
    name: 'Claude 3.5 Sonnet',
    provider: 'Claude',
  },
  {
    id: 'claude-3-5-haiku-20241022',
    name: 'Claude 3.5 Haiku',
    provider: 'Claude',
  },
  { id: 'claude-3-opus-20240229', name: 'Claude 3 Opus', provider: 'Claude' },
  // Gemini wildcards
  { id: '*gemini*', name: 'All Gemini models', provider: 'Gemini' },
  { id: '*flash*', name: 'All Flash models', provider: 'Gemini' },
  // Gemini models
  { id: 'gemini-2.5-pro', name: 'Gemini 2.5 Pro', provider: 'Gemini' },
  { id: 'gemini-2.5-flash', name: 'Gemini 2.5 Flash', provider: 'Gemini' },
  {
    id: 'gemini-2.5-flash-lite',
    name: 'Gemini 2.5 Flash Lite',
    provider: 'Gemini',
  },
  { id: 'gemini-2.0-flash', name: 'Gemini 2.0 Flash', provider: 'Gemini' },
  { id: 'gemini-1.5-pro', name: 'Gemini 1.5 Pro', provider: 'Gemini' },
  { id: 'gemini-1.5-flash', name: 'Gemini 1.5 Flash', provider: 'Gemini' },
  // OpenAI wildcards
  { id: '*gpt*', name: 'All GPT models', provider: 'OpenAI' },
  { id: '*o1*', name: 'All o1 models', provider: 'OpenAI' },
  { id: '*o3*', name: 'All o3 models', provider: 'OpenAI' },
  // NVIDIA/Meta wildcards
  { id: '*llama*', name: 'All Llama models', provider: 'NVIDIA' },
  { id: 'meta/*', name: 'All Meta models', provider: 'NVIDIA' },
  // OpenAI models
  { id: 'gpt-4o', name: 'GPT-4o', provider: 'OpenAI' },
  { id: 'gpt-4o-mini', name: 'GPT-4o Mini', provider: 'OpenAI' },
  { id: 'gpt-4-turbo', name: 'GPT-4 Turbo', provider: 'OpenAI' },
  { id: 'gpt-4', name: 'GPT-4', provider: 'OpenAI' },
  { id: 'gpt-3.5-turbo', name: 'GPT-3.5 Turbo', provider: 'OpenAI' },
  { id: 'o1', name: 'o1', provider: 'OpenAI' },
  { id: 'o1-mini', name: 'o1 Mini', provider: 'OpenAI' },
  { id: 'o1-pro', name: 'o1 Pro', provider: 'OpenAI' },
  { id: 'o3-mini', name: 'o3 Mini', provider: 'OpenAI' },
  // NVIDIA models
  { id: 'minimaxai/minimax-m2.1', name: 'MiniMax M2.1', provider: 'NVIDIA' },
  { id: 'z-ai/glm4.7', name: 'GLM 4.7', provider: 'NVIDIA' },
  { id: 'deepseek-ai/deepseek-rl', name: 'DeepSeek RL', provider: 'NVIDIA' },
  { id: 'qwen/qwen2.5-coder-32b-instruct', name: 'Qwen 2.5 Coder 32B', provider: 'NVIDIA' },
  { id: 'openai/gpt-oss-120b', name: 'GPT OSS 120B', provider: 'NVIDIA' },
  { id: 'google/gemma-3-27b-it', name: 'Gemma 3 27B', provider: 'NVIDIA' },
  {
    id: 'meta/llama-4-maverick-17b-128e-instruct',
    name: 'Llama 4 Maverick 17B',
    provider: 'NVIDIA',
  },
  { id: 'mistralai/devstral-2-123b-instruct-2512', name: 'Devstral 2 123B', provider: 'NVIDIA' },
  // Antigravity supported target models (use these as mapping targets)
  {
    id: 'claude-opus-4-5-thinking',
    name: 'Claude Opus 4.5 Thinking',
    provider: 'Antigravity',
  },
  {
    id: 'claude-sonnet-4-5',
    name: 'Claude Sonnet 4.5',
    provider: 'Antigravity',
  },
  {
    id: 'claude-sonnet-4-5-thinking',
    name: 'Claude Sonnet 4.5 Thinking',
    provider: 'Antigravity',
  },
  {
    id: 'gemini-2.5-flash-lite',
    name: 'Gemini 2.5 Flash Lite',
    provider: 'Antigravity',
  },
  { id: 'gemini-2.5-flash', name: 'Gemini 2.5 Flash', provider: 'Antigravity' },
  {
    id: 'gemini-2.5-flash-thinking',
    name: 'Gemini 2.5 Flash Thinking',
    provider: 'Antigravity',
  },
  { id: 'gemini-2.5-pro', name: 'Gemini 2.5 Pro', provider: 'Antigravity' },
  { id: 'gemini-3-flash', name: 'Gemini 3 Flash', provider: 'Antigravity' },
  { id: 'gemini-3-pro', name: 'Gemini 3 Pro', provider: 'Antigravity' },
  { id: 'gemini-3-pro-low', name: 'Gemini 3 Pro Low', provider: 'Antigravity' },
  {
    id: 'gemini-3-pro-high',
    name: 'Gemini 3 Pro High',
    provider: 'Antigravity',
  },
  {
    id: 'gemini-3-pro-preview',
    name: 'Gemini 3 Pro Preview',
    provider: 'Antigravity',
  },
  {
    id: 'gemini-3-pro-image',
    name: 'Gemini 3 Pro Image',
    provider: 'Antigravity',
  },
  // Generic wildcard
  { id: '*', name: 'All models (catch-all)', provider: 'Other' },
] as const;

type Model = (typeof COMMON_MODELS)[number];
type Provider = Model['provider'];

interface ModelInputProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  /** Filter to only show models from specific providers */
  providers?: Provider[];
}

// 简单的模糊匹配函数
function fuzzyMatch(text: string, pattern: string): boolean {
  const lowerText = text.toLowerCase();
  const lowerPattern = pattern.toLowerCase();

  // 先尝试普通包含匹配
  if (lowerText.includes(lowerPattern)) return true;

  // 模糊匹配：pattern 中的字符按顺序出现在 text 中
  let patternIdx = 0;
  for (let i = 0; i < lowerText.length && patternIdx < lowerPattern.length; i++) {
    if (lowerText[i] === lowerPattern[patternIdx]) {
      patternIdx++;
    }
  }
  return patternIdx === lowerPattern.length;
}

// 计算匹配分数（用于排序）
function matchScore(model: Model, pattern: string): number {
  const lowerPattern = pattern.toLowerCase();
  const lowerId = model.id.toLowerCase();
  const lowerName = model.name.toLowerCase();

  // 精确匹配得分最高
  if (lowerId === lowerPattern || lowerName === lowerPattern) return 100;

  // 前缀匹配次之
  if (lowerId.startsWith(lowerPattern) || lowerName.startsWith(lowerPattern)) return 80;

  // 包含匹配
  if (lowerId.includes(lowerPattern) || lowerName.includes(lowerPattern)) return 60;

  // 模糊匹配得分最低
  return 40;
}

export function ModelInput({
  value,
  onChange,
  placeholder,
  disabled = false,
  className,
  providers,
}: ModelInputProps) {
  const { t } = useTranslation();
  const actualPlaceholder = placeholder ?? t('modelInput.selectOrEnter');
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [focusedIndex, setFocusedIndex] = useState(-1);
  const focusedRef = useRef<HTMLButtonElement>(null);

  // Base models filtered by providers prop
  const baseModels = useMemo(() => {
    if (!providers || providers.length === 0) return [...COMMON_MODELS];
    return COMMON_MODELS.filter((model) => providers.includes(model.provider));
  }, [providers]);

  // 过滤和排序模型（支持模糊匹配）
  const filteredModels = useMemo(() => {
    if (!search.trim()) return baseModels;

    return baseModels
      .filter(
        (model) =>
          fuzzyMatch(model.id, search) ||
          fuzzyMatch(model.name, search) ||
          fuzzyMatch(model.provider, search),
      )
      .sort((a, b) => matchScore(b, search) - matchScore(a, search));
  }, [search, baseModels]);

  // 重置 focusedIndex 当过滤结果变化时
  useEffect(() => {
    setFocusedIndex(-1);
  }, [filteredModels.length]);

  // 自动滚动到高亮项
  useEffect(() => {
    if (focusedIndex >= 0 && focusedRef.current) {
      focusedRef.current.scrollIntoView({ block: 'nearest' });
    }
  }, [focusedIndex]);

  // 按 provider 分组
  const groupedModels = useMemo(() => {
    return filteredModels.reduce(
      (acc, model) => {
        if (!acc[model.provider]) {
          acc[model.provider] = [];
        }
        acc[model.provider].push(model);
        return acc;
      },
      {} as Record<string, Model[]>,
    );
  }, [filteredModels]);

  const handleOpen = () => {
    if (!disabled) {
      setSearch(value); // 初始化搜索框为当前值
      setIsOpen(true);
    }
  };

  const handleSelect = (modelId: string) => {
    onChange(modelId);
    setIsOpen(false);
    setSearch('');
  };

  const handleClear = (e: React.MouseEvent) => {
    e.stopPropagation();
    onChange('');
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      // 如果有高亮项，选择高亮项；否则使用搜索框内容
      if (focusedIndex >= 0 && focusedIndex < filteredModels.length) {
        handleSelect(filteredModels[focusedIndex].id);
      } else if (search.trim()) {
        handleSelect(search.trim());
      }
      return;
    }

    // Tab/Shift+Tab 切换高亮项
    if (e.key === 'Tab' && filteredModels.length > 0) {
      e.preventDefault();

      if (e.shiftKey) {
        // Shift+Tab: 上一个
        setFocusedIndex((prev) => (prev <= 0 ? filteredModels.length - 1 : prev - 1));
      } else {
        // Tab: 下一个
        setFocusedIndex((prev) => (prev >= filteredModels.length - 1 ? 0 : prev + 1));
      }
    }

    // 上下箭头也可以切换
    if (e.key === 'ArrowDown' && filteredModels.length > 0) {
      e.preventDefault();
      setFocusedIndex((prev) => (prev >= filteredModels.length - 1 ? 0 : prev + 1));
    }
    if (e.key === 'ArrowUp' && filteredModels.length > 0) {
      e.preventDefault();
      setFocusedIndex((prev) => (prev <= 0 ? filteredModels.length - 1 : prev - 1));
    }
  };

  return (
    <>
      {/* 触发按钮 */}
      <button
        type="button"
        onClick={handleOpen}
        disabled={disabled}
        className={cn(
          'w-full flex items-center justify-between gap-2 px-3 py-2',
          'bg-card border border-border rounded-md',
          'text-sm text-left',
          'hover:border-border-hover focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary',
          'disabled:opacity-50 disabled:cursor-not-allowed',
          'transition-colors',
          className,
        )}
      >
        <span
          className={cn('flex-1 truncate', value ? 'text-foreground' : 'text-muted-foreground')}
        >
          {value || actualPlaceholder}
        </span>
        <div className="flex items-center gap-1">
          {value && !disabled && (
            <span
              role="button"
              tabIndex={0}
              onClick={handleClear}
              onKeyDown={(e) => e.key === 'Enter' && handleClear(e as unknown as React.MouseEvent)}
              className="p-0.5 hover:bg-accent rounded text-muted-foreground hover:text-foreground"
            >
              <X size={14} />
            </span>
          )}
          <ChevronDown size={16} className="text-muted-foreground" />
        </div>
      </button>

      {/* Dialog */}
      <Dialog open={isOpen} onOpenChange={setIsOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t('modelInput.selectModel')}</DialogTitle>
          </DialogHeader>

          {/* 搜索框 */}
          <div className="relative">
            <Search
              size={16}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder={t('modelInput.searchOrEnter')}
              className="pl-9"
              autoFocus
            />
          </div>

          {/* 模型列表 */}
          <div className="h-80 overflow-y-auto -mx-6 px-6">
            {Object.keys(groupedModels).length > 0 ? (
              <div className="space-y-4">
                {Object.entries(groupedModels).map(([provider, models]) => (
                  <div key={provider}>
                    <div className="text-xs font-semibold text-muted-foreground mb-2 sticky top-0 bg-card py-1">
                      {provider}
                    </div>
                    <div className="space-y-1">
                      {models.map((model) => {
                        const modelIndex = filteredModels.findIndex((m) => m.id === model.id);
                        const isFocused = modelIndex === focusedIndex;
                        return (
                          <button
                            key={model.id}
                            ref={isFocused ? focusedRef : null}
                            type="button"
                            onClick={() => handleSelect(model.id)}
                            className={cn(
                              'w-full px-3 py-2 text-left text-sm rounded-md',
                              'hover:bg-accent transition-colors',
                              'flex flex-col gap-0.5',
                              value === model.id && 'bg-primary/10 ring-1 ring-primary/20',
                              isFocused && 'bg-accent ring-1 ring-primary/40',
                            )}
                          >
                            <span className="text-foreground font-medium">{model.name}</span>
                            <span className="text-xs text-muted-foreground font-mono">
                              {model.id}
                            </span>
                          </button>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            ) : search.trim() ? (
              <div className="py-4">
                <p className="text-sm text-text-secondary mb-3">
                  {t('modelInput.noMatchingModels')}
                </p>
                <button
                  type="button"
                  onClick={() => handleSelect(search.trim())}
                  className="w-full px-3 py-2 text-left text-sm bg-muted rounded-md hover:bg-accent transition-colors"
                >
                  <span className="text-text-primary">
                    {t('modelInput.useCustom')}
                    <span className="font-mono font-medium">{search.trim()}</span>
                  </span>
                </button>
              </div>
            ) : (
              <div className="h-full flex items-center justify-center text-sm text-text-muted">
                {t('modelInput.noModelsAvailable')}
              </div>
            )}
          </div>

          {/* 提示 */}
          <div className="text-xs text-text-muted pt-2 border-t border-border">
            {t('modelInput.pressToUse', { key: 'Enter' })}
            <kbd className="px-1.5 py-0.5 bg-surface-secondary rounded text-text-secondary font-mono">
              Enter
            </kbd>
            {t('modelInput.toUseCustomModel')}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
