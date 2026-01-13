/**
 * Client Icons
 * AI 客户端类型图标和颜色配置
 */

import claudeIcon from '@/assets/icons/claude.png';
import openaiIcon from '@/assets/icons/openai.png';
import codexIcon from '@/assets/icons/codex.png';
import geminiIcon from '@/assets/icons/gemini.png';
import type { ClientType } from '@/lib/transport';
import { getClientColorVar } from '@/lib/theme';

// 客户端图标映射
// OpenAI: 白底黑字 logo
// Codex: 黑底白字 logo (OpenAI Codex CLI)
const clientIconMap: Record<string, string> = {
  claude: claudeIcon,
  openai: openaiIcon,
  codex: codexIcon,
  gemini: geminiIcon,
};

/** @deprecated 使用 getClientColor() 或 getClientColorVar() 替代 */
export const clientColors: Record<ClientType, string> = {
  claude: '#D4A574',
  openai: '#10A37F',
  codex: '#10A37F',
  gemini: '#4285F4',
};

/**
 * 客户端显示名称
 */
export const clientNames: Record<ClientType, string> = {
  claude: 'Claude',
  openai: 'OpenAI',
  codex: 'Codex',
  gemini: 'Gemini',
};

/**
 * 获取客户端颜色（返回 CSS 变量引用）
 */
export function getClientColor(clientType: ClientType): string {
  return getClientColorVar(clientType as any) || 'var(--color-provider-custom)';
}

/**
 * 获取客户端显示名称
 */
export function getClientName(clientType: ClientType): string {
  return clientNames[clientType] || clientType;
}

interface ClientIconProps {
  type: ClientType;
  size?: number;
  className?: string;
}

/**
 * 客户端图标组件
 * 使用 PNG 图标，颜色与客户端类型对应
 */
export function ClientIcon({ type, size = 18, className }: ClientIconProps) {
  const iconSrc = clientIconMap[type];
  const color = getClientColor(type);

  // 如果没有图标，显示首字母的彩色圆形
  if (!iconSrc) {
    return (
      <div
        className={`flex items-center justify-center rounded ${className || ''}`}
        style={{
          width: size,
          height: size,
          backgroundColor: `${color}20`,
          color: color,
          fontSize: size * 0.4,
          fontWeight: 600,
        }}
      >
        {type.charAt(0).toUpperCase()}
      </div>
    );
  }

  return (
    <img
      src={iconSrc}
      alt={type}
      className={className}
      style={{
        width: size,
        height: size,
        objectFit: 'contain',
      }}
    />
  );
}

/**
 * 所有支持的客户端类型列表
 */
export const allClientTypes: ClientType[] = ['claude', 'openai', 'codex', 'gemini'];
