# Migration Naming Conventions - Vorzela

## 🎯 Strong Naming Standards

Use **clear, descriptive names** for your migrations. This makes your code self-documenting and easier to understand.

---

## Core Naming Pattern

### Format
```
<action>_<details>_<target>
```

### Components

| Part | Description | Example |
|------|-------------|---------|
| **Action** | What you're doing | `create`, `add`, `drop`, `rename`, `modify` |
| **Details** | What columns/constraints | `column_name`, `foreign_key`, `index` |
| **Target** | What table it affects | `users`, `posts`, `orders` |

---

## Migration Naming Examples

### ✅ DO THIS (Strong Naming)

**Table Operations:**
```
vc make migration create_users_table
vc make migration create_posts_table
vc make migration create_order_items_table
vc make migration drop_legacy_data_table
```

**Column Operations:**
```
vc make migration add_email_to_users
vc make migration add_phone_to_customers
vc make migration add_status_to_orders
vc make migration remove_deprecated_field_from_products
vc make migration rename_user_name_to_full_name
```

**Index Operations:**
```
vc make migration create_index_on_users_email
vc make migration create_unique_index_on_orders_tracking_id
vc make migration drop_index_on_posts_author_id
```

**Constraint Operations:**
```
vc make migration add_foreign_key_user_id_to_posts
vc make migration add_unique_constraint_to_users_email
vc make migration add_check_constraint_age_to_users
```

**Multiple Operations:**
```
vc make migration create_categories_table_and_add_to_products
vc make migration add_audit_columns_to_orders
```

---

## ❌ DON'T DO THIS (Weak Naming)

```
vc make migration update_table          # Too vague
vc make migration fix_database          # Not descriptive
vc make migration changes               # Meaningless
vc make migration migration1            # No context
vc make migration temp_fix              # Temporary = bad
vc make migration random123             # Unmaintainable
```

---

## Real-World Examples

### Example 1: Building a Blog System

**Good progression:**
```bash
vc make migration create_users_table
vc make migration create_posts_table
vc make migration create_comments_table
vc make migration add_slug_to_posts
vc make migration add_published_at_to_posts
vc make migration add_author_id_to_comments
vc make migration create_index_on_posts_slug
vc make migration add_cascade_delete_to_comments
```

**Generated files:**
```
1707120000_create_users_table.sql
1707120001_create_posts_table.sql
1707120002_create_comments_table.sql
1707120003_add_slug_to_posts.sql
1707120004_add_published_at_to_posts.sql
1707120005_add_author_id_to_comments.sql
1707120006_create_index_on_posts_slug.sql
1707120007_add_cascade_delete_to_comments.sql
```

**Migration Status Output:**
```
🐘 Migration Status [dev]
────────────────────────────────────────────────────────────────────────
Migration                              | Status
────────────────────────────────────────────────────────────────────────
1707120000_create_users_table.sql      | ✓ Batch 1
1707120001_create_posts_table.sql      | ✓ Batch 1
1707120002_create_comments_table.sql   | ✓ Batch 1
1707120003_add_slug_to_posts.sql       | ✓ Batch 1
1707120004_add_published_at_to_posts.sql | ✓ Batch 2
1707120005_add_author_id_to_comments.sql | ✓ Batch 2
1707120006_create_index_on_posts_slug.sql | ✓ Batch 2
1707120007_add_cascade_delete_to_comments.sql | ⏳ Pending
────────────────────────────────────────────────────────────────────────

Summary: 7 executed, 1 pending
```

### Example 2: E-commerce Platform

```bash
vc make migration create_customers_table
vc make migration create_products_table
vc make migration create_orders_table
vc make migration create_order_items_table
vc make migration add_inventory_to_products
vc make migration add_status_to_orders
vc make migration add_tracking_number_to_orders
vc make migration create_unique_index_on_customers_email
vc make migration create_foreign_key_customer_id_to_orders
```

### Example 3: SaaS Application

```bash
vc make migration create_organizations_table
vc make migration create_users_table
vc make migration create_memberships_table
vc make migration add_organization_id_to_users
vc make migration add_role_to_memberships
vc make migration create_api_keys_table
vc make migration add_api_key_id_to_organizations
vc make migration create_audit_logs_table
vc make migration add_deleted_at_to_organizations
```

---

## Action Verbs Reference

### Common Actions

| Action | Use Case | Example |
|--------|----------|---------|
| `create` | Create new table | `create_users_table` |
| `add` | Add column or constraint | `add_email_to_users` |
| `remove` | Remove column or constraint | `remove_deprecated_field_from_products` |
| `drop` | Drop table or index | `drop_legacy_data_table` |
| `rename` | Rename table or column | `rename_user_name_to_full_name` |
| `modify` | Modify column definition | `modify_user_email_to_text` |
| `create_index` | Create database index | `create_index_on_users_email` |
| `drop_index` | Drop database index | `drop_index_on_posts_author_id` |
| `add_foreign_key` | Add FK constraint | `add_foreign_key_user_id_to_posts` |
| `add_unique` | Add unique constraint | `add_unique_constraint_to_users_email` |
| `add_check` | Add check constraint | `add_check_constraint_age_to_users` |

---

## Best Practices

### 1. **Be Specific**
```bash
# Good
vc make migration add_email_to_users

# Bad
vc make migration add_field
vc make migration add_column
```

