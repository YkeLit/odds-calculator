package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"odds-calculator/backend/internal/models"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) (*Store, error) {
	if path == "" {
		path = "./odds.db"
	}
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
		f, err := os.OpenFile(path, os.O_CREATE, 0o644)
		if err != nil {
			return nil, fmt.Errorf("touch db file: %w", err)
		}
		_ = f.Close()
	}

	store := &Store{path: path}
	if err := store.initSchema(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return nil
}

func (s *Store) initSchema() error {
	schema := `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS histories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	game_type TEXT NOT NULL,
	request_json TEXT NOT NULL,
	response_json TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY(user_id) REFERENCES users(id)
);
CREATE INDEX IF NOT EXISTS idx_histories_user_created_at
ON histories(user_id, created_at DESC);
`
	if err := s.execSQL(schema); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}

func (s *Store) CreateUser(username, passwordHash string) (int64, error) {
	query := fmt.Sprintf(
		"INSERT INTO users(username, password_hash) VALUES(%s, %s); SELECT last_insert_rowid() as id;",
		sqlQuote(username),
		sqlQuote(passwordHash),
	)
	rows, err := s.queryJSON(query)
	if err != nil {
		return 0, fmt.Errorf("insert user: %w", err)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("insert user: missing row id")
	}
	id, err := toInt64(rows[0]["id"])
	if err != nil {
		return 0, fmt.Errorf("parse user id: %w", err)
	}
	return id, nil
}

func (s *Store) GetUserByUsername(username string) (models.User, error) {
	query := fmt.Sprintf(
		"SELECT id, username, password_hash, created_at FROM users WHERE username = %s LIMIT 1;",
		sqlQuote(username),
	)
	rows, err := s.queryJSON(query)
	if err != nil {
		return models.User{}, fmt.Errorf("query user: %w", err)
	}
	if len(rows) == 0 {
		return models.User{}, ErrNotFound
	}
	id, err := toInt64(rows[0]["id"])
	if err != nil {
		return models.User{}, err
	}
	return models.User{
		ID:           id,
		Username:     toString(rows[0]["username"]),
		PasswordHash: toString(rows[0]["password_hash"]),
		CreatedAt:    toString(rows[0]["created_at"]),
	}, nil
}

func (s *Store) CreateHistory(userID int64, gameType string, reqBody, respBody any) error {
	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	respJSON, err := json.Marshal(respBody)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	query := fmt.Sprintf(
		"INSERT INTO histories(user_id, game_type, request_json, response_json) VALUES(%d, %s, %s, %s);",
		userID,
		sqlQuote(gameType),
		sqlQuote(string(reqJSON)),
		sqlQuote(string(respJSON)),
	)
	if err := s.execSQL(query); err != nil {
		return fmt.Errorf("insert history: %w", err)
	}
	return nil
}

func (s *Store) ListHistory(userID int64, gameType string, page, pageSize int) ([]models.CalcHistoryRecord, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	countSQL := fmt.Sprintf("SELECT COUNT(1) as total FROM histories WHERE user_id = %d", userID)
	if gameType != "" {
		countSQL += fmt.Sprintf(" AND game_type = %s", sqlQuote(gameType))
	}
	countRows, err := s.queryJSON(countSQL + ";")
	if err != nil {
		return nil, 0, fmt.Errorf("count histories: %w", err)
	}
	total := 0
	if len(countRows) > 0 {
		total64, err := toInt64(countRows[0]["total"])
		if err != nil {
			return nil, 0, fmt.Errorf("parse history total: %w", err)
		}
		total = int(total64)
	}

	querySQL := fmt.Sprintf(`
SELECT id, game_type, request_json, response_json, created_at
FROM histories
WHERE user_id = %d`, userID)
	if gameType != "" {
		querySQL += fmt.Sprintf(" AND game_type = %s", sqlQuote(gameType))
	}
	querySQL += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d;", pageSize, offset)
	rows, err := s.queryJSON(querySQL)
	if err != nil {
		return nil, 0, fmt.Errorf("list histories: %w", err)
	}
	items := make([]models.CalcHistoryRecord, 0, len(rows))
	for _, row := range rows {
		id, err := toInt64(row["id"])
		if err != nil {
			return nil, 0, fmt.Errorf("parse history id: %w", err)
		}
		items = append(items, models.CalcHistoryRecord{
			ID:           id,
			GameType:     toString(row["game_type"]),
			RequestJSON:  toString(row["request_json"]),
			ResponseJSON: toString(row["response_json"]),
			CreatedAt:    toString(row["created_at"]),
		})
	}
	return items, total, nil
}

func (s *Store) execSQL(query string) error {
	_, err := s.runSQLite(query)
	return err
}

func (s *Store) queryJSON(query string) ([]map[string]any, error) {
	output, err := s.runSQLite(query)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(output) == "" {
		return []map[string]any{}, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return nil, fmt.Errorf("decode sqlite json: %w", err)
	}
	return rows, nil
}

func (s *Store) runSQLite(query string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cmd := exec.Command("sqlite3", "-json", s.path, query)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("sqlite3 error: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func sqlQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func toInt64(v any) (int64, error) {
	switch n := v.(type) {
	case float64:
		return int64(n), nil
	case string:
		return strconv.ParseInt(n, 10, 64)
	case json.Number:
		return n.Int64()
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", v)
	}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
