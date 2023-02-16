// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"code.gitea.io/gitea/modules/container"
	"code.gitea.io/gitea/modules/generate"
	"code.gitea.io/gitea/modules/json"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/user"
	"code.gitea.io/gitea/modules/util"

	gossh "golang.org/x/crypto/ssh"
	ini "gopkg.in/ini.v1"
)

// Scheme describes protocol types
type Scheme string

// enumerates all the scheme types
const (
	HTTP     Scheme = "http"
	HTTPS    Scheme = "https"
	FCGI     Scheme = "fcgi"
	FCGIUnix Scheme = "fcgi+unix"
	HTTPUnix Scheme = "http+unix"
)

// LandingPage describes the default page
type LandingPage string

// enumerates all the landing page types
const (
	LandingPageHome          LandingPage = "/"
	LandingPageExplore       LandingPage = "/explore"
	LandingPageOrganizations LandingPage = "/explore/organizations"
	LandingPageLogin         LandingPage = "/user/login"
)

// enumerates all the types of captchas
const (
	ImageCaptcha = "image"
	ReCaptcha    = "recaptcha"
	HCaptcha     = "hcaptcha"
	MCaptcha     = "mcaptcha"
	CfTurnstile  = "cfturnstile"
)

// settings
var (
	// AppVer is the version of the current build of Gitea. It is set in main.go from main.Version.
	AppVer string
	// AppBuiltWith represents a human readable version go runtime build version and build tags. (See main.go formatBuiltWith().)
	AppBuiltWith string
	// AppStartTime store time gitea has started
	AppStartTime time.Time
	// AppName is the Application name, used in the page title.
	// It maps to ini:"APP_NAME"
	AppName string
	// AppURL is the Application ROOT_URL. It always has a '/' suffix
	// It maps to ini:"ROOT_URL"
	AppURL string
	// AppSubURL represents the sub-url mounting point for gitea. It is either "" or starts with '/' and ends without '/', such as '/{subpath}'.
	// This value is empty if site does not have sub-url.
	AppSubURL string
	// AppPath represents the path to the gitea binary
	AppPath string
	// AppWorkPath is the "working directory" of Gitea. It maps to the environment variable GITEA_WORK_DIR.
	// If that is not set it is the default set here by the linker or failing that the directory of AppPath.
	//
	// AppWorkPath is used as the base path for several other paths.
	AppWorkPath string
	// AppDataPath is the default path for storing data.
	// It maps to ini:"APP_DATA_PATH" in [server] and defaults to AppWorkPath + "/data"
	AppDataPath string
	// LocalURL is the url for locally running applications to contact Gitea. It always has a '/' suffix
	// It maps to ini:"LOCAL_ROOT_URL" in [server]
	LocalURL string
	// AssetVersion holds a opaque value that is used for cache-busting assets
	AssetVersion string

	// Server settings
	Protocol                   Scheme
	UseProxyProtocol           bool // `ini:"USE_PROXY_PROTOCOL"`
	ProxyProtocolTLSBridging   bool //`ini:"PROXY_PROTOCOL_TLS_BRIDGING"`
	ProxyProtocolHeaderTimeout time.Duration
	ProxyProtocolAcceptUnknown bool
	Domain                     string
	HTTPAddr                   string
	HTTPPort                   string
	LocalUseProxyProtocol      bool
	RedirectOtherPort          bool
	RedirectorUseProxyProtocol bool
	PortToRedirect             string
	OfflineMode                bool
	CertFile                   string
	KeyFile                    string
	StaticRootPath             string
	StaticCacheTime            time.Duration
	EnableGzip                 bool
	LandingPageURL             LandingPage
	LandingPageCustom          string
	UnixSocketPermission       uint32
	EnablePprof                bool
	PprofDataPath              string
	EnableAcme                 bool
	AcmeTOS                    bool
	AcmeLiveDirectory          string
	AcmeEmail                  string
	AcmeURL                    string
	AcmeCARoot                 string
	SSLMinimumVersion          string
	SSLMaximumVersion          string
	SSLCurvePreferences        []string
	SSLCipherSuites            []string
	GracefulRestartable        bool
	GracefulHammerTime         time.Duration
	StartupTimeout             time.Duration
	PerWriteTimeout            = 30 * time.Second
	PerWritePerKbTimeout       = 10 * time.Second
	StaticURLPrefix            string
	AbsoluteAssetURL           string

	SSH = struct {
		Disabled                              bool               `ini:"DISABLE_SSH"`
		StartBuiltinServer                    bool               `ini:"START_SSH_SERVER"`
		BuiltinServerUser                     string             `ini:"BUILTIN_SSH_SERVER_USER"`
		UseProxyProtocol                      bool               `ini:"SSH_SERVER_USE_PROXY_PROTOCOL"`
		Domain                                string             `ini:"SSH_DOMAIN"`
		Port                                  int                `ini:"SSH_PORT"`
		User                                  string             `ini:"SSH_USER"`
		ListenHost                            string             `ini:"SSH_LISTEN_HOST"`
		ListenPort                            int                `ini:"SSH_LISTEN_PORT"`
		RootPath                              string             `ini:"SSH_ROOT_PATH"`
		ServerCiphers                         []string           `ini:"SSH_SERVER_CIPHERS"`
		ServerKeyExchanges                    []string           `ini:"SSH_SERVER_KEY_EXCHANGES"`
		ServerMACs                            []string           `ini:"SSH_SERVER_MACS"`
		ServerHostKeys                        []string           `ini:"SSH_SERVER_HOST_KEYS"`
		KeyTestPath                           string             `ini:"SSH_KEY_TEST_PATH"`
		KeygenPath                            string             `ini:"SSH_KEYGEN_PATH"`
		AuthorizedKeysBackup                  bool               `ini:"SSH_AUTHORIZED_KEYS_BACKUP"`
		AuthorizedPrincipalsBackup            bool               `ini:"SSH_AUTHORIZED_PRINCIPALS_BACKUP"`
		AuthorizedKeysCommandTemplate         string             `ini:"SSH_AUTHORIZED_KEYS_COMMAND_TEMPLATE"`
		AuthorizedKeysCommandTemplateTemplate *template.Template `ini:"-"`
		MinimumKeySizeCheck                   bool               `ini:"-"`
		MinimumKeySizes                       map[string]int     `ini:"-"`
		CreateAuthorizedKeysFile              bool               `ini:"SSH_CREATE_AUTHORIZED_KEYS_FILE"`
		CreateAuthorizedPrincipalsFile        bool               `ini:"SSH_CREATE_AUTHORIZED_PRINCIPALS_FILE"`
		ExposeAnonymous                       bool               `ini:"SSH_EXPOSE_ANONYMOUS"`
		AuthorizedPrincipalsAllow             []string           `ini:"SSH_AUTHORIZED_PRINCIPALS_ALLOW"`
		AuthorizedPrincipalsEnabled           bool               `ini:"-"`
		TrustedUserCAKeys                     []string           `ini:"SSH_TRUSTED_USER_CA_KEYS"`
		TrustedUserCAKeysFile                 string             `ini:"SSH_TRUSTED_USER_CA_KEYS_FILENAME"`
		TrustedUserCAKeysParsed               []gossh.PublicKey  `ini:"-"`
		PerWriteTimeout                       time.Duration      `ini:"SSH_PER_WRITE_TIMEOUT"`
		PerWritePerKbTimeout                  time.Duration      `ini:"SSH_PER_WRITE_PER_KB_TIMEOUT"`
	}{
		Disabled:                      false,
		StartBuiltinServer:            false,
		Domain:                        "",
		Port:                          22,
		ServerCiphers:                 []string{"chacha20-poly1305@openssh.com", "aes128-ctr", "aes192-ctr", "aes256-ctr", "aes128-gcm@openssh.com", "aes256-gcm@openssh.com"},
		ServerKeyExchanges:            []string{"curve25519-sha256", "ecdh-sha2-nistp256", "ecdh-sha2-nistp384", "ecdh-sha2-nistp521", "diffie-hellman-group14-sha256", "diffie-hellman-group14-sha1"},
		ServerMACs:                    []string{"hmac-sha2-256-etm@openssh.com", "hmac-sha2-256", "hmac-sha1"},
		KeygenPath:                    "ssh-keygen",
		MinimumKeySizeCheck:           true,
		MinimumKeySizes:               map[string]int{"ed25519": 256, "ed25519-sk": 256, "ecdsa": 256, "ecdsa-sk": 256, "rsa": 2047},
		ServerHostKeys:                []string{"ssh/gitea.rsa", "ssh/gogs.rsa"},
		AuthorizedKeysCommandTemplate: "{{.AppPath}} --config={{.CustomConf}} serv key-{{.Key.ID}}",
		PerWriteTimeout:               PerWriteTimeout,
		PerWritePerKbTimeout:          PerWritePerKbTimeout,
	}

	// Security settings
	InstallLock                        bool
	SecretKey                          string
	LogInRememberDays                  int
	CookieUserName                     string
	CookieRememberName                 string
	ReverseProxyAuthUser               string
	ReverseProxyAuthEmail              string
	ReverseProxyAuthFullName           string
	ReverseProxyLimit                  int
	ReverseProxyTrustedProxies         []string
	MinPasswordLength                  int
	ImportLocalPaths                   bool
	DisableGitHooks                    bool
	DisableWebhooks                    bool
	OnlyAllowPushIfGiteaEnvironmentSet bool
	PasswordComplexity                 []string
	PasswordHashAlgo                   string
	PasswordCheckPwn                   bool
	SuccessfulTokensCacheSize          int

	Camo = struct {
		Enabled   bool
		ServerURL string `ini:"SERVER_URL"`
		HMACKey   string `ini:"HMAC_KEY"`
		Allways   bool
	}{}

	// UI settings
	UI = struct {
		ExplorePagingNum      int
		SitemapPagingNum      int
		IssuePagingNum        int
		RepoSearchPagingNum   int
		MembersPagingNum      int
		FeedMaxCommitNum      int
		FeedPagingNum         int
		PackagesPagingNum     int
		GraphMaxCommitNum     int
		CodeCommentLines      int
		ReactionMaxUserNum    int
		ThemeColorMetaTag     string
		MaxDisplayFileSize    int64
		ShowUserEmail         bool
		DefaultShowFullName   bool
		DefaultTheme          string
		Themes                []string
		Reactions             []string
		ReactionsLookup       container.Set[string] `ini:"-"`
		CustomEmojis          []string
		CustomEmojisMap       map[string]string `ini:"-"`
		SearchRepoDescription bool
		UseServiceWorker      bool

		Notification struct {
			MinTimeout            time.Duration
			TimeoutStep           time.Duration
			MaxTimeout            time.Duration
			EventSourceUpdateTime time.Duration
		} `ini:"ui.notification"`

		SVG struct {
			Enabled bool `ini:"ENABLE_RENDER"`
		} `ini:"ui.svg"`

		CSV struct {
			MaxFileSize int64
		} `ini:"ui.csv"`

		Admin struct {
			UserPagingNum   int
			RepoPagingNum   int
			NoticePagingNum int
			OrgPagingNum    int
		} `ini:"ui.admin"`
		User struct {
			RepoPagingNum int
		} `ini:"ui.user"`
		Meta struct {
			Author      string
			Description string
			Keywords    string
		} `ini:"ui.meta"`
	}{
		ExplorePagingNum:    20,
		SitemapPagingNum:    20,
		IssuePagingNum:      20,
		RepoSearchPagingNum: 20,
		MembersPagingNum:    20,
		FeedMaxCommitNum:    5,
		FeedPagingNum:       20,
		PackagesPagingNum:   20,
		GraphMaxCommitNum:   100,
		CodeCommentLines:    4,
		ReactionMaxUserNum:  10,
		ThemeColorMetaTag:   `#6cc644`,
		MaxDisplayFileSize:  8388608,
		DefaultTheme:        `auto`,
		Themes:              []string{`auto`, `gitea`, `arc-green`},
		Reactions:           []string{`+1`, `-1`, `laugh`, `hooray`, `confused`, `heart`, `rocket`, `eyes`},
		CustomEmojis:        []string{`git`, `gitea`, `codeberg`, `gitlab`, `github`, `gogs`},
		CustomEmojisMap:     map[string]string{"git": ":git:", "gitea": ":gitea:", "codeberg": ":codeberg:", "gitlab": ":gitlab:", "github": ":github:", "gogs": ":gogs:"},
		Notification: struct {
			MinTimeout            time.Duration
			TimeoutStep           time.Duration
			MaxTimeout            time.Duration
			EventSourceUpdateTime time.Duration
		}{
			MinTimeout:            10 * time.Second,
			TimeoutStep:           10 * time.Second,
			MaxTimeout:            60 * time.Second,
			EventSourceUpdateTime: 10 * time.Second,
		},
		SVG: struct {
			Enabled bool `ini:"ENABLE_RENDER"`
		}{
			Enabled: true,
		},
		CSV: struct {
			MaxFileSize int64
		}{
			MaxFileSize: 524288,
		},
		Admin: struct {
			UserPagingNum   int
			RepoPagingNum   int
			NoticePagingNum int
			OrgPagingNum    int
		}{
			UserPagingNum:   50,
			RepoPagingNum:   50,
			NoticePagingNum: 25,
			OrgPagingNum:    50,
		},
		User: struct {
			RepoPagingNum int
		}{
			RepoPagingNum: 15,
		},
		Meta: struct {
			Author      string
			Description string
			Keywords    string
		}{
			Author:      "Gitea - Git with a cup of tea",
			Description: "Gitea (Git with a cup of tea) is a painless self-hosted Git service written in Go",
			Keywords:    "go,git,self-hosted,gitea",
		},
	}

	// Markdown settings
	Markdown = struct {
		EnableHardLineBreakInComments  bool
		EnableHardLineBreakInDocuments bool
		CustomURLSchemes               []string `ini:"CUSTOM_URL_SCHEMES"`
		FileExtensions                 []string
		EnableMath                     bool
	}{
		EnableHardLineBreakInComments:  true,
		EnableHardLineBreakInDocuments: false,
		FileExtensions:                 strings.Split(".md,.markdown,.mdown,.mkd", ","),
		EnableMath:                     true,
	}

	// Admin settings
	Admin struct {
		DisableRegularOrgCreation bool
		DefaultEmailNotification  string
	}

	// Log settings
	LogLevel           log.Level
	StacktraceLogLevel string
	LogRootPath        string
	EnableSSHLog       bool
	EnableXORMLog      bool

	DisableRouterLog bool

	EnableAccessLog   bool
	AccessLogTemplate string

	// Time settings
	TimeFormat string
	// UILocation is the location on the UI, so that we can display the time on UI.
	DefaultUILocation = time.Local

	CSRFCookieName     = "_csrf"
	CSRFCookieHTTPOnly = true

	ManifestData string

	// API settings
	API = struct {
		EnableSwagger          bool
		SwaggerURL             string
		MaxResponseItems       int
		DefaultPagingNum       int
		DefaultGitTreesPerPage int
		DefaultMaxBlobSize     int64
	}{
		EnableSwagger:          true,
		SwaggerURL:             "",
		MaxResponseItems:       50,
		DefaultPagingNum:       30,
		DefaultGitTreesPerPage: 1000,
		DefaultMaxBlobSize:     10485760,
	}

	OAuth2 = struct {
		Enable                     bool
		AccessTokenExpirationTime  int64
		RefreshTokenExpirationTime int64
		InvalidateRefreshTokens    bool
		JWTSigningAlgorithm        string `ini:"JWT_SIGNING_ALGORITHM"`
		JWTSecretBase64            string `ini:"JWT_SECRET"`
		JWTSigningPrivateKeyFile   string `ini:"JWT_SIGNING_PRIVATE_KEY_FILE"`
		MaxTokenLength             int
	}{
		Enable:                     true,
		AccessTokenExpirationTime:  3600,
		RefreshTokenExpirationTime: 730,
		InvalidateRefreshTokens:    false,
		JWTSigningAlgorithm:        "RS256",
		JWTSigningPrivateKeyFile:   "jwt/private.pem",
		MaxTokenLength:             math.MaxInt16,
	}

	// Metrics settings
	Metrics = struct {
		Enabled                  bool
		Token                    string
		EnabledIssueByLabel      bool
		EnabledIssueByRepository bool
	}{
		Enabled:                  false,
		Token:                    "",
		EnabledIssueByLabel:      false,
		EnabledIssueByRepository: false,
	}

	// I18n settings
	Langs []string
	Names []string

	// Highlight settings are loaded in modules/template/highlight.go

	// Other settings
	ShowFooterBranding         bool
	ShowFooterVersion          bool
	ShowFooterTemplateLoadTime bool
	EnableFeed                 bool

	// Global setting objects
	Cfg           *ini.File
	CustomPath    string // Custom directory path
	CustomConf    string
	PIDFile       = "/run/gitea.pid"
	WritePIDFile  bool
	RunMode       string
	IsProd        bool
	RunUser       string
	IsWindows     bool
	HasRobotsTxt  bool
	EnableSitemap bool
	InternalToken string // internal access token
)

