import { useState, useEffect } from 'react'
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Input,
  Badge,
} from '@/components/ui'
import {
  useRetryConfigs,
  useUpdateRetryConfig,
  useCreateRetryConfig,
} from '@/hooks/queries'
import { Save, RefreshCw, AlertTriangle, ShieldCheck } from 'lucide-react'
import type { RetryConfig } from '@/lib/transport'

export function RetryConfigsPage() {
  const { data: configs, isLoading, refetch } = useRetryConfigs()
  const updateConfig = useUpdateRetryConfig()
  const createConfig = useCreateRetryConfig()

  const [defaultConfig, setDefaultConfig] = useState<RetryConfig | undefined>()
  const [hasChanges, setHasChanges] = useState(false)

  // Form state
  const [maxRetries, setMaxRetries] = useState('3')
  const [initialInterval, setInitialInterval] = useState('1000')
  const [backoffRate, setBackoffRate] = useState('2')
  const [maxInterval, setMaxInterval] = useState('30000')

  useEffect(() => {
    if (configs) {
      const def = configs.find(c => c.isDefault)
      if (def) {
        setDefaultConfig(def)
        // Only update form if not already edited or if it's the first load
        if (!hasChanges) {
          setMaxRetries(String(def.maxRetries))
          setInitialInterval(String(def.initialInterval / 1_000_000))
          setBackoffRate(String(def.backoffRate))
          setMaxInterval(String(def.maxInterval / 1_000_000))
        }
      }
    }
  }, [configs])

  const handleInputChange = (setter: (val: string) => void, value: string) => {
    setter(value)
    setHasChanges(true)
  }

  const handleSave = () => {
    const data = {
      name: defaultConfig?.name || 'Default Config',
      isDefault: true,
      maxRetries: Number(maxRetries),
      initialInterval: Number(initialInterval) * 1_000_000,
      backoffRate: Number(backoffRate),
      maxInterval: Number(maxInterval) * 1_000_000,
    }

    if (defaultConfig) {
      updateConfig.mutate(
        { id: defaultConfig.id, data },
        {
          onSuccess: () => {
            setHasChanges(false)
            refetch()
          },
        }
      )
    } else {
      // Create if doesn't exist
      createConfig.mutate(data, {
        onSuccess: () => {
          setHasChanges(false)
          refetch()
        },
      })
    }
  }

  const handleReset = () => {
    if (defaultConfig) {
      setMaxRetries(String(defaultConfig.maxRetries))
      setInitialInterval(String(defaultConfig.initialInterval / 1_000_000))
      setBackoffRate(String(defaultConfig.backoffRate))
      setMaxInterval(String(defaultConfig.maxInterval / 1_000_000))
      setHasChanges(false)
    }
  }

  const isSaving = updateConfig.isPending || createConfig.isPending

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex flex-col items-center gap-2">
          <RefreshCw className="h-8 w-8 animate-spin text-accent" />
          <p className="text-sm text-text-secondary">
            Loading configuration...
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full bg-background">
      {/* Header */}
      <div className="h-[73px] flex items-center justify-between px-6 border-b border-border bg-surface-primary shrink-0">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-accent rounded-lg">
            <ShieldCheck size={20} />
          </div>
          <div>
            <h2 className="text-lg font-semibold text-text-primary leading-tight">
              Retry Policy
            </h2>
            <p className="text-xs text-text-secondary">
              Configure global retry behavior for failed requests
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {hasChanges && (
            <Button variant="ghost" onClick={handleReset} disabled={isSaving}>
              Discard Changes
            </Button>
          )}
          <Button
            onClick={handleSave}
            disabled={!hasChanges || isSaving}
            className="gap-2"
          >
            {isSaving ? (
              <RefreshCw className="h-4 w-4 animate-spin" />
            ) : (
              <Save className="h-4 w-4" />
            )}
            Save Changes
          </Button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-6">
        <div className="space-y-6">
          {!defaultConfig && !isLoading && (
            <div className="bg-warning/10 border border-warning/20 rounded-lg p-4 flex items-start gap-3">
              <AlertTriangle className="h-5 w-5 text-warning shrink-0 mt-0.5" />
              <div>
                <h3 className="text-sm font-medium text-warning">
                  No Default Policy Found
                </h3>
                <p className="text-xs text-warning/80 mt-1">
                  There is currently no default retry policy configured. Saving
                  the settings below will create a new global default policy.
                </p>
              </div>
            </div>
          )}

          <Card className="border-border bg-surface-primary shadow-sm">
            <CardHeader className="border-b border-border pb-4">
              <div className="flex items-center justify-between">
                <CardTitle className="text-base font-medium">
                  Global Settings
                </CardTitle>
                <Badge variant="info">Default</Badge>
              </div>
            </CardHeader>
            <CardContent className="pt-6 space-y-8">
              {/* Max Retries */}
              <div className="grid gap-2">
                <label className="text-sm font-medium text-text-primary">
                  Max Retries
                </label>
                <p className="text-xs text-text-secondary mb-2">
                  Maximum number of retry attempts to make before giving up.
                </p>
                <div className="flex items-center gap-4">
                  <Input
                    type="number"
                    value={maxRetries}
                    onChange={e =>
                      handleInputChange(setMaxRetries, e.target.value)
                    }
                    min="0"
                    max="10"
                    className="max-w-[120px] font-mono"
                  />
                  <span className="text-xs text-text-muted">attempts</span>
                </div>
              </div>

              <div className="h-px bg-border/50" />

              {/* Timing Settings */}
              <div className="grid md:grid-cols-3 gap-6">
                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-primary">
                    Initial Interval
                  </label>
                  <p className="text-xs text-text-secondary min-h-[32px]">
                    Delay before the first retry attempt.
                  </p>
                  <div className="relative">
                    <Input
                      type="number"
                      value={initialInterval}
                      onChange={e =>
                        handleInputChange(setInitialInterval, e.target.value)
                      }
                      min="0"
                      className="font-mono pr-12"
                    />
                    <span className="absolute right-3 top-2.5 text-xs text-text-muted">
                      ms
                    </span>
                  </div>
                </div>

                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-primary">
                    Backoff Rate
                  </label>
                  <p className="text-xs text-text-secondary min-h-[32px]">
                    Multiplier applied to the interval after each retry.
                  </p>
                  <div className="relative">
                    <Input
                      type="number"
                      value={backoffRate}
                      onChange={e =>
                        handleInputChange(setBackoffRate, e.target.value)
                      }
                      min="1"
                      step="0.1"
                      className="font-mono pr-8"
                    />
                    <span className="absolute right-3 top-2.5 text-xs text-text-muted">
                      x
                    </span>
                  </div>
                </div>

                <div className="space-y-2">
                  <label className="text-sm font-medium text-text-primary">
                    Max Interval
                  </label>
                  <p className="text-xs text-text-secondary min-h-[32px]">
                    Maximum delay between retry attempts.
                  </p>
                  <div className="relative">
                    <Input
                      type="number"
                      value={maxInterval}
                      onChange={e =>
                        handleInputChange(setMaxInterval, e.target.value)
                      }
                      min="0"
                      className="font-mono pr-12"
                    />
                    <span className="absolute right-3 top-2.5 text-xs text-text-muted">
                      ms
                    </span>
                  </div>
                </div>
              </div>

              <div className="bg-surface-secondary/30 rounded-lg p-4 text-xs border border-border/50">
                <div className="text-text-muted mb-3">
                  Total attempts:{' '}
                  <span className="text-text-primary font-semibold">
                    {Number(maxRetries) + 1}
                  </span>{' '}
                  (1 initial + {maxRetries} retries)
                </div>
                <div className="space-y-1 font-mono text-text-secondary">
                  <div className="flex justify-between">
                    <span>Initial request</span>
                    <span className="text-text-primary">
                      Execute immediately
                    </span>
                  </div>
                  {Array.from(
                    { length: Math.min(Number(maxRetries), 5) },
                    (_, i) => {
                      const delay = Math.min(
                        Number(maxInterval),
                        Number(initialInterval) *
                          Math.pow(Number(backoffRate), i)
                      )
                      return (
                        <div key={i} className="flex justify-between">
                          <span>Retry {i + 1}</span>
                          <span className="text-text-primary">
                            Wait {delay.toFixed(0)}ms
                          </span>
                        </div>
                      )
                    }
                  )}
                  {Number(maxRetries) > 5 && (
                    <div className="text-text-muted">
                      ... and {Number(maxRetries) - 5} more retries
                    </div>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}
