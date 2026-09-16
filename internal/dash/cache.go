package dash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// --- bench cache ---------------------------------------------------------------

type cacheDoc struct {
	At     time.Time `json:"at"`
	Report string    `json:"report"`
}

func benchCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "harness", "dash-bench.json"), nil
}

func loadBenchCache() *BenchCache {
	path, err := benchCachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc cacheDoc
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	return &BenchCache{Report: doc.Report, At: doc.At}
}

func writeBenchCache(at time.Time, report string) error {
	path, err := benchCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cacheDoc{At: at, Report: report})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
