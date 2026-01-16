import { useState } from 'react'
import { Button } from '@/components/ui'
import { Copy, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface CopyButtonProps {
  content: string
  label?: string
}

export function CopyButton({ content, label }: CopyButtonProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  return (
    <Button
      variant="outline"
      size="sm"
      onClick={handleCopy}
      className="h-6 px-2 text-[10px] gap-1"
    >
      {copied ? (
        <>
          <Check className="h-3 w-3" />
          {t('common.copied')}
        </>
      ) : (
        <>
          <Copy className="h-3 w-3" />
          {label || t('common.copy')}
        </>
      )}
    </Button>
  )
}