func getAppPath() (string, error) {
	var appPath string
	var err error
	if IsWindows && filepath.IsAbs(os.Args[0]) {
		appPath = filepath.Clean(os.Args[0])
	} else {
		appPath, err = exec.LookPath(os.Args[0])
	}

	if err != nil {
		if !errors.Is(err, exec.ErrDot) {
			return "", err
		}
		appPath, err = filepath.Abs(os.Args[0])
	}
	if err != nil {
		return "", err
	}
	appPath, err = filepath.Abs(appPath)
	if err != nil {
		return "", err
	}
	// Note: we don't use path.Dir here because it does not handle case
	//	which path starts with two "/" in Windows: "//psf/Home/..."
	return strings.ReplaceAll(appPath, "\\", "/"), err
}

func getWorkPath(appPath string) string {
	workPath := AppWorkPath

	if giteaWorkPath, ok := os.LookupEnv("GITEA_WORK_DIR"); ok {
		workPath = giteaWorkPath
	}
	if len(workPath) == 0 {
		i := strings.LastIndex(appPath, "/")
		if i == -1 {
			workPath = appPath
		} else {
			workPath = appPath[:i]
		}
	}
	workPath = strings.ReplaceAll(workPath, "\\", "/")
	if !filepath.IsAbs(workPath) {
		log.Info("Provided work path %s is not absolute - will be made absolute against the current working directory", workPath)

		absPath, err := filepath.Abs(workPath)
		if err != nil {
			log.Error("Unable to absolute %s against the current working directory %v. Will absolute against the AppPath %s", workPath, err, appPath)
			workPath = filepath.Join(appPath, workPath)
		} else {
			workPath = absPath
		}
	}
	return strings.ReplaceAll(workPath, "\\", "/")
}

