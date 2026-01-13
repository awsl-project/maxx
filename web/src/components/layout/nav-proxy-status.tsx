import { useState } from 'react'
import { Radio, Check, Copy } from 'lucide-react'
import { useProxyStatus } from '@/hooks/queries'
import { useSidebar } from '@/components/ui/sidebar'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

export function NavProxyStatus() {
  const { data: proxyStatus } = useProxyStatus()
  const { state } = useSidebar()
  const [copied, setCopied] = useState(false)

  const proxyAddress = proxyStatus?.address ?? '...'
  const fullUrl = `http://${proxyAddress}`
  const isCollapsed = state === 'collapsed'

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(fullUrl)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  const buttonContent = (
    <>
      <div className="w-8 h-8 rounded-lg bg-emerald-400/10 flex items-center justify-center shrink-0">
        <Radio size={16} className="text-emerald-400" />
      </div>
      <div className="flex flex-col items-start flex-1 min-w-0 group-data-[collapsible=icon]:hidden">
        <span className="text-caption text-text-muted">Listening on</span>
        <span className="text-body font-mono font-medium text-text-primary truncate">
          {proxyAddress}
        </span>
      </div>
      <div className="shrink-0 text-text-muted group-data-[collapsible=icon]:hidden">
        {copied ? (
          <Check size={14} className="text-emerald-400" />
        ) : (
          <Copy
            size={14}
            className="opacity-0 group-hover:opacity-100 transition-opacity"
          />
        )}
      </div>
    </>
  )

  if (isCollapsed) {
    return (
      <Tooltip>
        <TooltipTrigger
          onClick={handleCopy}
          className="flex items-center justify-center gap-sm group w-full rounded-lg p-1 hover:bg-surface-hover transition-colors cursor-pointer"
        >
          {buttonContent}
        </TooltipTrigger>
        <TooltipContent side="right" align="center">
          <div className="flex flex-col gap-1">
            <span className="text-xs">Listening on</span>
            <span className="font-mono font-medium">{proxyAddress}</span>
            <span className="text-xs">Click to copy</span>
          </div>
        </TooltipContent>
      </Tooltip>
    )
  }

  return (
    <button
      onClick={handleCopy}
      className="flex items-center gap-sm group w-full rounded-lg p-1 hover:bg-surface-hover transition-colors cursor-pointer"
      title={`Click to copy: ${fullUrl}`}
    >
      {buttonContent}
    </button>
  )
}
