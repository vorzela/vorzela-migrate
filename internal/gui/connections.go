package gui

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vorzela/vorzela-migrate/v2/internal/database"
)

func normalizeEngine(engine string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "", "postgres", "postgresql", "pg":
		return "postgres", nil
	case "mysql":
		return "mysql", nil
	case "mariadb":
		return "mariadb", nil
	default:
		return "", fmt.Errorf("unsupported engine %q (use postgres, mysql or mariadb)", engine)
	}
}

func defaultPort(engine string) int {
	switch engine {
	case "mysql", "mariadb":
		return 3306
	default:
		return 5432
	}
}

func buildDSN(conn *Connection, password string) (string, error) {
	if conn == nil {
		return "", fmt.Errorf("connection is required")
	}
	engine, err := normalizeEngine(conn.Engine)
	if err != nil {
		return "", err
	}
	host := strings.TrimSpace(conn.Host)
	if host == "" {
		return "", fmt.Errorf("connection host is required")
	}
	dbName := strings.TrimSpace(conn.Database)
	if dbName == "" {
		return "", fmt.Errorf("database name is required")
	}
	port := conn.Port
	if port == 0 {
		port = defaultPort(engine)
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be between 1 and 65535")
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   "/" + dbName,
	}
	if engine == "mysql" || engine == "mariadb" {
		u.Scheme = "mysql"
		q := url.Values{}
		q.Set("parseTime", "true")
		u.RawQuery = q.Encode()
	} else if ssl := strings.TrimSpace(conn.SSLMode); ssl != "" {
		q := url.Values{}
		q.Set("sslmode", ssl)
		u.RawQuery = q.Encode()
	}
	if conn.Username != "" {
		u.User = url.UserPassword(conn.Username, password)
	} else if password != "" {
		u.User = url.UserPassword("", password)
	}
	return u.String(), nil
}

func redactedDSN(conn *Connection) string {
	if conn == nil {
		return ""
	}
	engine, err := normalizeEngine(conn.Engine)
	if err != nil {
		engine = "postgres"
	}
	port := conn.Port
	if port == 0 {
		port = defaultPort(engine)
	}
	user := ""
	if conn.Username != "" {
		user = url.User(conn.Username).String() + "@"
	}
	return fmt.Sprintf("%s://%s%s:%d/%s", engine, user, conn.Host, port, conn.Database)
}

func publicConnection(conn Connection) PublicConnection {
	return PublicConnection{
		ID:           conn.ID,
		Name:         conn.Name,
		Engine:       conn.Engine,
		Host:         conn.Host,
		Port:         conn.Port,
		Database:     conn.Database,
		Username:     conn.Username,
		SSLMode:      conn.SSLMode,
		HasPassword:  conn.PasswordEnc != "",
		CreatedAt:    conn.CreatedAt,
		UpdatedAt:    conn.UpdatedAt,
		LastTestedAt: conn.LastTestedAt,
		LastTestOK:   conn.LastTestOK,
		DSN:          redactedDSN(&conn),
	}
}

// testConnection opens a short-lived database connection and pings it.
func testConnection(conn *Connection, password string) error {
	dsn, err := buildDSN(conn, password)
	if err != nil {
		return err
	}
	db, err := database.Connect(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return nil
}

func boolPtr(v bool) *bool { return &v }

func timePtr(t time.Time) *time.Time { return &t }
