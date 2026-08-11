package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/aleksclark/crosstalk/cli/ffmpeg"
	"github.com/aleksclark/crosstalk/cli/httpapi"
	"github.com/aleksclark/crosstalk/cli/play"
	"github.com/aleksclark/crosstalk/cli/sessionmedia"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// deps holds injectable collaborators for tests.
type deps struct {
	Stdout io.Writer
	Stderr io.Writer
	Logger *slog.Logger
	// NewService builds the play service for a resolved host. Nil uses production wiring.
	NewService func(host string) *play.Service
	// OnResolved is invoked with the fully resolved config (tests only).
	OnResolved func(cfg play.Config)
}

func newRootCommand(d *deps) *cobra.Command {
	if d == nil {
		d = &deps{}
	}
	if d.Stdout == nil {
		d.Stdout = os.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = os.Stderr
	}
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.NewService == nil {
		d.NewService = defaultService
	}

	v := viper.New()
	v.SetEnvPrefix("CT_PLAY")
	v.AutomaticEnv() // scoped by prefix; still bind exact keys below
	// Map hyphenated flags to underscore env/config keys.
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	var cfgFile string

	root := &cobra.Command{
		Use:   "ct-play [audio-file]",
		Short: "Play WAV/MP3 audio into a CrossTalk session channel",
		Long: strings.TrimSpace(`
ct-play authenticates to a CrossTalk server, lists assigned sessions and
channels, and streams a WAV or MP3 file into a selected channel over the
production one-time media-ticket WebRTC path.

Configuration precedence: explicit flag > CT_PLAY_* environment > config file > default.

Prefer CT_PLAY_PASSWORD or a permission-restricted config file over --password
so credentials do not appear in process listings.

Default config search paths:
  $XDG_CONFIG_HOME/crosstalk/ct-play.{yaml,yml,json,toml}
  $HOME/.config/crosstalk/ct-play.*

Environment variables:
  CT_PLAY_HOST, CT_PLAY_USERNAME, CT_PLAY_PASSWORD,
  CT_PLAY_SESSION_ID, CT_PLAY_CHANNEL_ID, CT_PLAY_CONFIG

Playback requires ffmpeg on PATH. Session and channel listing do not.
`),
		Example: strings.TrimSpace(`
  ct-play --host https://ct.example --username alice --password "$CT_PLAY_PASSWORD" session
  ct-play --session SESSION_ID channel
  ct-play --session SESSION_ID --channel CHANNEL_ID tone.wav
  CT_PLAY_HOST=https://ct.example CT_PLAY_USERNAME=alice CT_PLAY_PASSWORD=secret \
    CT_PLAY_SESSION_ID=... CT_PLAY_CHANNEL_ID=... ct-play speech.mp3
  ct-play --config ~/.config/crosstalk/ct-play.yaml speech.mp3
`),
		Args: cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig(cmd, v, cfgFile, d.Stderr)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			applyChangedFlags(cmd, v)
			cfg, err := resolveConfig(v)
			if err != nil {
				return err
			}
			if d.OnResolved != nil {
				d.OnResolved(cfg)
			}
			svc := d.NewService(cfg.Host)
			svc.Stdout = d.Stdout
			svc.Stderr = d.Stderr
			svc.Logger = d.Logger
			return svc.Play(cmd.Context(), cfg, args[0])
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.SetOut(d.Stdout)
	root.SetErr(d.Stderr)

	pf := root.PersistentFlags()
	pf.StringVar(&cfgFile, "config", "", "config file (default search: $XDG_CONFIG_HOME/crosstalk/ct-play.*)")
	pf.String("host", "", "server base URL (env CT_PLAY_HOST)")
	pf.String("username", "", "login username (env CT_PLAY_USERNAME)")
	pf.String("password", "", "login password (prefer CT_PLAY_PASSWORD)")
	pf.String("session", "", "session ID (env CT_PLAY_SESSION_ID)")
	pf.String("channel", "", "channel ID (env CT_PLAY_CHANNEL_ID)")

	// Exact env bindings. Flags are applied in resolveConfig only when Changed
	// so empty flag defaults never override env/config values.
	_ = v.BindEnv("host", "CT_PLAY_HOST")
	_ = v.BindEnv("username", "CT_PLAY_USERNAME")
	_ = v.BindEnv("password", "CT_PLAY_PASSWORD")
	_ = v.BindEnv("session_id", "CT_PLAY_SESSION_ID")
	_ = v.BindEnv("channel_id", "CT_PLAY_CHANNEL_ID")
	_ = v.BindEnv("config", "CT_PLAY_CONFIG")

	root.AddCommand(newSessionCommand(d, v))
	root.AddCommand(newChannelCommand(d, v))
	return root
}

func newSessionCommand(d *deps, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "session",
		Short: "List sessions assigned to the authenticated user",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			applyChangedFlags(cmd, v)
			cfg, err := resolveConfig(v)
			if err != nil {
				return err
			}
			if d.OnResolved != nil {
				d.OnResolved(cfg)
			}
			svc := d.NewService(cfg.Host)
			svc.Stdout = d.Stdout
			svc.Stderr = d.Stderr
			svc.Logger = d.Logger
			return svc.ListSessions(cmd.Context(), cfg)
		},
	}
}

