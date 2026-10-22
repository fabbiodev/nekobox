package internal

import (
	"context"
	"fmt"
	"io"
	"log"
	"nekobox_core/internal/boxbox"
	"nekobox_core/internal/process"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"net"

	"github.com/sagernet/sing-box/common/settings"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/metadata"
)

const (
	Gb = 1000 * Mb
	Mb = 1000 * Kb
	Kb = 1000
)

var ruleset_cachedir string

var instanceMu sync.RWMutex
var boxInstance *boxbox.Box
var instanceCancel context.CancelFunc

func CurrentInstance() (*boxbox.Box, context.CancelFunc) {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return boxInstance, instanceCancel
}
func CurrentBox() *boxbox.Box { instance, _ := CurrentInstance(); return instance }
func SetInstance(instance *boxbox.Box, cancel context.CancelFunc) {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	boxInstance, instanceCancel = instance, cancel
}
func ClearInstance(instance *boxbox.Box) {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	if boxInstance == instance {
		boxInstance, instanceCancel = nil, nil
	}
}

var ExtraProcess *process.Process
var NeedUnsetDNS bool

var SystemProxyController settings.SystemProxy
var SystemProxyAddr string
var SystemProxyPort uint16
var SystemProxySupportSOCKS bool
var systemProxyMu sync.Mutex

var Debug bool

func IsSystemProxyEnabled() bool {
	systemProxyMu.Lock()
	defer systemProxyMu.Unlock()
	return isSystemProxyEnabled()
}

func isSystemProxyEnabled() bool {
	return SystemProxyController != nil && SystemProxyController.IsEnabled()
}

func ResetSystemProxy() error {
	systemProxyMu.Lock()
	defer systemProxyMu.Unlock()
	return resetSystemProxy()
}

func resetSystemProxy() error {
	if isSystemProxyEnabled() {
		return SystemProxyController.Disable()
	}
	return nil
}

func SetSystemProxy(ctx context.Context, serverAddr string, serverPort uint16, supportSOCKS bool) error {
	systemProxyMu.Lock()
	defer systemProxyMu.Unlock()
	if CurrentBox() == nil {
		return nil
	}
	if serverAddr == SystemProxyAddr && serverPort == SystemProxyPort && supportSOCKS == SystemProxySupportSOCKS {
		if isSystemProxyEnabled() {
			return nil
		}
	} else {
		if err := resetSystemProxy(); err != nil {
			return err
		}
	}
	addr := metadata.ParseSocksaddrHostPort(serverAddr, serverPort)
	controller, err := settings.NewSystemProxy(ctx, addr, supportSOCKS)
	if err != nil {
		return err
	}
	if controller == nil {
		return fmt.Errorf("system proxy controller is unavailable")
	}
	SystemProxyAddr = serverAddr
	SystemProxyPort = serverPort
	SystemProxySupportSOCKS = supportSOCKS
	SystemProxyController = controller
	return SystemProxyController.Enable()
}

func SetRulesetCachedir(v string) bool {
	ruleset_cachedir = v
	return true
}

func BoxCreateHttpClient(instance *boxbox.Box) *http.Client {
	if instance == nil {
		return &http.Client{Timeout: 15 * time.Second}
	}
	outbound := instance.Outbound().Default()
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				return outbound.DialContext(ctx, "tcp", metadata.ParseSocksaddr(addr))
			},
		},
	}
	return client
}

