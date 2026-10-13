import { useEffect, useRef, useState } from 'react'
import {
  Badge,
  Box,
  Button,
  Checkbox,
  Flex,
  HStack,
  IconButton,
  Input,
  InputGroup,
  InputLeftElement,
  InputRightElement,
  Menu,
  MenuButton,
  MenuItem,
  MenuList,
  MenuDivider,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Popover,
  PopoverBody,
  PopoverFooter,
  PopoverHeader,
  PopoverContent,
  PopoverTrigger,
  Portal,
  Select,
  Spinner,
  Table,
  Tbody,
  Td,
  Text,
  Th,
  Thead,
  Tr,
  Tabs,
  TabList,
  Tab,
  Tag,
  TagCloseButton,
  TagLabel,
  useBreakpointValue,
} from '@chakra-ui/react'
import {
  AddIcon,
  ChevronDownIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  DeleteIcon,
  EditIcon,
  RepeatIcon,
  SearchIcon,
  SmallCloseIcon,
  TriangleDownIcon,
  TriangleUpIcon,
} from '@chakra-ui/icons'
import type { InventoryRow, InventoryObjectType } from './inventoryData'
import { filterInventoryRows, getInventoryQualityOptions } from './inventoryData'
import { INVENTORY_PAGE_SIZES, type InventoryTableState } from './useInventoryTable'
import { inventoryCreatedAt, inventoryKindLabel } from './inventoryColumns'
import { resolveElementIconUrl } from '../utils/elementIcon'
import { TYPE_COLORS } from '../types'

const types = [
  ['all', 'All'],
  ['elements', 'Elements'],
  ['views', 'Views'],
  ['connectors', 'Connectors'],
] as const

type Props = {
  state: InventoryTableState
  rows: InventoryRow[]
  loading: boolean
  error: string | null
  busy: boolean
  availableTags: string[]
  tagColorMap: Record<string, { color: string }>
  groupTagMeta: Record<string, { label: string }>
  onRefresh: () => void
  onEdit: (row: InventoryRow) => void
  onCreate: (type: InventoryObjectType) => void
  onDelete: (rows: InventoryRow[]) => Promise<boolean>
  onTag: (tag: string, remove: boolean) => void
  onDeleteTag: (tag: string) => Promise<void>
}

