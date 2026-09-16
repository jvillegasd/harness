package bench

import (
	"fmt"
	"testing"
	"time"
)

// benchCollect is the one call site the pre- and post-change measurements share,
// so both runs time the same workload. Before the change collect had no date
// boundary and read every session; after, the requested boundary is passed in.
func benchCollect(home string, since *time.Time) ([]*session, error) {
	return collect(home, []string{"claude"}, nil, since)
}

func BenchmarkCollect(b *testing.B) {
	for _, records := range []int{100, 1000, 5000} {
		home := synthHome(b, 20, records)
		b.Run(fmt.Sprintf("records=%d", records), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := benchCollect(home, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	home := synthHome(b, 100, 400)
	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	b.Run("date-filtered", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if _, err := benchCollect(home, &since); err != nil {
				b.Fatal(err)
			}
		}
	})
}