func init() {
	IsWindows = runtime.GOOS == "windows"
	// We can rely on log.CanColorStdout being set properly because modules/log/console_windows.go comes before modules/setting/setting.go lexicographically
	// By default set this logger at Info - we'll change it later but we need to start with something.
	log.NewLogger(0, "console", "console", fmt.Sprintf(`{"level": "info", "colorize": %t, "stacktraceLevel": "none"}`, log.CanColorStdout))

	var err error
	if AppPath, err = getAppPath(); err != nil {
		log.Fatal("Failed to get app path: %v", err)
	}
	AppWorkPath = getWorkPath(AppPath)
}

func forcePathSeparator(path string) {
	if strings.Contains(path, "\\") {
		log.Fatal("Do not use '\\' or '\\\\' in paths, instead, please use '/' in all places")
	}
}

// IsRunUserMatchCurrentUser returns false if configured run user does not match
// actual user that runs the app. The first return value is the actual user name.
// This check is ignored under Windows since SSH remote login is not the main
// method to login on Windows.
func IsRunUserMatchCurrentUser(runUser string) (string, bool) {
	if IsWindows || SSH.StartBuiltinServer {
		return "", true
	}

	currentUser := user.CurrentUsername()
	return currentUser, runUser == currentUser
}

func createPIDFile(pidPath string) {
	currentPid := os.Getpid()
	if err := os.MkdirAll(filepath.Dir(pidPath), os.ModePerm); err != nil {
		log.Fatal("Failed to create PID folder: %v", err)
	}

	file, err := os.Create(pidPath)
	if err != nil {
		log.Fatal("Failed to create PID file: %v", err)
	}
	defer file.Close()
	if _, err := file.WriteString(strconv.FormatInt(int64(currentPid), 10)); err != nil {
		log.Fatal("Failed to write PID information: %v", err)
	}
}

// SetCustomPathAndConf will set CustomPath and CustomConf with reference to the
// GITEA_CUSTOM environment variable and with provided overrides before stepping
// back to the default
func SetCustomPathAndConf(providedCustom, providedConf, providedWorkPath string) {
	if len(providedWorkPath) != 0 {
		AppWorkPath = filepath.ToSlash(providedWorkPath)
	}
	if giteaCustom, ok := os.LookupEnv("GITEA_CUSTOM"); ok {
		CustomPath = giteaCustom
	}
	if len(providedCustom) != 0 {
		CustomPath = providedCustom
	}
	if len(CustomPath) == 0 {
		CustomPath = path.Join(AppWorkPath, "custom")
	} else if !filepath.IsAbs(CustomPath) {
		CustomPath = path.Join(AppWorkPath, CustomPath)
	}

	if len(providedConf) != 0 {
		CustomConf = providedConf
	}
	if len(CustomConf) == 0 {
		CustomConf = path.Join(CustomPath, "conf/app.ini")
	} else if !filepath.IsAbs(CustomConf) {
		CustomConf = path.Join(CustomPath, CustomConf)
		log.Warn("Using 'custom' directory as relative origin for configuration file: '%s'", CustomConf)
	}
}

