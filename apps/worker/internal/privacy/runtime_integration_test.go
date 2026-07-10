package privacy

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"git.subcult.tv/subculture-collective/echo-trails/apps/worker/internal/identifier"
	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationObjects struct {
	data    map[string][]byte
	deleted []string
}

func (o *integrationObjects) PutObject(_ context.Context, bucket, key, _ string, body []byte) error {
	if o.data == nil {
		o.data = map[string][]byte{}
	}
	o.data[bucket+"/"+key] = append([]byte(nil), body...)
	return nil
}

func (o *integrationObjects) DeleteObject(_ context.Context, bucket, key string) error {
	o.deleted = append(o.deleted, bucket+"/"+key)
	delete(o.data, bucket+"/"+key)
	return nil
}

func TestIntegrationPrivacyExportAndDeletion(t *testing.T) {
	if os.Getenv("ECHOTRAILS_INTEGRATION") != "1" {
		t.Skip("set ECHOTRAILS_INTEGRATION=1")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ECHOTRAILS_INTEGRATION_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	userID, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	exportJobID, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	deleteJobID, err := identifier.NewUUIDv7()
	if err != nil {
		t.Fatal(err)
	}
	email := "privacy-" + strings.ReplaceAll(userID, "-", "") + "@example.test"
	if _, err = pool.Exec(ctx, `INSERT INTO users (id, email, role, status) VALUES ($1, $2, 'user', 'active')`, userID, email); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID) })
	if _, err = pool.Exec(ctx, `INSERT INTO privacy_jobs (id, user_id, job_type, status) VALUES ($1, $3, 'export', 'queued'), ($2, $3, 'delete', 'queued')`, exportJobID, deleteJobID, userID); err != nil {
		t.Fatal(err)
	}

	objects := &integrationObjects{}
	processor, err := NewProcessor(pool, objects, "private")
	if err != nil {
		t.Fatal(err)
	}
	if err = processor.Process(ctx, exportJobID, "privacy_export"); err != nil {
		t.Fatal(err)
	}
	exportKey := "private/privacy/exports/" + exportJobID + "/export.json"
	var document map[string]any
	if err = json.Unmarshal(objects.data[exportKey], &document); err != nil {
		t.Fatal(err)
	}
	for _, section := range []string{"account", "profile", "echoes", "replies", "media", "transcripts", "reactions", "reports", "subscriptions", "consents", "notification_preferences", "notification_mutes", "notifications", "blocks", "hidden_content", "appeals", "moderation_actions", "analytics", "devices", "sessions"} {
		if _, ok := document[section]; !ok {
			t.Fatalf("export missing %q section", section)
		}
	}
	encoded := string(objects.data[exportKey])
	for _, forbidden := range []string{"push_token_ciphertext", "refresh_token_hash", "access_token_hash", "exact_location", "location_ciphertext"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("export contains forbidden field %q", forbidden)
		}
	}

	if err = processor.Process(ctx, deleteJobID, "privacy_delete"); err != nil {
		t.Fatal(err)
	}
	if _, ok := objects.data[exportKey]; ok {
		t.Fatal("privacy deletion retained prior export object")
	}
	var status string
	var resultKey *string
	if err = pool.QueryRow(ctx, `SELECT status, result_object_key FROM privacy_jobs WHERE id = $1`, exportJobID).Scan(&status, &resultKey); err != nil {
		t.Fatal(err)
	}
	if resultKey != nil {
		t.Fatalf("privacy deletion retained export pointer %q", *resultKey)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "deleted" {
		t.Fatalf("user status=%q want deleted", status)
	}
}
