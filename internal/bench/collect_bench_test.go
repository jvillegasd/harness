package bench

import (
	"fmt"
	"testing"
	"time"
)

// benchCollect isolates collection: a nil since decodes every transcript, and a
// since date decodes only the sessions the window keeps. The benchmark is
// unchanged across the before and after runs, so the delta reports the work the
// window now saves rather than a change to the benchmark itself.
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