// LoadFromExisting initializes setting options from an existing config file (app.ini)
func LoadFromExisting() {
	loadFromConf(false, "")
}

// LoadAllowEmpty initializes setting options, it's also fine that if the config file (app.ini) doesn't exist
func LoadAllowEmpty() {
	loadFromConf(true, "")
}

// LoadForTest initializes setting options for tests
func LoadForTest(extraConfigs ...string) {
	loadFromConf(true, strings.Join(extraConfigs, "\n"))
	if err := PrepareAppDataPath(); err != nil {
		log.Fatal("Can not prepare APP_DATA_PATH: %v", err)
	}
}

func deprecatedSetting(oldSection, oldKey, newSection, newKey string) {
	if Cfg.Section(oldSection).HasKey(oldKey) {
		log.Error("Deprecated fallback `[%s]` `%s` present. Use `[%s]` `%s` instead. This fallback will be removed in v1.19.0", oldSection, oldKey, newSection, newKey)
	}
}

// deprecatedSettingDB add a hint that the configuration has been moved to database but still kept in app.ini
func deprecatedSettingDB(oldSection, oldKey string) {
	if Cfg.Section(oldSection).HasKey(oldKey) {
		log.Error("Deprecated `[%s]` `%s` present which has been copied to database table sys_setting", oldSection, oldKey)
	}
}

