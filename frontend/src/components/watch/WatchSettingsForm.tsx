import {
  Box,
  Checkbox,
  FormControl,
  FormLabel,
  HStack,
  Input,
  NumberInput,
  NumberInputField,
  Select,
  SimpleGrid,
  Switch,
  Text,
  VStack,
} from '@chakra-ui/react'
import type { WatchEmbeddingConfig, WatchSettings, WatchSettingsDescriptor } from '../../api/client'
import {
  EMBEDDING_PROVIDER_OPTIONS,
  SCALE_STRATEGY_OPTIONS,
  WATCHER_OPTIONS,
  WATCH_LANGUAGE_OPTIONS,
  WATCH_SETTINGS_GROUPS,
  descriptorsByKey,
  durationSecondsToNanos,
  formatDurationNanos,
  nanosToDurationSeconds,
  ungroupedDescriptors,
} from './watchSettings'

type Props = {
  settings: WatchSettings
  embedding: WatchEmbeddingConfig
  descriptors: WatchSettingsDescriptor[]
  disabled?: boolean
  onChange: (next: { settings: WatchSettings; embedding: WatchEmbeddingConfig }) => void
}

export default function WatchSettingsForm({ settings, embedding, descriptors, disabled, onChange }: Props) {
  const byKey = descriptorsByKey(descriptors)

  const updateSettings = (patch: Partial<WatchSettings>) => {
    onChange({ settings: { ...settings, ...patch }, embedding })
  }
  const updateEmbedding = (patch: Partial<WatchEmbeddingConfig>) => {
    onChange({ settings, embedding: { ...embedding, ...patch } })
  }

  return (
    <VStack align="stretch" spacing={6} w="full">
      {WATCH_SETTINGS_GROUPS.map((group) => (
        <VStack key={group.id} align="stretch" spacing={3}>
          <Box>
            <Text fontSize="xs" fontWeight="semibold" textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
              {group.label}
            </Text>
            {group.description && (
              <Text fontSize="xs" color="gray.500" mt={1}>
                {group.description}
              </Text>
            )}
          </Box>
          <SimpleGrid columns={{ base: 1, sm: 2 }} spacing={3}>
            {group.keys.map((key) => (
              <WatchSettingField
                key={key}
                settingKey={key}
                description={byKey[key]?.description ?? ''}
                settings={settings}
                embedding={embedding}
                disabled={disabled}
                onSettingsChange={updateSettings}
                onEmbeddingChange={updateEmbedding}
              />
            ))}
          </SimpleGrid>
        </VStack>
      ))}
      <WatchUngroupedFields
        descriptors={ungroupedDescriptors(descriptors)}
        settings={settings}
        embedding={embedding}
        disabled={disabled}
        onSettingsChange={updateSettings}
        onEmbeddingChange={updateEmbedding}
      />
    </VStack>
  )
}

type FieldProps = {
  settingKey: string
  description: string
  settings: WatchSettings
  embedding: WatchEmbeddingConfig
  disabled?: boolean
  onSettingsChange: (patch: Partial<WatchSettings>) => void
  onEmbeddingChange: (patch: Partial<WatchEmbeddingConfig>) => void
}

function WatchUngroupedFields({
  descriptors,
  settings,
  embedding,
  disabled,
  onSettingsChange,
  onEmbeddingChange,
}: Omit<FieldProps, 'settingKey' | 'description'> & { descriptors: WatchSettingsDescriptor[] }) {
  if (descriptors.length === 0) {
    return null
  }
  return (
    <VStack align="stretch" spacing={3}>
      <Text fontSize="xs" fontWeight="semibold" textTransform="uppercase" letterSpacing="0.12em" color="gray.400">
        Other
      </Text>
      <SimpleGrid columns={{ base: 1, sm: 2 }} spacing={3}>
        {descriptors.map((descriptor) => (
          <WatchSettingField
            key={descriptor.key}
            settingKey={descriptor.key}
            description={descriptor.description ?? ''}
            settings={settings}
            embedding={embedding}
            disabled={disabled}
            onSettingsChange={onSettingsChange}
            onEmbeddingChange={onEmbeddingChange}
          />
        ))}
      </SimpleGrid>
    </VStack>
  )
}

