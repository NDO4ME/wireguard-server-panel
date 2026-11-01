package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	WireGuard WireGuardConfig `mapstructure:"wireguard"`
	Security  SecurityConfig  `mapstructure:"security"`
	Admin     AdminConfig     `mapstructure:"admin"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type JWTConfig struct {
	AccessSecret        string `mapstructure:"access_secret"`
	RefreshSecret       string `mapstructure:"refresh_secret"`
	AccessExpiryMinutes int    `mapstructure:"access_expiry_minutes"`
	RefreshExpiryDays   int    `mapstructure:"refresh_expiry_days"`
}

type WireGuardConfig struct {
	Interface       string `mapstructure:"interface"`
	ServerPublicKey string `mapstructure:"server_public_key"`
	ServerEndpoint  string `mapstructure:"server_endpoint"`
	DNS             string `mapstructure:"dns"`
	AllowedIPs      string `mapstructure:"allowed_ips"`
	Subnet          string `mapstructure:"subnet"`
	ConfigPath      string `mapstructure:"config_path"`
}

type SecurityConfig struct {
	BcryptCost             int `mapstructure:"bcrypt_cost"`
	RateLimitRequests      int `mapstructure:"rate_limit_requests"`
	RateLimitWindowSeconds int `mapstructure:"rate_limit_window_seconds"`
}

type AdminConfig struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

var AppConfig *Config

func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Environment variable bindings for secrets
	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Explicit bindings for critical secrets
	viper.BindEnv("jwt.access_secret", "JWT_ACCESS_SECRET")
	viper.BindEnv("jwt.refresh_secret", "JWT_REFRESH_SECRET")
	viper.BindEnv("admin.username", "ADMIN_USERNAME")
	viper.BindEnv("admin.password", "ADMIN_PASSWORD")
	viper.BindEnv("server.port", "PANEL_PORT")
	viper.BindEnv("database.path", "DATABASE_PATH")
	viper.BindEnv("wireguard.dns", "WG_DNS")
	viper.BindEnv("wireguard.subnet", "WG_NETWORK")
	viper.BindEnv("wireguard.server_endpoint", "WG_SERVER_ENDPOINT")
	viper.BindEnv("wireguard.server_public_key", "WG_SERVER_PUBLIC_KEY")

	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Warning: Could not read config file: %v. Using defaults and env vars.", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, err
	}

	// Validate critical settings: a default or short JWT secret would let anyone forge tokens.
	for name, v := range map[string]string{"JWT_ACCESS_SECRET": config.JWT.AccessSecret, "JWT_REFRESH_SECRET": config.JWT.RefreshSecret} {
		if strings.HasPrefix(v, "change-me") || len(v) < 16 {
			return nil, fmt.Errorf("%s is unset, default or shorter than 16 characters; set a long random value", name)
		}
	}

	AppConfig = &config
	return &config, nil
}
