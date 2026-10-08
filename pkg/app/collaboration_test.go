package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestCollaborationThreadCommentReactionDrawingRoundTrip(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	view, err := store.CreateView(ctx, "Collab View", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	viewID := int32(view.ID)

	element, err := store.CreateElement(ctx, LibraryElement{Name: "Collab Element"})
	if err != nil {
		t.Fatal(err)
	}
	elementID := int32(element.ID)

	// uuid.Nil must map to the "local" workspace sentinel.
	threadID, err := store.CreateViewThread(ctx, uuid.Nil, viewID, &elementID, nil, "user-1", "User One")
	if err != nil {
		t.Fatal(err)
	}
	thread, err := store.ViewThreadByID(ctx, uuid.Nil, viewID, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.WorkspaceID != "local" {
		t.Fatalf("workspace_id = %q, want local", thread.WorkspaceID)
	}
	if thread.Status != "open" || thread.ElementID == nil || *thread.ElementID != elementID {
		t.Fatalf("thread = %+v, want open anchored to element %d", thread, elementID)
	}

	threads, err := store.ListViewThreads(ctx, uuid.Nil, viewID, &elementID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].ID != threadID {
		t.Fatalf("threads = %+v, want the created thread", threads)
	}

	comment, err := store.CreateViewComment(ctx, uuid.Nil, viewID, threadID, "user-1", "User One", "hello")
	if err != nil {
		t.Fatal(err)
	}
	if comment.Body != "hello" || comment.ThreadID != threadID {
		t.Fatalf("comment = %+v, want body hello on thread %d", comment, threadID)
	}
	comments, err := store.ListViewComments(ctx, uuid.Nil, viewID, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].ID != comment.ID {
		t.Fatalf("comments = %+v, want the created comment", comments)
	}

	if err := store.SetViewThreadResolved(ctx, uuid.Nil, viewID, threadID, true); err != nil {
		t.Fatal(err)
	}
	thread, err = store.ViewThreadByID(ctx, uuid.Nil, viewID, threadID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Status != "resolved" || thread.ResolvedAt == nil {
		t.Fatalf("thread after resolve = %+v, want resolved with timestamp", thread)
	}

	added, err := store.ToggleElementReaction(ctx, uuid.Nil, viewID, elementID, "user-1", "👍")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("first toggle should add the reaction")
	}
	reactions, err := store.ListElementReactions(ctx, uuid.Nil, viewID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(reactions) != 1 || reactions[0].Count != 1 || !reactions[0].ReactedByMe {
		t.Fatalf("reactions = %+v, want one reaction by the user", reactions)
	}
	removed, err := store.ToggleElementReaction(ctx, uuid.Nil, viewID, elementID, "user-1", "👍")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("second toggle should remove the reaction")
	}
	reactions, err = store.ListElementReactions(ctx, uuid.Nil, viewID, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(reactions) != 0 {
		t.Fatalf("reactions after removal = %+v, want none", reactions)
	}

	drawing := Drawing{PathID: "path-1", UserID: "user-1", Points: []byte(`[{"x":1,"y":2}]`), Color: "#fff", Width: 2, Text: "note", FontSize: 12}
	if err := store.UpsertDrawing(ctx, uuid.Nil, viewID, drawing); err != nil {
		t.Fatal(err)
	}
	drawings, err := store.ListDrawings(ctx, uuid.Nil, viewID)
	if err != nil {
		t.Fatal(err)
	}
	if len(drawings) != 1 || drawings[0].PathID != "path-1" || drawings[0].Text != "note" {
		t.Fatalf("drawings = %+v, want the upserted drawing", drawings)
	}
	if err := store.DeleteDrawing(ctx, uuid.Nil, viewID, "path-1"); err != nil {
		t.Fatal(err)
	}
	drawings, err = store.ListDrawings(ctx, uuid.Nil, viewID)
	if err != nil {
		t.Fatal(err)
	}
	if len(drawings) != 0 {
		t.Fatalf("drawings after delete = %+v, want none", drawings)
	}
}
