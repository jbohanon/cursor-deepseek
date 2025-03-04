package main

import (
	"context"
	"log"
	"strings"

	"github.com/danilofalcao/cursor-deepseek/internal/backend"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/deepseek"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/ollama"
	"github.com/danilofalcao/cursor-deepseek/internal/backend/openrouter"
	deepseekconstants "github.com/danilofalcao/cursor-deepseek/internal/constants/deepseek"
	openrouterconstants "github.com/danilofalcao/cursor-deepseek/internal/constants/openrouter"
	"github.com/danilofalcao/cursor-deepseek/internal/server"
	"github.com/spf13/viper"
)

type BackendConfig struct {
	Endpoint string `mapstructure:"endpoint"`
	Apikey   string `mapstructure:"api_key"`
	Model    string `mapstructure:"model"`
}
type config struct {
	Deepseek   BackendConfig `mapstructure:"deepseek"`
	Openrouter BackendConfig `mapstructure:"openrouter"`
	Ollama     BackendConfig `mapstructure:"ollama"`
	Port       string        `mapstructure:"port"`
	Loglevel   string        `mapstructure:"log_level"`
	Timeout    string        `mapstructure:"timeout"`
}

func main() {
	ctx := context.Background()
	exitCh := make(chan string, 1)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetDefault("deepseek.model", deepseekconstants.DefaultChatModel)
	viper.SetDefault("deepseek.endpoint", deepseekconstants.DefaultEndpoint)
	viper.SetDefault("openrouter.model", openrouterconstants.DefaultModel)
	viper.SetDefault("openrouter.endpoint", openrouterconstants.DefaultEndpoint)

	err := viper.ReadInConfig()
	if err != nil {
		log.Fatalf("Failed to read config file: %s", err)
	}

	var cfg config
	if err = viper.Unmarshal(&cfg); err != nil {
		log.Fatal("unmarshaling config")
	}

	svr, err := server.New(ctx, server.Options{
		Port:     "9000",
		Backend:  getBackend(),
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

func getBackend() backend.Backend {
	var be backend.Backend
	switch {
	case viper.IsSet("deepseek.api_key"):
		be = deepseek.NewDeepseekBackend(deepseek.Options{
			Endpoint: viper.GetString("deepseek.endpoint"),
			Model:    viper.GetString("deepseek.model"),
			ApiKey:   viper.GetString("deepseek.api_key"),
			Timeout:  viper.GetDuration("timeout"),
		})
	case viper.IsSet("openrouter.api_key"):
		be = openrouter.NewOpenrouterBackend(openrouter.Options{
			Endpoint: viper.GetString("openrouter.endpoint"),
			Model:    viper.GetString("openrouter.model"),
			ApiKey:   viper.GetString("openrouter.api_key"),
			Timeout:  viper.GetDuration("timeout"),
		})
	case viper.IsSet("ollama.endpoint"):
		be = ollama.NewOllamaBackend(ollama.Options{
			Endpoint: viper.GetString("ollama.endpoint"),
			Model:    viper.GetString("ollama.model"),
			ApiKey:   viper.GetString("ollama.api_key"),
			Timeout:  viper.GetDuration("timeout"),
		})
	default:
		log.Fatal("unable to determine backend")
	}
	return be
}
