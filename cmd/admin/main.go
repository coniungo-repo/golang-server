// Command admin manages staff admin invites from the command line, chiefly to invite the first admin.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/database"
	"github.com/coniungo-repo/golang-server/internal/users"
)

const usage = `usage:
  admin invite <email>    invite a staff email to sign in as admin
  admin invites           list invites`

const timeout = 30 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.Admin.Enabled() {
		return errors.New("admin sign-in is disabled: set OIDC_ISSUER and ADMIN_EMAIL_DOMAIN")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()
	svc := users.New(pool, clock.New(), cfg.Admin.EmailDomain)

	switch {
	case args[0] == "invite" && len(args) == 2:
		inv, err := svc.CreateAdminInvite(ctx, args[1], nil)
		if err != nil {
			return err
		}
		fmt.Printf("invited %s (invite %d). They can now sign in with Google on the admin app.\n", inv.Email, inv.ID)
		return nil
	case args[0] == "invites" && len(args) == 1:
		invites, err := svc.ListAdminInvites(ctx)
		if err != nil {
			return err
		}
		for _, inv := range invites {
			fmt.Printf("%d\t%s\t%s\n", inv.ID, inv.Email, inviteState(inv))
		}
		return nil
	default:
		return errors.New(usage)
	}
}

func inviteState(inv users.Invite) string {
	switch {
	case inv.RevokedAt != nil:
		return "revoked"
	case inv.AcceptedAt != nil:
		return "accepted"
	default:
		return "pending"
	}
}