// loadFromConf initializes configuration context.
// NOTE: do not print any log except error.
func loadFromConf(allowEmpty bool, extraConfig string) {
	Cfg = ini.Empty()

	if WritePIDFile && len(PIDFile) > 0 {
		createPIDFile(PIDFile)
	}

	isFile, err := util.IsFile(CustomConf)
	if err != nil {
		log.Error("Unable to check if %s is a file. Error: %v", CustomConf, err)
	}
	if isFile {
		if err := Cfg.Append(CustomConf); err != nil {
			log.Fatal("Failed to load custom conf '%s': %v", CustomConf, err)
		}
	} else if !allowEmpty {
		log.Fatal("Unable to find configuration file: %q.\nEnsure you are running in the correct environment or set the correct configuration file with -c.", CustomConf)
	} // else: no config file, a config file might be created at CustomConf later (might not)

	if extraConfig != "" {
		if err = Cfg.Append([]byte(extraConfig)); err != nil {
			log.Fatal("Unable to append more config: %v", err)
		}
	}

	Cfg.NameMapper = ini.SnackCase

	homeDir, err := util.HomeDir()
	if err != nil {
		log.Fatal("Failed to get home directory: %v", err)
	}
	homeDir = strings.ReplaceAll(homeDir, "\\", "/")

	LogLevel = getLogLevel(Cfg.Section("log"), "LEVEL", log.INFO)
	StacktraceLogLevel = getStacktraceLogLevel(Cfg.Section("log"), "STACKTRACE_LEVEL", "None")
	LogRootPath = Cfg.Section("log").Key("ROOT_PATH").MustString(path.Join(AppWorkPath, "log"))
	forcePathSeparator(LogRootPath)

	sec := Cfg.Section("server")
	AppName = Cfg.Section("").Key("APP_NAME").MustString("Gitea: Git with a cup of tea")

	Domain = sec.Key("DOMAIN").MustString("localhost")
	HTTPAddr = sec.Key("HTTP_ADDR").MustString("0.0.0.0")
	HTTPPort = sec.Key("HTTP_PORT").MustString("3000")

	Protocol = HTTP
	protocolCfg := sec.Key("PROTOCOL").String()
	switch protocolCfg {
	case "https":
		Protocol = HTTPS
		// FIXME: DEPRECATED to be removed in v1.18.0
		if sec.HasKey("ENABLE_ACME") {
			EnableAcme = sec.Key("ENABLE_ACME").MustBool(false)
		} else {
			deprecatedSetting("server", "ENABLE_LETSENCRYPT", "server", "ENABLE_ACME")
			EnableAcme = sec.Key("ENABLE_LETSENCRYPT").MustBool(false)
		}
		if EnableAcme {
			AcmeURL = sec.Key("ACME_URL").MustString("")
			AcmeCARoot = sec.Key("ACME_CA_ROOT").MustString("")
			// FIXME: DEPRECATED to be removed in v1.18.0
			if sec.HasKey("ACME_ACCEPTTOS") {
				AcmeTOS = sec.Key("ACME_ACCEPTTOS").MustBool(false)
			} else {
				deprecatedSetting("server", "LETSENCRYPT_ACCEPTTOS", "server", "ACME_ACCEPTTOS")
				AcmeTOS = sec.Key("LETSENCRYPT_ACCEPTTOS").MustBool(false)
			}
			if !AcmeTOS {
				log.Fatal("ACME TOS is not accepted (ACME_ACCEPTTOS).")
			}
			// FIXME: DEPRECATED to be removed in v1.18.0
			if sec.HasKey("ACME_DIRECTORY") {
				AcmeLiveDirectory = sec.Key("ACME_DIRECTORY").MustString("https")
			} else {
				deprecatedSetting("server", "LETSENCRYPT_DIRECTORY", "server", "ACME_DIRECTORY")
				AcmeLiveDirectory = sec.Key("LETSENCRYPT_DIRECTORY").MustString("https")
			}
			// FIXME: DEPRECATED to be removed in v1.18.0
			if sec.HasKey("ACME_EMAIL") {
				AcmeEmail = sec.Key("ACME_EMAIL").MustString("")
			} else {
				deprecatedSetting("server", "LETSENCRYPT_EMAIL", "server", "ACME_EMAIL")
				AcmeEmail = sec.Key("LETSENCRYPT_EMAIL").MustString("")
			}
		} else {
			CertFile = sec.Key("CERT_FILE").String()
			KeyFile = sec.Key("KEY_FILE").String()
			if len(CertFile) > 0 && !filepath.IsAbs(CertFile) {
				CertFile = filepath.Join(CustomPath, CertFile)
			}
			if len(KeyFile) > 0 && !filepath.IsAbs(KeyFile) {
				KeyFile = filepath.Join(CustomPath, KeyFile)
			}
		}
		SSLMinimumVersion = sec.Key("SSL_MIN_VERSION").MustString("")
		SSLMaximumVersion = sec.Key("SSL_MAX_VERSION").MustString("")
		SSLCurvePreferences = sec.Key("SSL_CURVE_PREFERENCES").Strings(",")
		SSLCipherSuites = sec.Key("SSL_CIPHER_SUITES").Strings(",")
	case "fcgi":
		Protocol = FCGI
	case "fcgi+unix", "unix", "http+unix":
		switch protocolCfg {
		case "fcgi+unix":
			Protocol = FCGIUnix
		case "unix":
			log.Warn("unix PROTOCOL value is deprecated, please use http+unix")
			fallthrough
		case "http+unix":
			Protocol = HTTPUnix
		}
		UnixSocketPermissionRaw := sec.Key("UNIX_SOCKET_PERMISSION").MustString("666")
		UnixSocketPermissionParsed, err := strconv.ParseUint(UnixSocketPermissionRaw, 8, 32)
		if err != nil || UnixSocketPermissionParsed > 0o777 {
			log.Fatal("Failed to parse unixSocketPermission: %s", UnixSocketPermissionRaw)
		}

		UnixSocketPermission = uint32(UnixSocketPermissionParsed)
		if !filepath.IsAbs(HTTPAddr) {
			HTTPAddr = filepath.Join(AppWorkPath, HTTPAddr)
		}
	}
	UseProxyProtocol = sec.Key("USE_PROXY_PROTOCOL").MustBool(false)
	ProxyProtocolTLSBridging = sec.Key("PROXY_PROTOCOL_TLS_BRIDGING").MustBool(false)
	ProxyProtocolHeaderTimeout = sec.Key("PROXY_PROTOCOL_HEADER_TIMEOUT").MustDuration(5 * time.Second)
	ProxyProtocolAcceptUnknown = sec.Key("PROXY_PROTOCOL_ACCEPT_UNKNOWN").MustBool(false)
	GracefulRestartable = sec.Key("ALLOW_GRACEFUL_RESTARTS").MustBool(true)
	GracefulHammerTime = sec.Key("GRACEFUL_HAMMER_TIME").MustDuration(60 * time.Second)
	StartupTimeout = sec.Key("STARTUP_TIMEOUT").MustDuration(0 * time.Second)
	PerWriteTimeout = sec.Key("PER_WRITE_TIMEOUT").MustDuration(PerWriteTimeout)
	PerWritePerKbTimeout = sec.Key("PER_WRITE_PER_KB_TIMEOUT").MustDuration(PerWritePerKbTimeout)

	defaultAppURL := string(Protocol) + "://" + Domain + ":" + HTTPPort
	AppURL = sec.Key("ROOT_URL").MustString(defaultAppURL)

	// Check validity of AppURL
	appURL, err := url.Parse(AppURL)
	if err != nil {
		log.Fatal("Invalid ROOT_URL '%s': %s", AppURL, err)
	}
	// Remove default ports from AppURL.
	// (scheme-based URL normalization, RFC 3986 section 6.2.3)
	if (appURL.Scheme == string(HTTP) && appURL.Port() == "80") || (appURL.Scheme == string(HTTPS) && appURL.Port() == "443") {
		appURL.Host = appURL.Hostname()
	}
	// This should be TrimRight to ensure that there is only a single '/' at the end of AppURL.
	AppURL = strings.TrimRight(appURL.String(), "/") + "/"

	// Suburl should start with '/' and end without '/', such as '/{subpath}'.
	// This value is empty if site does not have sub-url.
	AppSubURL = strings.TrimSuffix(appURL.Path, "/")
	StaticURLPrefix = strings.TrimSuffix(sec.Key("STATIC_URL_PREFIX").MustString(AppSubURL), "/")

	// Check if Domain differs from AppURL domain than update it to AppURL's domain
	urlHostname := appURL.Hostname()
	if urlHostname != Domain && net.ParseIP(urlHostname) == nil && urlHostname != "" {
		Domain = urlHostname
	}

	AbsoluteAssetURL = MakeAbsoluteAssetURL(AppURL, StaticURLPrefix)
	AssetVersion = strings.ReplaceAll(AppVer, "+", "~") // make sure the version string is clear (no real escaping is needed)

	manifestBytes := MakeManifestData(AppName, AppURL, AbsoluteAssetURL)
	ManifestData = `application/json;base64,` + base64.StdEncoding.EncodeToString(manifestBytes)

	var defaultLocalURL string
	switch Protocol {
	case HTTPUnix:
		defaultLocalURL = "http://unix/"
	case FCGI:
		defaultLocalURL = AppURL
	case FCGIUnix:
		defaultLocalURL = AppURL
	default:
		defaultLocalURL = string(Protocol) + "://"
		if HTTPAddr == "0.0.0.0" {
			defaultLocalURL += net.JoinHostPort("localhost", HTTPPort) + "/"
		} else {
			defaultLocalURL += net.JoinHostPort(HTTPAddr, HTTPPort) + "/"
		}
	}
	LocalURL = sec.Key("LOCAL_ROOT_URL").MustString(defaultLocalURL)
	LocalURL = strings.TrimRight(LocalURL, "/") + "/"
	LocalUseProxyProtocol = sec.Key("LOCAL_USE_PROXY_PROTOCOL").MustBool(UseProxyProtocol)
	RedirectOtherPort = sec.Key("REDIRECT_OTHER_PORT").MustBool(false)
	PortToRedirect = sec.Key("PORT_TO_REDIRECT").MustString("80")
	RedirectorUseProxyProtocol = sec.Key("REDIRECTOR_USE_PROXY_PROTOCOL").MustBool(UseProxyProtocol)
	OfflineMode = sec.Key("OFFLINE_MODE").MustBool()
	DisableRouterLog = sec.Key("DISABLE_ROUTER_LOG").MustBool()
	if len(StaticRootPath) == 0 {
		StaticRootPath = AppWorkPath
	}
	StaticRootPath = sec.Key("STATIC_ROOT_PATH").MustString(StaticRootPath)
	StaticCacheTime = sec.Key("STATIC_CACHE_TIME").MustDuration(6 * time.Hour)
	AppDataPath = sec.Key("APP_DATA_PATH").MustString(path.Join(AppWorkPath, "data"))
	if !filepath.IsAbs(AppDataPath) {
		log.Info("The provided APP_DATA_PATH: %s is not absolute - it will be made absolute against the work path: %s", AppDataPath, AppWorkPath)
		AppDataPath = filepath.ToSlash(filepath.Join(AppWorkPath, AppDataPath))
	}

	EnableGzip = sec.Key("ENABLE_GZIP").MustBool()
	EnablePprof = sec.Key("ENABLE_PPROF").MustBool(false)
	PprofDataPath = sec.Key("PPROF_DATA_PATH").MustString(path.Join(AppWorkPath, "data/tmp/pprof"))
	if !filepath.IsAbs(PprofDataPath) {
		PprofDataPath = filepath.Join(AppWorkPath, PprofDataPath)
	}

	landingPage := sec.Key("LANDING_PAGE").MustString("home")
	switch landingPage {
	case "explore":
		LandingPageURL = LandingPageExplore
	case "organizations":
		LandingPageURL = LandingPageOrganizations
	case "login":
		LandingPageURL = LandingPageLogin
	case "":
	case "home":
		LandingPageURL = LandingPageHome
	default:
		LandingPageURL = LandingPage(landingPage)
	}

	if len(SSH.Domain) == 0 {
		SSH.Domain = Domain
	}
	SSH.RootPath = path.Join(homeDir, ".ssh")
	serverCiphers := sec.Key("SSH_SERVER_CIPHERS").Strings(",")
	if len(serverCiphers) > 0 {
		SSH.ServerCiphers = serverCiphers
	}
	serverKeyExchanges := sec.Key("SSH_SERVER_KEY_EXCHANGES").Strings(",")
	if len(serverKeyExchanges) > 0 {
		SSH.ServerKeyExchanges = serverKeyExchanges
	}
	serverMACs := sec.Key("SSH_SERVER_MACS").Strings(",")
	if len(serverMACs) > 0 {
		SSH.ServerMACs = serverMACs
	}
	SSH.KeyTestPath = os.TempDir()
	if err = Cfg.Section("server").MapTo(&SSH); err != nil {
		log.Fatal("Failed to map SSH settings: %v", err)
	}
	for i, key := range SSH.ServerHostKeys {
		if !filepath.IsAbs(key) {
			SSH.ServerHostKeys[i] = filepath.Join(AppDataPath, key)
		}
	}

	SSH.KeygenPath = sec.Key("SSH_KEYGEN_PATH").MustString("ssh-keygen")
	SSH.Port = sec.Key("SSH_PORT").MustInt(22)
	SSH.ListenPort = sec.Key("SSH_LISTEN_PORT").MustInt(SSH.Port)
	SSH.UseProxyProtocol = sec.Key("SSH_SERVER_USE_PROXY_PROTOCOL").MustBool(false)

	// When disable SSH, start builtin server value is ignored.
	if SSH.Disabled {
		SSH.StartBuiltinServer = false
	}

	SSH.TrustedUserCAKeysFile = sec.Key("SSH_TRUSTED_USER_CA_KEYS_FILENAME").MustString(filepath.Join(SSH.RootPath, "gitea-trusted-user-ca-keys.pem"))

	for _, caKey := range SSH.TrustedUserCAKeys {
		pubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(caKey))
		if err != nil {
			log.Fatal("Failed to parse TrustedUserCaKeys: %s %v", caKey, err)
		}

		SSH.TrustedUserCAKeysParsed = append(SSH.TrustedUserCAKeysParsed, pubKey)
	}
	if len(SSH.TrustedUserCAKeys) > 0 {
		// Set the default as email,username otherwise we can leave it empty
		sec.Key("SSH_AUTHORIZED_PRINCIPALS_ALLOW").MustString("username,email")
	} else {
		sec.Key("SSH_AUTHORIZED_PRINCIPALS_ALLOW").MustString("off")
	}

	SSH.AuthorizedPrincipalsAllow, SSH.AuthorizedPrincipalsEnabled = parseAuthorizedPrincipalsAllow(sec.Key("SSH_AUTHORIZED_PRINCIPALS_ALLOW").Strings(","))

	SSH.MinimumKeySizeCheck = sec.Key("MINIMUM_KEY_SIZE_CHECK").MustBool(SSH.MinimumKeySizeCheck)
	minimumKeySizes := Cfg.Section("ssh.minimum_key_sizes").Keys()
	for _, key := range minimumKeySizes {
		if key.MustInt() != -1 {
			SSH.MinimumKeySizes[strings.ToLower(key.Name())] = key.MustInt()
		} else {
			delete(SSH.MinimumKeySizes, strings.ToLower(key.Name()))
		}
	}

	SSH.AuthorizedKeysBackup = sec.Key("SSH_AUTHORIZED_KEYS_BACKUP").MustBool(true)
	SSH.CreateAuthorizedKeysFile = sec.Key("SSH_CREATE_AUTHORIZED_KEYS_FILE").MustBool(true)

	SSH.AuthorizedPrincipalsBackup = false
	SSH.CreateAuthorizedPrincipalsFile = false
	if SSH.AuthorizedPrincipalsEnabled {
		SSH.AuthorizedPrincipalsBackup = sec.Key("SSH_AUTHORIZED_PRINCIPALS_BACKUP").MustBool(true)
		SSH.CreateAuthorizedPrincipalsFile = sec.Key("SSH_CREATE_AUTHORIZED_PRINCIPALS_FILE").MustBool(true)
	}

	SSH.ExposeAnonymous = sec.Key("SSH_EXPOSE_ANONYMOUS").MustBool(false)
	SSH.AuthorizedKeysCommandTemplate = sec.Key("SSH_AUTHORIZED_KEYS_COMMAND_TEMPLATE").MustString(SSH.AuthorizedKeysCommandTemplate)

	SSH.AuthorizedKeysCommandTemplateTemplate = template.Must(template.New("").Parse(SSH.AuthorizedKeysCommandTemplate))

	SSH.PerWriteTimeout = sec.Key("SSH_PER_WRITE_TIMEOUT").MustDuration(PerWriteTimeout)
	SSH.PerWritePerKbTimeout = sec.Key("SSH_PER_WRITE_PER_KB_TIMEOUT").MustDuration(PerWritePerKbTimeout)

	if err = Cfg.Section("oauth2").MapTo(&OAuth2); err != nil {
		log.Fatal("Failed to OAuth2 settings: %v", err)
		return
	}

	if !filepath.IsAbs(OAuth2.JWTSigningPrivateKeyFile) {
		OAuth2.JWTSigningPrivateKeyFile = filepath.Join(AppDataPath, OAuth2.JWTSigningPrivateKeyFile)
	}

	sec = Cfg.Section("admin")
	Admin.DefaultEmailNotification = sec.Key("DEFAULT_EMAIL_NOTIFICATIONS").MustString("enabled")

	sec = Cfg.Section("security")
	InstallLock = sec.Key("INSTALL_LOCK").MustBool(false)
	LogInRememberDays = sec.Key("LOGIN_REMEMBER_DAYS").MustInt(7)
	CookieUserName = sec.Key("COOKIE_USERNAME").MustString("gitea_awesome")
	SecretKey = loadSecret(sec, "SECRET_KEY_URI", "SECRET_KEY")
	if SecretKey == "" {
		// FIXME: https://github.com/go-gitea/gitea/issues/16832
		// Until it supports rotating an existing secret key, we shouldn't move users off of the widely used default value
		SecretKey = "!#@FDEWREWR&*(" //nolint:gosec
	}

	CookieRememberName = sec.Key("COOKIE_REMEMBER_NAME").MustString("gitea_incredible")

	ReverseProxyAuthUser = sec.Key("REVERSE_PROXY_AUTHENTICATION_USER").MustString("X-WEBAUTH-USER")
	ReverseProxyAuthEmail = sec.Key("REVERSE_PROXY_AUTHENTICATION_EMAIL").MustString("X-WEBAUTH-EMAIL")
	ReverseProxyAuthFullName = sec.Key("REVERSE_PROXY_AUTHENTICATION_FULL_NAME").MustString("X-WEBAUTH-FULLNAME")

	ReverseProxyLimit = sec.Key("REVERSE_PROXY_LIMIT").MustInt(1)
	ReverseProxyTrustedProxies = sec.Key("REVERSE_PROXY_TRUSTED_PROXIES").Strings(",")
	if len(ReverseProxyTrustedProxies) == 0 {
		ReverseProxyTrustedProxies = []string{"127.0.0.0/8", "::1/128"}
	}

	MinPasswordLength = sec.Key("MIN_PASSWORD_LENGTH").MustInt(6)
	ImportLocalPaths = sec.Key("IMPORT_LOCAL_PATHS").MustBool(false)
	DisableGitHooks = sec.Key("DISABLE_GIT_HOOKS").MustBool(true)
	DisableWebhooks = sec.Key("DISABLE_WEBHOOKS").MustBool(false)
	OnlyAllowPushIfGiteaEnvironmentSet = sec.Key("ONLY_ALLOW_PUSH_IF_GITEA_ENVIRONMENT_SET").MustBool(true)
	PasswordHashAlgo = sec.Key("PASSWORD_HASH_ALGO").MustString("pbkdf2")
	CSRFCookieHTTPOnly = sec.Key("CSRF_COOKIE_HTTP_ONLY").MustBool(true)
	PasswordCheckPwn = sec.Key("PASSWORD_CHECK_PWN").MustBool(false)
	SuccessfulTokensCacheSize = sec.Key("SUCCESSFUL_TOKENS_CACHE_SIZE").MustInt(20)

	InternalToken = loadSecret(sec, "INTERNAL_TOKEN_URI", "INTERNAL_TOKEN")
	if InstallLock && InternalToken == "" {
		// if Gitea has been installed but the InternalToken hasn't been generated (upgrade from an old release), we should generate
		// some users do cluster deployment, they still depend on this auto-generating behavior.
		generateSaveInternalToken()
	}

	cfgdata := sec.Key("PASSWORD_COMPLEXITY").Strings(",")
	if len(cfgdata) == 0 {
		cfgdata = []string{"off"}
	}
	PasswordComplexity = make([]string, 0, len(cfgdata))
	for _, name := range cfgdata {
		name := strings.ToLower(strings.Trim(name, `"`))
		if name != "" {
			PasswordComplexity = append(PasswordComplexity, name)
		}
	}

	newAttachmentService()
	newLFSService()

	timeFormatKey := Cfg.Section("time").Key("FORMAT").MustString("")
	if timeFormatKey != "" {
		TimeFormat = map[string]string{
			"ANSIC":       time.ANSIC,
			"UnixDate":    time.UnixDate,
			"RubyDate":    time.RubyDate,
			"RFC822":      time.RFC822,
			"RFC822Z":     time.RFC822Z,
			"RFC850":      time.RFC850,
			"RFC1123":     time.RFC1123,
			"RFC1123Z":    time.RFC1123Z,
			"RFC3339":     time.RFC3339,
			"RFC3339Nano": time.RFC3339Nano,
			"Kitchen":     time.Kitchen,
			"Stamp":       time.Stamp,
			"StampMilli":  time.StampMilli,
			"StampMicro":  time.StampMicro,
			"StampNano":   time.StampNano,
		}[timeFormatKey]
		// When the TimeFormatKey does not exist in the previous map e.g.'2006-01-02 15:04:05'
		if len(TimeFormat) == 0 {
			TimeFormat = timeFormatKey
			TestTimeFormat, _ := time.Parse(TimeFormat, TimeFormat)
			if TestTimeFormat.Format(time.RFC3339) != "2006-01-02T15:04:05Z" {
				log.Warn("Provided TimeFormat: %s does not create a fully specified date and time.", TimeFormat)
				log.Warn("In order to display dates and times correctly please check your time format has 2006, 01, 02, 15, 04 and 05")
			}
			log.Trace("Custom TimeFormat: %s", TimeFormat)
		}
	}

	zone := Cfg.Section("time").Key("DEFAULT_UI_LOCATION").String()
	if zone != "" {
		DefaultUILocation, err = time.LoadLocation(zone)
		if err != nil {
			log.Fatal("Load time zone failed: %v", err)
		} else {
			log.Info("Default UI Location is %v", zone)
		}
	}
	if DefaultUILocation == nil {
		DefaultUILocation = time.Local
	}

	RunUser = Cfg.Section("").Key("RUN_USER").MustString(user.CurrentUsername())
	// The following is a purposefully undocumented option. Please do not run Gitea as root. It will only cause future headaches.
	// Please don't use root as a bandaid to "fix" something that is broken, instead the broken thing should instead be fixed properly.
	unsafeAllowRunAsRoot := Cfg.Section("").Key("I_AM_BEING_UNSAFE_RUNNING_AS_ROOT").MustBool(false)
	RunMode = os.Getenv("GITEA_RUN_MODE")
	if RunMode == "" {
		RunMode = Cfg.Section("").Key("RUN_MODE").MustString("prod")
	}
	IsProd = strings.EqualFold(RunMode, "prod")
	// Does not check run user when the install lock is off.
	if InstallLock {
		currentUser, match := IsRunUserMatchCurrentUser(RunUser)
		if !match {
			log.Fatal("Expect user '%s' but current user is: %s", RunUser, currentUser)
		}
	}

	// check if we run as root
	if os.Getuid() == 0 {
		if !unsafeAllowRunAsRoot {
			// Special thanks to VLC which inspired the wording of this messaging.
			log.Fatal("Gitea is not supposed to be run as root. Sorry. If you need to use privileged TCP ports please instead use setcap and the `cap_net_bind_service` permission")
		}
		log.Critical("You are running Gitea using the root user, and have purposely chosen to skip built-in protections around this. You have been warned against this.")
	}

	SSH.BuiltinServerUser = Cfg.Section("server").Key("BUILTIN_SSH_SERVER_USER").MustString(RunUser)
	SSH.User = Cfg.Section("server").Key("SSH_USER").MustString(SSH.BuiltinServerUser)

	newRepository()

	newPictureService()

	newPackages()

	newActions()

	if err = Cfg.Section("ui").MapTo(&UI); err != nil {
		log.Fatal("Failed to map UI settings: %v", err)
	} else if err = Cfg.Section("markdown").MapTo(&Markdown); err != nil {
		log.Fatal("Failed to map Markdown settings: %v", err)
	} else if err = Cfg.Section("admin").MapTo(&Admin); err != nil {
		log.Fatal("Fail to map Admin settings: %v", err)
	} else if err = Cfg.Section("api").MapTo(&API); err != nil {
		log.Fatal("Failed to map API settings: %v", err)
	} else if err = Cfg.Section("metrics").MapTo(&Metrics); err != nil {
		log.Fatal("Failed to map Metrics settings: %v", err)
	} else if err = Cfg.Section("camo").MapTo(&Camo); err != nil {
		log.Fatal("Failed to map Camo settings: %v", err)
	}

	if Camo.Enabled {
		if Camo.ServerURL == "" || Camo.HMACKey == "" {
			log.Fatal(`Camo settings require "SERVER_URL" and HMAC_KEY`)
		}
	}

	u := *appURL
	u.Path = path.Join(u.Path, "api", "swagger")
	API.SwaggerURL = u.String()

	newGit()

	newMirror()

	Langs = Cfg.Section("i18n").Key("LANGS").Strings(",")
	if len(Langs) == 0 {
		Langs = defaultI18nLangs()
	}
	Names = Cfg.Section("i18n").Key("NAMES").Strings(",")
	if len(Names) == 0 {
		Names = defaultI18nNames()
	}

	ShowFooterBranding = Cfg.Section("other").Key("SHOW_FOOTER_BRANDING").MustBool(false)
	ShowFooterVersion = Cfg.Section("other").Key("SHOW_FOOTER_VERSION").MustBool(true)
	ShowFooterTemplateLoadTime = Cfg.Section("other").Key("SHOW_FOOTER_TEMPLATE_LOAD_TIME").MustBool(true)
	EnableSitemap = Cfg.Section("other").Key("ENABLE_SITEMAP").MustBool(true)
	EnableFeed = Cfg.Section("other").Key("ENABLE_FEED").MustBool(true)

	UI.ShowUserEmail = Cfg.Section("ui").Key("SHOW_USER_EMAIL").MustBool(true)
	UI.DefaultShowFullName = Cfg.Section("ui").Key("DEFAULT_SHOW_FULL_NAME").MustBool(false)
	UI.SearchRepoDescription = Cfg.Section("ui").Key("SEARCH_REPO_DESCRIPTION").MustBool(true)
	UI.UseServiceWorker = Cfg.Section("ui").Key("USE_SERVICE_WORKER").MustBool(false)

	HasRobotsTxt, err = util.IsFile(path.Join(CustomPath, "robots.txt"))
	if err != nil {
		log.Error("Unable to check if %s is a file. Error: %v", path.Join(CustomPath, "robots.txt"), err)
	}

	newMarkup()

	UI.ReactionsLookup = make(container.Set[string])
	for _, reaction := range UI.Reactions {
		UI.ReactionsLookup.Add(reaction)
	}
	UI.CustomEmojisMap = make(map[string]string)
	for _, emoji := range UI.CustomEmojis {
		UI.CustomEmojisMap[emoji] = ":" + emoji + ":"
	}
}

