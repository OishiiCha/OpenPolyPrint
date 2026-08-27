package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/lucas/openpolyprint/internal/filament"
	"github.com/lucas/openpolyprint/internal/history"
)

// Stats holds aggregated analytics data.
type Stats struct {
	TotalPrints      int     `json:"totalPrints"`
	SuccessfulPrints int     `json:"successfulPrints"`
	FailedPrints     int     `json:"failedPrints"`
	CancelledPrints  int     `json:"cancelledPrints"`
	SuccessRate      float64 `json:"successRate"`
	TotalPrintTime   int64   `json:"totalPrintTimeSeconds"`
	TotalFilamentG   float64 `json:"totalFilamentUsedG"`
	TotalCost        float64 `json:"totalEstimatedCost"`
	AvgPrintTime     int64   `json:"avgPrintTimeSeconds"`

	PrintsPerPrinter map[string]int `json:"printsPerPrinter"`
	PrintsPerFile    map[string]int `json:"printsPerFile"`
	PrintsPerDay     map[string]int `json:"printsPerDay"`

	FilamentByType  map[string]float64 `json:"filamentByType"`
	FilamentByBrand map[string]float64 `json:"filamentByBrand"`

	RecentFailures []RecentPrint `json:"recentFailures"`
	LongestPrints  []RecentPrint `json:"longestPrints"`
}

// RecentPrint is a summary of a single print for lists in analytics.
type RecentPrint struct {
	ID       string `json:"id"`
	Printer  string `json:"printer"`
	File     string `json:"file"`
	Result   string `json:"result"`
	Started  string `json:"started"`
	Duration string `json:"duration"`
}

// Compute aggregates analytics from the history and filament stores.
func Compute(hist *history.Store, fil *filament.Store) Stats {
	records := hist.List()
	s := Stats{
		PrintsPerPrinter: make(map[string]int),
		PrintsPerFile:    make(map[string]int),
		PrintsPerDay:     make(map[string]int),
		FilamentByType:   make(map[string]float64),
		FilamentByBrand:  make(map[string]float64),
	}

	var totalTime int64
	var successCount int

	for _, r := range records {
		s.TotalPrints++
		s.PrintsPerPrinter[r.Printer]++
		s.PrintsPerFile[r.File]++
		day := r.StartedAt.Format("2006-01-02")
		s.PrintsPerDay[day]++

		switch r.Result {
		case "Success":
			s.SuccessfulPrints++
			successCount++
		case "Failed":
			s.FailedPrints++
			s.RecentFailures = append(s.RecentFailures, RecentPrint{
				ID: r.ID, Printer: r.Printer, File: r.File,
				Result: r.Result, Started: r.Started, Duration: r.Duration,
			})
		case "Cancelled":
			s.CancelledPrints++
		}

		dur := parseDurationStr(r.Duration)
		totalTime += dur
		if dur > 0 {
			s.LongestPrints = append(s.LongestPrints, RecentPrint{
				ID: r.ID, Printer: r.Printer, File: r.File,
				Result: r.Result, Started: r.Started, Duration: r.Duration,
			})
		}
	}

	sortByDurationDesc(s.LongestPrints)
	if len(s.LongestPrints) > 10 {
		s.LongestPrints = s.LongestPrints[:10]
	}
	if len(s.RecentFailures) > 10 {
		s.RecentFailures = s.RecentFailures[:10]
	}

	if s.TotalPrints > 0 {
		s.SuccessRate = float64(successCount) / float64(s.TotalPrints) * 100
	}
	s.TotalPrintTime = totalTime
	if s.TotalPrints > 0 {
		s.AvgPrintTime = totalTime / int64(s.TotalPrints)
	}

	if fil != nil {
		for _, sp := range fil.List() {
			usedG := sp.WeightG - sp.RemainingG
			if usedG < 0 {
				usedG = 0
			}
			s.TotalFilamentG += usedG
			s.FilamentByType[sp.Type] += usedG
			s.FilamentByBrand[sp.Brand] += usedG
			if sp.WeightG > 0 {
				s.TotalCost += usedG * (sp.Cost / sp.WeightG)
			}
		}
	}

	return s
}

// parseDurationStr parses "HH:MM:SS" into seconds.
func parseDurationStr(s string) int64 {
	var h, m, sec int
	if n, _ := fmt.Sscanf(s, "%d:%d:%d", &h, &m, &sec); n == 3 {
		return int64(h*3600 + m*60 + sec)
	}
	return 0
}

func sortByDurationDesc(prints []RecentPrint) {
	sort.SliceStable(prints, func(i, j int) bool {
		return parseDurationStr(prints[i].Duration) > parseDurationStr(prints[j].Duration)
	})
}

// CacheStore provides a simple file cache for analytics stats.
type CacheStore struct {
	mu   sync.RWMutex
	file string
}

func NewCacheStore(dir string) *CacheStore {
	return &CacheStore{file: filepath.Join(dir, "analytics_cache.json")}
}

func (c *CacheStore) Save(s Stats) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.file, data, 0o600)
}
