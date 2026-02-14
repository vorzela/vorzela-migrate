package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/gocql/gocql"
)

// CassandraDB implements DB using gocql.Session
type CassandraDB struct {
	Session  *gocql.Session
	Keyspace string
}

func (c *CassandraDB) Exec(ctx context.Context, query string, args ...interface{}) error {
	q := c.Session.Query(query, args...)
	return q.Exec()
}

// Query implements the Query method for Cassandra
// Note: For simplicity, we fetch all rows upfront since migration queries are small
func (c *CassandraDB) Query(ctx context.Context, query string, args ...interface{}) (Rows, error) {
	q := c.Session.Query(query, args...)

	// Get all rows at once (acceptable for migration queries which are small)
	rows, err := q.Iter().SliceMap()
	if err != nil {
		return nil, err
	}

	return &cassandraRows{rows: rows, index: -1}, nil
}

// QueryRow implements QueryRow for Cassandra
func (c *CassandraDB) QueryRow(ctx context.Context, query string, args ...interface{}) Row {
	q := c.Session.Query(query, args...)
	return &cassandraRow{query: q}
}

func (c *CassandraDB) Ping(ctx context.Context) error {
	// lightweight check: attempt to query system.local
	return c.Exec(ctx, "SELECT now() FROM system.local")
}

func (c *CassandraDB) Close() { c.Session.Close() }

// cassandraRows implements the Rows interface for Cassandra
// Uses SliceMap to fetch all rows upfront (acceptable for small migration queries)
type cassandraRows struct {
	rows  []map[string]interface{}
	index int
}

func (r *cassandraRows) Next() bool {
	r.index++
	return r.index < len(r.rows)
}

func (r *cassandraRows) Scan(dest ...interface{}) error {
	if r.index < 0 || r.index >= len(r.rows) {
		return fmt.Errorf("invalid row index")
	}

	row := r.rows[r.index]

	// For Cassandra migrations table, the columns are well-known
	// We match by expected schema based on position
	// Schema can be: (batch, migration, executed_at) for Cassandra

	// Try to extract columns by their names
	i := 0
	for _, colName := range []string{"batch", "migration", "executed_at", "id"} {
		if val, ok := row[colName]; ok && i < len(dest) {
			switch d := dest[i].(type) {
			case *string:
				if v, ok := val.(string); ok {
					*d = v
				}
			case *int:
				if v, ok := val.(int); ok {
					*d = v
				}
			case *int64:
				if v, ok := val.(int64); ok {
					*d = v
				}
			case *int32:
				if v, ok := val.(int32); ok {
					*d = v
				}
			}
			i++
		}
	}

	return nil
}

func (r *cassandraRows) Close() {
	// No-op for slice-based rows
}

func (r *cassandraRows) Err() error {
	return nil
}

// cassandraRow implements the Row interface for Cassandra
type cassandraRow struct {
	query *gocql.Query
}

func (r *cassandraRow) Scan(dest ...interface{}) error {
	return r.query.Scan(dest...)
}

// ConnectCassandra parses a cassandra://host1,host2/keyspace style DSN
// and creates the keyspace if it doesn't exist
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

	if keyspace == "" {
		return nil, fmt.Errorf("keyspace is required in DSN (format: cassandra://host1,host2/keyspace)")
	}

	hosts := strings.Split(hostsPart, ",")

	// First connect without keyspace to create it if needed
	cluster := gocql.NewCluster(hosts...)
	cluster.Keyspace = "system" // Use system keyspace initially
	cluster.Consistency = gocql.Quorum

	systemSession, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create cassandra system session: %w", err)
	}

	// Create keyspace if it doesn't exist
	createKeyspaceQuery := fmt.Sprintf(`
		CREATE KEYSPACE IF NOT EXISTS %s
		WITH replication = {'class': 'SimpleStrategy', 'replication_factor': '1'}
	`, keyspace)

	if err := systemSession.Query(createKeyspaceQuery).Exec(); err != nil {
		systemSession.Close()
		return nil, fmt.Errorf("failed to create keyspace %s: %w", keyspace, err)
	}

	systemSession.Close()

	// Now connect to the actual keyspace
	cluster = gocql.NewCluster(hosts...)
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.Quorum

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create cassandra session for keyspace %s: %w", keyspace, err)
	}

	return &CassandraDB{Session: session, Keyspace: keyspace}, nil
}
