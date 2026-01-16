import { Settings, Moon, Sun, Monitor, Laptop, FolderOpen, Zap, Plus, Trash2, ArrowRight, RotateCcw, GripVertical, Database } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DndContext, closestCenter, KeyboardSensor, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import { arrayMove, SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { useTheme } from '@/components/theme-provider'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Switch,
} from '@/components/ui'
import { ModelInput } from '@/components/ui/model-input'
import { PageHeader } from '@/components/layout/page-header'
import { useSettings, useUpdateSetting, useModelMappings, useCreateModelMapping, useUpdateModelMapping, useDeleteModelMapping } from '@/hooks/queries'
import type { ModelMapping } from '@/lib/transport/types'

type Theme = 'light' | 'dark' | 'system'

export function SettingsPage() {
  const { t } = useTranslation()

  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Settings}
        iconClassName="text-zinc-500"
        title={t('settings.title')}
        description={t('settings.description')}
      />

      <div className="flex-1 overflow-y-auto p-6">
        <div className="space-y-6">
          <GeneralSection />
          <DataRetentionSection />
          <ForceProjectSection />
          <ModelMappingSection />
        </div>
      </div>
    </div>
  )
}

function GeneralSection() {
  const { theme, setTheme } = useTheme()
  const { t, i18n } = useTranslation()

  const themes: { value: Theme; label: string; icon: typeof Sun }[] = [
    { value: 'light', label: t('settings.theme.light'), icon: Sun },
    { value: 'dark', label: t('settings.theme.dark'), icon: Moon },
    { value: 'system', label: t('settings.theme.system'), icon: Laptop },
  ]

  const languages = [
    { value: 'en', label: t('settings.languages.en') },
    { value: 'zh', label: t('settings.languages.zh') },
  ]

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Monitor className="h-4 w-4 text-muted-foreground" />
          {t('settings.appearance')}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        <div className="flex items-center gap-6">
          <label className="text-sm font-medium text-muted-foreground w-40 shrink-0">
            {t('settings.themePreference')}
          </label>
          <div className="flex flex-wrap gap-3">
            {themes.map(({ value, label, icon: Icon }) => (
              <Button
                key={value}
                onClick={() => setTheme(value)}
                variant={theme === value ? 'default' : 'outline'}
              >
                <Icon size={16} />
                <span className="text-sm font-medium">{label}</span>
              </Button>
            ))}
          </div>
        </div>
        <div className="flex items-center gap-6">
          <label className="text-sm font-medium text-muted-foreground w-40 shrink-0">
            {t('settings.languagePreference')}
          </label>
          <div className="flex flex-wrap gap-3">
            {languages.map(({ value, label }) => (
              <Button
                key={value}
                onClick={() => i18n.changeLanguage(value)}
                variant={i18n.language === value ? 'default' : 'outline'}
              >
                <span className="text-sm font-medium">{label}</span>
              </Button>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function DataRetentionSection() {
  const { data: settings, isLoading } = useSettings()
  const updateSetting = useUpdateSetting()
  const { t } = useTranslation()

  const requestRetentionDays = settings?.request_retention_days || '7'
  const statsRetentionDays = settings?.stats_retention_days || '30'

  const handleRequestRetentionChange = async (value: string) => {
    const numValue = parseInt(value, 10)
    if (!isNaN(numValue) && numValue >= 0) {
      await updateSetting.mutateAsync({
        key: 'request_retention_days',
        value: value,
      })
    }
  }

  const handleStatsRetentionChange = async (value: string) => {
    const numValue = parseInt(value, 10)
    if (!isNaN(numValue) && numValue >= 0) {
      await updateSetting.mutateAsync({
        key: 'stats_retention_days',
        value: value,
      })
    }
  }

  if (isLoading) return null

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <Database className="h-4 w-4 text-muted-foreground" />
          {t('settings.dataRetention')}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6">
        <div className="flex items-center gap-8">
          <div className="flex items-center gap-3">
            <label className="text-sm font-medium text-muted-foreground shrink-0">
              {t('settings.requestRetentionDays')}
            </label>
            <Input
              type="number"
              value={requestRetentionDays}
              onChange={e => handleRequestRetentionChange(e.target.value)}
              className="w-20"
              min={0}
              disabled={updateSetting.isPending}
            />
          </div>
          <div className="flex items-center gap-3">
            <label className="text-sm font-medium text-muted-foreground shrink-0">
              {t('settings.statsRetentionDays')}
            </label>
            <Input
              type="number"
              value={statsRetentionDays}
              onChange={e => handleStatsRetentionChange(e.target.value)}
              className="w-20"
              min={0}
              disabled={updateSetting.isPending}
            />
          </div>
          <span className="text-xs text-muted-foreground">{t('settings.retentionDaysHint')}</span>
        </div>
      </CardContent>
    </Card>
  )
}

function ForceProjectSection() {
  const { data: settings, isLoading } = useSettings()
  const updateSetting = useUpdateSetting()
  const { t } = useTranslation()

  const forceProjectEnabled = settings?.force_project_binding === 'true'
  const timeout = settings?.force_project_timeout || '30'

  const handleToggle = async (checked: boolean) => {
    await updateSetting.mutateAsync({
      key: 'force_project_binding',
      value: checked ? 'true' : 'false',
    })
  }

  const handleTimeoutChange = async (value: string) => {
    const numValue = parseInt(value, 10)
    if (numValue >= 5 && numValue <= 300) {
      await updateSetting.mutateAsync({
        key: 'force_project_timeout',
        value: value,
      })
    }
  }

  if (isLoading) return null

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <CardTitle className="text-base font-medium flex items-center gap-2">
          <FolderOpen className="h-4 w-4 text-muted-foreground" />
          {t('settings.forceProjectBinding')}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <label className="text-sm font-medium text-foreground">
              {t('settings.enableForceProjectBinding')}
            </label>
            <p className="text-xs text-muted-foreground mt-1">
              {t('settings.forceProjectBindingDesc')}
            </p>
          </div>
          <Switch
            checked={forceProjectEnabled}
            onCheckedChange={handleToggle}
            disabled={updateSetting.isPending}
          />
        </div>

        {forceProjectEnabled && (
          <div className="flex items-center gap-6 pt-4 border-t border-border">
            <label className="text-sm font-medium text-muted-foreground w-32 shrink-0">
              {t('settings.waitTimeout')}
            </label>
            <Input
              type="number"
              value={timeout}
              onChange={e => handleTimeoutChange(e.target.value)}
              className="w-24"
              min={5}
              max={300}
              disabled={updateSetting.isPending}
            />
            <span className="text-xs text-muted-foreground">{t('settings.waitTimeoutRange')}</span>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

interface SortableRuleItemProps {
  id: string
  index: number
  rule: ModelMapping
  onRemove: () => void
  onUpdate: (pattern: string, target: string) => void
  disabled: boolean
}

function SortableRuleItem({ id, index, rule, onRemove, onUpdate, disabled }: SortableRuleItemProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`flex items-center gap-2 group ${isDragging ? 'opacity-50' : ''}`}
    >
      <button
        {...attributes}
        {...listeners}
        className="cursor-grab active:cursor-grabbing p-1 hover:bg-accent rounded"
        disabled={disabled}
      >
        <GripVertical className="h-4 w-4 text-muted-foreground" />
      </button>
      <span className="text-xs text-muted-foreground w-6">{index + 1}.</span>
      <ModelInput
        value={rule.pattern}
        onChange={pattern => onUpdate(pattern, rule.target)}
        placeholder="匹配模式"
        disabled={disabled}
        className="flex-1 max-w-xs h-7 text-xs"
      />
      <ArrowRight className="h-3 w-3 text-muted-foreground shrink-0" />
      <ModelInput
        value={rule.target}
        onChange={target => onUpdate(rule.pattern, target)}
        placeholder="目标模型"
        disabled={disabled}
        className="flex-1 max-w-xs h-7 text-xs"
        providers={['Antigravity']}
      />
      <Button
        variant="ghost"
        size="sm"
        onClick={onRemove}
        disabled={disabled}
      >
        <Trash2 className="h-4 w-4 text-destructive" />
      </Button>
    </div>
  )
}

function ModelMappingSection() {
  const { data: mappings, isLoading } = useModelMappings()
  const createMapping = useCreateModelMapping()
  const updateMapping = useUpdateModelMapping()
  const deleteMapping = useDeleteModelMapping()
  const [newPattern, setNewPattern] = useState('')
  const [newTarget, setNewTarget] = useState('')
  const { t } = useTranslation()

  const rules = mappings || []

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  )

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event
    if (!over || active.id === over.id) return

    const oldIndex = rules.findIndex(r => `rule-${r.id}` === active.id)
    const newIndex = rules.findIndex(r => `rule-${r.id}` === over.id)

    if (oldIndex !== -1 && newIndex !== -1) {
      const reordered = arrayMove(rules, oldIndex, newIndex)
      // Update priorities based on new order
      for (let i = 0; i < reordered.length; i++) {
        const rule = reordered[i]
        if (rule.priority !== i * 10) {
          await updateMapping.mutateAsync({
            id: rule.id,
            data: {
              pattern: rule.pattern,
              target: rule.target,
              priority: i * 10,
              isEnabled: rule.isEnabled,
            },
          })
        }
      }
    }
  }

  const handleAddRule = async () => {
    if (!newPattern.trim() || !newTarget.trim()) return

    await createMapping.mutateAsync({
      pattern: newPattern.trim(),
      target: newTarget.trim(),
      clientType: 'claude',
      priority: rules.length * 10 + 1000, // Add at end
      isEnabled: true,
    })
    setNewPattern('')
    setNewTarget('')
  }

  const handleRemoveRule = async (id: number) => {
    await deleteMapping.mutateAsync(id)
  }

  const handleUpdateRule = async (rule: ModelMapping, pattern: string, target: string) => {
    await updateMapping.mutateAsync({
      id: rule.id,
      data: {
        pattern,
        target,
        priority: rule.priority,
        isEnabled: rule.isEnabled,
      },
    })
  }

  const handleReset = async () => {
    // Delete all existing mappings, then the backend will re-seed defaults on next load
    for (const rule of rules) {
      await deleteMapping.mutateAsync(rule.id)
    }
  }

  if (isLoading) return null

  const isPending = createMapping.isPending || updateMapping.isPending || deleteMapping.isPending

  return (
    <Card className="border-border bg-card">
      <CardHeader className="border-b border-border py-4">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base font-medium flex items-center gap-2">
            <Zap className="h-4 w-4 text-muted-foreground" />
            {t('settings.antigravityModelMapping')}
          </CardTitle>
          <Button
            variant="outline"
            size="sm"
            onClick={handleReset}
            disabled={isPending || rules.length === 0}
          >
            <RotateCcw className="h-4 w-4 mr-1" />
            {t('settings.resetToPreset')}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-6 space-y-4">
        <p className="text-xs text-muted-foreground">
          {t('settings.modelMappingDesc')}
        </p>

        {rules.length > 0 && (
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
          >
            <SortableContext
              items={rules.map(r => `rule-${r.id}`)}
              strategy={verticalListSortingStrategy}
            >
              <div className="space-y-1.5">
                {rules.map((rule, index) => (
                  <SortableRuleItem
                    key={`rule-${rule.id}`}
                    id={`rule-${rule.id}`}
                    index={index}
                    rule={rule}
                    onRemove={() => handleRemoveRule(rule.id)}
                    onUpdate={(pattern, target) => handleUpdateRule(rule, pattern, target)}
                    disabled={isPending}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        )}

        <div className="flex items-center gap-2 pt-2 border-t border-border">
          <ModelInput
            value={newPattern}
            onChange={setNewPattern}
            placeholder={t('settings.matchPattern')}
            disabled={isPending}
            className="flex-1 max-w-xs"
          />
          <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
          <ModelInput
            value={newTarget}
            onChange={setNewTarget}
            placeholder={t('settings.targetModel')}
            disabled={isPending}
            className="flex-1 max-w-xs"
            providers={['Antigravity']}
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleAddRule}
            disabled={!newPattern.trim() || !newTarget.trim() || isPending}
          >
            <Plus className="h-4 w-4 mr-1" />
            {t('common.add')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

export default SettingsPage
