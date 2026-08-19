package storage

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestLocalStorageUploadDeleteAndURL(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir(), "https://files.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Upload(context.Background(), "pets/a.txt", bytes.NewBufferString("hello"), 5, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.RootDir + "/pets/a.txt"); err != nil {
		t.Fatal(err)
	}
	url, err := store.URL(context.Background(), "pets/a.txt")
	if err != nil || url != "https://files.example.test/pets%2Fa.txt" {
		t.Fatalf("url=%q err=%v", url, err)
	}
	if err := store.Delete(context.Background(), "pets/a.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestLocalStorageRejectsPathTraversal(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Upload(context.Background(), "../secret", bytes.NewBufferString("x"), 1, ""); err == nil {
		t.Fatal("Upload() error = nil, want invalid key error")
	}
}
