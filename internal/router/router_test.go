package router

import (
	"math/rand"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestWeightedShuffleRoutesRespectsWeights(t *testing.T) {
	base := []*domain.Route{
		{ID: 1, Weight: 100},
		{ID: 2, Weight: 10},
		{ID: 3, Weight: 1},
	}

	rng := rand.New(rand.NewSource(42))
	firstCounts := map[uint64]int{
		1: 0,
		2: 0,
		3: 0,
	}

	const rounds = 5000
	for i := 0; i < rounds; i++ {
		routes := []*domain.Route{base[0], base[1], base[2]}
		weightedShuffleRoutes(routes, rng.Intn)
		firstCounts[routes[0].ID]++
	}

	if !(firstCounts[1] > firstCounts[2] && firstCounts[2] > firstCounts[3]) {
		t.Fatalf("unexpected first-pick distribution: %#v", firstCounts)
	}
	if firstCounts[1] < 4200 {
		t.Fatalf("expected route#1 to dominate first picks, got %#v", firstCounts)
	}
	if firstCounts[3] > 150 {
		t.Fatalf("expected smallest weight to be rarely first, got %#v", firstCounts)
	}
}

func TestWeightedShuffleRoutesUsesDefaultWeightForNonPositiveValues(t *testing.T) {
	if got := normalizedRouteWeight(nil); got != domain.DefaultRouteWeight {
		t.Fatalf("nil route weight = %d, want %d", got, domain.DefaultRouteWeight)
	}
	if got := normalizedRouteWeight(&domain.Route{Weight: 0}); got != domain.DefaultRouteWeight {
		t.Fatalf("zero route weight = %d, want %d", got, domain.DefaultRouteWeight)
	}
	if got := normalizedRouteWeight(&domain.Route{Weight: -1}); got != domain.DefaultRouteWeight {
		t.Fatalf("negative route weight = %d, want %d", got, domain.DefaultRouteWeight)
	}
	if got := normalizedRouteWeight(&domain.Route{Weight: 25}); got != 25 {
		t.Fatalf("positive route weight = %d, want 25", got)
	}
}

func TestSortRoutesPriorityByPosition(t *testing.T) {
	r := &Router{}
	routes := []*domain.Route{
		{ID: 1, Position: 3},
		{ID: 2, Position: 1},
		{ID: 3, Position: 2},
	}

	r.sortRoutes(routes, &domain.RoutingStrategy{Type: domain.RoutingStrategyPriority})

	if routes[0].ID != 2 || routes[1].ID != 3 || routes[2].ID != 1 {
		t.Fatalf("priority sort failed, got order: %d,%d,%d", routes[0].ID, routes[1].ID, routes[2].ID)
	}
}