export default function InventoryList({
  state,
  rows,
  loading,
  error,
  busy,
  availableTags,
  tagColorMap,
  groupTagMeta,
  onRefresh,
  onEdit,
  onCreate,
  onDelete,
  onTag,
  onDeleteTag,
}: Props) {
  const {
    table,
    query,
    filters,
    selectedType,
    selectedRow,
    selectedRows,
    updateParams,
    toggleFilter,
    resetFilters,
  } = state
  const mobile = useBreakpointValue({ base: true, md: false }) ?? false
  const searchRef = useRef<HTMLInputElement>(null)
  const [deleteRows, setDeleteRows] = useState<InventoryRow[] | null>(null)
  const [deleteTag, setDeleteTag] = useState<string | null>(null)
  const [tagBusy, setTagBusy] = useState(false)
  const [tagError, setTagError] = useState('')
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === 'k') {
        event.preventDefault()
        searchRef.current?.focus()
        searchRef.current?.select()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  const active = filters.tags.length + filters.qualities.length + Number(Boolean(filters.kind))
  const total = table.getFilteredRowModel().rows.length
  const page = table.getState().pagination
  const pageRows = table.getRowModel().rows
  const hiddenSelected = selectedRows.length - table.getFilteredSelectedRowModel().rows.length
  const selectedTags = Array.from(new Set(selectedRows.flatMap((row) => row.tags))).sort()
  const facets = (key: 'tags' | 'kind' | 'qualities') =>
    filterInventoryRows(rows, { ...filters, [key]: key === 'kind' ? '' : [] })
  const tagRows = facets('tags')
  const kindRows = facets('kind')
  const qualityRows = facets('qualities')
  const kindOptions = Array.from(new Set(kindRows.map((row) => row.typeLabel))).sort()
  const visibleColumns = table.getVisibleLeafColumns().filter((column) => !mobile || column.id === 'name')

  const renderTagFilter = (trigger: 'button' | 'icon' = 'button') => (
    <FilterPicker
      trigger={trigger}
      title="Tags"
      selected={filters.tags}
      options={availableTags.map((tag) => ({
        value: tag,
        label: groupTagMeta[tag]?.label ?? tag,
        count: tagRows.filter((row) => row.filterTags.includes(tag)).length,
      }))}
      onToggle={(tag) => toggleFilter('tags', tag)}
      onDelete={(tag) => {
        setDeleteTag(tag)
        setTagError('')
      }}
    />
  )
  const renderKindFilter = (trigger: 'button' | 'icon' = 'button') => (
    <FilterPicker
      trigger={trigger}
      title={inventoryKindLabel(selectedType)}
      filterId="kind"
      multiple={false}
      selected={filters.kind ? [filters.kind] : []}
      options={kindOptions.map((kind) => ({
        value: kind,
        label: kind,
        count: kindRows.filter((row) => row.typeLabel === kind).length,
      }))}
      onToggle={(kind) => updateParams({ kind: kind === filters.kind ? '' : kind })}
    />
  )

  const pageCheckbox = (
    <Checkbox
      aria-label="Select this page"
      isChecked={table.getIsAllPageRowsSelected() && pageRows.length > 0}
      isIndeterminate={table.getIsSomePageRowsSelected()}
      onChange={table.getToggleAllPageRowsSelectedHandler()}
      isDisabled={busy || loading || !pageRows.length}
    />
  )
  const mobileSort = (
    <Select
      aria-label="Sort inventory"
      size="sm"
      w="full"
      value={`${table.getState().sorting[0].id}:${table.getState().sorting[0].desc ? 'desc' : 'asc'}`}
      onChange={(event) => {
        const [id, direction] = event.target.value.split(':')
        table.setSorting([{ id, desc: direction === 'desc' }])
      }}
    >
      {table
        .getAllLeafColumns()
        .filter((column) => column.getCanSort())
        .flatMap((column) =>
          (['asc', 'desc'] as const).map((direction) => (
            <option key={`${column.id}:${direction}`} value={`${column.id}:${direction}`}>
              {String(column.columnDef.header)} {direction === 'asc' ? '↑' : '↓'}
            </option>
          )),
        )}
    </Select>
  )

  return (
    <Flex
      direction="column"
      flex={{ base: '0 0 auto', md: 1 }}
      h={{ base: 'calc(100dvh - 80px)', md: 'auto' }}
      minH={{ base: '460px', md: 0 }}
      minW={0}
    >
      <Box px={{ base: 3, md: 5 }} pt={4} pb={3} borderBottom="1px solid" borderColor="var(--border-main)">
        <Flex gap={2} align="center" flexWrap="wrap">
          <InputGroup size="sm" flex="1" minW={{ base: '100%', sm: '220px' }}>
            <InputLeftElement pointerEvents="none">
              <SearchIcon color="gray.400" />
            </InputLeftElement>
            <Input
              ref={searchRef}
              aria-label="Search inventory"
              data-testid="inventory-search"
              placeholder="Search names, descriptions, tags…"
              value={query}
              onChange={(event) => updateParams({ q: event.target.value })}
              onKeyDown={(event) => {
                if (event.key === 'Escape') updateParams({ q: '' })
              }}
              pr={8}
            />
            {query && (
              <InputRightElement>
                <IconButton
                  aria-label="Clear search"
                  size="xs"
                  variant="ghost"
                  icon={<SmallCloseIcon />}
                  onClick={() => {
                    updateParams({ q: '' })
                    searchRef.current?.focus()
                  }}
                />
              </InputRightElement>
            )}
          </InputGroup>
          {(mobile || !table.getColumn('tags')?.getIsVisible()) && renderTagFilter()}
          {(mobile || !table.getColumn('type')?.getIsVisible()) && renderKindFilter()}
          <FilterPicker
            title="Quality"
            selected={filters.qualities}
            options={getInventoryQualityOptions(selectedType).map((quality) => ({
              value: quality.value,
              label: quality.value,
              description: quality.description,
              count: qualityRows.filter((row) => row.qualityFlags.includes(quality.value)).length,
            }))}
            onToggle={(quality) => toggleFilter('qualities', quality)}
          />
          {!mobile && (
            <Menu closeOnSelect={false}>
              <MenuButton as={Button} size="sm" variant="outline" rightIcon={<ChevronDownIcon />}>
                Columns
              </MenuButton>
              <Portal>
                <MenuList maxH="min(70vh, 480px)" overflowY="auto">
                  {table
                    .getAllLeafColumns()
                    .filter((column) => column.id !== 'name')
                    .map((column) => (
                      <MenuItem key={column.id} onClick={() => column.toggleVisibility()}>
                        <Checkbox pointerEvents="none" isChecked={column.getIsVisible()} mr={2} />
                        {String(column.columnDef.header)}
                      </MenuItem>
                    ))}
                  <MenuDivider />
                  <MenuItem onClick={state.resetColumns}>Reset columns</MenuItem>
                </MenuList>
              </Portal>
            </Menu>
          )}
        </Flex>
        {(active > 0 || query) && (
          <Flex gap={2} align="center" flexWrap="wrap" mt={3}>
            {filters.tags.map((tag) => (
              <FilterChip
                key={tag}
                label={`tag: ${groupTagMeta[tag]?.label ?? tag}`}
                onRemove={() => toggleFilter('tags', tag)}
              />
            ))}
            {filters.kind && (
              <FilterChip
                label={`${inventoryKindLabel(selectedType).toLowerCase()}: ${filters.kind}`}
                onRemove={() => updateParams({ kind: '' })}
              />
            )}
            {filters.qualities.map((quality) => (
              <FilterChip key={quality} label={quality} onRemove={() => toggleFilter('qualities', quality)} />
            ))}
            <Button size="xs" variant="ghost" onClick={resetFilters}>
              Clear all
            </Button>
          </Flex>
        )}
        <Flex align="center" flexWrap="wrap" gap={2} mt={3}>
          <Tabs
            index={types.findIndex(([value]) => value === selectedType)}
            onChange={(index) => updateParams({ type: types[index][0] === 'all' ? '' : types[index][0] })}
            variant="soft-rounded"
            colorScheme="blue"
            size="sm"
            flex={{ base: '1 0 100%', md: 1 }}
            minW={0}
          >
            <TabList aria-label="Inventory object type" gap={1}>
              {types.map(([value, label]) => (
                <Tab
                  key={value}
                  data-testid={`inventory-tab-${value}`}
                  flex={{ base: 1, md: 'initial' }}
                  px={{ base: 2, md: 3 }}
                  borderRadius="md"
                  color="gray.300"
                  _selected={{ bg: 'rgba(var(--accent-rgb), 0.18)', color: 'var(--accent)' }}
                  _hover={{ bg: 'rgba(var(--accent-rgb), 0.10)' }}
                >
                  {label}
                  <Badge ml={2} display={{ base: 'none', sm: 'inline' }}>
                    {value === 'all'
                      ? rows.length
                      : rows.filter((row) => `${row.objectType}s` === value).length}
                  </Badge>
                </Tab>
              ))}
            </TabList>
          </Tabs>
          <HStack spacing={1} ml="auto" flexShrink={0}>
            <IconButton
              aria-label="Refresh inventory"
              icon={<RepeatIcon />}
              variant="ghost"
              size="sm"
              onClick={onRefresh}
              isDisabled={loading || busy}
            />
            <Menu>
              <MenuButton
                as={Button}
                size="sm"
                leftIcon={<AddIcon />}
                rightIcon={<ChevronDownIcon />}
                colorScheme="blue"
                data-testid="inventory-new"
                isDisabled={loading || busy}
              >
                New
              </MenuButton>
              <Portal>
                <MenuList>
                  {(['element', 'view', 'connector'] as const).map((type) => (
                    <MenuItem key={type} onClick={() => onCreate(type)}>
                      New {type}
                    </MenuItem>
                  ))}
                </MenuList>
              </Portal>
            </Menu>
          </HStack>
        </Flex>
      </Box>
      {error && (
        <Flex role="alert" px={4} py={2} gap={2} align="center" bg="red.900">
          <Text flex={1} fontSize="sm">
            {error}
          </Text>
          <Button size="xs" onClick={onRefresh}>
            Retry
          </Button>
        </Flex>
      )}
      <Box flex={1} minH={0} overflow="auto" position="relative" aria-busy={loading}>
        {loading && !rows.length ? (
          <Flex h="200px" align="center" justify="center" gap={3}>
            <Spinner />
            <Text>Loading inventory…</Text>
          </Flex>
        ) : (
          <>
            {loading && (
              <Text role="status" px={4} py={1} fontSize="xs">
                Refreshing inventory…
              </Text>
            )}
            <Table
              size="sm"
              variant="simple"
              sx={{ tableLayout: 'fixed' }}
              w="full"
              minW={mobile ? '0' : `${visibleColumns.reduce((sum, column) => sum + column.getSize(), 100)}px`}
            >
              {mobile && (
                <colgroup>
                  <col style={{ width: 40 }} />
                  <col />
                  <col style={{ width: 48 }} />
                </colgroup>
              )}
              {mobile && (
                <Thead position="sticky" top={0} zIndex={1} bg="rgb(var(--bg-main-rgb))">
                  <Tr>
                    <Th w="40px" px={3} aria-label="Selection">
                      {pageCheckbox}
                    </Th>
                    <Th px={3}>{mobileSort}</Th>
                    <Th w="48px" aria-label="Actions" />
                  </Tr>
                </Thead>
              )}
              {!mobile && (
                <Thead position="sticky" top={0} zIndex={1} bg="rgb(var(--bg-main-rgb))">
                  <Tr>
                    <Th w="44px" px={3} aria-label="Selection">
                      {pageCheckbox}
                    </Th>
                    {visibleColumns.map((column) => (
                      <Th
                        key={column.id}
                        position="relative"
                        w={`${column.getSize()}px`}
                        px={3}
                        aria-sort={
                          column.getIsSorted() === 'asc'
                            ? 'ascending'
                            : column.getIsSorted() === 'desc'
                              ? 'descending'
                              : 'none'
                        }
                      >
                        <HStack spacing={1} pr={1} w="full" minW={0}>
                          <Button
                            title={column.id === 'tags' ? 'Sort by tag count' : undefined}
                            size="xs"
                            variant="unstyled"
                            fontSize="11px"
                            textTransform="uppercase"
                            display="flex"
                            flex={1}
                            minW={0}
                            gap={column.id === 'type' || column.id === 'tags' ? 1 : 2}
                            alignItems="center"
                            onClick={column.getToggleSortingHandler()}
                            isDisabled={!column.getCanSort()}
                          >
                            <Box as="span" flex={1} textAlign="left" isTruncated>
                              {String(column.columnDef.header)}
                            </Box>
                            {column.getIsSorted() === 'asc' ? (
                              <TriangleUpIcon boxSize={2} />
                            ) : column.getIsSorted() === 'desc' ? (
                              <TriangleDownIcon boxSize={2} />
                            ) : null}
                          </Button>
                          {column.id === 'tags' && renderTagFilter('icon')}
                          {column.id === 'type' && renderKindFilter('icon')}
                        </HStack>
                        <Box
                          role="separator"
                          aria-label={`Resize ${column.columnDef.header} column`}
                          aria-orientation="vertical"
                          tabIndex={0}
                          position="absolute"
                          right={0}
                          top={0}
                          h="full"
                          w="8px"
                          cursor="col-resize"
                          sx={{ touchAction: 'none' }}
                          borderRight="1px solid"
                          borderColor="var(--border-main)"
                          onMouseDown={table
                            .getHeaderGroups()[0]
                            .headers.find((header) => header.column.id === column.id)
                            ?.getResizeHandler()}
                          onTouchStart={table
                            .getHeaderGroups()[0]
                            .headers.find((header) => header.column.id === column.id)
                            ?.getResizeHandler()}
                          onKeyDown={(event) => {
                            if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
                              event.preventDefault()
                              table.setColumnSizing((previous) => ({
                                ...previous,
                                [column.id]: Math.max(
                                  column.columnDef.minSize ?? 90,
                                  column.getSize() + (event.key === 'ArrowRight' ? 20 : -20),
                                ),
                              }))
                            }
                          }}
                        />
                      </Th>
                    ))}
                    <Th w="56px" aria-label="Actions" />
                  </Tr>
                </Thead>
              )}
              <Tbody>
                {pageRows.map((row) => (
                  <Tr
                    key={row.id}
                    data-testid="inventory-row"
                    data-inventory-key={row.id}
                    bg={
                      row.getIsSelected() || selectedRow?.key === row.id
                        ? 'rgba(var(--accent-rgb), 0.10)'
                        : 'transparent'
                    }
                    _hover={{ bg: 'rgba(var(--accent-rgb), 0.06)' }}
                  >
                    <Td w={mobile ? '36px' : '44px'} px={3} borderColor="var(--border-main)">
                      <Checkbox
                        aria-label={`Select ${row.original.name || row.original.subtitle}`}
                        data-testid={`inventory-row-select-${row.id}`}
                        isChecked={row.getIsSelected()}
                        onChange={row.getToggleSelectedHandler()}
                        isDisabled={busy || loading}
                      />
                    </Td>
                    {visibleColumns.map((column) => (
                      <Td key={column.id} px={3} py={3} borderColor="var(--border-main)">
                        {column.id === 'name' ? (
                          <Button
                            variant="unstyled"
                            display="block"
                            textAlign="left"
                            whiteSpace="normal"
                            w="full"
                            h="auto"
                            minW={0}
                            onClick={() => state.selectRow(row.original)}
                            aria-label={`Inspect ${row.original.name || row.original.subtitle}`}
                          >
                            <HStack align="start" spacing={3}>
                              <RowIcon row={row.original} />
                              <Box minW={0}>
                                <Text
                                  fontSize="sm"
                                  fontWeight="semibold"
                                  noOfLines={2}
                                  overflowWrap="anywhere"
                                >
                                  {row.original.name ||
                                    (selectedType === 'connectors'
                                      ? 'Unlabeled connector'
                                      : row.original.subtitle)}
                                </Text>
                                <Text
                                  fontSize="xs"
                                  fontWeight="normal"
                                  color="var(--chakra-colors-chakra-subtle-text)"
                                  noOfLines={1}
                                >
                                  {row.original.name ? row.original.subtitle : row.original.viewName}
                                </Text>
                                {mobile && (
                                  <Text
                                    fontSize="xs"
                                    fontWeight="normal"
                                    color="var(--chakra-colors-chakra-subtle-text)"
                                    mt={1}
                                    noOfLines={1}
                                  >
                                    {row.original.objectType} ·{' '}
                                    {selectedType === 'connectors'
                                      ? [row.original.connector?.direction, row.original.viewName]
                                          .filter(Boolean)
                                          .join(' · ')
                                      : row.original.usageLabel}
                                  </Text>
                                )}
                              </Box>
                            </HStack>
                          </Button>
                        ) : column.id === 'tags' ? (
                          <Flex gap={1} flexWrap="wrap">
                            {row.original.tags.slice(0, 2).map((tag) => (
                              <Button
                                key={tag}
                                size="xs"
                                h="auto"
                                py={1}
                                maxW="full"
                                variant="outline"
                                color={tagColorMap[tag]?.color}
                                onClick={() => toggleFilter('tags', tag)}
                                title={tag}
                              >
                                <Text isTruncated>{tag}</Text>
                              </Button>
                            ))}
                            {row.original.tags.length > 2 && (
                              <Menu>
                                <MenuButton as={Button} size="xs" variant="ghost">
                                  +{row.original.tags.length - 2}
                                </MenuButton>
                                <Portal>
                                  <MenuList>
                                    {row.original.tags.slice(2).map((tag) => (
                                      <MenuItem key={tag} onClick={() => toggleFilter('tags', tag)}>
                                        {tag}
                                      </MenuItem>
                                    ))}
                                  </MenuList>
                                </Portal>
                              </Menu>
                            )}
                          </Flex>
                        ) : (
                          <Text
                            title={formatCell(row.original, column.id, row.getValue(column.id))}
                            fontSize="xs"
                            color="var(--chakra-colors-chakra-subtle-text)"
                            noOfLines={2}
                          >
                            {formatCell(row.original, column.id, row.getValue(column.id))}
                          </Text>
                        )}
                      </Td>
                    ))}
                    <Td w="48px" px={1} borderColor="var(--border-main)">
                      <Menu>
                        <MenuButton
                          as={IconButton}
                          aria-label={`Actions for ${row.original.name || row.original.subtitle}`}
                          icon={<ChevronDownIcon />}
                          variant="ghost"
                          size="sm"
                          isDisabled={busy || loading}
                        />
                        <Portal>
                          <MenuList>
                            <MenuItem icon={<EditIcon />} onClick={() => onEdit(row.original)}>
                              Edit
                            </MenuItem>
                            <MenuItem onClick={() => state.selectRow(row.original)}>Inspect</MenuItem>
                            <MenuItem
                              color="red.400"
                              icon={<DeleteIcon />}
                              onClick={() => setDeleteRows([row.original])}
                            >
                              Delete
                            </MenuItem>
                          </MenuList>
                        </Portal>
                      </Menu>
                    </Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
            {!total && !loading && (
              <Flex px={6} py={10} direction="column" align="center" textAlign="center" gap={3}>
                <SearchIcon boxSize={6} color="var(--chakra-colors-chakra-subtle-text)" />
                <Text fontWeight="semibold">
                  {query || active ? 'No matching objects' : 'Nothing here yet'}
                </Text>
                <Text fontSize="sm" color="var(--chakra-colors-chakra-subtle-text)">
                  {query
                    ? `No results for “${query}”. Try another search or clear your filters.`
                    : active
                      ? 'Try adjusting your filters.'
                      : 'Create an element or view to get started.'}
                </Text>
                {(query || active > 0) && (
                  <Button size="sm" onClick={resetFilters}>
                    Clear all
                  </Button>
                )}
              </Flex>
            )}
          </>
        )}
      </Box>
      <Flex
        px={3}
        py={2}
        gap={2}
        align="center"
        flexWrap="wrap"
        borderTop="1px solid"
        borderColor="var(--border-main)"
      >
        {selectedRows.length > 0 && (
          <Flex
            gap={2}
            align="center"
            flexWrap="wrap"
            flex="1 1 auto"
            minW="min(100%, 520px)"
            role="toolbar"
            aria-label="Selected object actions"
          >
            <Text fontSize="sm" fontWeight="semibold">
              {selectedRows.length} selected
            </Text>
            {hiddenSelected > 0 && (
              <Text fontSize="xs" color="orange.300">
                {hiddenSelected} outside current filters
              </Text>
            )}
            <Button
              size="xs"
              variant="ghost"
              onClick={() =>
                state.setRowSelection(
                  Object.fromEntries(table.getFilteredRowModel().rows.map((row) => [row.id, true])),
                )
              }
              isDisabled={busy}
            >
              Select all {total} matching
            </Button>
            <Menu>
              <MenuButton
                as={Button}
                size="xs"
                variant="outline"
                data-testid="inventory-bulk-add-tag"
                isDisabled={busy}
              >
                Add tag
              </MenuButton>
              <Portal>
                <MenuList maxH="260px" overflowY="auto">
                  {availableTags
                    .filter((tag) => !groupTagMeta[tag])
                    .map((tag) => (
                      <MenuItem
                        key={tag}
                        data-testid={`inventory-bulk-add-tag-${tag}`}
                        onClick={() => onTag(tag, false)}
                      >
                        {tag}
                      </MenuItem>
                    ))}
                  {availableTags.every((tag) => groupTagMeta[tag]) && (
                    <MenuItem isDisabled>No tags available</MenuItem>
                  )}
                </MenuList>
              </Portal>
            </Menu>
            {!!selectedTags.length && (
              <Menu>
                <MenuButton
                  as={Button}
                  size="xs"
                  variant="outline"
                  data-testid="inventory-bulk-remove-tag"
                  isDisabled={busy}
                >
                  Remove tag
                </MenuButton>
                <Portal>
                  <MenuList maxH="260px" overflowY="auto">
                    {selectedTags.map((tag) => (
                      <MenuItem
                        key={tag}
                        data-testid={`inventory-bulk-remove-tag-${tag}`}
                        onClick={() => onTag(tag, true)}
                      >
                        {tag}
                      </MenuItem>
                    ))}
                  </MenuList>
                </Portal>
              </Menu>
            )}
            <IconButton
              aria-label="Delete selected"
              data-testid="inventory-bulk-delete"
              icon={<DeleteIcon />}
              size="sm"
              colorScheme="red"
              variant="ghost"
              isDisabled={busy}
              onClick={() => setDeleteRows(selectedRows)}
            />
            <IconButton
              aria-label="Clear selection"
              icon={<SmallCloseIcon />}
              size="sm"
              variant="ghost"
              isDisabled={busy}
              onClick={() => table.resetRowSelection()}
            />
            {busy && <Spinner size="sm" />}
          </Flex>
        )}
        {selectedRows.length === 0 && (
          <Text
            data-testid="inventory-pagination-summary"
            aria-live="polite"
            fontSize="xs"
            color="var(--chakra-colors-chakra-subtle-text)"
            flex={1}
            whiteSpace="nowrap"
          >
            {total ? page.pageIndex * page.pageSize + 1 : 0}–
            {Math.min((page.pageIndex + 1) * page.pageSize, total)} of {total}
          </Text>
        )}
        <HStack spacing={2} ml="auto" flexShrink={0}>
          <Select
            size="sm"
            w="120px"
            aria-label="Rows per page"
            value={page.pageSize}
            onChange={(event) => state.setPageSize(Number(event.target.value))}
          >
            {INVENTORY_PAGE_SIZES.map((size) => (
              <option key={size} value={size}>
                {size} / page
              </option>
            ))}
          </Select>
          <IconButton
            aria-label="Previous page"
            icon={<ChevronLeftIcon />}
            size="sm"
            variant="ghost"
            isDisabled={!table.getCanPreviousPage() || loading}
            onClick={() => state.setPage(page.pageIndex - 1)}
          />
          <Text fontSize="xs">
            {page.pageIndex + 1} / {Math.max(1, table.getPageCount())}
          </Text>
          <IconButton
            aria-label="Next page"
            icon={<ChevronRightIcon />}
            size="sm"
            variant="ghost"
            isDisabled={!table.getCanNextPage() || loading}
            onClick={() => state.setPage(page.pageIndex + 1)}
          />
        </HStack>
      </Flex>
      <Modal
        isOpen={!!deleteRows}
        onClose={() => {
          if (!busy) setDeleteRows(null)
        }}
        isCentered
        size="sm"
        closeOnOverlayClick={!busy}
        closeOnEsc={!busy}
      >
        <ModalOverlay />
        <ModalContent mx={3} data-testid="inventory-bulk-delete-confirm">
          <ModalHeader>
            Delete {deleteRows?.length === 1 ? 'object' : `${deleteRows?.length} objects`}?
          </ModalHeader>
          <ModalBody>
            <Text>
              {deleteRows?.length === 1
                ? `Delete “${deleteRows[0].name || deleteRows[0].subtitle}”?`
                : 'Delete the selected objects?'}{' '}
              This cannot be undone. Related placements and connectors may also be removed.
            </Text>
          </ModalBody>
          <ModalFooter gap={2}>
            <Button onClick={() => setDeleteRows(null)} isDisabled={busy}>
              Cancel
            </Button>
            <Button
              colorScheme="red"
              data-testid="inventory-bulk-delete-submit"
              isLoading={busy}
              onClick={async () => {
                if (deleteRows) {
                  await onDelete(deleteRows)
                  setDeleteRows(null)
                }
              }}
            >
              Delete
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
      <Modal
        isOpen={!!deleteTag}
        onClose={() => {
          if (!tagBusy) setDeleteTag(null)
        }}
        isCentered
        size="sm"
        closeOnOverlayClick={!tagBusy}
        closeOnEsc={!tagBusy}
      >
        <ModalOverlay />
        <ModalContent mx={3}>
          <ModalHeader>Delete {deleteTag && groupTagMeta[deleteTag] ? 'group' : 'tag'}?</ModalHeader>
          <ModalBody>
            Remove “{deleteTag && (groupTagMeta[deleteTag]?.label ?? deleteTag)}” from all objects? Other tags
            will be kept.
            {tagError && (
              <Text role="alert" color="red.400" mt={2}>
                {tagError}
              </Text>
            )}
          </ModalBody>
          <ModalFooter gap={2}>
            <Button isDisabled={tagBusy} onClick={() => setDeleteTag(null)}>
              Cancel
            </Button>
            <Button
              colorScheme="red"
              isLoading={tagBusy}
              onClick={async () => {
                if (!deleteTag) return
                setTagBusy(true)
                try {
                  await onDeleteTag(deleteTag)
                  setDeleteTag(null)
                } catch {
                  setTagError('Could not delete this tag. Please retry.')
                } finally {
                  setTagBusy(false)
                }
              }}
            >
              Delete
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </Flex>
  )
}

function FilterChip({ label, onRemove }: { label: string; onRemove: () => void }) {
  return (
    <Tag size="sm" maxW="full">
      <TagLabel noOfLines={1}>{label}</TagLabel>
      <TagCloseButton aria-label={`Remove ${label} filter`} onClick={onRemove} />
    </Tag>
  )
}

function FilterPicker({
  filterId,
  multiple = true,
  trigger = 'button',
  title,
  options,
  selected,
  onToggle,
  onDelete,
}: {
  filterId?: string
  multiple?: boolean
  trigger?: 'button' | 'icon'
  title: string
  options: { value: string; label: string; count: number; description?: string }[]
  selected: string[]
  onToggle: (value: string) => void
  onDelete?: (value: string) => void
}) {
  const filterKey = filterId ?? title.toLowerCase()
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const matches = options.filter((option) => option.label.toLowerCase().includes(search.trim().toLowerCase()))
  return (
    <Popover
      placement="bottom-start"
      isLazy
      isOpen={open}
      initialFocusRef={searchInputRef}
      onOpen={() => setOpen(true)}
      onClose={() => setOpen(false)}
    >
      <PopoverTrigger>
        {trigger === 'icon' ? (
          <IconButton
            data-testid={`inventory-filter-section-${filterKey}`}
            aria-label={`Filter by ${title.toLowerCase()}`}
            title={
              selected.length
                ? `${selected.length} active ${title.toLowerCase()} filter(s)`
                : `Filter by ${title.toLowerCase()}`
            }
            icon={
              <Box
                as="svg"
                viewBox="0 0 16 16"
                boxSize="12px"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.5"
                aria-hidden="true"
              >
                <path d="M2 3h12L9 8v4l-2 1V8z" />
              </Box>
            }
            size="xs"
            variant="ghost"
            color={selected.length ? 'var(--accent)' : 'inherit'}
            bg={selected.length ? 'rgba(var(--accent-rgb), 0.18)' : undefined}
          />
        ) : (
          <Button
            data-testid={`inventory-filter-section-${filterKey}`}
            aria-label={`Filter by ${title.toLowerCase()}`}
            size="sm"
            variant="outline"
            color={selected.length ? 'var(--accent)' : 'inherit'}
            rightIcon={<ChevronDownIcon />}
          >
            <HStack as="span" spacing={2} minW={0}>
              <Box as="span" isTruncated>
                {title}
              </Box>
              {selected.length > 0 && <Badge colorScheme="blue">{selected.length}</Badge>}
            </HStack>
          </Button>
        )}
      </PopoverTrigger>
      <Portal>
        <PopoverContent w="min(320px, calc(100vw - 24px))" bg="rgb(var(--bg-main-rgb))">
          <PopoverHeader>
            <Flex align="center" justify="space-between" gap={2}>
              <Text fontSize="sm" fontWeight="semibold">
                {title}
              </Text>
              <IconButton
                aria-label={`Close ${title.toLowerCase()} filters`}
                icon={<SmallCloseIcon />}
                variant="ghost"
                size="sm"
                onClick={() => setOpen(false)}
              />
            </Flex>
          </PopoverHeader>
          <PopoverBody p={3}>
            <Input
              ref={searchInputRef}
              aria-label={`Search ${title.toLowerCase()} filters`}
              placeholder={`Find ${title.toLowerCase()}…`}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              size="sm"
              mb={2}
            />
            <Text fontSize="xs" color="var(--chakra-colors-chakra-subtle-text)" mb={2}>
              {multiple ? 'Match any selected option.' : `Choose one ${title.toLowerCase()}.`}
            </Text>
            <Box maxH="280px" overflowY="auto">
              {matches.map((option) => (
                <Flex key={option.value} align="center" gap={1}>
                  <Checkbox
                    data-testid={`inventory-${filterKey === 'tags' ? 'tag' : filterKey}-filter-${option.value}`}
                    flex={1}
                    minW={0}
                    py={2}
                    isChecked={selected.includes(option.value)}
                    onChange={() => onToggle(option.value)}
                  >
                    <Text fontSize="sm" overflowWrap="anywhere">
                      {option.label}{' '}
                      <Box as="span" color="var(--chakra-colors-chakra-subtle-text)">
                        ({option.count})
                      </Box>
                    </Text>
                    {option.description && (
                      <Text fontSize="xs" color="var(--chakra-colors-chakra-subtle-text)" mt={0.5}>
                        {option.description}
                      </Text>
                    )}
                  </Checkbox>
                  {onDelete && (
                    <IconButton
                      aria-label={`Delete tag ${option.label}`}
                      icon={<DeleteIcon />}
                      size="xs"
                      variant="ghost"
                      onClick={() => onDelete(option.value)}
                    />
                  )}
                </Flex>
              ))}
              {!matches.length && (
                <Text fontSize="sm" py={3}>
                  No matching options
                </Text>
              )}
            </Box>
          </PopoverBody>
          <PopoverFooter>
            <Button size="sm" w="full" onClick={() => setOpen(false)}>
              Done
            </Button>
          </PopoverFooter>
        </PopoverContent>
      </Portal>
    </Popover>
  )
}

function RowIcon({ row }: { row: InventoryRow }) {
  const url = row.element
    ? resolveElementIconUrl(row.element.logo_url, row.element.technology_connectors)
    : null
  const color = row.element
    ? (TYPE_COLORS[row.element.kind?.toLowerCase() ?? ''] ?? 'gray')
    : row.objectType === 'view'
      ? 'purple'
      : 'orange'
  return (
    <Flex
      w="30px"
      h="30px"
      rounded="md"
      bg={`${color}.900`}
      color={`${color}.200`}
      align="center"
      justify="center"
      flexShrink={0}
    >
      {url ? (
        <Box as="img" src={url} alt="" boxSize="22px" objectFit="contain" />
      ) : (
        <Text fontSize="sm">
          {row.objectType === 'connector'
            ? '→'
            : row.objectType === 'view'
              ? '▦'
              : (row.element?.kind ?? 'E').charAt(0).toUpperCase()}
        </Text>
      )}
    </Flex>
  )
}

function formatUpdated(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime())
    ? '—'
    : date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function formatCell(row: InventoryRow, columnId: string, value: unknown) {
  if (columnId === 'updatedAt') return formatUpdated(row.updatedAt)
  if (columnId === 'createdAt') return formatUpdated(inventoryCreatedAt(row))
  if (typeof value === 'boolean') return value ? 'Yes' : 'No'
  if (value == null || value === '') return '—'
  return String(value)
}
