package database

// DB wraps the database connection
type DB struct {
	ConnectionString string
}

// New creates a new database connection
func New(connStr string) (*DB, error) {
	return &DB{ConnectionString: connStr}, nil
}
