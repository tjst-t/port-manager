package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database for lease management.
type DB struct {
	db *sql.DB
}

// Lease represents a port lease record.
type Lease struct {
	ID           int
	Port         int
	Project      string
	Worktree     string
	WorktreePath string
	Repo         string
	Name         string
	Hostname     string
	Expose       bool
	State        string // "active" or "stale"
	StaleSince   *time.Time
	CreatedAt    time.Time
	LastUsed     time.Time
}

// Open opens or creates the SQLite database at the given path.
// It enables WAL mode and creates tables if they don't exist.
func Open(dbPath string) (*DB, error) {
	dsn := dbPath + "?_pragma=busy_timeout%3d5000&_pragma=journal_mode%3dWAL"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	d := &DB{db: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return d, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// migrate creates tables if they don't exist.
func (d *DB) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS leases (
    id INTEGER PRIMARY KEY,
    port INTEGER UNIQUE NOT NULL,
    project TEXT NOT NULL,
    worktree TEXT NOT NULL,
    worktree_path TEXT NOT NULL,
    repo TEXT NOT NULL,
    name TEXT NOT NULL,
    hostname TEXT UNIQUE NOT NULL,
    expose BOOLEAN DEFAULT FALSE,
    state TEXT DEFAULT 'active',
    stale_since TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(project, worktree, name)
);

CREATE TABLE IF NOT EXISTS gc_state (
    key TEXT PRIMARY KEY,
    value TEXT
);`

	_, err := d.db.Exec(schema)
	return err
}

// FindLease finds a lease by project, worktree, and name.
func (d *DB) FindLease(project, worktree, name string) (*Lease, error) {
	row := d.db.QueryRow(
		`SELECT id, port, project, worktree, worktree_path, repo, name, hostname,
		        expose, state, stale_since, created_at, last_used
		 FROM leases WHERE project = ? AND worktree = ? AND name = ?`,
		project, worktree, name,
	)
	return scanLease(row)
}

// FindLeaseByHostname finds a lease by hostname.
func (d *DB) FindLeaseByHostname(hostname string) (*Lease, error) {
	row := d.db.QueryRow(
		`SELECT id, port, project, worktree, worktree_path, repo, name, hostname,
		        expose, state, stale_since, created_at, last_used
		 FROM leases WHERE hostname = ?`,
		hostname,
	)
	return scanLease(row)
}

// CreateLease inserts a new lease record.
func (d *DB) CreateLease(lease *Lease) error {
	result, err := d.db.Exec(
		`INSERT INTO leases (port, project, worktree, worktree_path, repo, name, hostname, expose, state)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		lease.Port, lease.Project, lease.Worktree, lease.WorktreePath,
		lease.Repo, lease.Name, lease.Hostname, lease.Expose, lease.State,
	)
	if err != nil {
		return fmt.Errorf("creating lease: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting insert ID: %w", err)
	}
	lease.ID = int(id)
	return nil
}

// UpdateLeaseState updates the state and stale_since of a lease.
func (d *DB) UpdateLeaseState(id int, state string, staleSince *time.Time) error {
	_, err := d.db.Exec(
		`UPDATE leases SET state = ?, stale_since = ? WHERE id = ?`,
		state, staleSince, id,
	)
	if err != nil {
		return fmt.Errorf("updating lease state: %w", err)
	}
	return nil
}

// UpdateLastUsed updates the last_used timestamp for a lease.
func (d *DB) UpdateLastUsed(id int) error {
	_, err := d.db.Exec(
		`UPDATE leases SET last_used = CURRENT_TIMESTAMP WHERE id = ?`,
		id,
	)
	if err != nil {
		return fmt.Errorf("updating last_used: %w", err)
	}
	return nil
}

// UpdateLeaseExpose updates the expose flag for a lease.
func (d *DB) UpdateLeaseExpose(id int, expose bool) error {
	_, err := d.db.Exec(
		`UPDATE leases SET expose = ? WHERE id = ?`,
		expose, id,
	)
	if err != nil {
		return fmt.Errorf("updating lease expose: %w", err)
	}
	return nil
}