func parseAuthorizedPrincipalsAllow(values []string) ([]string, bool) {
	anything := false
	email := false
	username := false
	for _, value := range values {
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "off":
			return []string{"off"}, false
		case "email":
			email = true
		case "username":
			username = true
		case "anything":
			anything = true
		}
	}
	if anything {
		return []string{"anything"}, true
	}

	authorizedPrincipalsAllow := []string{}
	if username {
		authorizedPrincipalsAllow = append(authorizedPrincipalsAllow, "username")
	}
	if email {
		authorizedPrincipalsAllow = append(authorizedPrincipalsAllow, "email")
	}

	return authorizedPrincipalsAllow, true
}

// loadSecret load the secret from ini by uriKey or verbatimKey, only one of them could be set
// If the secret is loaded from uriKey (file), the file should be non-empty, to guarantee the behavior stable and clear.
func loadSecret(sec *ini.Section, uriKey, verbatimKey string) string {
	// don't allow setting both URI and verbatim string
	uri := sec.Key(uriKey).String()
	verbatim := sec.Key(verbatimKey).String()
	if uri != "" && verbatim != "" {
		log.Fatal("Cannot specify both %s and %s", uriKey, verbatimKey)
	}

	// if we have no URI, use verbatim
	if uri == "" {
		return verbatim
	}

	tempURI, err := url.Parse(uri)
	if err != nil {
		log.Fatal("Failed to parse %s (%s): %v", uriKey, uri, err)
	}
	switch tempURI.Scheme {
	case "file":
		buf, err := os.ReadFile(tempURI.RequestURI())
		if err != nil {
			log.Fatal("Failed to read %s (%s): %v", uriKey, tempURI.RequestURI(), err)
		}
		val := strings.TrimSpace(string(buf))
		if val == "" {
			// The file shouldn't be empty, otherwise we can not know whether the user has ever set the KEY or KEY_URI
			// For example: if INTERNAL_TOKEN_URI=file:///empty-file,
			// Then if the token is re-generated during installation and saved to INTERNAL_TOKEN
			// Then INTERNAL_TOKEN and INTERNAL_TOKEN_URI both exist, that's a fatal error (they shouldn't)
			log.Fatal("Failed to read %s (%s): the file is empty", uriKey, tempURI.RequestURI())
		}
		return val

	// only file URIs are allowed
	default:
		log.Fatal("Unsupported URI-Scheme %q (INTERNAL_TOKEN_URI = %q)", tempURI.Scheme, uri)
		return ""
	}
}

