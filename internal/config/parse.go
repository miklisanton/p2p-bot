package config

import (
	"flag"
	"fmt"
	"os"
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Database struct {
		Host     string `yaml:"host" validate:"required"`
		Port     string `yaml:"port" validate:"required"`
		Name     string `yaml:"name" validate:"required"`
		User     string `yaml:"user" validate:"required"`
		Password string `yaml:"password" validate:"required"`
		SSL      string `yaml:"ssl" validate:"required"`
	} `yaml:"database" validate:"required"`

	Redis struct {
		Host     string `yaml:"host" validate:"required"`
		Port     string `yaml:"port" validate:"required"`
		Password string `yaml:"password" validate:"required"`
	} `yaml:"redis" validate:"required"`

	RabbitMQ struct {
		URL string `yaml:"url" validate:"required"`
	} `yaml:"rabbitmq" validate:"required"`

	Telegram struct {
		APIkey     string `yaml:"api-key" validate:"required"`
		InviteLink string `yaml:"bot-link" validate:"required"`
		MiniAppURL string `yaml:"mini-app" validate:"required"`
	} `yaml:"telegram" validate:"required"`

	Exchange struct {
		MaxRetries int `yaml:"max-retries" validate:"required"`
		RetryDelay int `yaml:"retry-delay" validate:"required"`
	} `yaml:"exchange" validate:"required"`

	Website struct {
		Port        string `yaml:"port" validate:"required"`
		BackendPort string `yaml:"backend-port" validate:"required"`
		CertFile    string `yaml:"cert-file" validate:"required"`
		KeyFile     string `yaml:"key-file" validate:"required"`
		FrontURL    string `yaml:"front-url" validate:"required"`
		JWTSecret   string `yaml:"jwt-secret" validate:"required"`
		SubPrice    string `yaml:"subscription-price" validate:"required"`
		SubCurrency string `yaml:"subscription-currency" validate:"required"`

		Trial struct {
			Window int `yaml:"days_window" validate:"required"`
			Limit  int `yaml:"max_notifications" validate:"required"`
		} `yaml:"trial" validate:"required"`
	} `yaml:"website" validate:"required"`
}

func NewConfig(path string) (*Config, error) {
	config := &Config{}

	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting working directory:", err)
		return nil, err
	}
	fmt.Println("Current working directory:", dir)
	if err := godotenv.Load(".env"); err != nil {
		return nil, fmt.Errorf("no .env file found")
	}

	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	file, err = replaceEnvVars(file)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(file, &config)
	if err != nil {
		return nil, err
	}
	validate := validator.New()
	if err := validate.Struct(config); err != nil {
		for _, err := range err.(validator.ValidationErrors) {
			log.Printf("Error in field %s: %s", err.Field(), err.Tag())
		}
		return nil, fmt.Errorf("config validation failed")
	}

	return config, nil
}

func ParseCLI() (string, error) {
	var path string

	flag.StringVar(&path, "config", "./config.yaml", "path to config file")
	flag.Parse()

	s, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot get stat for %s", path)
	}
	if s.IsDir() {
		return "", fmt.Errorf("'%s' is a directory, not a normal file", path)
	}

	return path, nil
}

func replaceEnvVars(input []byte) ([]byte, error) {
	envVarRegexp := regexp.MustCompile(`\$\{(\w+)\}`)
	return envVarRegexp.ReplaceAllFunc(input, func(match []byte) []byte {
		key := string(match[2 : len(match)-1])
		return []byte(os.Getenv(key))
	}), nil
}
