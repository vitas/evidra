package config

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Addr        string `mapstructure:"addr"`
	ExportDir   string `mapstructure:"export_dir"`
	DevInsecure bool   `mapstructure:"dev_insecure"`

	DBDriver   string `mapstructure:"db_driver"`
	DBDSN      string `mapstructure:"db_dsn"`
	DBDialect  string `mapstructure:"db_dialect"`
	DBMigrate  bool   `mapstructure:"db_migrate"`
	DBHost     string `mapstructure:"db_host"`
	DBPort     string `mapstructure:"db_port"`
	DBName     string `mapstructure:"db_name"`
	DBUser     string `mapstructure:"db_user"`
	DBPassword string `mapstructure:"db_password"`

	Auth AuthConfig       `mapstructure:"auth"`
	DB   DBTLSConfig      `mapstructure:"db"`
	Argo ArgoConfig       `mapstructure:"argo"`
	K8s  KubernetesConfig `mapstructure:"k8s"`
	TLS  TLSConfig        `mapstructure:"tls"`
}

type AuthConfig struct {
	Read      BearerAuth    `mapstructure:"read"`
	Ingest    IngestAuth    `mapstructure:"ingest"`
	Providers ProviderAuth  `mapstructure:"providers"`
	OIDC      OIDCAuth      `mapstructure:"oidc"`
	JWT       JWTAuth       `mapstructure:"jwt"`
	Audit     AuditAuth     `mapstructure:"audit"`
	RateLimit RateLimitAuth `mapstructure:"rate_limit"`
}

type BearerAuth struct {
	Token string `mapstructure:"token"`
}

type IngestAuth struct {
	Bearer         BearerAuth `mapstructure:"bearer"`
	GenericWebhook HMACAuth   `mapstructure:"generic_webhook"`
}

type HMACAuth struct {
	Secret string `mapstructure:"secret"`
}

type HeaderTokenAuth struct {
	Token string `mapstructure:"token"`
}

type ProviderAuth struct {
	GitHub    HMACAuth        `mapstructure:"github"`
	GitLab    HeaderTokenAuth `mapstructure:"gitlab"`
	Bitbucket HMACAuth        `mapstructure:"bitbucket"`
}

type OIDCAuth struct {
	Enabled     bool   `mapstructure:"enabled"`
	RolesHeader string `mapstructure:"roles_header"`
}

type JWTAuth struct {
	Enabled           bool          `mapstructure:"enabled"`
	Issuer            string        `mapstructure:"issuer"`
	Audience          string        `mapstructure:"audience"`
	RolesClaim        string        `mapstructure:"roles_claim"`
	HS256Secret       string        `mapstructure:"hs256_secret"`
	RS256PublicKeyPEM string        `mapstructure:"rs256_public_key_pem"`
	JWKSURL           string        `mapstructure:"jwks_url"`
	JWKSRefresh       time.Duration `mapstructure:"jwks_refresh"`
}

type AuditAuth struct {
	LogFile string `mapstructure:"log_file"`
}

type RateLimitAuth struct {
	Enabled         bool `mapstructure:"enabled"`
	ReadPerMinute   int  `mapstructure:"read_per_min"`
	ExportPerMinute int  `mapstructure:"export_per_min"`
	IngestPerMinute int  `mapstructure:"ingest_per_min"`
}

type DBTLSConfig struct {
	SSLMode     string `mapstructure:"sslmode"`
	SSLRootCert string `mapstructure:"sslrootcert"`
	SSLCert     string `mapstructure:"sslcert"`
	SSLKey      string `mapstructure:"sslkey"`
}

type ArgoConfig struct {
	Backend           string        `mapstructure:"backend"`
	CollectorEnabled  bool          `mapstructure:"collector_enabled"`
	CollectorInterval time.Duration `mapstructure:"collector_interval"`
	APIURL            string        `mapstructure:"api_url"`
	APIToken          string        `mapstructure:"api_token"`
	DefaultEnv        string        `mapstructure:"default_env"`
	CheckpointFile    string        `mapstructure:"checkpoint_file"`
}

