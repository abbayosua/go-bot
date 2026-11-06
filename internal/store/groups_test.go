package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGroupBoundsAndIsolation(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a, err := st.CreateBot(ctx, "Ana", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.CreateBot(ctx, "Bo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Too few, too many, unknown member, cross-space member: all refused.
	if _, _, err := st.CreateGroup(ctx, "", []string{a.ID}, "x"); err == nil {
		t.Fatal("solo group allowed")
	}
	many := []string{a.ID, b.ID}
	for i := 0; i < 9; i++ {
		extra, err := st.CreateBot(ctx, "Extra", "", "")
		if err != nil {
			t.Fatal(err)
		}
		many = append(many, extra.ID)
	}
	if _, _, err := st.CreateGroup(ctx, "", many, "x"); err == nil {
		t.Fatal("oversize group allowed")
	}
	if _, _, err := st.CreateGroup(ctx, "", []string{a.ID, "bot_missing"}, "x"); err == nil {
		t.Fatal("unknown member allowed")
	}
	other, err := st.CreateSpace(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := st.CreateBotIn(ctx, other.ID, "Out", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CreateGroup(ctx, "", []string{a.ID, outsider.ID}, "x"); err == nil {
		t.Fatal("cross-space group allowed")
	}
	group, thread, err := st.CreateGroup(ctx, a.SpaceID, []string{a.ID, b.ID}, "Duo")
	if err != nil {
		t.Fatal(err)
	}
	if len(group.MemberIDs) != 2 || group.SpaceID != a.SpaceID {
		t.Fatalf("group %+v", group)
	}
	found, err := st.GetGroupByThread(ctx, thread.ID)
	if err != nil || found == nil || found.ID != group.ID {
		t.Fatalf("lookup %+v err %v", found, err)
	}
	plain, err := st.CreateThread(ctx, a.ID, "Solo")
	if err != nil {
		t.Fatal(err)
	}
	none, err := st.GetGroupByThread(ctx, plain.ID)
	if err != nil || none != nil {
		t.Fatalf("plain thread group %+v err %v", none, err)
	}
	groups, err := st.ListGroups(ctx, a.SpaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups %+v", groups)
	}
}
