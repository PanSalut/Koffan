package main

import (
	"bytes"
	"path/filepath"
	"shopping-list/db"
	"strings"
	"testing"
)

func initRecoveryTestDB(t *testing.T) {
	t.Helper()
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "recovery.db"))
	db.InitForAdminRecovery()
	t.Cleanup(db.Close)
}

func TestAdminRecoveryCreatesAdministrator(t *testing.T) {
	initRecoveryTestDB(t)
	var output bytes.Buffer
	input := strings.NewReader("2\nrecovery-admin\nRecovery Admin\npassword123\npassword123\n")
	if err := runAdminRecovery(input, &output); err != nil {
		t.Fatal(err)
	}
	user, err := db.GetUserByUsername("recovery-admin")
	if err != nil {
		t.Fatal(err)
	}
	if !user.IsAdmin || user.PasswordHash == nil || !db.VerifyPassword(*user.PasswordHash, "password123") {
		t.Fatalf("invalid recovery administrator: %#v", user)
	}
}

func TestAdminRecoveryResetsPasswordAndEnablesDisabledLocalUser(t *testing.T) {
	initRecoveryTestDB(t)
	user, err := db.CreateLocalUser("locked", "Locked User", "oldpassword", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB.Exec(`UPDATE users SET disabled=TRUE WHERE id=?`, user.ID); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	input := strings.NewReader("1\n1\nnewpassword123\nnewpassword123\ny\n")
	if err = runAdminRecovery(input, &output); err != nil {
		t.Fatal(err)
	}
	user, err = db.GetUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if user.Disabled || user.PasswordHash == nil || !db.VerifyPassword(*user.PasswordHash, "newpassword123") {
		t.Fatalf("user was not recovered: %#v", user)
	}
}
