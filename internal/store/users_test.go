package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestUsersSessionsRoles(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "go-bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// Policy: short and simple refused.
	if _, err := st.CreateUser(ctx, "a", "short"); err == nil {
		t.Fatal("weak password accepted")
	}
	if _, err := st.CreateUser(ctx, "a", "longpasswordwithoutdigit!"); err == nil {
		t.Fatal("digitless password accepted")
	}
	owner, err := st.CreateUser(ctx, "owner", "correct horse 1!")
	if err != nil {
		t.Fatal(err)
	}
	if owner.Role != RoleOwner {
		t.Fatalf("first user role %q", owner.Role)
	}
	viewer, err := st.CreateUser(ctx, "viewer", "another pass 2@")
	if err != nil {
		t.Fatal(err)
	}
	if viewer.Role != RoleViewer {
		t.Fatalf("second user role %q", viewer.Role)
	}
	// Wrong password refused.
	if _, err := st.AuthUser(ctx, "owner", "wrong password!!", "ua"); err == nil {
		t.Fatal("wrong password accepted")
	}
	session, err := st.AuthUser(ctx, "owner", "correct horse 1!", "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := st.UserForToken(ctx, session.Token)
	if err != nil || user.ID != owner.ID {
		t.Fatalf("token %+v err %v", user, err)
	}
	sessions, err := st.ListSessions(ctx, owner.ID)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions %+v err %v", sessions, err)
	}
	// Remote sign-out kills the token.
	if err := st.RevokeSession(ctx, owner.ID, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.UserForToken(ctx, session.Token); err == nil {
		t.Fatal("revoked token works")
	}
	// Role promotion sticks.
	if err := st.SetUserRole(ctx, viewer.ID, RoleEditor); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetUser(ctx, viewer.ID)
	if err != nil || got.Role != RoleEditor {
		t.Fatalf("role %+v err %v", got, err)
	}
	if err := st.SetUserRole(ctx, viewer.ID, "superadmin"); err == nil {
		t.Fatal("bad role accepted")
	}
	// Space roles attach.
	space, err := st.DefaultSpace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SpaceRole(ctx, viewer.ID, space.ID, RoleEditor); err != nil {
		t.Fatal(err)
	}
	users, err := st.ListUsers(ctx)
	if err != nil || len(users) != 2 {
		t.Fatalf("users %+v err %v", users, err)
	}
	if !strings.Contains(users[0].Name, "owner") {
		t.Fatalf("order %+v", users)
	}
}
