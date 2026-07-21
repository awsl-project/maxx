import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core';
import type { DragEndEvent } from '@dnd-kit/core';
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Columns3, GripVertical, RotateCcw } from 'lucide-react';
import { Button } from '@/components/ui';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { cn } from '@/lib/utils';
import {
  type ColumnAvailability,
  type RequestColumnId,
  type RequestColumnPrefs,
  columnLabelKey,
  createDefaultColumnPrefs,
  isColumnAvailable,
} from './column-prefs';

type ColumnSettingsProps = {
  prefs: RequestColumnPrefs;
  availability: ColumnAvailability;
  onChange: (next: RequestColumnPrefs) => void;
};

function SortableColumnRow({
  id,
  checked,
  disabled,
  label,
  onToggle,
}: {
  id: RequestColumnId;
  checked: boolean;
  disabled: boolean;
  label: string;
  onToggle: (id: RequestColumnId, next: boolean) => void;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    disabled,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn(
        'flex items-center gap-2 rounded-md px-1.5 py-1.5',
        isDragging && 'bg-accent/40 opacity-80',
        disabled && 'opacity-50',
      )}
    >
      <button
        type="button"
        className="cursor-grab active:cursor-grabbing p-0.5 rounded hover:bg-accent shrink-0 touch-none"
        disabled={disabled}
        aria-label="Reorder"
        {...attributes}
        {...listeners}
      >
        <GripVertical className="h-3.5 w-3.5 text-muted-foreground" />
      </button>
      <label className="flex flex-1 items-center gap-2 min-w-0 cursor-pointer select-none">
        <input
          type="checkbox"
          className="size-3.5 accent-primary shrink-0"
          checked={checked}
          disabled={disabled}
          onChange={(event) => onToggle(id, event.target.checked)}
        />
        <span className="text-sm truncate">{label}</span>
      </label>
    </div>
  );
}

export function RequestsColumnSettings({ prefs, availability, onChange }: ColumnSettingsProps) {
  const { t } = useTranslation();
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const configurableIds = useMemo(
    () => prefs.order.filter((id) => isColumnAvailable(id, availability)),
    [availability, prefs.order],
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) {
      return;
    }
    const oldIndex = prefs.order.indexOf(active.id as RequestColumnId);
    const newIndex = prefs.order.indexOf(over.id as RequestColumnId);
    if (oldIndex < 0 || newIndex < 0) {
      return;
    }
    onChange({
      ...prefs,
      order: arrayMove(prefs.order, oldIndex, newIndex),
    });
  };

  const handleToggle = (id: RequestColumnId, next: boolean) => {
    if (!next) {
      const remaining = configurableIds.filter(
        (columnId) => columnId !== id && prefs.visibility[columnId] !== false,
      );
      if (remaining.length === 0) {
        return;
      }
    }
    onChange({
      ...prefs,
      visibility: {
        ...prefs.visibility,
        [id]: next,
      },
    });
  };

  const handleReset = () => {
    onChange(createDefaultColumnPrefs());
  };

  return (
    <Popover>
      <PopoverTrigger
        className={cn(
          'flex items-center gap-2 px-3 py-1.5 rounded-lg text-sm font-medium transition-all',
          'bg-muted/50 hover:bg-muted border border-border/50 hover:border-border',
          'text-muted-foreground hover:text-foreground',
        )}
      >
        <Columns3 size={14} />
        <span>{t('requests.columns.action')}</span>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 p-3 gap-2">
        <div className="flex items-center justify-between gap-2 px-1">
          <div className="text-sm font-medium text-foreground">{t('requests.columns.title')}</div>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 px-2 text-xs"
            onClick={handleReset}
          >
            <RotateCcw className="mr-1 h-3.5 w-3.5" />
            {t('requests.columns.reset')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground px-1">{t('requests.columns.hint')}</p>
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={configurableIds} strategy={verticalListSortingStrategy}>
            <div className="max-h-72 overflow-y-auto pr-0.5">
              {configurableIds.map((id) => (
                <SortableColumnRow
                  key={id}
                  id={id}
                  checked={prefs.visibility[id] !== false}
                  disabled={false}
                  label={t(columnLabelKey(id))}
                  onToggle={handleToggle}
                />
              ))}
            </div>
          </SortableContext>
        </DndContext>
      </PopoverContent>
    </Popover>
  );
}