func DownloadFile(originalURL, targetPath string, use_default_outbound bool) error {
	// Create all necessary directories for the target path
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	// Never leave a truncated cache entry after a failed download.
	outFile, err := os.CreateTemp(dir, ".download-*")
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer func() {
		outFile.Close()
		os.Remove(outFile.Name())
	}()
	// Download the file
	var client *http.Client

	if use_default_outbound {
		goto def_cli
	} else {
		instance := CurrentBox()
		if instance == nil {
			goto def_cli
		} else {
			client = BoxCreateHttpClient(instance)
			goto skip_def_cli
		}
	}
def_cli:

	client = &http.Client{Timeout: 15 * time.Second}
skip_def_cli:

	parsedURL, err := url.Parse(originalURL)
	if err != nil {
		return fmt.Errorf("error parsing URL: %v", err)
	}

	// Extract credentials (if available)
	var username, password string
	if parsedURL.User != nil {
		username = parsedURL.User.Username()
		password, _ = parsedURL.User.Password()
		// Remove the credentials from the URL (we don't need them in the URL anymore)
		parsedURL.User = nil
	}

	// Rebuild the cleaned URL without credentials
	cleanedURL := parsedURL.String()

	// Create a new GET request with the cleaned URL
	req, err := http.NewRequest("GET", cleanedURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// If credentials were found, set Basic Authentication header
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	// Copy the downloaded content to the file
	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %v", err)
	}
	if err := outFile.Sync(); err != nil {
		return err
	}
	if err := outFile.Close(); err != nil {
		return err
	}
	return os.Rename(outFile.Name(), targetPath)
}

func urlToPath(url string) string {
	url = strings.Replace(url, ":/", "/", 1)
	url = strings.ReplaceAll(url, "_", "_0_")
	url = strings.ReplaceAll(url, ":", "_1_")
	url = strings.ReplaceAll(url, "@", "_2_")
	url = strings.ReplaceAll(url, "?", "_3_")
	url = strings.ReplaceAll(url, "=", "_4_")
	url = strings.ReplaceAll(url, "&", "_5_")
	url = strings.ReplaceAll(url, "\"", "_6_")
	url = strings.ReplaceAll(url, "'", "_7_")
	url = strings.ReplaceAll(url, "*", "_8_")
	url = filepath.Clean(filepath.FromSlash(url))
	url = filepath.Clean(filepath.Join(ruleset_cachedir, url))
	return url
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func CacheHttpBool(url string, use_default_outbound bool, s *bool) string {
	if url == "" {
		if s != nil {
			*s = false
		}
		return ""
	}
	path := urlToPath(url)
	if !fileExists(path) {
		log.Printf("Downloading %s %s", url, func() string {
			if use_default_outbound {
				return "with proxy"
			}
			return "without proxy"
		}())
		err := DownloadFile(url, path, use_default_outbound)
		if err != nil {
			log.Printf("Error while downloading ruleset: %s", err.Error())
			if s != nil {
				*s = false
			}
			// The missing file is reported by configuration validation, not os.Exit.
			return path
		}
		if s != nil {
			*s = true
		}
	} else if s != nil {
		log.Printf("Cached %s", url)
		*s = false
	}
	return path
}

func CacheHttp(url string, use_default_outbound bool) string {
	return CacheHttpBool(url, use_default_outbound, nil)
}

func cacheRuleSet(url string, format string, tag string) option.RuleSet {
	var ruleset option.RuleSet
	ruleset.Tag = tag
	ruleset.Type = C.RuleSetTypeLocal
	ruleset.Format = format
	ruleset.LocalOptions.Path = CacheHttp(url, false)
	return ruleset
}

func ClearRulesets() {
	paths := []string{
		"ftps",
		"ftp",
		"http",
		"https",
	}

	for _, path := range paths {
		os.RemoveAll(filepath.Clean(filepath.Join(ruleset_cachedir, path)))
	}
}

func ModifyRulesets(opt *option.Options) {
	if ruleset_cachedir == "" {
		return
	}
	if opt.Route != nil {
		for u, i := range opt.Route.RuleSet {
			if i.Type == C.RuleSetTypeRemote {
				url := i.RemoteOptions.URL
				opt.Route.RuleSet[u] = cacheRuleSet(url, i.Format, i.Tag)
			}
		}
	}
}

func GetRulesetCachedir() string {
	return ruleset_cachedir
}

func BrateToStr(brate float64) string {
	brate *= 8
	if brate >= Gb {
		return fmt.Sprintf("%.2f%s", brate/Gb, "Gbps")
	}
	if brate >= Mb {
		return fmt.Sprintf("%.2f%s", brate/Mb, "Mbps")
	}
	return fmt.Sprintf("%.2f%s", brate/Kb, "Kbps")
}

func CalculateBRate(bytes float64, startTime time.Time) float64 {
	elapsed := time.Since(startTime).Seconds()
	return bytes / elapsed
}
