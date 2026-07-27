package tls_client

import (
	"net/url"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/cookiejar"
)

type CookieJarOption func(config *cookieJarConfig)

type cookieJarConfig struct {
	logger            Logger
	skipExisting      bool
	debug             bool
	debugLogs         bool
	allowEmptyCookies bool
}

func WithSkipExisting() CookieJarOption {
	return func(config *cookieJarConfig) {
		config.skipExisting = true
	}
}

func WithAllowEmptyCookies() CookieJarOption {
	return func(config *cookieJarConfig) {
		config.allowEmptyCookies = true
	}
}

func WithDebugLogger() CookieJarOption {
	return func(config *cookieJarConfig) {
		config.debug = true
	}
}

func WithLogger(logger Logger) CookieJarOption {
	return func(config *cookieJarConfig) {
		config.logger = logger
	}
}

type CookieJar interface {
	http.CookieJar
	GetAllCookies() map[string][]*http.Cookie
}

type cookieJar struct {
	jar        *cookiejar.Jar
	config     *cookieJarConfig
	allCookies map[string][]*http.Cookie
	sync.RWMutex
}

func NewCookieJar(options ...CookieJarOption) CookieJar {
	realJar, _ := cookiejar.New(nil)

	config := &cookieJarConfig{}

	for _, opt := range options {
		opt(config)
	}

	if config.logger == nil {
		config.logger = NewNoopLogger()
	}

	if config.debug {
		config.logger = NewDebugLogger(config.logger)
	}
	config.debugLogs = loggerDebugEnabled(config.logger)

	c := &cookieJar{
		jar:        realJar,
		config:     config,
		allCookies: make(map[string][]*http.Cookie),
	}

	return c
}

func (jar *cookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if u == nil {
		return
	}
	jar.Lock()
	defer jar.Unlock()

	clonedCookies := cloneCookies(cookies)
	for _, cookie := range clonedCookies {
		if cookie.Path == "" || cookie.Path[0] != '/' {
			cookie.Path = defaultCookiePath(u.Path)
		}
	}
	uniqueCookies := jar.unique(jar.nonEmpty(clonedCookies))
	hostKey := jar.buildCookieHostKey(u)
	existingCookies := jar.allCookies[hostKey]

	if jar.config.skipExisting {
		existing := make(map[string]struct{}, len(existingCookies))
		for _, cookie := range existingCookies {
			existing[cookieIdentity(cookie)] = struct{}{}
		}

		filtered := uniqueCookies[:0]
		now := time.Now()
		for _, cookie := range uniqueCookies {
			identity := cookieIdentity(cookie)
			if _, found := existing[identity]; found && !isCookieDeletion(cookie, now) {
				if jar.config.debugLogs {
					jar.config.logger.Debug("cookie %s is already in jar, skipping", cookie.Name)
				}
				continue
			}
			filtered = append(filtered, cookie)
		}
		uniqueCookies = filtered
	}

	jar.jar.SetCookies(u, uniqueCookies)
	jar.allCookies[hostKey] = mergeCookieSnapshots(existingCookies, uniqueCookies)
}

func (jar *cookieJar) Cookies(u *url.URL) []*http.Cookie {
	if u == nil {
		return nil
	}
	return jar.notExpired(cloneCookies(jar.jar.Cookies(u)))
}

func (jar *cookieJar) GetAllCookies() map[string][]*http.Cookie {
	jar.RLock()
	defer jar.RUnlock()

	copied := make(map[string][]*http.Cookie, len(jar.allCookies))
	for u, c := range jar.allCookies {
		copied[u] = jar.notExpired(cloneCookies(c))
	}

	return copied
}

func (jar *cookieJar) buildCookieHostKey(u *url.URL) string {
	if u == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
}

func (jar *cookieJar) unique(cookies []*http.Cookie) []*http.Cookie {
	filteredCookies := make([]*http.Cookie, 0, len(cookies))
	seen := make(map[string]struct{}, len(cookies))

	for i := len(cookies) - 1; i >= 0; i-- {
		c := cookies[i]
		identity := cookieIdentity(c)
		if _, found := seen[identity]; found {
			continue
		}

		filteredCookies = append(filteredCookies, c)
		seen[identity] = struct{}{}
	}

	for left, right := 0, len(filteredCookies)-1; left < right; left, right = left+1, right-1 {
		filteredCookies[left], filteredCookies[right] = filteredCookies[right], filteredCookies[left]
	}

	return filteredCookies
}

func (jar *cookieJar) nonEmpty(cookies []*http.Cookie) []*http.Cookie {
	if jar.config.allowEmptyCookies {
		return cookies
	}

	var filteredCookies []*http.Cookie
	now := time.Now()

	for _, c := range cookies {
		if c.Value == "" && !isCookieDeletion(c, now) {
			if jar.config.debugLogs {
				jar.config.logger.Debug("cookie %s is empty and will be filtered out", c.Name)
			}
			continue
		}

		filteredCookies = append(filteredCookies, c)
	}

	return filteredCookies
}

func (jar *cookieJar) notExpired(cookies []*http.Cookie) []*http.Cookie {
	filteredCookies := make([]*http.Cookie, 0, len(cookies))
	now := time.Now()

	for _, c := range cookies {
		if isCookieDeletion(c, now) {
			if jar.config.debugLogs {
				jar.config.logger.Debug("cookie %s is expired and will be excluded", c.Name)
			}
			continue
		}

		filteredCookies = append(filteredCookies, c)
	}

	return filteredCookies
}

func mergeCookieSnapshots(existing, updates []*http.Cookie) []*http.Cookie {
	merged := make(map[string]*http.Cookie, len(existing)+len(updates))
	order := make([]string, 0, len(existing)+len(updates))
	for _, cookie := range existing {
		identity := cookieIdentity(cookie)
		if _, found := merged[identity]; !found {
			order = append(order, identity)
		}
		merged[identity] = cookie
	}

	now := time.Now()
	for _, cookie := range updates {
		identity := cookieIdentity(cookie)
		if isCookieDeletion(cookie, now) {
			delete(merged, identity)
			continue
		}
		if _, found := merged[identity]; !found {
			order = append(order, identity)
		}
		merged[identity] = cookie
	}

	result := make([]*http.Cookie, 0, len(merged))
	for _, identity := range order {
		if cookie := merged[identity]; cookie != nil {
			result = append(result, cookie)
		}
	}
	return result
}

func cookieIdentity(cookie *http.Cookie) string {
	if cookie == nil {
		return "<nil>"
	}
	return cookie.Name + "\x00" + strings.ToLower(cookie.Domain) + "\x00" + cookie.Path
}

func defaultCookiePath(requestPath string) string {
	if requestPath == "" || requestPath[0] != '/' {
		return "/"
	}
	lastSlash := strings.LastIndexByte(requestPath, '/')
	if lastSlash <= 0 {
		return "/"
	}
	return requestPath[:lastSlash]
}

func isCookieDeletion(cookie *http.Cookie, now time.Time) bool {
	if cookie == nil || cookie.MaxAge < 0 {
		return true
	}
	return !cookie.Expires.IsZero() && !cookie.Expires.After(now)
}

func cloneCookies(cookies []*http.Cookie) []*http.Cookie {
	cloned := make([]*http.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil {
			continue
		}
		copy := *cookie
		copy.Unparsed = append([]string(nil), cookie.Unparsed...)
		cloned = append(cloned, &copy)
	}
	return cloned
}
