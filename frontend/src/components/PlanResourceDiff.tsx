import { Text, VStack } from '@chakra-ui/react'
import type { PlanResource } from '../api/plans'

export default function PlanResourceDiff({ before, after, fields }: { before?: PlanResource; after?: PlanResource; fields?: string[] }) {
  const old = before?.value.value as unknown as Record<string, unknown> | undefined
  const next = after?.value.value as unknown as Record<string, unknown> | undefined
  const paths = fields ?? [...new Set([...Object.keys(old ?? {}), ...Object.keys(next ?? {})])]
  const ignored = new Set(['$typeName', 'id', 'createdAt', 'updatedAt', 'orgId'])
  const changes = paths.map(path => ({ path, field: path.replace(/_([a-z])/g, (_, c: string) => c.toUpperCase()) }))
    .filter(({ field }) => !ignored.has(field) && JSON.stringify(old?.[field]) !== JSON.stringify(next?.[field]))
  const display = (value: unknown) => {
    if (value === undefined || value === null || value === '') return 'empty'
    const text = typeof value === 'string' ? value : JSON.stringify(value)
    return text.length > 160 ? text.slice(0, 157) + '…' : text
  }
  return <VStack align="stretch" spacing={1}>
    {changes.map(({ path, field }) => <Text key={field} fontSize="xs" overflowWrap="anywhere">
      {path.replace(/_/g, ' ').replace(/([a-z])([A-Z])/g, '$1 $2')}: {display(old?.[field])} → {display(next?.[field])}
    </Text>)}
  </VStack>
}
