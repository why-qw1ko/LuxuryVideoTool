package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/auth"
	"github.com/why-qw1ko/LuxuryVideoTool/services/api_go/internal/database"
)

func main() {
	if err := run(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}

func run() error {
	if len(os.Args) < 2 { return fmt.Errorf("usage: admin <create-user|reset-password|set-active> [flags]") }
	databasePath := os.Getenv("DATABASE_PATH")
	if databasePath == "" { databasePath = "./data/app.db" }
	db, err := database.Open(context.Background(), databasePath)
	if err != nil { return err }
	defer db.Close()
	repository := auth.NewSQLiteRepository(db)
	tokens, _ := auth.NewTokenManager(make([]byte, 32), time.Minute)
	service, _ := auth.NewService(repository, tokens, time.Hour)

	switch os.Args[1] {
	case "create-user":
		flags := flag.NewFlagSet("create-user", flag.ContinueOnError)
		username := flags.String("username", "", "username")
		displayName := flags.String("display-name", "", "display name")
		role := flags.String("role", "user", "admin or user")
		passwordFile := flags.String("password-file", "", "path to a private password file")
		if err := flags.Parse(os.Args[2:]); err != nil { return err }
		password, err := readPassword(*passwordFile); if err != nil { return err }
		user, err := service.CreateUser(context.Background(), *username, *displayName, password, auth.Role(*role))
		if err != nil { return err }
		fmt.Printf("created user %s (%s)\n", user.ID, user.UsernameNormalized)
		return nil
	case "reset-password":
		flags := flag.NewFlagSet("reset-password", flag.ContinueOnError)
		userID := flags.String("user-id", "", "user id")
		passwordFile := flags.String("password-file", "", "path to a private password file")
		if err := flags.Parse(os.Args[2:]); err != nil { return err }
		password, err := readPassword(*passwordFile); if err != nil { return err }
		if *userID == "" { return fmt.Errorf("user-id is required") }
		return service.ResetPassword(context.Background(), *userID, password)
	case "set-active":
		flags := flag.NewFlagSet("set-active", flag.ContinueOnError)
		userID := flags.String("user-id", "", "user id")
		active := flags.Bool("active", false, "whether the account is active")
		if err := flags.Parse(os.Args[2:]); err != nil { return err }
		if *userID == "" { return fmt.Errorf("user-id is required") }
		return service.SetUserActive(context.Background(), *userID, *active)
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

func readPassword(path string) (string, error) {
	if path == "" { return "", fmt.Errorf("password-file is required") }
	info, err := os.Stat(path); if err != nil { return "", fmt.Errorf("stat password file: %w", err) }
	if !info.Mode().IsRegular() { return "", fmt.Errorf("password file must be a regular file") }
	if os.PathSeparator != '\\' && info.Mode().Perm()&0o077 != 0 { return "", fmt.Errorf("password file permissions must not allow group or other access") }
	if info.Size() > 2048 { return "", fmt.Errorf("password file is too large") }
	value, err := os.ReadFile(path); if err != nil { return "", fmt.Errorf("read password file: %w", err) }
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r') { value = value[:len(value)-1] }
	return string(value), nil
}
