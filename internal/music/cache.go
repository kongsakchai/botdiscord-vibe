package music

import (
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type SearchCache struct {
	db *sql.DB
	mu sync.Mutex
}

func NewSearchCache(path string) (*SearchCache, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS search_cache (
		query TEXT PRIMARY KEY,
		results TEXT NOT NULL,
		cached_at INTEGER NOT NULL
	)`); err != nil {
		db.Close()
		return nil, err
	}
	return &SearchCache{db: db}, nil
}

func (c *SearchCache) Lookup(query string) ([]VideoResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var resultsJSON string
	var cachedAt int64
	err := c.db.QueryRow("SELECT results, cached_at FROM search_cache WHERE query = ?", query).Scan(&resultsJSON, &cachedAt)
	if err != nil {
		return nil, false
	}
	if time.Now().Unix()-cachedAt > 86400 {
		return nil, false
	}
	var results []VideoResult
	if err := json.Unmarshal([]byte(resultsJSON), &results); err != nil {
		return nil, false
	}
	return results, true
}

func (c *SearchCache) Store(query string, results []VideoResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	jsonBytes, _ := json.Marshal(results)
	c.db.Exec("INSERT OR REPLACE INTO search_cache (query, results, cached_at) VALUES (?, ?, ?)",
		query, string(jsonBytes), time.Now().Unix())
}

func (c *SearchCache) Close() {
	c.db.Close()
}

var searchCache *SearchCache

func InitSearchCache(dbPath string) error {
	if dbPath == "" {
		dbPath = "search_cache.db"
	}
	var err error
	searchCache, err = NewSearchCache(dbPath)
	return err
}

func CloseSearchCache() {
	if searchCache != nil {
		searchCache.Close()
	}
}

func getSearchCache() *SearchCache {
	return searchCache
}