function WatchSettingField({ settingKey, description, settings, embedding, disabled, onSettingsChange, onEmbeddingChange }: FieldProps) {
  const lspCommandMatch = /^watch\.lsp\.commands\.(.+)$/.exec(settingKey)
  const label = humanizeKey(settingKey)

  if (settingKey === 'watch.languages') {
    return (
      <SettingShell label={label} description={description}>
        <HStack spacing={3} flexWrap="wrap">
          {WATCH_LANGUAGE_OPTIONS.map((language) => (
            <Checkbox
              key={language}
              size="sm"
              isDisabled={disabled}
              isChecked={settings.languages.includes(language)}
              onChange={(event) => {
                const next = event.target.checked
                  ? [...settings.languages, language].sort()
                  : settings.languages.filter((item) => item !== language)
                onSettingsChange({ languages: next })
              }}
            >
              <Text fontSize="xs" color="gray.300">
                {language}
              </Text>
            </Checkbox>
          ))}
        </HStack>
      </SettingShell>
    )
  }

  if (lspCommandMatch) {
    const language = lspCommandMatch[1]
    return (
      <SettingShell label={label} description={description}>
        <Input
          size="sm"
          isDisabled={disabled}
          value={settings.lsp.commands?.[language] ?? ''}
          placeholder="e.g. gopls"
          onChange={(event) =>
            onSettingsChange({
              lsp: { ...settings.lsp, commands: { ...(settings.lsp.commands ?? {}), [language]: event.target.value } },
            })
          }
        />
      </SettingShell>
    )
  }

  switch (settingKey) {
    case 'watch.watcher':
      return (
        <SettingShell label={label} description={description}>
          <Select size="sm" isDisabled={disabled} value={settings.watcher} onChange={(e) => onSettingsChange({ watcher: e.target.value })}>
            {WATCHER_OPTIONS.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </Select>
        </SettingShell>
      )
    case 'watch.poll_interval':
    case 'watch.debounce':
      return (
        <SettingShell label={label} description={description}>
          <DurationField
            disabled={disabled}
            nanos={settingKey === 'watch.poll_interval' ? settings.poll_interval : settings.debounce}
            onChange={(nanos) => onSettingsChange(settingKey === 'watch.poll_interval' ? { poll_interval: nanos } : { debounce: nanos })}
          />
        </SettingShell>
      )
    case 'watch.scale.strategy':
      return (
        <SettingShell label={label} description={description}>
          <Select size="sm" isDisabled={disabled} value={settings.scale.strategy} onChange={(e) => onSettingsChange({ scale: { ...settings.scale, strategy: e.target.value } })}>
            {SCALE_STRATEGY_OPTIONS.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </Select>
        </SettingShell>
      )
    case 'watch.dependencies.enabled':
      return (
        <SettingShell label={label} description={description}>
          <Switch
            size="sm"
            isDisabled={disabled}
            isChecked={settings.dependencies.enabled}
            onChange={(e) => onSettingsChange({ dependencies: { enabled: e.target.checked } })}
          />
        </SettingShell>
      )
    case 'watch.lsp.enabled':
      return (
        <SettingShell label={label} description={description}>
          <Switch size="sm" isDisabled={disabled} isChecked={settings.lsp.enabled} onChange={(e) => onSettingsChange({ lsp: { ...settings.lsp, enabled: e.target.checked } })} />
        </SettingShell>
      )
    case 'watch.lsp.health_interval':
      return (
        <SettingShell label={label} description={description}>
          <DurationField
            disabled={disabled}
            nanos={settings.lsp.health_interval}
            onChange={(nanos) => onSettingsChange({ lsp: { ...settings.lsp, health_interval: nanos } })}
          />
        </SettingShell>
      )
    case 'watch.lsp.memory_limit_bytes':
      return (
        <SettingShell label={label} description={description}>
          <NumberField
            disabled={disabled}
            value={settings.lsp.memory_limit_bytes}
            onChange={(value) => onSettingsChange({ lsp: { ...settings.lsp, memory_limit_bytes: value } })}
          />
        </SettingShell>
      )
    case 'watch.embedding.provider':
      return (
        <SettingShell label={label} description={description}>
          <Select size="sm" isDisabled={disabled} value={embedding.provider} onChange={(e) => onEmbeddingChange({ provider: e.target.value })}>
            {EMBEDDING_PROVIDER_OPTIONS.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </Select>
        </SettingShell>
      )
    case 'watch.embedding.endpoint':
      return (
        <SettingShell label={label} description={description}>
          <Input
            size="sm"
            isDisabled={disabled}
            value={embedding.endpoint ?? ''}
            placeholder="http://127.0.0.1:8000/v1/embeddings"
            onChange={(e) => onEmbeddingChange({ endpoint: e.target.value })}
          />
        </SettingShell>
      )
    case 'watch.embedding.model':
      return (
        <SettingShell label={label} description={description}>
          <Input size="sm" isDisabled={disabled} value={embedding.model} onChange={(e) => onEmbeddingChange({ model: e.target.value })} />
        </SettingShell>
      )
    case 'watch.embedding.dimension':
      return (
        <SettingShell label={label} description={description}>
          <NumberField disabled={disabled} value={embedding.dimension} onChange={(value) => onEmbeddingChange({ dimension: value })} />
        </SettingShell>
      )
    case 'watch.embedding.max_tokens':
      return (
        <SettingShell label={label} description={description}>
          <NumberField disabled={disabled} value={embedding.max_tokens ?? 0} onChange={(value) => onEmbeddingChange({ max_tokens: value })} />
        </SettingShell>
      )
    case 'watch.embedding.runtime_path':
      return (
        <SettingShell label={label} description={description}>
          <Input size="sm" isDisabled={disabled} value={embedding.runtime_path ?? ''} onChange={(e) => onEmbeddingChange({ runtime_path: e.target.value })} />
        </SettingShell>
      )
    case 'watch.embedding.health_threshold':
      return (
        <SettingShell label={label} description={description}>
          <NumberField
            disabled={disabled}
            step={0.05}
            value={embedding.health_threshold ?? 0}
            onChange={(value) => onEmbeddingChange({ health_threshold: value })}
          />
        </SettingShell>
      )
    default:
      if (settingKey.startsWith('watch.thresholds.')) {
        const field = settingKey.replace('watch.thresholds.', '') as keyof WatchSettings['thresholds']
        return (
          <SettingShell label={label} description={description}>
            <NumberField
              disabled={disabled}
              value={settings.thresholds[field]}
              onChange={(value) => onSettingsChange({ thresholds: { ...settings.thresholds, [field]: value } })}
            />
          </SettingShell>
        )
      }
      if (settingKey.startsWith('watch.scale.')) {
        const field = settingKey.replace('watch.scale.', '') as keyof WatchSettings['scale']
        if (field === 'strategy') return null
        return (
          <SettingShell label={label} description={description}>
            <NumberField
              disabled={disabled}
              value={settings.scale[field]}
              onChange={(value) => onSettingsChange({ scale: { ...settings.scale, [field]: value } })}
            />
          </SettingShell>
        )
      }
      if (settingKey === 'watch.visibility.core_threshold_enabled') {
        return (
          <SettingShell label={label} description={description}>
            <Switch
              size="sm"
              isDisabled={disabled}
              isChecked={settings.visibility.core_threshold_enabled}
              onChange={(e) => onSettingsChange({ visibility: { ...settings.visibility, core_threshold_enabled: e.target.checked } })}
            />
          </SettingShell>
        )
      }
      if (settingKey.startsWith('watch.visibility.weights.')) {
        const field = settingKey.replace('watch.visibility.weights.', '') as keyof WatchSettings['visibility']['weights']
        return (
          <SettingShell label={label} description={description}>
            <NumberField
              disabled={disabled}
              step={0.5}
              value={settings.visibility.weights[field]}
              onChange={(value) => onSettingsChange({ visibility: { ...settings.visibility, weights: { ...settings.visibility.weights, [field]: value } } })}
            />
          </SettingShell>
        )
      }
      if (settingKey.startsWith('watch.visibility.')) {
        const field = settingKey.replace('watch.visibility.', '') as keyof Pick<WatchSettings['visibility'], 'core_threshold' | 'tier_multiplier' | 'max_expansion_multiplier'>
        return (
          <SettingShell label={label} description={description}>
            <NumberField
              disabled={disabled}
              step={0.1}
              value={settings.visibility[field]}
              onChange={(value) => onSettingsChange({ visibility: { ...settings.visibility, [field]: value } })}
            />
          </SettingShell>
        )
      }
      if (settingKey.startsWith('watch.layout.')) {
        return (
          <SettingShell label={label} description={description}>
            <Input size="sm" isDisabled value="global config" readOnly />
          </SettingShell>
        )
      }
      return (
        <SettingShell label={label} description={description}>
          <Input size="sm" isDisabled value="see tld config set" readOnly />
        </SettingShell>
      )
  }
}

function SettingShell({ label, description, children }: { label: string; description: string; children: React.ReactNode }) {
  return (
    <FormControl>
      <FormLabel fontSize="xs" color="gray.400" mb={1} title={description}>
        {label}
      </FormLabel>
      {children}
    </FormControl>
  )
}

function NumberField({ value, onChange, disabled, step }: { value: number; onChange: (value: number) => void; disabled?: boolean; step?: number }) {
  return (
    <NumberInput size="sm" isDisabled={disabled} value={value} min={0} step={step ?? 1} onChange={(_value, valueAsNumber) => onChange(Number.isNaN(valueAsNumber) ? 0 : valueAsNumber)}>
      <NumberInputField />
    </NumberInput>
  )
}

function DurationField({ nanos, onChange, disabled }: { nanos: number; onChange: (nanos: number) => void; disabled?: boolean }) {
  const seconds = nanosToDurationSeconds(nanos)
  return (
    <HStack spacing={2}>
      <NumberInput
        size="sm"
        isDisabled={disabled}
        value={seconds}
        min={0}
        step={0.5}
        onChange={(_value, valueAsNumber) => onChange(durationSecondsToNanos(Number.isNaN(valueAsNumber) ? 0 : valueAsNumber))}
      >
        <NumberInputField />
      </NumberInput>
      <Text fontSize="xs" color="gray.500" whiteSpace="nowrap">
        {formatDurationNanos(nanos)}
      </Text>
    </HStack>
  )
}

function humanizeKey(key: string): string {
  return key
    .replace(/^watch\./, '')
    .split('.')
    .map((part) => part.replace(/_/g, ' '))
    .map((part) => (part.length > 0 ? part[0].toUpperCase() + part.slice(1) : part))
    .join(' · ')
}
