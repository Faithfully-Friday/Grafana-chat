package plugin

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStoreConversationLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	conv, err := s.CreateConversation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if conv.Title != "New chat" {
		t.Fatalf("unexpected title %q", conv.Title)
	}

	if err := s.AddMessage(ctx, conv.ID, "user", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessage(ctx, conv.ID, "assistant", "hi there"); err != nil {
		t.Fatal(err)
	}

	msgs, err := s.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Content != "hi there" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}

	if err := s.SetTitle(ctx, conv.ID, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetContextID(ctx, conv.ID, "ctx-1"); err != nil {
		t.Fatal(err)
	}
	cid, err := s.ContextID(ctx, conv.ID)
	if err != nil || cid != "ctx-1" {
		t.Fatalf("context id: %q err=%v", cid, err)
	}

	convs, err := s.ListConversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 1 || convs[0].Title != "hello" {
		t.Fatalf("unexpected conversations: %+v", convs)
	}

	if err := s.DeleteConversation(ctx, conv.ID); err != nil {
		t.Fatal(err)
	}
	msgs, err = s.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected cascade delete, got %d messages", len(msgs))
	}
}
