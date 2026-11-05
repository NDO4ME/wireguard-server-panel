package db

import "time"

// AuditEntry is one recorded administrative request.
type AuditEntry struct {
	ID     int64     `json:"id"`
	At     time.Time `json:"at"`
	IP     string    `json:"ip"`
	Method string    `json:"method"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
}

const auditKeep = 1000

// AddAudit stores an entry and keeps only the newest auditKeep rows.
func (d *Database) AddAudit(ip, method, path string, status int) error {
	if _, err := d.conn.Exec("INSERT INTO audit_log (ip, method, path, status) VALUES (?, ?, ?, ?)", ip, method, path, status); err != nil {
		return err
	}
	_, err := d.conn.Exec("DELETE FROM audit_log WHERE id <= (SELECT COALESCE(MAX(id),0) FROM audit_log) - ?", auditKeep)
	return err
}

// ListAudit returns the newest entries first.
func (d *Database) ListAudit(limit int) ([]AuditEntry, error) {
	rows, err := d.conn.Query("SELECT id, at, ip, method, path, status FROM audit_log ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.At, &e.IP, &e.Method, &e.Path, &e.Status); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