// generateSaveInternalToken generates and saves the internal token to app.ini
func generateSaveInternalToken() {
	token, err := generate.NewInternalToken()
	if err != nil {
		log.Fatal("Error generate internal token: %v", err)
	}

	InternalToken = token
	CreateOrAppendToCustomConf("security.INTERNAL_TOKEN", func(cfg *ini.File) {
		cfg.Section("security").Key("INTERNAL_TOKEN").SetValue(token)
	})
}

// MakeAbsoluteAssetURL returns the absolute asset url prefix without a trailing slash
func MakeAbsoluteAssetURL(appURL, staticURLPrefix string) string {
	parsedPrefix, err := url.Parse(strings.TrimSuffix(staticURLPrefix, "/"))
	if err != nil {
		log.Fatal("Unable to parse STATIC_URL_PREFIX: %v", err)
	}

	if err == nil && parsedPrefix.Hostname() == "" {
		if staticURLPrefix == "" {
			return strings.TrimSuffix(appURL, "/")
		}

		// StaticURLPrefix is just a path
		return util.URLJoin(appURL, strings.TrimSuffix(staticURLPrefix, "/"))
	}

	return strings.TrimSuffix(staticURLPrefix, "/")
}

// MakeManifestData generates web app manifest JSON
func MakeManifestData(appName, appURL, absoluteAssetURL string) []byte {
	type manifestIcon struct {
		Src   string `json:"src"`
		Type  string `json:"type"`
		Sizes string `json:"sizes"`
	}

	type manifestJSON struct {
		Name      string         `json:"name"`
		ShortName string         `json:"short_name"`
		StartURL  string         `json:"start_url"`
		Icons     []manifestIcon `json:"icons"`
	}

	bytes, err := json.Marshal(&manifestJSON{
		Name:      appName,
		ShortName: appName,
		StartURL:  appURL,
		Icons: []manifestIcon{
			{
				Src:   absoluteAssetURL + "/assets/img/logo.png",
				Type:  "image/png",
				Sizes: "512x512",
			},
			{
				Src:   absoluteAssetURL + "/assets/img/logo.svg",
				Type:  "image/svg+xml",
				Sizes: "512x512",
			},
		},
	})
	if err != nil {
		log.Error("unable to marshal manifest JSON. Error: %v", err)
		return make([]byte, 0)
	}

	return bytes
}

