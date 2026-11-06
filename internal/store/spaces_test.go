package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSpacesIsolateBots(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	personal, err := st.DefaultSpace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if personal.Name == "" {
		t.Fatal("default space has no name")
	}
	// Legacy bot joins the default space.
	legacy, err := st.CreateBot(ctx, "Legacy", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.SpaceID != personal.ID {
		t.Fatalf("legacy bot space %q, want %q", legacy.SpaceID, personal.ID)
	}
	work, err := st.CreateSpace(ctx, "Work")
	if err != nil {
		t.Fatal(err)
	}
	workBot, err := st.CreateBotIn(ctx, work.ID, "Worker", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if workBot.SpaceID != work.ID {
		t.Fatalf("work bot space %q, want %q", workBot.SpaceID, work.ID)
	}
	personalBots, err := st.ListBotsIn(ctx, personal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(personalBots) != 1 || personalBots[0].ID != legacy.ID {
		t.Fatalf("personal bots %+v", personalBots)
	}
	workBots, err := st.ListBotsIn(ctx, work.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workBots) != 1 || workBots[0].ID != workBot.ID {
		t.Fatalf("work bots %+v", workBots)
	}
	spaces, err := st.ListSpaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(spaces) != 2 {
		t.Fatalf("spaces %+v", spaces)
	}
	// Non-empty space refuses deletion; empty space goes away.
	if err := st.DeleteSpace(ctx, work.ID); err == nil {
		t.Fatal("non-empty space deleted")
	}
	if err := st.DeleteSpace(ctx, "spc_missing"); err == nil {
		t.Fatal("missing space deleted")
	}
	empty, err := st.CreateSpace(ctx, "Empty")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSpace(ctx, empty.ID); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyBotsJoinDefaultSpace(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-spaces database: NULL the space column.
	bot, err := st.CreateBot(ctx, "Old", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE bots SET space_id = '' WHERE id = ?`, bot.ID); err != nil {
		t.Fatal(err)
	}
	st.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetBot(ctx, bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	space, err := reopened.DefaultSpace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.SpaceID != space.ID {
		t.Fatalf("backfilled space %q, want %q", got.SpaceID, space.ID)
	}
}
