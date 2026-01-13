import { useState } from 'react'
import { Button } from '@/components/ui'
import { GitCompare } from 'lucide-react'
import { DiffModal } from './DiffModal'

interface DiffButtonProps {
  clientContent: string
  upstreamContent: string
  title: string
}

export function DiffButton({
  clientContent,
  upstreamContent,
  title,
}: DiffButtonProps) {
  const [isOpen, setIsOpen] = useState(false)

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setIsOpen(true)}
        className="h-6 px-2 text-[10px] gap-1"
      >
        <GitCompare className="h-3 w-3" />
        Diff
      </Button>
      <DiffModal
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
        title={title}
        leftContent={clientContent}
        rightContent={upstreamContent}
      />
    </>
  )
}
