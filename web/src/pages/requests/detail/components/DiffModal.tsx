import { useMemo, useEffect } from 'react';
import * as Diff from 'diff';
import { Button } from '@/components/ui';
import { GitCompare, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';

interface DiffModalProps {
  isOpen: boolean;
  onClose: () => void;
  title: string;
  leftContent: string;
  rightContent: string;
}

export function DiffModal({ isOpen, onClose, title, leftContent, rightContent }: DiffModalProps) {
  const { t } = useTranslation();
  // Compute diff (must be before any conditional returns)
  const diffResult = useMemo(() => {
    return Diff.diffLines(leftContent, rightContent);
  }, [leftContent, rightContent]);

  // Handle ESC key press
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation(); // Prevent event from bubbling to parent
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown, { capture: true });
    return () => window.removeEventListener('keydown', handleKeyDown, { capture: true });
  }, [isOpen, onClose]);

  // Early return AFTER all hooks
  if (!isOpen) return null;

  // Handle backdrop click
  const handleBackdropClick = (e: React.MouseEvent) => {
    if (e.target === e.currentTarget) {
      e.stopPropagation();
      onClose();
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm"
      onClick={handleBackdropClick}
      onKeyDown={(e) => e.stopPropagation()} // Prevent any key events from bubbling
    >
      <div
        className="bg-card border border-border rounded-lg shadow-2xl w-[90vw] h-[85vh] flex flex-col"
        onClick={(e) => e.stopPropagation()} // Prevent clicks inside modal from closing it
      >
        {/* Modal Header */}
        <div className="h-14 border-b border-border px-6 flex items-center justify-between shrink-0">
          <div className="flex items-center gap-3">
            <GitCompare className="h-5 w-5 text-accent" />
            <h3 className="text-sm font-semibold text-foreground">{title}</h3>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={onClose}
            className="h-8 w-8 text-muted-foreground hover:text-foreground"
          >
            <X className="h-5 w-5" />
          </Button>
        </div>

        {/* Legend */}
        <div className="h-10 border-b border-border px-6 flex items-center gap-6 shrink-0 bg-muted/20">
          <div className="flex items-center gap-2 text-xs">
            <div className="w-3 h-3 bg-green-500/20 border border-green-500/50 rounded"></div>
            <span className="text-muted-foreground">{t('requests.diff.addedUpstream')}</span>
          </div>
          <div className="flex items-center gap-2 text-xs">
            <div className="w-3 h-3 bg-red-500/20 border border-red-500/50 rounded"></div>
            <span className="text-muted-foreground">{t('requests.diff.removedClient')}</span>
          </div>
          <div className="flex items-center gap-2 text-xs">
            <div className="w-3 h-3 bg-muted border border-border rounded"></div>
            <span className="text-muted-foreground">{t('requests.diff.unchanged')}</span>
          </div>
        </div>

        {/* Unified Diff View */}
        <div className="flex-1 overflow-auto p-4 bg-[#1a1a1a]">
          <div className="font-mono text-xs">
            {diffResult.map((part, index) => {
              const lines = part.value.split('\n').filter((line, idx, arr) => {
                // Remove the last empty line if it exists
                return idx < arr.length - 1 || line !== '';
              });

              return lines.map((line, lineIndex) => {
                let bgColor = '';
                let textColor = 'text-foreground';
                let prefix = ' ';
                let borderColor = 'border-border/30';

                if (part.added) {
                  bgColor = 'bg-green-500/10';
                  textColor = 'text-green-400';
                  prefix = '+';
                  borderColor = 'border-green-500/30';
                } else if (part.removed) {
                  bgColor = 'bg-red-500/10';
                  textColor = 'text-red-400';
                  prefix = '-';
                  borderColor = 'border-red-500/30';
                } else {
                  bgColor = 'bg-card/20';
                  textColor = 'text-muted-foreground';
                }

                return (
                  <div
                    key={`${index}-${lineIndex}`}
                    className={`${bgColor} ${textColor} px-4 py-1 border-l-2 ${borderColor} whitespace-pre-wrap break-all leading-relaxed hover:bg-opacity-80 transition-colors`}
                  >
                    <span className="inline-block w-4 opacity-60 select-none">{prefix}</span>
                    {line || ' '}
                  </div>
                );
              });
            })}
          </div>
        </div>

        {/* Modal Footer */}
        <div className="h-14 border-t border-border px-6 flex items-center justify-between shrink-0">
          <div className="text-xs text-muted-foreground">
            {diffResult.filter((p) => p.added).length > 0 && (
              <span className="text-green-400 mr-4">
                +
                {diffResult
                  .filter((p) => p.added)
                  .reduce((acc, p) => acc + p.value.split('\n').length - 1, 0)}{' '}
                {t('requests.diff.lines')}
              </span>
            )}
            {diffResult.filter((p) => p.removed).length > 0 && (
              <span className="text-red-400">
                -
                {diffResult
                  .filter((p) => p.removed)
                  .reduce((acc, p) => acc + p.value.split('\n').length - 1, 0)}{' '}
                {t('requests.diff.lines')}
              </span>
            )}
          </div>
          <Button variant="outline" onClick={onClose}>
            {t('common.close')}
          </Button>
        </div>
      </div>
    </div>
  );
}
