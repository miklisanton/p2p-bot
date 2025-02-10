package config

import (
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"regexp"
)

type Config struct {
	Database struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Name     string `yaml:"name"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		SSL      string `yaml:"ssl"`
	}
	Redis struct {
		Host string `yaml:"host"`
		Port string `yaml:"port"`
	}
	RabbitMQ struct {
		URL string `yaml:"url"`
	}
	Telegram struct {
		APIkey     string `yaml:"api-key"`
		InviteLink string `yaml:"bot-link"`
		MiniAppURL string `yaml:"mini-app"`
	}
	Exchange struct {
		MaxRetries int `yaml:"max-retries"`
		RetryDelay int `yaml:"retry-delay"`
	}
	Website struct {
		Port        string `yaml:"port"`
		BackendPort string `yaml:"backend-port"`
		CertFile    string `yaml:"cert-file"`
		KeyFile     string `yaml:"key-file"`
		FrontURL    string `yaml:"front-url"`
		JWTSecret   string `yaml:"jwt-secret"`
		SubPrice    string `yaml:"subscription-price"`
		SubCurrency string `yaml:"subscription-currency"`
	}
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
		log.Fatal(err)
	}

	err = yaml.Unmarshal(file, &config)
	if err != nil {
		log.Fatalf("Error unmarshalling YAML: %v", err)
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
