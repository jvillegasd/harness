package tickets

import (
	"fmt"
	"sort"
)

type Frontier struct {
	Feature string    `json:"feature,omitempty"`
	Ready   []int     `json:"ready"`
	Done    []int     `json:"done"`
	Blocked []Waiting `json:"blocked"`
}

type Waiting struct {
	Ticket    int   `json:"ticket"`
	WaitingOn []int `json:"waiting_on"`
}

func Compute(ts []Ticket, done []int) (Frontier, error) {
	isDone := map[int]bool{}
	known := map[int]bool{}
	for _, t := range ts {
		known[t.Number] = true
		if t.Done {
			isDone[t.Number] = true
		}
	}
	for _, n := range done {
		if !known[n] {
			return Frontier{}, fmt.Errorf("%02d is not a ticket of this feature", n)
		}
		isDone[n] = true
	}
	f := Frontier{Ready: []int{}, Done: []int{}, Blocked: []Waiting{}}
	sorted := append([]Ticket(nil), ts...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Number < sorted[j].Number })
	for _, t := range sorted {
		if isDone[t.Number] {
			f.Done = append(f.Done, t.Number)
			continue
		}
		var waiting []int
		for _, b := range t.BlockedBy {
			if !isDone[b] {
				waiting = append(waiting, b)
			}
		}
		if len(waiting) == 0 {
			f.Ready = append(f.Ready, t.Number)
		} else {
			f.Blocked = append(f.Blocked, Waiting{t.Number, waiting})
		}
	}
	return f, nil
}