type KubernetesConfig struct {
	CollectorEnabled  bool          `mapstructure:"collector_enabled"`
	CollectorInterval time.Duration `mapstructure:"collector_interval"`
	Namespace         string        `mapstructure:"namespace"`
	Cluster           string        `mapstructure:"cluster"`
	Environment       string        `mapstructure:"environment"`
	Kubeconfig        string        `mapstructure:"kubeconfig"`
	Context           string        `mapstructure:"context"`
}

type TLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

func LoadFromEnv() Config {
	v := viper.New()
	v.SetEnvPrefix("EVIDRA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Default values previously handled by envOr
	v.SetDefault("addr", ":8080")
	v.SetDefault("export_dir", "./var/exports")
	v.SetDefault("dev_insecure", false)
	v.SetDefault("db_migrate", true)
	v.SetDefault("auth.oidc.enabled", false)
	v.SetDefault("auth.oidc.roles_header", "X-Auth-Roles")
	v.SetDefault("auth.jwt.enabled", false)
	v.SetDefault("auth.jwt.roles_claim", "roles")
	v.SetDefault("auth.jwt.jwks_refresh", 5*time.Minute)
	v.SetDefault("auth.rate_limit.enabled", false)
	v.SetDefault("auth.rate_limit.read_per_min", 600)
	v.SetDefault("auth.rate_limit.export_per_min", 120)
	v.SetDefault("auth.rate_limit.ingest_per_min", 240)
	v.SetDefault("argo.backend", "argocd-client")
	v.SetDefault("argo.collector_enabled", false)
	v.SetDefault("argo.collector_interval", 30*time.Second)
	v.SetDefault("argo.default_env", "unknown")
	v.SetDefault("argo.checkpoint_file", "./var/argo_checkpoint.json")
	v.SetDefault("k8s.collector_enabled", false)
	v.SetDefault("k8s.collector_interval", 30*time.Second)
	v.SetDefault("k8s.namespace", "")
	v.SetDefault("k8s.cluster", "unknown")
	v.SetDefault("k8s.environment", "unknown")
	v.SetDefault("tls.enabled", false)

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("/etc/evidra/")

	_ = v.ReadInConfig() // ignore if not found

	// Explicitly bind env vars for nested structs to ensure Viper maps them correctly
	viper.BindEnv("auth.read.token", "EVIDRA_READ_TOKEN")
	viper.BindEnv("auth.ingest.bearer.token", "EVIDRA_INGEST_TOKEN")
	viper.BindEnv("auth.ingest.generic_webhook.secret", "EVIDRA_WEBHOOK_SECRET")
	viper.BindEnv("auth.providers.github.secret", "EVIDRA_GITHUB_WEBHOOK_SECRET")
	viper.BindEnv("auth.providers.gitlab.token", "EVIDRA_GITLAB_WEBHOOK_TOKEN")
	viper.BindEnv("auth.providers.bitbucket.secret", "EVIDRA_BITBUCKET_WEBHOOK_SECRET")
	viper.BindEnv("argo.api_url", "EVIDRA_ARGO_API_URL")
	viper.BindEnv("argo.api_token", "EVIDRA_ARGO_API_TOKEN")
	viper.BindEnv("auth.jwt.jwks_url", "EVIDRA_AUTH_JWT_JWKS_URL")
	viper.BindEnv("auth.jwt.jwks_refresh", "EVIDRA_AUTH_JWT_JWKS_REFRESH")
	viper.BindEnv("auth.rate_limit.read_per_min", "EVIDRA_AUTH_RATE_LIMIT_READ_PER_MIN")
	viper.BindEnv("auth.rate_limit.export_per_min", "EVIDRA_AUTH_RATE_LIMIT_EXPORT_PER_MIN")
	viper.BindEnv("auth.rate_limit.ingest_per_min", "EVIDRA_AUTH_RATE_LIMIT_INGEST_PER_MIN")
	viper.BindEnv("argo.collector_interval", "EVIDRA_ARGO_COLLECTOR_INTERVAL")
	viper.BindEnv("k8s.collector_interval", "EVIDRA_K8S_COLLECTOR_INTERVAL")

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		fmt.Printf("Warning: failed to unmarshal config: %v\n", err)
	}

	// Legacy ENV override mapping to support EVIDRA_DB_DSN translating to root structs
	if v := v.GetString("db_driver"); v != "" {
		cfg.DBDriver = v
	}
	if v := v.GetString("db_dsn"); v != "" {
		cfg.DBDSN = v
	}
	if v := v.GetString("db_dialect"); v != "" {
		cfg.DBDialect = v
	}
	if v := v.GetString("db_host"); v != "" {
		cfg.DBHost = v
	}
	if v := v.GetString("db_port"); v != "" {
		cfg.DBPort = v
	}
	if v := v.GetString("db_name"); v != "" {
		cfg.DBName = v
	}
	if v := v.GetString("db_user"); v != "" {
		cfg.DBUser = v
	}
	if v := v.GetString("db_password"); v != "" {
		cfg.DBPassword = v
	}
	if val := v.GetString("auth.read.token"); val != "" {
		cfg.Auth.Read.Token = val
	}
	if val := v.GetString("auth.ingest.bearer.token"); val != "" {
		cfg.Auth.Ingest.Bearer.Token = val
	}
	if val := v.GetString("auth.ingest.generic_webhook.secret"); val != "" {
		cfg.Auth.Ingest.GenericWebhook.Secret = val
	}
	if val := v.GetString("argo.api_url"); val != "" {
		cfg.Argo.APIURL = val
	}
	if val := v.GetString("argo.api_token"); val != "" {
		cfg.Argo.APIToken = val
	}
	if val := v.GetString("auth.jwt.jwks_url"); val != "" {
		cfg.Auth.JWT.JWKSURL = val
	}
	if val := v.GetString("auth.jwt.jwks_refresh"); val != "" {
		cfg.Auth.JWT.JWKSRefresh, _ = time.ParseDuration(val)
	}
	if val := v.GetString("auth.oidc.roles_header"); val != "" {
		cfg.Auth.OIDC.RolesHeader = val
	}
	if v.GetBool("auth.oidc.enabled") {
		cfg.Auth.OIDC.Enabled = true
	}
	if v.GetBool("auth.jwt.enabled") {
		cfg.Auth.JWT.Enabled = true
	}
	if val := v.GetString("auth.jwt.issuer"); val != "" {
		cfg.Auth.JWT.Issuer = val
	}
	if val := v.GetString("auth.jwt.audience"); val != "" {
		cfg.Auth.JWT.Audience = val
	}
	if val := v.GetString("auth.jwt.roles_claim"); val != "" {
		cfg.Auth.JWT.RolesClaim = val
	}
	if val := v.GetString("auth.jwt.hs256_secret"); val != "" {
		cfg.Auth.JWT.HS256Secret = val
	}
	if val := v.GetString("auth.audit.log_file"); val != "" {
		cfg.Auth.Audit.LogFile = val
	}
	if val := v.GetString("db.sslmode"); val != "" {
		cfg.DB.SSLMode = val
	}
	if val := v.GetString("db.sslrootcert"); val != "" {
		cfg.DB.SSLRootCert = val
	}
	if val := v.GetString("db.sslcert"); val != "" {
		cfg.DB.SSLCert = val
	}
	if val := v.GetString("db.sslkey"); val != "" {
		cfg.DB.SSLKey = val
	}
	if v.GetBool("tls.enabled") {
		cfg.TLS.Enabled = true
	}
	if val := v.GetString("tls.cert_file"); val != "" {
		cfg.TLS.CertFile = val
	}
	if val := v.GetString("tls.key_file"); val != "" {
		cfg.TLS.KeyFile = val
	}
	if val := v.GetString("k8s.namespace"); val != "" {
		cfg.K8s.Namespace = val
	}
	if val := v.GetString("k8s.cluster"); val != "" {
		cfg.K8s.Cluster = val
	}
	if val := v.GetString("k8s.environment"); val != "" {
		cfg.K8s.Environment = val
	}
	if val := v.GetString("k8s.kubeconfig"); val != "" {
		cfg.K8s.Kubeconfig = val
	}
	if val := v.GetString("k8s.context"); val != "" {
		cfg.K8s.Context = val
	}
	if val := v.GetString("argo.default_env"); val != "" {
		cfg.Argo.DefaultEnv = val
	}
	if val := v.GetString("argo.checkpoint_file"); val != "" {
		cfg.Argo.CheckpointFile = val
	}

	if cfg.DBDialect == "" {
		cfg.DBDialect = cfg.DBDriver
	}
	if cfg.DBDSN == "" {
		cfg.DBDSN = buildDSNFromParts(cfg)
	}
	return cfg
}

