-- Cassandra/Scylla UP migration template
-- Use CQL statements. Transactions are NOT supported in CQL.
-- Provide idempotent statements where possible.

-- Example: create keyspace and table
-- CREATE KEYSPACE IF NOT EXISTS my_keyspace WITH replication = {'class': 'SimpleStrategy', 'replication_factor': '1'};
-- USE my_keyspace;

-- CREATE TABLE IF NOT EXISTS users (
--   id uuid PRIMARY KEY,
--   name text,
--   email text
-- );

-- Note: Avoid relying on SQL transactions; write DOWN migrations that safely reverse changes.
