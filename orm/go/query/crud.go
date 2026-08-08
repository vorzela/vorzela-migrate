package query

import "context"

// Find runs FindOptions then Get.
func (e *Entity[T]) Find(ctx context.Context, db DB, opts FindOptions) ([]T, error) {
	return e.New().ApplyFind(opts).Get(ctx, db)
}

// FindOne returns the first match for FindOptions.
func (e *Entity[T]) FindOne(ctx context.Context, db DB, opts FindOptions) (*T, error) {
	opts.Take = 1
	return e.New().ApplyFind(opts).First(ctx, db)
}

// FindByID loads by primary key.
func (e *Entity[T]) FindByID(ctx context.Context, db DB, id any) (*T, error) {
	return e.New().Where(e.meta.PrimaryKey, id).First(ctx, db)
}

// Create inserts values.
func (e *Entity[T]) Create(ctx context.Context, db DB, values map[string]any) (int64, error) {
	return e.New().Create(ctx, db, values)
}

// Update applies values with no WHERE — prefer Users.Where(...).Update(...).
func (e *Entity[T]) Update(ctx context.Context, db DB, values map[string]any) (int64, error) {
	return e.New().Update(ctx, db, values)
}

// SoftDelete soft-deletes by primary key (sets deleted_at).
func (e *Entity[T]) SoftDelete(ctx context.Context, db DB, id any) (int64, error) {
	return e.New().Where(e.meta.PrimaryKey, id).Delete(ctx, db)
}

// ForceDelete permanently deletes by primary key (ignores soft deletes).
func (e *Entity[T]) ForceDelete(ctx context.Context, db DB, id any) (int64, error) {
	return e.New().Where(e.meta.PrimaryKey, id).ForceDelete(ctx, db)
}

// Paginate runs offset/cursor pagination from the entity root.
func (e *Entity[T]) Paginate(ctx context.Context, db DB, req PageRequest) (*PageResult[T], error) {
	return e.New().Paginate(ctx, db, req)
}
