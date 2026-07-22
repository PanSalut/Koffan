package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type ListActivity struct {
	ID           int64           `json:"id"`
	ListID       int64           `json:"list_id"`
	ItemID       *int64          `json:"item_id,omitempty"`
	ActorUserID  *int64          `json:"actor_user_id,omitempty"`
	ActorName    string          `json:"actor_name"`
	Action       string          `json:"action"`
	MetadataJSON json.RawMessage `json:"metadata"`
	CreatedAt    string          `json:"created_at"`
}

func LogListActivity(listID, itemID, actorUserID int64, action string, metadata interface{}) error {
	var encoded []byte
	var err error
	if metadata != nil {
		encoded, err = json.Marshal(metadata)
		if err != nil {
			return err
		}
	}
	_, err = DB.Exec(`INSERT INTO list_activity(list_id,item_id,actor_user_id,action,metadata_json) VALUES(?,NULLIF(?,0),NULLIF(?,0),?,NULLIF(?,''))`, listID, itemID, actorUserID, action, string(encoded))
	return err
}

func GetListActivity(listID int64, limit int) ([]ListActivity, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := DB.Query(`SELECT a.id,a.list_id,a.item_id,a.actor_user_id,COALESCE(u.display_name,u.username,'Deleted user'),a.action,COALESCE(a.metadata_json,'{}'),a.created_at FROM list_activity a LEFT JOIN users u ON u.id=a.actor_user_id WHERE a.list_id=? ORDER BY a.id DESC LIMIT ?`, listID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ListActivity
	for rows.Next() {
		var entry ListActivity
		var metadata string
		if err = rows.Scan(&entry.ID, &entry.ListID, &entry.ItemID, &entry.ActorUserID, &entry.ActorName, &entry.Action, &metadata, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entry.MetadataJSON = json.RawMessage(metadata)
		result = append(result, entry)
	}
	return result, rows.Err()
}

type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"display_name"`
	Email        *string    `json:"email,omitempty"`
	PasswordHash *string    `json:"-"`
	AuthSource   string     `json:"auth_source"`
	OIDCIssuer   *string    `json:"-"`
	OIDCSubject  *string    `json:"-"`
	IsAdmin      bool       `json:"is_admin"`
	Disabled     bool       `json:"disabled"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

type ListPermission int

const (
	ListPermissionNone ListPermission = iota
	ListPermissionView
	ListPermissionEdit
	ListPermissionManage
	ListPermissionOwner
)

func migrateMultiUser() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS users (
		 id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL COLLATE NOCASE,
		 display_name TEXT NOT NULL, email TEXT, password_hash TEXT, auth_source TEXT NOT NULL CHECK(auth_source IN ('local','oidc')),
		 oidc_issuer TEXT, oidc_subject TEXT, is_admin BOOLEAN NOT NULL DEFAULT FALSE, disabled BOOLEAN NOT NULL DEFAULT FALSE,
		 created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, last_login_at DATETIME)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_username_nocase ON users(username COLLATE NOCASE)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS users_oidc_identity ON users(oidc_issuer, oidc_subject) WHERE oidc_issuer IS NOT NULL AND oidc_subject IS NOT NULL`,
	}
	for _, q := range statements {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	columns := []struct{ table, name, definition string }{
		{"sessions", "user_id", "INTEGER REFERENCES users(id) ON DELETE CASCADE"},
		{"sessions", "created_at", "INTEGER"},
		{"sessions", "last_seen_at", "INTEGER"},
		{"sessions", "csrf_token", "TEXT"},
		{"lists", "owner_user_id", "INTEGER REFERENCES users(id) ON DELETE RESTRICT"},
		{"items", "created_by_user_id", "INTEGER REFERENCES users(id) ON DELETE SET NULL"},
		{"items", "updated_by_user_id", "INTEGER REFERENCES users(id) ON DELETE SET NULL"},
		{"templates", "owner_user_id", "INTEGER REFERENCES users(id) ON DELETE SET NULL"},
	}
	for _, col := range columns {
		var n int
		if err := tx.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name=?", col.table), col.name).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", col.table, col.name, col.definition)); err != nil {
				return err
			}
		}
	}
	more := []string{
		`CREATE TABLE IF NOT EXISTS list_members (list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, role TEXT NOT NULL CHECK(role IN ('viewer','editor')), granted_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(list_id,user_id))`,
		`CREATE TABLE IF NOT EXISTS user_preferences (user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE, active_list_id INTEGER REFERENCES lists(id) ON DELETE SET NULL)`,
		`CREATE TABLE IF NOT EXISTS user_list_preferences (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE, position INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(user_id,list_id))`,
		`CREATE TABLE IF NOT EXISTS list_activity (id INTEGER PRIMARY KEY AUTOINCREMENT, list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE, item_id INTEGER REFERENCES items(id) ON DELETE SET NULL, actor_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL, action TEXT NOT NULL, metadata_json TEXT, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS groups (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL COLLATE NOCASE, description TEXT NOT NULL DEFAULT '', is_admin BOOLEAN NOT NULL DEFAULT FALSE, oidc_group_value TEXT COLLATE NOCASE, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS groups_name_nocase ON groups(name COLLATE NOCASE)`,
		`CREATE TABLE IF NOT EXISTS user_groups (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE, source TEXT NOT NULL DEFAULT 'local' CHECK(source IN ('local','oidc')), created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(user_id,group_id,source))`,
		`CREATE TABLE IF NOT EXISTS group_list_members (list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE, group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE, role TEXT NOT NULL CHECK(role IN ('viewer','editor')), granted_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(list_id,group_id))`,
		`CREATE INDEX IF NOT EXISTS user_groups_group_idx ON user_groups(group_id)`,
		`CREATE INDEX IF NOT EXISTS group_list_members_group_idx ON group_list_members(group_id)`,
		`CREATE INDEX IF NOT EXISTS list_members_user_id_idx ON list_members(user_id)`, `CREATE INDEX IF NOT EXISTS lists_owner_user_id_idx ON lists(owner_user_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS lists_owner_name_nocase ON lists(owner_user_id,name COLLATE NOCASE) WHERE owner_user_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS items_created_by_user_id_idx ON items(created_by_user_id)`, `CREATE INDEX IF NOT EXISTS list_activity_list_created_idx ON list_activity(list_id,created_at)`,
		`INSERT OR IGNORE INTO schema_migrations(version) VALUES (1)`,
	}
	for _, q := range more {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	var groupAdminColumn int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('groups') WHERE name='is_admin'`).Scan(&groupAdminColumn); err != nil {
		return err
	}
	if groupAdminColumn == 0 {
		if _, err := tx.Exec(`ALTER TABLE groups ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT FALSE`); err != nil {
			return err
		}
	}
	var memberSchema string
	if err := tx.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='list_members'`).Scan(&memberSchema); err != nil {
		return err
	}
	if !strings.Contains(memberSchema, "'manager'") {
		migration := []string{
			`CREATE TABLE list_members_full_control (list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, role TEXT NOT NULL CHECK(role IN ('viewer','editor','manager')), granted_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(list_id,user_id))`,
			`INSERT INTO list_members_full_control SELECT list_id,user_id,role,granted_by_user_id,created_at,updated_at FROM list_members`,
			`DROP TABLE list_members`,
			`ALTER TABLE list_members_full_control RENAME TO list_members`,
			`CREATE INDEX IF NOT EXISTS list_members_user_id_idx ON list_members(user_id)`,
			`CREATE TABLE group_list_members_full_control (list_id INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE, group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE, role TEXT NOT NULL CHECK(role IN ('viewer','editor','manager')), granted_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(list_id,group_id))`,
			`INSERT INTO group_list_members_full_control SELECT list_id,group_id,role,granted_by_user_id,created_at,updated_at FROM group_list_members`,
			`DROP TABLE group_list_members`,
			`ALTER TABLE group_list_members_full_control RENAME TO group_list_members`,
			`CREATE INDEX IF NOT EXISTS group_list_members_group_idx ON group_list_members(group_id)`,
		}
		for _, statement := range migration {
			if _, err := tx.Exec(statement); err != nil {
				return fmt.Errorf("migrate full-control list roles: %w", err)
			}
		}
	}
	var historyUserColumn int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('item_history') WHERE name='user_id'`).Scan(&historyUserColumn); err != nil {
		return err
	}
	if historyUserColumn == 0 {
		historyMigration := []string{
			`CREATE TABLE item_history_per_user (id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER REFERENCES users(id) ON DELETE CASCADE, name TEXT NOT NULL COLLATE NOCASE, last_section_id INTEGER REFERENCES sections(id) ON DELETE SET NULL, usage_count INTEGER DEFAULT 1, last_used_at INTEGER DEFAULT (strftime('%s','now')), UNIQUE(user_id,name COLLATE NOCASE))`,
			// Legacy databases can contain history references to sections that were
			// subsequently deleted while foreign-key enforcement was disabled. Keep
			// the suggestion, but clear an invalid section reference so copying into
			// the new foreign-key-enforced table cannot abort the whole migration.
			`INSERT INTO item_history_per_user(id,name,last_section_id,usage_count,last_used_at)
			 SELECT h.id,h.name,
			        CASE WHEN s.id IS NOT NULL THEN h.last_section_id ELSE NULL END,
			        h.usage_count,h.last_used_at
			 FROM item_history h
			 LEFT JOIN sections s ON s.id=h.last_section_id`,
			`DROP TABLE item_history`,
			`ALTER TABLE item_history_per_user RENAME TO item_history`,
			`CREATE INDEX IF NOT EXISTS idx_item_history_name ON item_history(user_id,name COLLATE NOCASE)`,
		}
		for _, statement := range historyMigration {
			if _, err := tx.Exec(statement); err != nil {
				return fmt.Errorf("migrate per-user item history: %w", err)
			}
		}
	}
	if _, err := tx.Exec(`UPDATE item_history SET user_id=(SELECT id FROM users WHERE is_admin=TRUE ORDER BY id LIMIT 1) WHERE user_id IS NULL AND EXISTS(SELECT 1 FROM users WHERE is_admin=TRUE)`); err != nil {
		return err
	}
	return tx.Commit()
}

func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func BootstrapInitialAdmin() error {
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil || count > 0 {
		return err
	}
	if strings.EqualFold(os.Getenv("DISABLE_AUTH"), "true") {
		return nil
	}
	username, password := strings.TrimSpace(os.Getenv("ADMIN_USERNAME")), os.Getenv("ADMIN_PASSWORD")
	display := strings.TrimSpace(os.Getenv("ADMIN_DISPLAY_NAME"))
	if username == "" || password == "" {
		if legacy := os.Getenv("APP_PASSWORD"); legacy != "" {
			log.Print("DEPRECATION: APP_PASSWORD was used once to bootstrap user 'admin'; configure ADMIN_USERNAME and ADMIN_PASSWORD")
			username, password = "admin", legacy
		} else if os.Getenv("APP_ENV") != "production" {
			log.Print("WARNING: development administrator created with the documented default credentials")
			username, password = "admin", "shopping123"
		} else {
			return errors.New("no users exist; set ADMIN_USERNAME and ADMIN_PASSWORD")
		}
	}
	if display == "" {
		display = username
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	res, err := DB.Exec(`INSERT INTO users(username,display_name,password_hash,auth_source,is_admin) VALUES(?,?,?,'local',TRUE)`, username, display, hash)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	_, err = DB.Exec(`UPDATE lists SET owner_user_id=? WHERE owner_user_id IS NULL`, id)
	_, _ = DB.Exec(`UPDATE templates SET owner_user_id=? WHERE owner_user_id IS NULL`, id)
	_, _ = DB.Exec(`UPDATE item_history SET user_id=? WHERE user_id IS NULL`, id)
	return err
}

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.PasswordHash, &u.AuthSource, &u.OIDCIssuer, &u.OIDCSubject, &u.IsAdmin, &u.Disabled, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	return u, err
}

const userColumns = `id,username,display_name,email,password_hash,auth_source,oidc_issuer,oidc_subject,is_admin,disabled,created_at,updated_at,last_login_at`

func GetUserByUsername(username string) (*User, error) {
	return scanUser(DB.QueryRow(`SELECT `+userColumns+` FROM users WHERE username=? COLLATE NOCASE`, strings.TrimSpace(username)))
}
func GetUserByID(id int64) (*User, error) {
	return scanUser(DB.QueryRow(`SELECT `+userColumns+` FROM users WHERE id=?`, id))
}
func MarkUserLogin(id int64) error {
	_, err := DB.Exec(`UPDATE users SET last_login_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}
func InvalidateUserSessions(id int64) error {
	_, err := DB.Exec(`DELETE FROM sessions WHERE user_id=?`, id)
	return err
}

func GetUserListPermission(userID, listID int64, isAdmin bool) (ListPermission, error) {
	if isAdmin {
		var n int
		err := DB.QueryRow(`SELECT COUNT(*) FROM lists WHERE id=?`, listID).Scan(&n)
		if err != nil || n == 0 {
			return ListPermissionNone, err
		}
		return ListPermissionOwner, nil
	}
	var owner int64
	var role sql.NullString
	err := DB.QueryRow(`SELECT l.owner_user_id,
	 CASE MAX(CASE WHEN m.role='manager' OR gm.role='manager' THEN 3 WHEN m.role='editor' OR gm.role='editor' THEN 2 WHEN m.role='viewer' OR gm.role='viewer' THEN 1 ELSE 0 END)
	  WHEN 3 THEN 'manager' WHEN 2 THEN 'editor' WHEN 1 THEN 'viewer' END
	 FROM lists l
	 LEFT JOIN list_members m ON m.list_id=l.id AND m.user_id=?
	 LEFT JOIN user_groups ug ON ug.user_id=?
	 LEFT JOIN group_list_members gm ON gm.list_id=l.id AND gm.group_id=ug.group_id
	 WHERE l.id=? GROUP BY l.id`, userID, userID, listID).Scan(&owner, &role)
	if err != nil {
		return ListPermissionNone, err
	}
	if owner == userID {
		return ListPermissionOwner, nil
	}
	if role.String == "manager" {
		return ListPermissionManage, nil
	}
	if role.String == "editor" {
		return ListPermissionEdit, nil
	}
	if role.String == "viewer" {
		return ListPermissionView, nil
	}
	return ListPermissionNone, nil
}
func CanViewList(uid, lid int64, admin bool) (bool, error) {
	p, e := GetUserListPermission(uid, lid, admin)
	return p >= ListPermissionView, e
}
func CanEditList(uid, lid int64, admin bool) (bool, error) {
	p, e := GetUserListPermission(uid, lid, admin)
	return p >= ListPermissionEdit, e
}
func CanManageList(uid, lid int64, admin bool) (bool, error) {
	p, e := GetUserListPermission(uid, lid, admin)
	return p >= ListPermissionManage, e
}
func ListIDForSection(id int64) (int64, error) {
	var listID int64
	err := DB.QueryRow(`SELECT list_id FROM sections WHERE id=?`, id).Scan(&listID)
	return listID, err
}
func ListIDForItem(id int64) (int64, error) {
	var listID int64
	err := DB.QueryRow(`SELECT s.list_id FROM items i JOIN sections s ON s.id=i.section_id WHERE i.id=?`, id).Scan(&listID)
	return listID, err
}

func GetListsForUser(userID int64, isAdmin bool) ([]List, error) {
	query := listSelectWithStats + ` LEFT JOIN user_list_preferences ulp ON ulp.list_id=l.id AND ulp.user_id=?`
	args := []any{userID}
	if !isAdmin {
		query += ` LEFT JOIN list_members lm_access ON lm_access.list_id=l.id AND lm_access.user_id=? WHERE l.owner_user_id=? OR lm_access.user_id=? OR EXISTS (SELECT 1 FROM user_groups ug JOIN group_list_members glm ON glm.group_id=ug.group_id WHERE ug.user_id=? AND glm.list_id=l.id)`
		args = append(args, userID, userID, userID, userID)
	}
	query += ` GROUP BY l.id ORDER BY COALESCE(ulp.position,l.sort_order) ASC,l.id ASC`
	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lists []List
	for rows.Next() {
		l, e := scanListWithStats(rows)
		if e != nil {
			return nil, e
		}
		l.CanManage = isAdmin || l.OwnerUserID == userID
		lists = append(lists, l)
	}
	_ = rows.Close()
	for i := range lists {
		if lists[i].OwnerUserID == userID {
			lists[i].AccessRole = "owner"
			lists[i].CanManage = true
		} else if isAdmin {
			lists[i].AccessRole = "admin"
			lists[i].CanManage = true
		} else {
			permission, e := GetUserListPermission(userID, lists[i].ID, false)
			if e != nil {
				return nil, e
			}
			if permission == ListPermissionManage {
				lists[i].AccessRole = "manager"
				lists[i].CanManage = true
			} else if permission == ListPermissionEdit {
				lists[i].AccessRole = "editor"
			} else {
				lists[i].AccessRole = "viewer"
			}
		}
	}
	return lists, rows.Err()
}

func MoveListForUser(userID int64, isAdmin bool, listID int64, direction int) error {
	lists, err := GetListsForUser(userID, isAdmin)
	if err != nil {
		return err
	}
	index := -1
	for i := range lists {
		if lists[i].ID == listID {
			index = i
			break
		}
	}
	if index < 0 {
		return sql.ErrNoRows
	}
	target := index + direction
	if target < 0 || target >= len(lists) {
		return nil
	}
	lists[index], lists[target] = lists[target], lists[index]
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for position, list := range lists {
		if _, err = tx.Exec(`INSERT INTO user_list_preferences(user_id,list_id,position) VALUES(?,?,?) ON CONFLICT(user_id,list_id) DO UPDATE SET position=excluded.position`, userID, list.ID, position); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func GetListsOwnedByUser(userID int64) ([]List, error) {
	query := listSelectWithStats + ` WHERE l.owner_user_id=? GROUP BY l.id ORDER BY l.sort_order ASC`
	rows, err := DB.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lists []List
	for rows.Next() {
		list, scanErr := scanListWithStats(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		list.AccessRole = "owner"
		list.CanManage = true
		lists = append(lists, list)
	}
	return lists, rows.Err()
}

func ListNameExistsForOwner(name string, ownerUserID, excludeID int64) (bool, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM lists WHERE owner_user_id=? AND name=? COLLATE NOCASE AND id!=?`, ownerUserID, name, excludeID).Scan(&count)
	return count > 0, err
}

func CreateListForUser(userID int64, name, icon string) (*List, error) {
	var maxOrder int
	_ = DB.QueryRow(`SELECT COALESCE(MAX(sort_order),-1) FROM lists`).Scan(&maxOrder)
	if icon == "" {
		icon = "🛒"
	}
	res, err := DB.Exec(`INSERT INTO lists(name,icon,sort_order,is_active,owner_user_id) VALUES(?,?,?,FALSE,?)`, name, icon, maxOrder+1, userID)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return GetListByID(id)
}

func GetActiveListForUser(userID int64, isAdmin bool) (*List, error) {
	var id int64
	err := DB.QueryRow(`SELECT active_list_id FROM user_preferences WHERE user_id=?`, userID).Scan(&id)
	if err == nil {
		ok, e := CanViewList(userID, id, isAdmin)
		if e == nil && ok {
			return GetListByID(id)
		}
	}
	lists, e := GetListsForUser(userID, isAdmin)
	if e != nil {
		return nil, e
	}
	if len(lists) == 0 {
		return nil, sql.ErrNoRows
	}
	return &lists[0], nil
}
func SetActiveListForUser(userID, listID int64) error {
	_, err := DB.Exec(`INSERT INTO user_preferences(user_id,active_list_id) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET active_list_id=excluded.active_list_id`, userID, listID)
	return err
}

type ListMember struct {
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

func GetListMembers(listID int64) ([]ListMember, error) {
	rows, err := DB.Query(`SELECT u.id,u.username,u.display_name,m.role FROM list_members m JOIN users u ON u.id=m.user_id WHERE m.list_id=? ORDER BY u.display_name COLLATE NOCASE`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ListMember
	for rows.Next() {
		var m ListMember
		if err := rows.Scan(&m.UserID, &m.Username, &m.DisplayName, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func SetListMember(listID, userID, grantedBy int64, role string) error {
	if role != "viewer" && role != "editor" && role != "manager" {
		return errors.New("invalid member role")
	}
	var owner int64
	if err := DB.QueryRow(`SELECT owner_user_id FROM lists WHERE id=?`, listID).Scan(&owner); err != nil {
		return err
	}
	if owner == userID {
		return errors.New("owner cannot be a member")
	}
	_, err := DB.Exec(`INSERT INTO list_members(list_id,user_id,role,granted_by_user_id) VALUES(?,?,?,?) ON CONFLICT(list_id,user_id) DO UPDATE SET role=excluded.role,granted_by_user_id=excluded.granted_by_user_id,updated_at=CURRENT_TIMESTAMP`, listID, userID, role, grantedBy)
	return err
}
func RemoveListMember(listID, userID int64) error {
	_, err := DB.Exec(`DELETE FROM list_members WHERE list_id=? AND user_id=?`, listID, userID)
	return err
}

// TransferListOwnership gives a list to another enabled user. The previous
// owner retains manager access. Name conflicts are resolved without exposing
// or overwriting the destination owner's existing list.
func TransferListOwnership(listID, newOwnerID, grantedBy int64) (*List, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var oldOwnerID int64
	var originalName, oldOwnerName string
	if err = tx.QueryRow(`SELECT l.owner_user_id,l.name,u.display_name FROM lists l JOIN users u ON u.id=l.owner_user_id WHERE l.id=?`, listID).Scan(&oldOwnerID, &originalName, &oldOwnerName); err != nil {
		return nil, err
	}
	if oldOwnerID == newOwnerID {
		return nil, errors.New("selected user already owns this list")
	}
	var enabled bool
	if err = tx.QueryRow(`SELECT disabled=FALSE FROM users WHERE id=?`, newOwnerID).Scan(&enabled); err != nil {
		return nil, err
	}
	if !enabled {
		return nil, errors.New("the new owner must be an enabled user")
	}

	name := originalName
	var conflict int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM lists WHERE owner_user_id=? AND name=? COLLATE NOCASE`, newOwnerID, name).Scan(&conflict); err != nil {
		return nil, err
	}
	if conflict > 0 {
		for suffixNumber := 1; ; suffixNumber++ {
			suffix := " (from " + oldOwnerName + ")"
			if suffixNumber > 1 {
				suffix = fmt.Sprintf(" (from %s %d)", oldOwnerName, suffixNumber)
			}
			base := []rune(originalName)
			maxBase := 100 - len([]rune(suffix))
			if maxBase < 1 {
				maxBase = 1
			}
			if len(base) > maxBase {
				base = base[:maxBase]
			}
			name = strings.TrimSpace(string(base)) + suffix
			if err = tx.QueryRow(`SELECT COUNT(*) FROM lists WHERE owner_user_id=? AND name=? COLLATE NOCASE`, newOwnerID, name).Scan(&conflict); err != nil {
				return nil, err
			}
			if conflict == 0 {
				break
			}
		}
	}

	if _, err = tx.Exec(`DELETE FROM list_members WHERE list_id=? AND user_id=?`, listID, newOwnerID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`UPDATE lists SET owner_user_id=?,name=?,updated_at=strftime('%s','now') WHERE id=?`, newOwnerID, name, listID); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(`INSERT INTO list_members(list_id,user_id,role,granted_by_user_id) VALUES(?,?,'manager',?) ON CONFLICT(list_id,user_id) DO UPDATE SET role='manager',granted_by_user_id=excluded.granted_by_user_id,updated_at=CURRENT_TIMESTAMP`, listID, oldOwnerID, grantedBy); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetListByID(listID)
}

func SearchUsers(query string, limit int) ([]User, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	q := "%" + strings.TrimSpace(query) + "%"
	rows, err := DB.Query(`SELECT `+userColumns+` FROM users WHERE disabled=FALSE AND (username LIKE ? COLLATE NOCASE OR display_name LIKE ? COLLATE NOCASE) ORDER BY display_name LIMIT ?`, q, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}
func ListUsers() ([]User, error) {
	rows, err := DB.Query(`SELECT ` + userColumns + ` FROM users ORDER BY username COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}
func CreateLocalUser(username, display, password string, admin bool) (*User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	res, err := DB.Exec(`INSERT INTO users(username,display_name,password_hash,auth_source,is_admin) VALUES(?,?,?,'local',?)`, strings.TrimSpace(username), strings.TrimSpace(display), hash, admin)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return GetUserByID(id)
}

func ResetLocalUserPassword(userID int64, password string, enable bool) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	var source string
	if err = DB.QueryRow(`SELECT auth_source FROM users WHERE id=?`, userID).Scan(&source); err != nil {
		return err
	}
	if source != "local" {
		return errors.New("password recovery is only available for local accounts")
	}
	if enable {
		_, err = DB.Exec(`UPDATE users SET password_hash=?,disabled=FALSE,updated_at=CURRENT_TIMESTAMP WHERE id=?`, hash, userID)
	} else {
		_, err = DB.Exec(`UPDATE users SET password_hash=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, hash, userID)
	}
	if err != nil {
		return err
	}
	return InvalidateUserSessions(userID)
}

func CreateRecoveryAdmin(username, display, password string) (*User, error) {
	u, err := CreateLocalUser(username, display, password, true)
	if err != nil {
		return nil, err
	}
	if _, err = DB.Exec(`UPDATE lists SET owner_user_id=? WHERE owner_user_id IS NULL`, u.ID); err != nil {
		return nil, err
	}
	if _, err = DB.Exec(`UPDATE templates SET owner_user_id=? WHERE owner_user_id IS NULL`, u.ID); err != nil {
		return nil, err
	}
	return u, nil
}

type Group struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsAdmin     bool   `json:"is_admin"`
	Users       []User `json:"users,omitempty"`
}
type GroupListAccess struct {
	ListID    int64  `json:"list_id"`
	ListName  string `json:"list_name"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Role      string `json:"role"`
}

func ListGroups() ([]Group, error) {
	rows, err := DB.Query(`SELECT id,name,description,COALESCE(is_admin,FALSE) FROM groups ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.IsAdmin); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	for i := range out {
		users, e := GetGroupUsers(out[i].ID)
		if e != nil {
			return nil, e
		}
		out[i].Users = users
	}
	return out, nil
}
func CreateGroup(name, description string, oidcValue *string) (*Group, error) {
	return CreateGroupWithAdmin(name, description, oidcValue, false)
}
func CreateGroupWithAdmin(name, description string, oidcValue *string, isAdmin bool) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("group name is required")
	}
	res, err := DB.Exec(`INSERT INTO groups(name,description,is_admin,oidc_group_value) VALUES(?,?,?,NULL)`, name, strings.TrimSpace(description), isAdmin)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return GetGroup(id)
}
func GetGroup(id int64) (*Group, error) {
	var g Group
	err := DB.QueryRow(`SELECT id,name,description,COALESCE(is_admin,FALSE) FROM groups WHERE id=?`, id).Scan(&g.ID, &g.Name, &g.Description, &g.IsAdmin)
	if err != nil {
		return nil, err
	}
	g.Users, err = GetGroupUsers(id)
	return &g, err
}
func DeleteGroup(id int64) error { _, err := DB.Exec(`DELETE FROM groups WHERE id=?`, id); return err }
func SetGroupAdmin(id int64, isAdmin bool) error {
	_, err := DB.Exec(`UPDATE groups SET is_admin=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, isAdmin, id)
	return err
}
func UpdateGroup(id int64, name, description string) (*Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("group name is required")
	}
	result, err := DB.Exec(`UPDATE groups SET name=?,description=?,oidc_group_value=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, strings.TrimSpace(description), id)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, sql.ErrNoRows
	}
	return GetGroup(id)
}
func IsUserAdministrator(userID int64) (bool, error) {
	var admin bool
	err := DB.QueryRow(`SELECT CASE WHEN u.is_admin=TRUE OR EXISTS(SELECT 1 FROM user_groups ug JOIN groups g ON g.id=ug.group_id WHERE ug.user_id=u.id AND g.is_admin=TRUE) THEN TRUE ELSE FALSE END FROM users u WHERE u.id=?`, userID).Scan(&admin)
	return admin, err
}
func GetGroupUsers(groupID int64) ([]User, error) {
	rows, err := DB.Query(`SELECT u.id,u.username,u.display_name,u.email,u.password_hash,u.auth_source,u.oidc_issuer,u.oidc_subject,u.is_admin,u.disabled,u.created_at,u.updated_at,u.last_login_at FROM users u JOIN (SELECT DISTINCT user_id FROM user_groups WHERE group_id=?) ug ON ug.user_id=u.id ORDER BY u.display_name COLLATE NOCASE`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}
func AddUserToGroup(userID, groupID int64, source string) error {
	var authSource string
	if err := DB.QueryRow(`SELECT auth_source FROM users WHERE id=?`, userID).Scan(&authSource); err != nil {
		return err
	}
	if authSource == "oidc" && source != "oidc" {
		return errors.New("OIDC user group membership is managed by the identity provider")
	}
	if source != "oidc" {
		source = "local"
	}
	_, err := DB.Exec(`INSERT OR IGNORE INTO user_groups(user_id,group_id,source) VALUES(?,?,?)`, userID, groupID, source)
	return err
}
func RemoveUserFromGroup(userID, groupID int64) error {
	_, err := DB.Exec(`DELETE FROM user_groups WHERE user_id=? AND group_id=? AND source='local'`, userID, groupID)
	return err
}
func SetGroupListAccess(listID, groupID, grantedBy int64, role string) error {
	if role != "viewer" && role != "editor" && role != "manager" {
		return errors.New("invalid group role")
	}
	_, err := DB.Exec(`INSERT INTO group_list_members(list_id,group_id,role,granted_by_user_id) VALUES(?,?,?,?) ON CONFLICT(list_id,group_id) DO UPDATE SET role=excluded.role,granted_by_user_id=excluded.granted_by_user_id,updated_at=CURRENT_TIMESTAMP`, listID, groupID, role, grantedBy)
	return err
}
func RemoveGroupListAccess(listID, groupID int64) error {
	_, err := DB.Exec(`DELETE FROM group_list_members WHERE list_id=? AND group_id=?`, listID, groupID)
	return err
}
func ListGroupAccess() ([]GroupListAccess, error) {
	rows, err := DB.Query(`SELECT glm.list_id,l.name,glm.group_id,g.name,glm.role FROM group_list_members glm JOIN lists l ON l.id=glm.list_id JOIN groups g ON g.id=glm.group_id ORDER BY l.name COLLATE NOCASE,g.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GroupListAccess
	for rows.Next() {
		var a GroupListAccess
		if err := rows.Scan(&a.ListID, &a.ListName, &a.GroupID, &a.GroupName, &a.Role); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func GetListGroupAccess(listID int64) ([]GroupListAccess, error) {
	rows, err := DB.Query(`SELECT glm.list_id,l.name,glm.group_id,g.name,glm.role FROM group_list_members glm JOIN lists l ON l.id=glm.list_id JOIN groups g ON g.id=glm.group_id WHERE glm.list_id=? ORDER BY g.name COLLATE NOCASE`, listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GroupListAccess
	for rows.Next() {
		var a GroupListAccess
		if err := rows.Scan(&a.ListID, &a.ListName, &a.GroupID, &a.GroupName, &a.Role); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func UpdateUserByAdmin(actorID, userID int64, username, display string, isAdmin, disabled bool, password string) (*User, error) {
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if actorID == userID && disabled {
		return nil, errors.New("you cannot disable your own account")
	}
	var currentAdmin, currentDisabled bool
	if err = tx.QueryRow(`SELECT is_admin,disabled FROM users WHERE id=?`, userID).Scan(&currentAdmin, &currentDisabled); err != nil {
		return nil, err
	}
	if currentAdmin && !currentDisabled && (!isAdmin || disabled) {
		var n int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=TRUE AND disabled=FALSE`).Scan(&n); err != nil {
			return nil, err
		}
		if n <= 1 {
			return nil, errors.New("cannot remove or disable the last enabled administrator")
		}
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(display) == "" {
		return nil, errors.New("username and display name are required")
	}
	if _, err = tx.Exec(`UPDATE users SET username=?,display_name=?,is_admin=?,disabled=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, strings.TrimSpace(username), strings.TrimSpace(display), isAdmin, disabled, userID); err != nil {
		return nil, err
	}
	if password != "" {
		hash, e := HashPassword(password)
		if e != nil {
			return nil, e
		}
		if _, err = tx.Exec(`UPDATE users SET password_hash=?,auth_source='local',updated_at=CURRENT_TIMESTAMP WHERE id=?`, hash, userID); err != nil {
			return nil, err
		}
	}
	if disabled {
		if _, err = tx.Exec(`DELETE FROM sessions WHERE user_id=?`, userID); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return GetUserByID(userID)
}

func DeleteUserPermanently(actorID, userID int64) (int64, error) {
	if actorID == userID {
		return 0, errors.New("you cannot delete your own account")
	}
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var admin, disabled bool
	if err = tx.QueryRow(`SELECT is_admin,disabled FROM users WHERE id=?`, userID).Scan(&admin, &disabled); err != nil {
		return 0, err
	}
	if admin && !disabled {
		var count int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=TRUE AND disabled=FALSE`).Scan(&count); err != nil {
			return 0, err
		}
		if count <= 1 {
			return 0, errors.New("cannot delete the last enabled administrator")
		}
	}
	result, err := tx.Exec(`DELETE FROM lists WHERE owner_user_id=?`, userID)
	if err != nil {
		return 0, err
	}
	deletedLists, _ := result.RowsAffected()
	if _, err = tx.Exec(`DELETE FROM users WHERE id=?`, userID); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return deletedLists, nil
}

func OIDCAutoCreateGroups() (bool, error) {
	if raw, exists := os.LookupEnv("OIDC_AUTO_CREATE_GROUPS"); exists {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true", "1", "yes", "on":
			return true, nil
		case "false", "0", "no", "off", "":
			return false, nil
		default:
			return false, fmt.Errorf("OIDC_AUTO_CREATE_GROUPS must be true or false")
		}
	}
	return false, nil
}

// SyncOIDCGroups makes every group membership for an OIDC user exactly match
// the provider claim. Koffan never acts as a second source of truth for them.
func SyncOIDCGroups(userID int64, claims []string, autoCreate bool) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM user_groups WHERE user_id=?`, userID); err != nil {
		return err
	}
	seen := make(map[string]struct{})
	for _, claim := range claims {
		claim = strings.TrimSpace(claim)
		key := strings.ToLower(claim)
		if claim == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if autoCreate {
			if _, err = tx.Exec(`INSERT OR IGNORE INTO groups(name,description,oidc_group_value) VALUES(?,'Created from OIDC group claim',NULL)`, claim); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO user_groups(user_id,group_id,source) SELECT ?,id,'oidc' FROM groups WHERE name=? COLLATE NOCASE`, userID, claim); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func GetUserByOIDC(issuer, subject string) (*User, error) {
	return scanUser(DB.QueryRow(`SELECT `+userColumns+` FROM users WHERE oidc_issuer=? AND oidc_subject=?`, issuer, subject))
}
func UpsertOIDCUser(issuer, subject, username, display string, email *string, autoCreate, grantAdmin bool) (*User, error) {
	u, err := GetUserByOIDC(issuer, subject)
	if err == nil {
		// For OIDC identities, OIDC_ADMIN_GROUP is authoritative on every login.
		// This intentionally revokes a previous OIDC-derived grant when the claim
		// is no longer present.
		_, err = DB.Exec(`UPDATE users SET display_name=?,email=?,is_admin=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, display, email, grantAdmin, u.ID)
		if err != nil {
			return nil, err
		}
		return GetUserByID(u.ID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if !autoCreate {
		return nil, errors.New("OIDC account is not provisioned")
	}
	base := strings.TrimSpace(username)
	if base == "" {
		base = "oidc-user"
	}
	candidate := base
	for n := 2; ; n++ {
		var count int
		if e := DB.QueryRow(`SELECT COUNT(*) FROM users WHERE username=? COLLATE NOCASE`, candidate).Scan(&count); e != nil {
			return nil, e
		}
		if count == 0 {
			break
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
	if strings.TrimSpace(display) == "" {
		display = candidate
	}
	res, err := DB.Exec(`INSERT INTO users(username,display_name,email,auth_source,oidc_issuer,oidc_subject,is_admin) VALUES(?,?,?,'oidc',?,?,?)`, candidate, display, email, issuer, subject, grantAdmin)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return GetUserByID(id)
}
