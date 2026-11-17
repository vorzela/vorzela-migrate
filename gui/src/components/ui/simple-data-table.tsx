import {
  createTable,
  flexRender,
  type PaginationState,
  type RowData,
} from '@vorzelajs/table'
import { effect, onCleanup, signal, untrack } from '@vorzelajs/core'
import { For, Show } from '@vorzelajs/dom'

import { accessorOf } from '~/hooks/index.ts'
import { cx } from '~/lib/cx.ts'
import { appTableFeatures, type AppColumnDef } from '~/lib/table.ts'
import type { ReactNode } from '~/lib/types.ts'
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  ChevronUp,
  ChevronsUpDown,
  Search,
  X,
} from './icons.tsx'
import styles from '~/styles/components/ui/simple-data-table.module.scss'

type ClassValueOrFn<TContext> = string | ((context: TContext) => string)

export interface SimpleDataTableRowClassContext<TData> {
  row: TData
  rowIndex: number
  isEven: boolean
}

export interface SimpleDataTableCellClassContext<TData>
  extends SimpleDataTableRowClassContext<TData> {
  columnId: string
}

interface SimpleDataTableColumnMeta<TData> {
  headerClass?: string
  cellClass?: ClassValueOrFn<SimpleDataTableCellClassContext<TData>>
}

export interface SimpleDataTableClasses<TData> {
  toolbar?: string
  searchWrap?: string
  searchInput?: string
  actions?: string
  tableShell?: string
  table?: string
  head?: string
  headerRow?: string
  headerCell?: string
  body?: string
  row?: ClassValueOrFn<SimpleDataTableRowClassContext<TData>>
  cell?: ClassValueOrFn<SimpleDataTableCellClassContext<TData>>
  emptyState?: string
  pagination?: string
}

export type SimpleDataTableVariant = 'compact' | 'comfortable' | 'minimal'

function resolveClassName<TContext>(
  value: ClassValueOrFn<TContext> | undefined,
  context: TContext,
): string | undefined {
  return typeof value === 'function' ? value(context) : value
}

function getColumnMeta<TData>(column: {
  columnDef: { meta?: unknown }
}): SimpleDataTableColumnMeta<TData> | undefined {
  return column.columnDef.meta as SimpleDataTableColumnMeta<TData> | undefined
}

export interface SimpleDataTableProps<TData extends RowData> {
  data: TData[] | (() => TData[])
  columns: AppColumnDef<TData, unknown>[] | (() => AppColumnDef<TData, unknown>[])
  searchable?: boolean
  searchPlaceholder?: string
  variant?: SimpleDataTableVariant
  pagination?: boolean
  initialPageSize?: number
  pageSizeOptions?: number[]
  loading?: boolean | (() => boolean)
  skeletonRows?: number
  emptyMessage?: string
  toolbarActions?: ReactNode
  classes?: SimpleDataTableClasses<TData>
  className?: string
}

function SortIcon({
  direction,
}: {
  direction: () => 'asc' | 'desc' | false
}) {
  return (
    <span className={styles.sortIcon}>
      <Show when={() => direction() === 'asc'}>
        {() => <ChevronUp aria-hidden />}
      </Show>
      <Show when={() => direction() === 'desc'}>
        {() => <ChevronDown aria-hidden />}
      </Show>
      <Show when={() => direction() === false}>
        {() => <ChevronsUpDown aria-hidden />}
      </Show>
    </span>
  )
}

