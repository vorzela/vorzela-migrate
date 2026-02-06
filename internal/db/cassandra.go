package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/gocql/gocql"
)

// CassandraDB implements DB using gocql.Session
type CassandraDB struct {
	Session *gocql.Session
}

func (c *CassandraDB) Exec(ctx context.Context, query string, args ...interface{}) error {
	q := c.Session.Query(query, args...)
	return q.Exec()
}

// Query is not implemented for Cassandra/Scylla yet. Return a clear error so callers know.
func (c *CassandraDB) Query(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	return nil, fmt.Errorf("Query is not implemented for Cassandra/Scylla in this release")
}

// QueryRow is not implemented; migrations currently rely on SQL semantics which differ from CQL.
func (c *CassandraDB) QueryRow(ctx context.Context, query string, args ...interface{}) Row {
	return cassandraRow{err: fmt.Errorf("QueryRow is not implemented for Cassandra/Scylla")}
}

func (c *CassandraDB) Ping(ctx context.Context) error {
	// lightweight check: attempt to create a simple query against system.local
	return c.Exec(ctx, "SELECT now() FROM system.local")
}

func (c *CassandraDB) Close() { c.Session.Close() }

type cassandraRow struct {
	err error
}

func (r cassandraRow) Scan(dest ...interface{}) error { return r.err }

// ConnectCassandra parses a simple cassandra://host1,host2/keyspace style DSN
func ConnectCassandra(dsn string) (DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("empty dsn")
	}

	// strip protocol prefix if present
	if strings.HasPrefix(dsn, "cassandra://") {
		dsn = dsn[len("cassandra://"):]
	} else if strings.HasPrefix(dsn, "scylla://") {
		dsn = dsn[len("scylla://"):]
	}

	// split hosts and optional keyspace
	hostsPart := dsn
	keyspace := ""
	if idx := strings.Index(dsn, "/"); idx != -1 {
		hostsPart = dsn[:idx]
		keyspace = dsn[idx+1:]
	}

	hosts := strings.Split(hostsPart, ",")

	cluster := gocql.NewCluster(hosts...)
	if keyspace != "" {
		cluster.Keyspace = keyspace
	}

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create cassandra session: %w", err)
	}

	return &CassandraDB{Session: session}, nil
}
