package hub

import (
	"github.com/c83a/Clash.Meta/config"
	"github.com/c83a/Clash.Meta/hub/executor"
	"github.com/c83a/Clash.Meta/hub/route"
	"github.com/c83a/Clash.Meta/log"
)

type Option func(*config.Config)

func WithExternalUI(externalUI string) Option {
	return func(cfg *config.Config) {
		cfg.General.ExternalUI = externalUI
	}
}

func WithExternalController(externalController string) Option {
	return func(cfg *config.Config) {
		cfg.General.ExternalController = externalController
	}
}

func WithExternalControllerUnix(externalControllerUnix string) Option {
	return func(cfg *config.Config) {
		cfg.General.ExternalControllerUnix = externalControllerUnix
	}
}

func WithSecret(secret string) Option {
	return func(cfg *config.Config) {
		cfg.General.Secret = secret
	}
}

// Parse call at the beginning of mihomo
func Parse(options ...Option) error {
	cfg, err := executor.Parse()
	if err != nil {
		return err
	}

	for _, option := range options {
		option(cfg)
	}

	if cfg.General.ExternalUI != "" {
		route.SetUIPath(cfg.General.ExternalUI)
	}

	if cfg.General.ExternalController != "" {
		go route.Start(cfg.General.ExternalController, cfg.General.ExternalControllerTLS,
			cfg.General.Secret, cfg.TLS.Certificate, cfg.TLS.PrivateKey, cfg.General.LogLevel == log.DEBUG)
	}

	if cfg.General.ExternalControllerUnix != "" {
		go route.StartUnix(cfg.General.ExternalControllerUnix, cfg.General.LogLevel == log.DEBUG)
	}

	executor.ApplyConfig(cfg, true)
	return nil
}
