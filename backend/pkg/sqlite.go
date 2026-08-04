package pkg

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteConfig содержит параметры подключения и настройки пула для SQLite
type SQLiteConfig struct {
	DBPath          string        `json:"db_path" yaml:"db_path"`
	MaxOpenConns    int           `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime"`
	BusyTimeout     time.Duration `json:"busy_timeout" yaml:"busy_timeout"`
}

// DefaultSQLiteConfig возвращает конфигурацию с оптимальными настройками по умолчанию
func DefaultSQLiteConfig(dbPath *string) SQLiteConfig {
	baseDBPath := "app.db"
	if dbPath != nil {
		baseDBPath = *dbPath
	}
	dsn := fmt.Sprintf("file:%s?_journal=WAL&_busy_timeout=5000&_fk=true", baseDBPath)
	return SQLiteConfig{
		DBPath:          dsn,
		MaxOpenConns:    10,              // Ограничиваем конкурентность для записи
		MaxIdleConns:    5,               // Держим несколько соединений "горячими"
		ConnMaxLifetime: 1 * time.Hour,   // Время жизни соединения
		BusyTimeout:     5 * time.Second, // Ждем до 5 секунд при блокировке файла
	}
}

func SqliteOpen(dbPath string, conf *SQLiteConfig) (*sql.DB, error) {
	cfg := DefaultSQLiteConfig(&dbPath)
	if conf != nil {
		cfg = *conf
	}

	// _journal=WAL: включает Write-Ahead Logging (чтение не блокирует запись)
	// _busy_timeout=5000: ждет до 5 секунд, если база заблокирована, вместо ошибки
	// _fk=true: включает поддержку внешних ключей (Foreign Keys)
	dsn := fmt.Sprintf("%s?_journal=WAL&_busy_timeout=5000&_fk=true", dbPath)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite3 db: %v", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	// Применяем настройки пула из конфига
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	if err := tables(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to tables sqlite database: %w", err)
	}

	return db, nil
}

func tables(db *sql.DB) error {
	query := `
		CREATE TABLE IF NOT EXISTS tasks (
    		id TEXT PRIMARY KEY,
    		username TEXT NOT NULL,
    		status TEXT NOT NULL,          -- 'pending', 'processing', 'success', 'failure'
    		chunks TEXT,                   -- JSON: chunks.AudioChunks
    		wave TEXT,                     -- JSON: []float64
    		result TEXT,                   -- JSON: chunks.FileInferenceResult
    		retry_count INTEGER DEFAULT 0,
    		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);	
	`
	_, err := db.ExecContext(context.Background(), query)
	if err != nil {
		return err
	}
	return nil
}
