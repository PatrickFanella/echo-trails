package privacy

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type recordingObjects struct {
	deleted []string
	failKey string
}

func (r *recordingObjects) PutObject(context.Context, string, string, string, []byte) error {
	return nil
}
func (r *recordingObjects) DeleteObject(_ context.Context, bucket, key string) error {
	r.deleted = append(r.deleted, bucket+"/"+key)
	if key == r.failKey {
		return errors.New("delete failed")
	}
	return nil
}

func TestDeleteStoredObjectsRemovesEveryOriginalAndDerivative(t *testing.T) {
	objects := &recordingObjects{}
	err := deleteStoredObjects(context.Background(), objects, []storedObject{
		{bucket: "private", key: "media/original/a"},
		{bucket: "private", key: "media/derivatives/a/preview.jpg"},
		{bucket: "private", key: "media/derivatives/link/quote.png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"private/media/original/a", "private/media/derivatives/a/preview.jpg", "private/media/derivatives/link/quote.png"}
	if !reflect.DeepEqual(objects.deleted, want) {
		t.Fatalf("deleted=%v want=%v", objects.deleted, want)
	}
}

func TestDeleteStoredObjectsFailsClosed(t *testing.T) {
	objects := &recordingObjects{failKey: "bad"}
	if err := deleteStoredObjects(context.Background(), objects, []storedObject{{bucket: "private", key: "bad"}, {bucket: "private", key: "later"}}); err == nil {
		t.Fatal("expected object deletion failure")
	}
	if len(objects.deleted) != 1 {
		t.Fatalf("continued after failure: %v", objects.deleted)
	}
}
