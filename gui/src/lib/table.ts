/**
 * Shared TanStack Table v9 features for the VorzelaJS data tables.
 *
 * Both <DataTable> and <SimpleDataTable> consume this module, so the table
 * feature set and the exported column type cannot drift apart.
 */
import {
  columnFilteringFeature,
  columnSizingFeature,
  columnVisibilityFeature,
  createFilteredRowModel,
  createPaginatedRowModel,
  createSortedRowModel,
  filterFn_includesString,
  globalFilteringFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  rowSortingFeature,
  sortFn_alphanumeric,
  tableFeatures,
  type ColumnDef,
  type RowData,
} from '@vorzelajs/table'

export const appTableFeatures = tableFeatures({
  columnFilteringFeature,
  globalFilteringFeature,
  rowSortingFeature,
  rowPaginationFeature,
  rowSelectionFeature,
  columnVisibilityFeature,
  columnSizingFeature,
  filteredRowModel: createFilteredRowModel(),
  sortedRowModel: createSortedRowModel(),
  paginatedRowModel: createPaginatedRowModel(),
  filterFns: { includesString: filterFn_includesString },
  sortFns: { alphanumeric: sortFn_alphanumeric },
})

export type AppTableFeatures = typeof appTableFeatures

/** Column definition typed to the features used by both data tables. */
export type AppColumnDef<TData extends RowData, TValue = unknown> = ColumnDef<
  AppTableFeatures,
  TData,
  TValue
>
