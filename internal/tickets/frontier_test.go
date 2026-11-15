package tickets

import (
	"reflect"
	"testing"
)

func graphOf(edges map[int][]int, done ...int) []Ticket {
	var ts []Ticket
	for n := 1; n <= len(edges); n++ {
		t := Ticket{Number: n, BlockedBy: edges[n]}
		for _, d := range done {
			t.Done = t.Done || d == n
		}
		ts = append(ts, t)
	}
	return ts
}

var diamond = map[int][]int{1: nil, 2: nil, 3: {1}, 4: {1, 2}, 5: {4}}

func TestFrontierWithNothingDoneIsTheUnblockedTickets(t *testing.T) {
	f, err := Compute(graphOf(diamond), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Frontier{Ready: []int{1, 2}, Done: []int{}, Blocked: []Waiting{{3, []int{1}}, {4, []int{1, 2}}, {5, []int{4}}}}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %+v\nwant %+v", f, want)
	}
}

func TestFrontierMovesADependentOnlyWhenEveryBlockerIsDone(t *testing.T) {
	f, err := Compute(graphOf(diamond), []int{1})
	if err != nil {
		t.Fatal(err)
	}
	want := Frontier{Ready: []int{2, 3}, Done: []int{1}, Blocked: []Waiting{{4, []int{2}}, {5, []int{4}}}}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %+v\nwant %+v", f, want)
	}
}

func TestFrontierCountsAStatusDoneDraft(t *testing.T) {
	f, err := Compute(graphOf(diamond, 1, 2), []int{3})
	if err != nil {
		t.Fatal(err)
	}
	want := Frontier{Ready: []int{4}, Done: []int{1, 2, 3}, Blocked: []Waiting{{5, []int{4}}}}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %+v\nwant %+v", f, want)
	}
}

func TestFrontierRefusesADoneNumberThatIsNotATicket(t *testing.T) {
	if _, err := Compute(graphOf(diamond), []int{9}); err == nil {
		t.Fatal("an unknown --done number was accepted")
	}
}