// DeleteLease removes a lease by ID.
func (d *DB) DeleteLease(id int) error {
	_, err := d.db.Exec(`DELETE FROM leases WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting lease: %w", err)
	}
	return nil
}

// ListLeases returns all leases.
func (d *DB) ListLeases() ([]Lease, error) {
	return d.queryLeases(`SELECT id, port, project, worktree, worktree_path, repo, name, hostname,
		expose, state, stale_since, created_at, last_used FROM leases ORDER BY port`)
}

// ListActiveLeases returns leases with state='active'.
func (d *DB) ListActiveLeases() ([]Lease, error) {
	return d.queryLeases(`SELECT id, port, project, worktree, worktree_path, repo, name, hostname,
		expose, state, stale_since, created_at, last_used FROM leases WHERE state = 'active' ORDER BY port`)
}

// ListStaleLeases returns leases with state='stale'.
func (d *DB) ListStaleLeases() ([]Lease, error) {
	return d.queryLeases(`SELECT id, port, project, worktree, worktree_path, repo, name, hostname,
		expose, state, stale_since, created_at, last_used FROM leases WHERE state = 'stale' ORDER BY port`)
}

// ListExposeLeases returns leases with expose=true.
func (d *DB) ListExposeLeases() ([]Lease, error) {
	return d.queryLeases(`SELECT id, port, project, worktree, worktree_path, repo, name, hostname,
		expose, state, stale_since, created_at, last_used FROM leases WHERE expose = TRUE ORDER BY port`)
}

// AllocatedPorts returns all currently allocated port numbers.
func (d *DB) AllocatedPorts() ([]int, error) {
	rows, err := d.db.Query(`SELECT port FROM leases ORDER BY port`)
	if err != nil {
		return nil, fmt.Errorf("querying allocated ports: %w", err)
	}
	defer rows.Close()

	var ports []int
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, fmt.Errorf("scanning port: %w", err)
		}
		ports = append(ports, port)
	}
	return ports, rows.Err()
}

// GetLastGCTime returns the last GC execution time.
func (d *DB) GetLastGCTime() (time.Time, error) {
	var value string
	err := d.db.QueryRow(`SELECT value FROM gc_state WHERE key = 'last_gc_at'`).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("getting last GC time: %w", err)
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing last GC time: %w", err)
	}
	return t, nil
}

// SetLastGCTime updates the last GC execution time.
func (d *DB) SetLastGCTime(t time.Time) error {
	_, err := d.db.Exec(
		`INSERT INTO gc_state (key, value) VALUES ('last_gc_at', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		t.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("setting last GC time: %w", err)
	}
	return nil
}

// queryLeases is a helper for querying multiple leases.
func (d *DB) queryLeases(query string, args ...any) ([]Lease, error) {
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying leases: %w", err)
	}
	defer rows.Close()

	var leases []Lease
	for rows.Next() {
		var l Lease
		var staleSince sql.NullTime
		var createdAt, lastUsed sql.NullTime
		if err := rows.Scan(&l.ID, &l.Port, &l.Project, &l.Worktree, &l.WorktreePath,
			&l.Repo, &l.Name, &l.Hostname, &l.Expose, &l.State,
			&staleSince, &createdAt, &lastUsed); err != nil {
			return nil, fmt.Errorf("scanning lease: %w", err)
		}
		if staleSince.Valid {
			l.StaleSince = &staleSince.Time
		}
		if createdAt.Valid {
			l.CreatedAt = createdAt.Time
		}
		if lastUsed.Valid {
			l.LastUsed = lastUsed.Time
		}
		leases = append(leases, l)
	}
	return leases, rows.Err()
}

// scanner is an interface matching sql.Row and sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

// scanLease scans a single lease from a row.
func scanLease(row scanner) (*Lease, error) {
	var l Lease
	var staleSince sql.NullTime
	var createdAt, lastUsed sql.NullTime
	err := row.Scan(&l.ID, &l.Port, &l.Project, &l.Worktree, &l.WorktreePath,
		&l.Repo, &l.Name, &l.Hostname, &l.Expose, &l.State,
		&staleSince, &createdAt, &lastUsed)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scanning lease: %w", err)
	}
	if staleSince.Valid {
		l.StaleSince = &staleSince.Time
	}
	if createdAt.Valid {
		l.CreatedAt = createdAt.Time
	}
	if lastUsed.Valid {
		l.LastUsed = lastUsed.Time
	}
	return &l, nil
}