// CreateOrAppendToCustomConf creates or updates the custom config.
// Use the callback to set individual values.
func CreateOrAppendToCustomConf(purpose string, callback func(cfg *ini.File)) {
	if CustomConf == "" {
		log.Error("Custom config path must not be empty")
		return
	}

	cfg := ini.Empty()
	isFile, err := util.IsFile(CustomConf)
	if err != nil {
		log.Error("Unable to check if %s is a file. Error: %v", CustomConf, err)
	}
	if isFile {
		if err := cfg.Append(CustomConf); err != nil {
			log.Error("failed to load custom conf %s: %v", CustomConf, err)
			return
		}
	}

	callback(cfg)

	if err := os.MkdirAll(filepath.Dir(CustomConf), os.ModePerm); err != nil {
		log.Fatal("failed to create '%s': %v", CustomConf, err)
		return
	}
	if err := cfg.SaveTo(CustomConf); err != nil {
		log.Fatal("error saving to custom config: %v", err)
	}
	log.Info("Settings for %s saved to: %q", purpose, CustomConf)

	// Change permissions to be more restrictive
	fi, err := os.Stat(CustomConf)
	if err != nil {
		log.Error("Failed to determine current conf file permissions: %v", err)
		return
	}

	if fi.Mode().Perm() > 0o600 {
		if err = os.Chmod(CustomConf, 0o600); err != nil {
			log.Warn("Failed changing conf file permissions to -rw-------. Consider changing them manually.")
		}
	}
}

// NewServices initializes the services
func NewServices() {
	InitDBConfig()
	newService()
	newOAuth2Client()
	NewLogServices(false)
	newCacheService()
	newSessionService()
	newCORSService()
	parseMailerConfig(Cfg)
	newIncomingEmail()
	newRegisterMailService()
	newNotifyMailService()
	newProxyService()
	newWebhookService()
	newMigrationsService()
	newIndexerService()
	newTaskService()
	NewQueueService()
	newProject()
	newMimeTypeMap()
	newFederationService()
}

// NewServicesForInstall initializes the services for install
func NewServicesForInstall() {
	newService()
	parseMailerConfig(Cfg)
}
