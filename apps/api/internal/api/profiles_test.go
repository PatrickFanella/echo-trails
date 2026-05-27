package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestValidateHandleNormalizesAndRejectsInvalidHandles(t *testing.T) {
	valid, err := ValidateHandle(" @Echo_Trail1 ")
	if err != nil {
		t.Fatalf("ValidateHandle valid: %v", err)
	}
	if valid != "echo_trail1" {
		t.Fatalf("normalized handle = %q, want echo_trail1", valid)
	}

	invalid := []string{"", "ab", "1startsnumber", "has-dash", "has space", "users", "admin", "toolongtoolongtoolongtoolongtoolong"}
	for _, input := range invalid {
		t.Run(input, func(t *testing.T) {
			if _, err := ValidateHandle(input); !errors.Is(err, ErrInvalidHandle) {
				t.Fatalf("ValidateHandle(%q) error = %v, want ErrInvalidHandle", input, err)
			}
		})
	}
}

func TestMemoryProfileStoreEnforcesUniqueHandles(t *testing.T) {
	store := NewMemoryProfileStore()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	_, err := store.UpsertProfile(context.Background(), "usr_1", "one@example.com", UpsertProfileRequest{Handle: "same", DisplayName: "One"}, now)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	_, err = store.UpsertProfile(context.Background(), "usr_2", "two@example.com", UpsertProfileRequest{Handle: "SAME", DisplayName: "Two"}, now)
	if !errors.Is(err, ErrHandleTaken) {
		t.Fatalf("second upsert error = %v, want ErrHandleTaken", err)
	}
}

func TestMemoryProfileStoreHandleChangeCooldown(t *testing.T) {
	store := NewMemoryProfileStore()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	_, err := store.UpsertProfile(context.Background(), "usr_1", "one@example.com", UpsertProfileRequest{Handle: "first", DisplayName: "One"}, now)
	if err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	_, err = store.UpsertProfile(context.Background(), "usr_1", "one@example.com", UpsertProfileRequest{Handle: "second", DisplayName: "One"}, now.Add(24*time.Hour))
	if !errors.Is(err, ErrHandleChangeTooSoon) {
		t.Fatalf("cooldown upsert error = %v, want ErrHandleChangeTooSoon", err)
	}
	_, err = store.UpsertProfile(context.Background(), "usr_1", "one@example.com", UpsertProfileRequest{Handle: "second", DisplayName: "One"}, now.Add(handleChangeCooldown+time.Second))
	if err != nil {
		t.Fatalf("post-cooldown upsert: %v", err)
	}
}

func TestPublicProfileShapeIsPseudonymous(t *testing.T) {
	private := Profile{UserID: "usr_private", Email: "private@example.com", Handle: "storywalker", DisplayName: "Story Walker", Bio: "Listening to places.", CreatedAt: time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)}
	public := toPublicProfile(private)

	if public.Handle != private.Handle || public.DisplayName != private.DisplayName || public.Bio != private.Bio {
		t.Fatalf("public profile mismatch: %+v", public)
	}
	if public.CreatedAt.IsZero() {
		t.Fatal("public profile should include stable public created_at")
	}
}
