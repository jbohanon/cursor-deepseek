package cmd

import (
	"context"
	"log"
	"strings"

	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/deepseek"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/ollama"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/openrouter"
	deepseekconstants "github.com/danilofalcao/cursor-deepseek/internal/constants/deepseek"
	ollamaconstants "github.com/danilofalcao/cursor-deepseek/internal/constants/ollama"
	openrouterconstants "github.com/danilofalcao/cursor-deepseek/internal/constants/openrouter"
	"github.com/danilofalcao/cursor-deepseek/internal/server"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type BackendConfig struct {
	Endpoint     string            `mapstructure:"endpoint"`
	Apikey       string            `mapstructure:"api_key"`
	Models       map[string]string `mapstructure:"models"`
	DefaultModel string            `mapstructure:"default_model"`
}
type config struct {
	Deepseek   BackendConfig `mapstructure:"deepseek"`
	Openrouter BackendConfig `mapstructure:"openrouter"`
	Ollama     BackendConfig `mapstructure:"ollama"`
	Port       string        `mapstructure:"port"`
	Loglevel   string        `mapstructure:"log_level"`
	Timeout    string        `mapstructure:"timeout"`
}

func Run() {
	var configPath *string = pflag.StringP("config", "c", "", "sets the config file location e.g. $HOME/proxy-config.yaml")

	pflag.Parse()
	ctx := context.Background()
	exitCh := make(chan string, 1)

	// Have to use custom key delimiter to allow for models with periods in the name
	v := viper.NewWithOptions(
		viper.KeyDelimiter("#"),
		viper.EnvKeyReplacer(strings.NewReplacer("#", "_")),
	)

	if configPath != nil && *configPath != "" {
		v.SetConfigFile(*configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}

	v.SetDefault("deepseek#default_model", deepseekconstants.DefaultChatModel)
	v.SetDefault("deepseek#endpoint", deepseekconstants.DefaultEndpoint)
	v.SetDefault("openrouter#default_model", openrouterconstants.DefaultModel)
	v.SetDefault("openrouter#endpoint", openrouterconstants.DefaultEndpoint)
	v.SetDefault("ollama#default_model", ollamaconstants.DefaultModel)

	v.BindPFlags(pflag.CommandLine)

	// Alias the previous env syntax to the new
	v.BindEnv("ollama#endpoint", "OLLAMA_API_ENDPOINT")
	v.AutomaticEnv()

	err := v.ReadInConfig()
	if err != nil {
		err = errors.Wrap(err, "error reading config file")
		log.Fatal(err)
	}

	var cfg config
	if err = v.Unmarshal(&cfg); err != nil {
		err = errors.Wrap(err, "error unmarshaling config")
		log.Fatal(err)
	}

	svr, err := server.New(ctx, server.Options{
		Port:     cfg.Port,
		Backend:  getBackend(v),
		LogLevel: "debug",
		Timeout:  "30s",
		ExitCh:   exitCh,
	})
	if err != nil {
		log.Fatalf("unable to start server %s", err.Error())
	}

	go func() {
		if err := svr.Start(); err != nil {
			exitCh <- err.Error()
		}
	}()

	select {
	case s := <-exitCh:
		log.Fatalf("killed with message %s", s)
	case <-ctx.Done():
		log.Fatal("context cancelled")
	}

}

func getBackend(v *viper.Viper) backend.Backend {
	var be backend.Backend
	switch {
	case v.IsSet("deepseek#api_key"):
		be = deepseek.NewDeepseekBackend(deepseek.Options{
			Endpoint:     v.GetString("deepseek#endpoint"),
			DefaultModel: v.GetString("deepseek#default_model"),
			Models:       v.GetStringMapString("deepseek#models"),
			ApiKey:       v.GetString("deepseek#api_key"),
			Timeout:      v.GetDuration("timeout"),
		})
	case v.IsSet("openrouter#api_key"):
		be = openrouter.NewOpenrouterBackend(openrouter.Options{
			Endpoint:     v.GetString("openrouter#endpoint"),
			DefaultModel: v.GetString("openrouter#default_model"),
			Models:       v.GetStringMapString("openrouter#models"),
			ApiKey:       v.GetString("openrouter#api_key"),
			Timeout:      v.GetDuration("timeout"),
		})
	case v.IsSet("ollama#endpoint"):
		be = ollama.NewOllamaBackend(ollama.Options{
			Endpoint:     v.GetString("ollama#endpoint"),
			DefaultModel: v.GetString("ollama#default_model"),
			Models:       v.GetStringMapString("ollama#models"),
			ApiKey:       v.GetString("ollama#api_key"),
			Timeout:      v.GetDuration("timeout"),
		})
	default:
		log.Fatal("unable to determine backend")
	}
	return be
}
