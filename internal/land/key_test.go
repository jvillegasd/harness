package land

import (
	"strconv"
	"testing"

	"github.com/jvillegasd/harness/internal/review"
)

func TestTargetKeyMatchesLandCheck(t *testing.T) {
	f := setup(t)
	f.open(1, "feature", "main", map[string]string{"a.go": "changed\n"})
	fork := f.open(2, "patch", "main", map[string]string{"b.go": "b\n"})
	fork.IsCrossRepository = true
	for _, n := range []int{1, 2} {
		target, err := review.ResolveTarget(f.dir, review.Forge(f.forge), strconv.Itoa(n))
		if err != nil {
			t.Fatal(err)
		}
		if c := f.one(n); target.Key != c.Key || target.SHA != c.Head {
			t.Errorf("#%d: target %s at %s, land check %s at %s", n, target.Key, target.SHA, c.Key, c.Head)
		}
	}
}
