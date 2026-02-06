-- Cassandra/Scylla DOWN migration template
-- Revert the changes made in the UP migration. Be careful: dropping keyspaces/tables is destructive.

-- Example: drop table
-- USE my_keyspace;
-- DROP TABLE IF EXISTS users;

-- Example: drop keyspace
-- DROP KEYSPACE IF EXISTS my_keyspace;