### 2. **Include Table Names**
```bash
# Good
vc make migration add_status_to_orders
vc make migration add_category_to_products

# Bad
vc make migration add_status
vc make migration add_category
```

### 3. **Use Complete Names**
```bash
# Good
vc make migration create_order_items_table

# Bad
vc make migration create_items_table
vc make migration create_order_table_items
```

### 4. **One Logical Change Per Migration**
```bash
# Good - logical grouping
vc make migration add_email_and_phone_to_users

# Better - separate concerns
vc make migration add_email_to_users
vc make migration add_phone_to_users
```

### 5. **Use Snake Case (lowercase_with_underscores)**
```bash
# Good
vc make migration create_user_profiles_table
vc make migration add_birth_date_to_users

# Bad
vc make migration CreateUserProfilesTable
vc make migration AddBirthDateToUsers
vc make migration create_user_profiles_TABLE
```

---

## SQL Naming (Inside Migrations)

### Table Names

```sql
-- Good: plural, lowercase, underscores
CREATE TABLE users (...)
CREATE TABLE products (...)
CREATE TABLE order_items (...)

-- Bad: singular, camelCase, spaces
CREATE TABLE user (...)
CREATE TABLE Product (...)
CREATE TABLE "Order Item" (...)
```

### Column Names

```sql
-- Good
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255),
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- Bad
CREATE TABLE users (
    ID INT PRIMARY KEY,
    E_mail VARCHAR(255),
    firstname VARCHAR(100),
    lastname VARCHAR(100),
    CreatedAt TIMESTAMP,
    UpdatedAt TIMESTAMP
);
```

### Index Names

```sql
-- Good
CREATE INDEX idx_users_email ON users(email);
CREATE UNIQUE INDEX idx_users_username ON users(username);
CREATE INDEX idx_posts_author_id ON posts(author_id);

-- Bad
CREATE INDEX index1 ON users(email);
CREATE INDEX idx ON users(username);
CREATE INDEX i ON posts(author_id);
```

### Constraint Names

```sql
-- Good
ALTER TABLE posts 
ADD CONSTRAINT fk_posts_author_id 
FOREIGN KEY (author_id) REFERENCES users(id);

-- Bad
ALTER TABLE posts 
ADD CONSTRAINT fk1 
FOREIGN KEY (author_id) REFERENCES users(id);
```

---

## Migration File Content Template

```sql
-- Migration: ADD_EMAIL_TO_USERS
-- Created at: 2026-02-05 10:30:45
-- Environment: dev/server
-- Description: Add email column for user authentication

-- ⬆ Up (Run when migrating forward)
BEGIN;

ALTER TABLE users ADD COLUMN email VARCHAR(255) UNIQUE NOT NULL;
CREATE INDEX idx_users_email ON users(email);

COMMIT;

-- ⬇ Down (Run when rolling back)
BEGIN;

DROP INDEX IF EXISTS idx_users_email;
ALTER TABLE users DROP COLUMN email;

COMMIT;
```

---

## Batch Operations Pattern

### Related Migrations

Keep related migrations in the same batch:

```bash
# These will be in the same batch if run together
vc make migration create_users_table
vc make migration create_posts_table
vc make migration create_comments_table
vc make migration add_indexes_to_posts
vc make migration add_foreign_keys_to_comments

# Then run all at once
vc migrate

# Status will show: Batch 1 (5 migrations)
```

### Sequential Migrations

Run dependent migrations separately:

```bash
# Step 1: Create base tables
vc make migration create_users_table
vc migrate

# Step 2: Create related tables
vc make migration create_posts_table
vc make migration add_author_id_to_posts
vc migrate

# Step 3: Add indexes
vc make migration create_indexes
vc migrate
```

---

## Examples by Domain

### E-Commerce Domain
```
create_products_table
create_categories_table
create_customers_table
create_orders_table
create_order_items_table
add_price_to_products
add_stock_to_products
add_discount_to_orders
add_shipping_address_to_orders
create_index_on_products_sku
add_foreign_key_products_category_id
```

### SaaS Domain
```
create_organizations_table
create_users_table
create_subscriptions_table
create_api_keys_table
add_organization_id_to_users
add_subscription_id_to_organizations
add_expires_at_to_api_keys
create_audit_logs_table
add_soft_delete_to_organizations
```

### CMS Domain
```
create_pages_table
create_posts_table
create_categories_table
create_tags_table
create_post_tags_table
add_slug_to_pages
add_published_at_to_posts
add_author_id_to_posts
create_index_on_posts_slug
add_full_text_search_to_posts
```

---

## Checking Migration Names

Use `vc status` to verify your migration names are clear:

```bash
vc status

# Output should be readable and self-documenting:
# ✓ 1707120000_create_users_table.sql
# ✓ 1707120001_add_email_to_users.sql
# ✓ 1707120002_create_posts_table.sql
# ✓ 1707120003_add_author_id_to_posts.sql
# ⏳ 1707120004_add_published_status_to_posts.sql
```

---

## Summary

### ✅ DO
- Use **action_details_target** pattern
- Be **specific and descriptive**
- Include **table names**
- Use **snake_case**
- Keep migrations **logical and focused**

### ❌ DON'T
- Use vague names (update, change, fix)
- Abbreviate table names
- Mix naming conventions
- Create overly complex migrations
- Use temporary or "hack" language

**Good naming = Self-documenting code = Easier maintenance!** 🎯