func newChannelCommand(d *deps, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "channel",
		Short: "List channels for the selected session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			applyChangedFlags(cmd, v)
			cfg, err := resolveConfig(v)
			if err != nil {
				return err
			}
			if d.OnResolved != nil {
				d.OnResolved(cfg)
			}
			svc := d.NewService(cfg.Host)
			svc.Stdout = d.Stdout
			svc.Stderr = d.Stderr
			svc.Logger = d.Logger
			return svc.ListChannels(cmd.Context(), cfg)
		},
	}
}

func loadConfig(cmd *cobra.Command, v *viper.Viper, cfgFile string, stderr io.Writer) error {
	// Allow CT_PLAY_CONFIG when --config is unset.
	if cfgFile == "" {
		cfgFile = os.Getenv("CT_PLAY_CONFIG")
	}
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("config file %s: %w", cfgFile, err)
		}
		warnConfigPerms(cfgFile, stderr)
		return nil
	}

	// Implicit search; missing file is OK.
	v.SetConfigName("ct-play")
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		v.AddConfigPath(filepath.Join(xdg, "crosstalk"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ".config", "crosstalk"))
	}
	if err := v.ReadInConfig(); err != nil {
		// Not found is fine for implicit paths.
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return nil
		}
		// Also tolerate "no such file" style errors from some backends.
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("config: %w", err)
	}
	if used := v.ConfigFileUsed(); used != "" {
		warnConfigPerms(used, stderr)
	}
	return nil
}

func warnConfigPerms(path string, stderr io.Writer) {
	st, err := os.Stat(path)
	if err != nil {
		return
	}
	mode := st.Mode().Perm()
	if mode&0o077 != 0 {
		fmt.Fprintf(stderr, "warning: config file %s is group/world readable (mode %04o); restrict permissions if it contains a password\n", path, mode)
	}
}

func resolveConfig(v *viper.Viper) (play.Config, error) {
	// Apply explicitly-set flags last so they win over env and config.
	// Look up the root persistent flags via viper's bound command state is
	// unavailable here, so callers must invoke applyChangedFlags first.
	cfg := play.Config{
		Host:      strings.TrimRight(strings.TrimSpace(v.GetString("host")), "/"),
		Username:  strings.TrimSpace(v.GetString("username")),
		Password:  v.GetString("password"), // do not trim passwords
		SessionID: strings.TrimSpace(v.GetString("session_id")),
		ChannelID: strings.TrimSpace(v.GetString("channel_id")),
	}
	return cfg, nil
}

// applyChangedFlags copies only flags the user explicitly set into viper.
func applyChangedFlags(cmd *cobra.Command, v *viper.Viper) {
	fs := cmd.Root().PersistentFlags()
	set := func(flagName, key string) {
		f := fs.Lookup(flagName)
		if f != nil && f.Changed {
			v.Set(key, f.Value.String())
		}
	}
	set("host", "host")
	set("username", "username")
	set("password", "password")
	set("session", "session_id")
	set("channel", "channel_id")
}

func defaultService(host string) *play.Service {
	client := httpapi.New(host)
	return &play.Service{
		API:   httpapi.NewAdapter(client),
		Files: &ffmpeg.FileSource{},
		Media: func() play.MediaSink {
			return sessionmedia.New()
		},
	}
}
