package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/coniungo-repo/golang-server/internal/auth"
	"github.com/coniungo-repo/golang-server/internal/health"
	"github.com/coniungo-repo/golang-server/internal/platform/clock"
	"github.com/coniungo-repo/golang-server/internal/platform/config"
	"github.com/coniungo-repo/golang-server/internal/platform/database"
	"github.com/coniungo-repo/golang-server/internal/platform/email"
	"github.com/coniungo-repo/golang-server/internal/platform/phone"
	"github.com/coniungo-repo/golang-server/internal/platform/sms"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dependencies struct {
	Config  config.Config
	Log     *slog.Logger
	DB      *pgxpool.Pool
	Clock   clock.Clock
	Health  *health.Probe
	Phone   *phone.Validator
	Email   email.Sender
	SMS     sms.Sender
	Limiter *auth.Limiter
	Auth    *auth.Service
}

func newDependencies(ctx context.Context, cfg config.Config, log *slog.Logger) (*dependencies, error) {
	phones, err := newPhoneValidator(cfg.Phone)
	if err != nil {
		return nil, err
	}
	emailSender, err := newEmailSender(cfg.Delivery, log)
	if err != nil {
		return nil, err
	}
	smsSender, err := newSMSSender(cfg.Delivery, log)
	if err != nil {
		return nil, err
	}

	pool, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}

	clk := clock.New()
	limiter := auth.NewLimiter(pool, cfg.Security.FingerprintSecret.Reveal(), clk, cfg.Auth.RateLimits.Retention())

	authService, err := auth.New(auth.Deps{
		Config:         cfg.Auth,
		ClientIPHeader: cfg.Server.ClientIPHeader,
		Log:            log,
		Phones:         phones,
		Email:          emailSender,
		SMS:            smsSender,
		Limiter:        limiter,
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("configuring auth: %w", err)
	}

	return &dependencies{
		Config:  cfg,
		Log:     log,
		DB:      pool,
		Clock:   clk,
		Health:  health.New(pool),
		Phone:   phones,
		Email:   emailSender,
		SMS:     smsSender,
		Limiter: limiter,
		Auth:    authService,
	}, nil
}

func newPhoneValidator(cfg config.Phone) (*phone.Validator, error) {
	var opts []phone.Option
	if cfg.DefaultRegion != "" {
		opts = append(opts, phone.WithDefaultRegion(cfg.DefaultRegion))
	}
	if len(cfg.AllowedRegions) > 0 {
		opts = append(opts, phone.WithAllowedRegions(cfg.AllowedRegions...))
	}
	v, err := phone.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("configuring phone validator: %w", err)
	}
	return v, nil
}

func newEmailSender(cfg config.Delivery, log *slog.Logger) (email.Sender, error) {
	switch cfg.EmailProvider {
	case config.ProviderLog:
		return email.LogSender{Log: log}, nil
	default:
		return nil, fmt.Errorf("unknown email provider %q", cfg.EmailProvider)
	}
}

func newSMSSender(cfg config.Delivery, log *slog.Logger) (sms.Sender, error) {
	switch cfg.SMSProvider {
	case config.ProviderLog:
		return sms.LogSender{Log: log}, nil
	default:
		return nil, fmt.Errorf("unknown sms provider %q", cfg.SMSProvider)
	}
}

func (d *dependencies) close() {
	d.Log.Info("closing dependencies")
	d.DB.Close()
}
