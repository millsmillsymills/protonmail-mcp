package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"protonmail-mcp/internal/keychain"
	"protonmail-mcp/internal/proterr"
	"protonmail-mcp/internal/session"
)

func runLogin(ctx context.Context) error {
	apiURL := os.Getenv("PROTONMAIL_MCP_API_URL")
	if apiURL == "" {
		apiURL = "https://mail.proton.me/api"
	}
	sess := session.New(apiURL, keychain.New())

	username, err := prompt("Proton email: ")
	if err != nil {
		return err
	}
	password, err := promptHidden("Password: ")
	if err != nil {
		return err
	}

	in := session.LoginInput{Username: username, Password: password}

	// First attempt — no 2FA. If it fails with "2FA required", prompt and retry.
	err = sess.Login(ctx, in)
	if err != nil && strings.Contains(err.Error(), "2FA required") {
		fmt.Println()
		fmt.Println("2FA is enabled on this account.")
		fmt.Println("Paste an otpauth:// URI (preferred — enables silent refresh) OR a 6-digit code.")
		v, err2 := prompt("> ")
		if err2 != nil {
			return err2
		}
		if strings.HasPrefix(v, "otpauth://") {
			in.TOTPSecret = v
		} else if isAllDigits(v) && len(v) == 6 {
			in.TOTPCode = v
			fmt.Println("WARNING: a one-shot code was provided. Future automatic refreshes will fail; you'll need to log in again when the session expires.")
		} else {
			return errors.New("input is neither an otpauth:// URI nor a 6-digit code")
		}
		err = sess.Login(ctx, in)
	}
	if err != nil {
		// Surface mapped errors helpfully.
		if pe := proterr.Map(err); pe != nil {
			return fmt.Errorf("%s: %s\n%s", pe.Code, pe.Message, pe.Hint)
		}
		return err
	}

	fmt.Println("Logged in. You can now run `protonmail-mcp status` to verify.")
	return nil
}

func prompt(label string) (string, error) {
	fmt.Print(label)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptHidden(label string) (string, error) {
	fmt.Print(label)
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