/** Lightweight client-side data table with search, sorting and pagination. */
export function SimpleDataTable<TData extends RowData>({
  data,
  columns,
  searchable,
  searchPlaceholder,
  variant = 'comfortable',
  pagination,
  initialPageSize,
  pageSizeOptions,
  loading,
  skeletonRows,
  emptyMessage,
  toolbarActions,
  classes,
  className,
}: SimpleDataTableProps<TData>) {
  const isSearchable = searchable !== false
  const isPaginationEnabled = pagination !== false
  const skeletonCount = skeletonRows ?? 4
  const pageSizes = pageSizeOptions ?? [10, 25, 50]

  const dataAccessor = accessorOf<TData[]>(data)
  const columnsAccessor = accessorOf<AppColumnDef<TData, unknown>[]>(columns)
  const loadingAccessor = accessorOf<boolean>(loading ?? false)

  const [searchInput, setSearchInput] = signal('')
  let debounce: ReturnType<typeof setTimeout> | null = null

  onCleanup(() => {
    if (debounce) {
      clearTimeout(debounce)
      debounce = null
    }
  })

  const table = createTable({
    features: appTableFeatures,
    data: dataAccessor(),
    columns: columnsAccessor(),
    initialState: {
      pagination: {
        pageIndex: 0,
        pageSize: initialPageSize ?? 10,
      },
    },
    autoResetPageIndex: true,
  })

  // Keep the table instance alive when accessor props produce new arrays.
  // The first pass is skipped so an allocating accessor does not rebuild the
  // table on mount.
  let firstSync = true
  let lastData = dataAccessor()
  let lastColumns = columnsAccessor()
  effect(() => {
    const nextData = dataAccessor()
    const nextColumns = columnsAccessor()
    if (firstSync) {
      firstSync = false
      return
    }
    if (nextData === lastData && nextColumns === lastColumns) return
    lastData = nextData
    lastColumns = nextColumns
    untrack(() => {
      table.setOptions((previous) => ({
        ...previous,
        data: nextData,
        columns: nextColumns,
      }))
    })
  })

  const handleSearch = (raw: string) => {
    setSearchInput(raw)
    if (debounce) clearTimeout(debounce)
    debounce = setTimeout(() => {
      debounce = null
      table.setGlobalFilter(raw)
      table.setPageIndex(0)
    }, 300)
  }

  const clearSearch = () => {
    setSearchInput('')
    if (debounce) {
      clearTimeout(debounce)
      debounce = null
    }
    table.setGlobalFilter('')
    table.setPageIndex(0)
  }

  const paginationState = (): PaginationState =>
    table.atoms.pagination?.get() ?? {
      pageIndex: 0,
      pageSize: initialPageSize ?? 10,
    }
  const totalFiltered = () => table.getFilteredRowModel().rows.length
  const pageStart = () => {
    const state = paginationState()
    return state.pageIndex * state.pageSize + 1
  }
  const pageEnd = () => {
    const state = paginationState()
    const start = state.pageIndex * state.pageSize + 1
    return Math.min(start + state.pageSize - 1, totalFiltered())
  }
  const visibleLeafColumns = () =>
    table.getAllLeafColumns().filter((column) => column.getIsVisible())
  const skeletonIndices = () =>
    Array.from({ length: skeletonCount }, (_, index) => index)

  // Static rows and cells must not allocate one effect each. Only bind the
  // class when a class callback can change with selection or row data.
  const reactiveRowClass = typeof classes?.row === 'function'

  return (
    <div className={cx(styles.root, className)} data-variant={variant}>
      {isSearchable || toolbarActions ? (
        <div className={cx(styles.toolbar, classes?.toolbar)}>
          {isSearchable ? (
            <div className={cx(styles.searchWrap, classes?.searchWrap)}>
              <span className={styles.searchIcon}>
                <Search aria-hidden />
              </span>
              <input
                type="search"
                value={() => searchInput()}
                onInput={(event) =>
                  handleSearch((event.currentTarget as HTMLInputElement).value)
                }
                placeholder={searchPlaceholder ?? 'Search…'}
                aria-label={searchPlaceholder ?? 'Search'}
                className={cx(styles.search, classes?.searchInput)}
              />
              <Show when={() => searchInput().length > 0}>
                {() => (
                  <button
                    type="button"
                    onClick={clearSearch}
                    aria-label="Clear search"
                    className={styles.clear}
                  >
                    <X aria-hidden />
                  </button>
                )}
              </Show>
            </div>
          ) : null}

          {toolbarActions ? (
            <div className={cx(styles.actions, classes?.actions)}>
              {toolbarActions}
            </div>
          ) : null}
        </div>
      ) : null}

      <div
        className={cx(styles.shell, classes?.tableShell)}
        tabIndex={0}
        role="region"
        aria-label="Table"
      >
        <table className={cx(styles.table, classes?.table)}>
          <thead className={cx(styles.head, classes?.head)}>
            <For
              each={() => table.getHeaderGroups()}
              key={(headerGroup) => headerGroup.id}
            >
              {(headerGroup) => (
                <tr className={classes?.headerRow}>
                  <For
                    each={() => headerGroup.headers}
                    key={(header) => header.id}
                  >
                    {(header) => {
                      const columnMeta = getColumnMeta<TData>(header.column)
                      const canSort = () => header.column.getCanSort()
                      const sorted = () => header.column.getIsSorted()

                      return (
                        <th
                          colSpan={header.colSpan}
                          scope="col"
                          aria-sort={() =>
                            sorted() === 'asc'
                              ? 'ascending'
                              : sorted() === 'desc'
                                ? 'descending'
                                : canSort()
                                  ? 'none'
                                  : undefined
                          }
                          className={cx(
                            styles.headerCell,
                            columnMeta?.headerClass,
                            classes?.headerCell,
                          )}
                        >
                          <Show when={() => !header.isPlaceholder}>
                            {() =>
                              canSort() ? (
                                <button
                                  type="button"
                                  className={styles.sortButton}
                                  onClick={header.column.getToggleSortingHandler()}
                                >
                                  {flexRender(
                                    header.column.columnDef.header,
                                    header.getContext(),
                                  )}
                                  <SortIcon direction={sorted} />
                                </button>
                              ) : (
                                <span>
                                  {flexRender(
                                    header.column.columnDef.header,
                                    header.getContext(),
                                  )}
                                </span>
                              )
                            }
                          </Show>
                        </th>
                      )
                    }}
                  </For>
                </tr>
              )}
            </For>
          </thead>

          <tbody className={cx(styles.body, classes?.body)}>
            <Show when={loadingAccessor}>
              {() => (
                <For each={skeletonIndices} key={(index) => index}>
                  {(_index) => (
                    <tr className={styles.row}>
                      <For
                        each={visibleLeafColumns}
                        key={(column) => column.id}
                      >
                        {(_column) => (
                          <td className={styles.cell}>
                            <div className={styles.skeleton} />
                          </td>
                        )}
                      </For>
                    </tr>
                  )}
                </For>
              )}
            </Show>

            <Show
              when={() =>
                !loadingAccessor() && table.getRowModel().rows.length > 0
              }
            >
              {() => (
                <For
                  each={() => table.getRowModel().rows}
                  key={(row) => row.id}
                >
                  {(row) => {
                    const rowContext =
                      (): SimpleDataTableRowClassContext<TData> => ({
                        row: row.original,
                        rowIndex: row.index,
                        isEven: row.index % 2 === 0,
                      })
                    const rowClassFor = (
                      context: SimpleDataTableRowClassContext<TData>,
                    ) =>
                      cx(styles.row, resolveClassName(classes?.row, context))

                    return (
                      <tr
                        className={
                          reactiveRowClass
                            ? () => rowClassFor(rowContext())
                            : rowClassFor(rowContext())
                        }
                      >
                        <For
                          each={() => row.getVisibleCells()}
                          key={(cell) => cell.id}
                        >
                          {(cell) => {
                            const cellContext =
                              (): SimpleDataTableCellClassContext<TData> => ({
                                ...rowContext(),
                                columnId: cell.column.id,
                              })
                            const columnMeta = getColumnMeta<TData>(
                              cell.column,
                            )
                            const reactiveCellClass =
                              reactiveRowClass ||
                              typeof classes?.cell === 'function' ||
                              typeof columnMeta?.cellClass === 'function'
                            const cellClassFor = (
                              context: SimpleDataTableCellClassContext<TData>,
                            ) =>
                              cx(
                                styles.cell,
                                resolveClassName(classes?.cell, context),
                                resolveClassName(
                                  columnMeta?.cellClass,
                                  context,
                                ),
                              )

                            return (
                              <td
                                className={
                                  reactiveCellClass
                                    ? () => cellClassFor(cellContext())
                                    : cellClassFor(cellContext())
                                }
                              >
                                {flexRender(
                                  cell.column.columnDef.cell,
                                  cell.getContext(),
                                )}
                              </td>
                            )
                          }}
                        </For>
                      </tr>
                    )
                  }}
                </For>
              )}
            </Show>

            <Show
              when={() =>
                !loadingAccessor() && table.getRowModel().rows.length === 0
              }
            >
              {() => (
                <tr>
                  <td
                    colSpan={visibleLeafColumns().length}
                    className={cx(styles.empty, classes?.emptyState)}
                  >
                    {emptyMessage ?? 'No results found.'}
                  </td>
                </tr>
              )}
            </Show>
          </tbody>
        </table>
      </div>

      {isPaginationEnabled ? (
        <div className={cx(styles.pagination, classes?.pagination)}>
          <div className={styles.info}>
            <Show when={() => totalFiltered() > 0}>
              {() => (
                <span>
                  Showing{' '}
                  <span className={styles.strong}>
                    {() => `${pageStart()}–${pageEnd()}`}
                  </span>{' '}
                  of{' '}
                  <span className={styles.strong}>
                    {() => String(totalFiltered())}
                  </span>
                </span>
              )}
            </Show>

            <label className={styles.pageSize}>
              <span>Rows:</span>
              <select
                value={() => paginationState().pageSize}
                onChange={(event) =>
                  table.setPageSize(
                    Number((event.currentTarget as HTMLSelectElement).value),
                  )
                }
                className={styles.select}
              >
                <For each={() => pageSizes} key={(size) => size}>
                  {(size) => (
                    <option
                      value={size}
                      selected={() => paginationState().pageSize === size}
                    >
                      {size}
                    </option>
                  )}
                </For>
              </select>
            </label>
          </div>

          <div className={styles.pages}>
            <button
              type="button"
              disabled={() => !table.getCanPreviousPage()}
              onClick={() => table.firstPage()}
              className={styles.pageButton}
              aria-label="First page"
            >
              <ChevronsLeft aria-hidden />
            </button>
            <button
              type="button"
              disabled={() => !table.getCanPreviousPage()}
              onClick={() => table.previousPage()}
              className={styles.pageButton}
              aria-label="Previous page"
            >
              <ChevronLeft aria-hidden />
            </button>

            <For
              each={() =>
                pageNumbers(
                  table.getPageCount(),
                  paginationState().pageIndex,
                )
              }
              key={(item, index) =>
                `${item}-${index}-${paginationState().pageIndex}`
              }
            >
              {(item) =>
                item === '…' ? (
                  <span className={styles.ellipsis}>
                    <span aria-hidden>…</span>
                    <span className="vz-sr-only">More pages</span>
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={() => table.setPageIndex(Number(item) - 1)}
                    aria-label={`Page ${item}`}
                    aria-current={() =>
                      paginationState().pageIndex === Number(item) - 1
                        ? 'page'
                        : undefined
                    }
                    className={() =>
                      cx(
                        styles.pageNumber,
                        paginationState().pageIndex === Number(item) - 1 &&
                          styles.pageNumberActive,
                      )
                    }
                  >
                    {item}
                  </button>
                )
              }
            </For>

            <button
              type="button"
              disabled={() => !table.getCanNextPage()}
              onClick={() => table.nextPage()}
              className={styles.pageButton}
              aria-label="Next page"
            >
              <ChevronRight aria-hidden />
            </button>
            <button
              type="button"
              disabled={() => !table.getCanNextPage()}
              onClick={() => table.lastPage()}
              className={styles.pageButton}
              aria-label="Last page"
            >
              <ChevronsRight aria-hidden />
            </button>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function pageNumbers(total: number, current: number): string[] {
  if (total <= 7) {
    return Array.from({ length: total }, (_, index) => String(index + 1))
  }
  const pages: string[] = []
  const add = (page: number) => pages.push(String(page))
  add(1)
  if (current > 3) pages.push('…')
  for (
    let page = Math.max(2, current);
    page <= Math.min(total - 1, current + 2);
    page += 1
  ) {
    add(page)
  }
  if (current < total - 3) pages.push('…')
  add(total)
  return pages
}
