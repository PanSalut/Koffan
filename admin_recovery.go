package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"shopping-list/db"
	"strconv"
	"strings"

	"golang.org/x/term"
)

type recoveryConsole struct {
	in    io.Reader
	out   io.Writer
	lines *bufio.Reader
}

func (r *recoveryConsole) line(prompt string) (string, error) {
	fmt.Fprint(r.out, prompt)
	value, err := r.lines.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if errors.Is(err, io.EOF) && value == "" {
		return "", io.EOF
	}
	return strings.TrimSpace(value), nil
}
func (r *recoveryConsole) password(prompt string) (string, error) {
	fmt.Fprint(r.out, prompt)
	if file, ok := r.in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		value, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(r.out)
		return string(value), err
	}
	return r.line("")
}
func (r *recoveryConsole) newPassword() (string, error) {
	for {
		first, err := r.password("New password: ")
		if err != nil {
			return "", err
		}
		second, err := r.password("Confirm password: ")
		if err != nil {
			return "", err
		}
		if first != second {
			fmt.Fprintln(r.out, "Passwords do not match. Try again.")
			continue
		}
		if _, err = db.HashPassword(first); err != nil {
			fmt.Fprintf(r.out, "Invalid password: %v\n", err)
			continue
		}
		return first, nil
	}
}

func runAdminRecovery(in io.Reader, out io.Writer) error {
	r := &recoveryConsole{in: in, out: out, lines: bufio.NewReader(in)}
	fmt.Fprintln(out, "Koffan administrator recovery")
	fmt.Fprintln(out, "The web server will not be started in this mode.")
	for {
		fmt.Fprintln(out, "\n1) Reset an existing local user's password")
		fmt.Fprintln(out, "2) Create a new administrator")
		fmt.Fprintln(out, "3) Cancel")
		choice, err := r.line("Choose an option: ")
		if err != nil {
			return err
		}
		switch choice {
		case "1":
			return resetExistingUser(r)
		case "2":
			return createRecoveryAdministrator(r)
		case "3":
			fmt.Fprintln(out, "No changes made.")
			return nil
		default:
			fmt.Fprintln(out, "Invalid option.")
		}
	}
}

func resetExistingUser(r *recoveryConsole) error {
	users, err := db.ListUsers()
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return errors.New("there are no existing users; create a new administrator instead")
	}
	fmt.Fprintln(r.out, "\nUsers:")
	for i, u := range users {
		flags := []string{u.AuthSource}
		if u.IsAdmin {
			flags = append(flags, "administrator")
		}
		if u.Disabled {
			flags = append(flags, "disabled")
		}
		fmt.Fprintf(r.out, "%d) %s (%s) [%s]\n", i+1, u.Username, u.DisplayName, strings.Join(flags, ", "))
	}
	selection, err := r.line("Select a user number: ")
	if err != nil {
		return err
	}
	number, err := strconv.Atoi(selection)
	if err != nil || number < 1 || number > len(users) {
		return errors.New("invalid user selection")
	}
	user := users[number-1]
	if user.AuthSource != "local" {
		return errors.New("the selected account is OIDC-only; create a new local administrator or recover it in the identity provider")
	}
	password, err := r.newPassword()
	if err != nil {
		return err
	}
	enable := false
	if user.Disabled {
		answer, err := r.line("This account is disabled. Enable it? [y/N]: ")
		if err != nil {
			return err
		}
		enable = strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
	}
	if err = db.ResetLocalUserPassword(user.ID, password, enable); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Password reset for %s. Existing sessions were invalidated.\n", user.Username)
	return nil
}

func createRecoveryAdministrator(r *recoveryConsole) error {
	username, err := r.line("Username: ")
	if err != nil {
		return err
	}
	if username == "" {
		return errors.New("username is required")
	}
	display, err := r.line("Display name (leave blank to use username): ")
	if err != nil {
		return err
	}
	if display == "" {
		display = username
	}
	password, err := r.newPassword()
	if err != nil {
		return err
	}
	user, err := db.CreateRecoveryAdmin(username, display, password)
	if err != nil {
		return err
	}
	fmt.Fprintf(r.out, "Administrator %s created successfully.\n", user.Username)
	return nil
}