func (c Config) Validate() error {
	var problems []string
	if strings.TrimSpace(c.Addr) == "" {
		problems = append(problems, "EVIDRA_ADDR must not be empty")
	}
	if strings.TrimSpace(c.ExportDir) == "" {
		problems = append(problems, "EVIDRA_EXPORT_DIR must not be empty")
	}
	if c.DBDriver != "" && c.DBDSN == "" {
		problems = append(problems, "database connection is not configured; set EVIDRA_DB_DSN or EVIDRA_DB_HOST/EVIDRA_DB_PORT/EVIDRA_DB_NAME/EVIDRA_DB_USER/EVIDRA_DB_PASSWORD")
	}
	if c.DBDSN != "" && c.DBDriver == "" {
		problems = append(problems, "EVIDRA_DB_DRIVER is required when EVIDRA_DB_DSN is set")
	}
	if c.DBDSN == "" && hasAnyDBParts(c) && !hasAllDBParts(c) {
		problems = append(problems, "incomplete split DB config; set all of EVIDRA_DB_HOST/EVIDRA_DB_PORT/EVIDRA_DB_NAME/EVIDRA_DB_USER/EVIDRA_DB_PASSWORD")
	}
	if c.DBDSN == "" && hasAnyDBParts(c) && c.DBDriver == "" {
		problems = append(problems, "EVIDRA_DB_DRIVER is required when split DB connection variables are set")
	}
	if c.Argo.CollectorEnabled && strings.TrimSpace(c.Argo.APIURL) == "" {
		problems = append(problems, "EVIDRA_ARGO_API_URL is required when EVIDRA_ARGO_COLLECTOR_ENABLED=true")
	}
	if c.Argo.CollectorEnabled && strings.TrimSpace(c.Argo.CheckpointFile) == "" {
		problems = append(problems, "EVIDRA_ARGO_CHECKPOINT_FILE is required when EVIDRA_ARGO_COLLECTOR_ENABLED=true")
	}
	if c.Argo.CollectorEnabled {
		backend := strings.ToLower(strings.TrimSpace(c.Argo.Backend))
		if backend != "argocd-client" {
			problems = append(problems, "EVIDRA_ARGO_BACKEND must be: argocd-client")
		}
	}
	if c.K8s.CollectorEnabled {
		problems = append(problems, "EVIDRA_K8S_COLLECTOR_ENABLED=true is out of scope for Argo CD-first v1")
	}
	if !c.DevInsecure {
		readAuthConfigured := strings.TrimSpace(c.Auth.Read.Token) != "" || c.Auth.OIDC.Enabled || c.Auth.JWT.Enabled
		if !readAuthConfigured {
			problems = append(problems, "read/export auth is not configured; set EVIDRA_READ_TOKEN or enable OIDC/JWT, or explicitly set EVIDRA_DEV_INSECURE=true for local development only")
		}
	}
	if c.Auth.JWT.Enabled {
		if strings.TrimSpace(c.Auth.JWT.Issuer) == "" {
			problems = append(problems, "EVIDRA_AUTH_JWT_ISSUER is required when EVIDRA_AUTH_JWT_ENABLED=true")
		}
		if strings.TrimSpace(c.Auth.JWT.Audience) == "" {
			problems = append(problems, "EVIDRA_AUTH_JWT_AUDIENCE is required when EVIDRA_AUTH_JWT_ENABLED=true")
		}
		if strings.TrimSpace(c.Auth.JWT.HS256Secret) == "" &&
			strings.TrimSpace(c.Auth.JWT.RS256PublicKeyPEM) == "" &&
			strings.TrimSpace(c.Auth.JWT.JWKSURL) == "" {
			problems = append(problems, "one of EVIDRA_AUTH_JWT_HS256_SECRET, EVIDRA_AUTH_JWT_RS256_PUBLIC_KEY_PEM, or EVIDRA_AUTH_JWT_JWKS_URL is required when EVIDRA_AUTH_JWT_ENABLED=true")
		}
	}
	if c.TLS.Enabled && strings.TrimSpace(c.TLS.CertFile) == "" {
		problems = append(problems, "EVIDRA_TLS_CERT_FILE is required when EVIDRA_TLS_ENABLED=true")
	}
	if c.TLS.Enabled && strings.TrimSpace(c.TLS.KeyFile) == "" {
		problems = append(problems, "EVIDRA_TLS_KEY_FILE is required when EVIDRA_TLS_ENABLED=true")
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

type StartupSummary struct {
	RepositoryMode     string
	WebhookProviders   []string
	ArgoCollector      bool
	ArgoBackend        string
	ArgoCheckpointFile string
	K8sCollector       bool
	K8sNamespace       string
	OIDCEnabled        bool
	JWTEnabled         bool
	TLSEnabled         bool
	AuthRateLimit      bool
	DevInsecure        bool
}

func (c Config) Summary() StartupSummary {
	mode := "memory"
	if c.DBDriver != "" && c.DBDSN != "" {
		mode = "sql:" + c.DBDialect
	}
	return StartupSummary{
		RepositoryMode:     mode,
		WebhookProviders:   []string{"argocd"},
		ArgoCollector:      c.Argo.CollectorEnabled,
		ArgoBackend:        c.Argo.Backend,
		ArgoCheckpointFile: c.Argo.CheckpointFile,
		K8sCollector:       false,
		K8sNamespace:       c.K8s.Namespace,
		OIDCEnabled:        c.Auth.OIDC.Enabled,
		JWTEnabled:         c.Auth.JWT.Enabled,
		TLSEnabled:         c.TLS.Enabled,
		AuthRateLimit:      c.Auth.RateLimit.Enabled,
		DevInsecure:        c.DevInsecure,
	}
}

func parsePositiveIntOr(key string, fallback int) int {
	v := strings.TrimSpace(viper.GetString(key))
	if v == "" {
		return fallback
	}
	n := 0
	for _, r := range v {
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return fallback
	}
	return n
}

func hasAnyDBParts(c Config) bool {
	return strings.TrimSpace(c.DBHost) != "" ||
		strings.TrimSpace(c.DBPort) != "" ||
		strings.TrimSpace(c.DBName) != "" ||
		strings.TrimSpace(c.DBUser) != "" ||
		strings.TrimSpace(c.DBPassword) != ""
}

func hasAllDBParts(c Config) bool {
	return strings.TrimSpace(c.DBHost) != "" &&
		strings.TrimSpace(c.DBPort) != "" &&
		strings.TrimSpace(c.DBName) != "" &&
		strings.TrimSpace(c.DBUser) != "" &&
		strings.TrimSpace(c.DBPassword) != ""
}

func buildDSNFromParts(c Config) string {
	if !hasAllDBParts(c) {
		return ""
	}
	port := strings.TrimSpace(c.DBPort)
	if port == "" {
		port = "5432"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return ""
	}
	sslMode := strings.TrimSpace(c.DB.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.DBUser, c.DBPassword),
		Host:   fmt.Sprintf("%s:%s", c.DBHost, port),
		Path:   "/" + url.PathEscape(c.DBName),
	}
	q := url.Values{}
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()
	return u.String()
}
