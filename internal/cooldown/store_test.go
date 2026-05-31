package cooldown

import (
	"context"
	"testing"
	"time"
)

func TestMemoryStoreGetSetDelete(t *testing.T) {
	s := NewMemoryCooldownStore()
	ctx := context.Background()
	key := CooldownKey{ProviderID: 1, ClientType: "claude", Model: "opus"}

	if _, ok, err := s.Get(ctx, key); err != nil || ok {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}

	until := time.Now().Add(time.Minute)
	if err := s.Set(ctx, key, until); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := s.Get(ctx, key)
	if err != nil || !ok {
		t.Fatalf("expected hit, got ok=%v err=%v", ok, err)
	}
	if !got.Equal(until) {
		t.Fatalf("until = %v, want %v", got, until)
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := s.Get(ctx, key); ok {
		t.Fatal("expected miss after Delete")
	}
}

func TestMemoryStoreSetIfLater(t *testing.T) {
	s := NewMemoryCooldownStore()
	ctx := context.Background()
	key := CooldownKey{ProviderID: 1}

	earlier := time.Now().Add(time.Minute)
	later := earlier.Add(time.Minute)

	if ok, _ := s.SetIfLater(ctx, key, earlier); !ok {
		t.Fatal("first SetIfLater should accept")
	}
	if ok, _ := s.SetIfLater(ctx, key, later); !ok {
		t.Fatal("later SetIfLater should accept")
	}
	if ok, _ := s.SetIfLater(ctx, key, earlier); ok {
		t.Fatal("earlier SetIfLater should be rejected")
	}

	got, _, _ := s.Get(ctx, key)
	if !got.Equal(later) {
		t.Fatalf("until = %v, want %v", got, later)
	}
}

func TestMemoryStoreListByProvider(t *testing.T) {
	s := NewMemoryCooldownStore()
	ctx := context.Background()
	until := time.Now().Add(time.Minute)

	s.Set(ctx, CooldownKey{ProviderID: 1, Model: "a"}, until)
	s.Set(ctx, CooldownKey{ProviderID: 1, Model: "b"}, until)
	s.Set(ctx, CooldownKey{ProviderID: 2, Model: "z"}, until)

	entries, err := s.ListByProvider(ctx, 1)
	if err != nil {
		t.Fatalf("ListByProvider: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for provider 1, got %d", len(entries))
	}
}

func TestMemoryStoreGeneration(t *testing.T) {
	s := NewMemoryCooldownStore()
	ctx := context.Background()

	if g, _ := s.GetGeneration(ctx, 1); g != 0 {
		t.Fatalf("initial gen = %d, want 0", g)
	}
	g1, _ := s.BumpGeneration(ctx, 1)
	g2, _ := s.BumpGeneration(ctx, 1)
	if g1 != 1 || g2 != 2 {
		t.Fatalf("expected 1,2; got %d,%d", g1, g2)
	}
	// provider 2 独立
	if g, _ := s.GetGeneration(ctx, 2); g != 0 {
		t.Fatalf("provider 2 gen = %d, want 0", g)
	}
}

func TestMemoryStoreDeleteByProvider(t *testing.T) {
	s := NewMemoryCooldownStore()
	ctx := context.Background()
	until := time.Now().Add(time.Minute)

	s.Set(ctx, CooldownKey{ProviderID: 1, Model: "a"}, until)
	s.Set(ctx, CooldownKey{ProviderID: 1, Model: "b"}, until)
	s.Set(ctx, CooldownKey{ProviderID: 2, Model: "z"}, until)

	if err := s.DeleteByProvider(ctx, 1); err != nil {
		t.Fatalf("DeleteByProvider: %v", err)
	}
	entries, _ := s.ListByProvider(ctx, 1)
	if len(entries) != 0 {
		t.Fatalf("expected empty for provider 1, got %d", len(entries))
	}
	entries, _ = s.ListByProvider(ctx, 2)
	if len(entries) != 1 {
		t.Fatalf("provider 2 should be untouched, got %d entries", len(entries))
	}
}
