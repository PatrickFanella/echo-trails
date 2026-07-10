package api

import (
	"context"
	"testing"
	"time"
)

func TestReleasePaginationUsesTimestampAndUUIDCursors(t *testing.T) {
	base := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	t.Run("place echoes descending", func(t *testing.T) {
		placeID := PlacePrincePondID
		records := []EchoRecord{
			{ID: "019f4b00-0001-7000-8000-000000000001", PlaceID: placeID, Status: EchoStatusPublished, PublishedAt: ptrTime(base.Add(time.Minute)), CreatedAt: base},
			{ID: "019f4b00-0002-7000-8000-000000000002", PlaceID: placeID, Status: EchoStatusPublished, PublishedAt: ptrTime(base.Add(2 * time.Minute)), CreatedAt: base},
			{ID: "019f4b00-0003-7000-8000-000000000003", PlaceID: placeID, Status: EchoStatusPublished, PublishedAt: ptrTime(base.Add(3 * time.Minute)), CreatedAt: base},
		}
		store := NewMemoryEchoStore(records)
		first, err := store.ListPublishedByPlacePage(context.Background(), placeID, "", 2)
		if err != nil || len(first.Records) != 2 || first.NextCursor == "" {
			t.Fatalf("first Echo page = %+v err=%v", first, err)
		}
		second, err := store.ListPublishedByPlacePage(context.Background(), placeID, first.NextCursor, 2)
		if err != nil || len(second.Records) != 1 || second.NextCursor != "" || second.Records[0].ID == first.Records[0].ID || second.Records[0].ID == first.Records[1].ID {
			t.Fatalf("second Echo page = %+v err=%v", second, err)
		}
	})

	t.Run("replies oldest first", func(t *testing.T) {
		store := NewSocialStoreWithEchoAuthors(map[string]string{"echo-page": "author"})
		repository := NewMemorySocialRepository(store)
		for index := 0; index < 3; index++ {
			if _, err := repository.AddReply(context.Background(), "echo-page", ReplyRequest{UserID: "participant", Body: "Reply"}, base.Add(time.Duration(index)*time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
		first, err := repository.ListReplyPage(context.Background(), "echo-page", "participant", "", 2)
		if err != nil || len(first.Replies) != 2 || first.NextCursor == "" {
			t.Fatalf("first Reply page = %+v err=%v", first, err)
		}
		second, err := repository.ListReplyPage(context.Background(), "echo-page", "participant", first.NextCursor, 2)
		if err != nil || len(second.Replies) != 1 || second.NextCursor != "" || !second.Replies[0].PostedAt.After(first.Replies[1].PostedAt) {
			t.Fatalf("second Reply page = %+v err=%v", second, err)
		}
	})

	t.Run("private subscriptions descending", func(t *testing.T) {
		repository := NewMemoryEngagementRepository()
		placeIDs := []string{PlacePrincePondID, PlaceLibraryID, PlaceTivoliTheatreID}
		for index, placeID := range placeIDs {
			if _, err := repository.PutSubscription(context.Background(), "user", placeID, true, base.Add(time.Duration(index)*time.Minute)); err != nil {
				t.Fatal(err)
			}
		}
		first, err := repository.ListSubscriptionPage(context.Background(), "user", "", 2)
		if err != nil || len(first.Subscriptions) != 2 || first.NextCursor == "" {
			t.Fatalf("first subscription page = %+v err=%v", first, err)
		}
		second, err := repository.ListSubscriptionPage(context.Background(), "user", first.NextCursor, 2)
		if err != nil || len(second.Subscriptions) != 1 || second.NextCursor != "" {
			t.Fatalf("second subscription page = %+v err=%v", second, err)
		}
	})
}

func ptrTime(value time.Time) *time.Time { return &value }
