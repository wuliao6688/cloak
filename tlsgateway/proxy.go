// Package tlsgateway provides a local HTTP/HTTPS proxy that applies TLS
// ClientHello fingerprints to outbound connections. Any HTTP client in
// any language can use it by setting HTTP_PROXY.
//
// # Architecture
//
//	HTTP  request: client → proxy (plain) → upstream (TLS via tlsgateway)
//	HTTPS CONNECT: client → proxy (tunnel) → upstream (TLS via tlsgateway)
//
// The CONNECT tunnel preserves end-to-end TLS between the client and
// upstream while still applying the ClientHello fingerprint on the
// proxy-upstream leg. This means the upstream server sees the desired
// JA3 fingerprint.
//
// # Usage (Go library)
//
//	p := tlsgateway.NewProxy(":8080", profiles.Chrome_150)
//	go p.ListenAndServe()
//
// # Usage (CLI)
//
//	go run ./cmd/tlsgateway-proxy -addr :8080 -profile chrome_150
//
//	# Any language:
//	HTTPS_PROXY=http://localhost:8080 curl https://example.com
//	HTTPS_PROXY=http://localhost:8080 python3 -c "import requests; requests.get('https://example.com')"
package tlsgateway

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/bogdanfinn/tls-client/profiles"
)

// Proxy is a local HTTP forward proxy that applies TLS ClientHello
// fingerprints to outbound connections.
type Proxy struct {
	addr    string
	server  *http.Server
	profile profiles.ClientProfile
	mu      sync.RWMutex

	// Transport used for upstream requests.
	transport *Transport

	// OnReload is called after a successful profile reload (optional).
	OnReload func(key string)

	// Logger (defaults to log.Default).
	Logger *log.Logger
}

// NewProxy creates a new local forward proxy.
func NewProxy(addr string, profile profiles.ClientProfile) *Proxy {
	return &Proxy{
		addr:    addr,
		profile: profile,
		Logger:  log.Default(),
	}
}

// ListenAndServe starts the proxy and blocks until the server stops.
func (p *Proxy) ListenAndServe() error {
	p.rebuildTransport()

	p.server = &http.Server{
		Addr:         p.addr,
		Handler:      http.HandlerFunc(p.serve),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	p.Logger.Printf("tlsgateway proxy listening on %s (profile: %s)",
		p.addr, p.profile.GetClientHelloStr())
	return p.server.ListenAndServe()
}

// Shutdown gracefully stops the proxy.
func (p *Proxy) Shutdown(ctx context.Context) error {
	if p.server != nil {
		return p.server.Shutdown(ctx)
	}
	return nil
}

// serve dispatches incoming requests to the appropriate handler.
func (p *Proxy) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/health" && r.Method == http.MethodGet:
		p.handleHealth(w, r)
	case r.URL.Path == "/reload" && r.Method == http.MethodPost:
		p.handleReload(w, r)
	default:
		p.handleRequest(w, r)
	}
}

// SetProfile updates the TLS profile for subsequent connections.
// Existing connections keep their original profile.
func (p *Proxy) SetProfile(profile profiles.ClientProfile) {
	p.mu.Lock()
	p.profile = profile
	p.mu.Unlock()
	p.rebuildTransport()

	if p.OnReload != nil {
		p.OnReload(profile.GetClientHelloStr())
	}
}

func (p *Proxy) rebuildTransport() {
	p.mu.RLock()
	profile := p.profile
	p.mu.RUnlock()

	tr := NewTransport(profile)
	// H2 transport's TLSClientConfig already has InsecureSkipVerify=true.
	tr.h2.TLSClientConfig.InsecureSkipVerify = true

	p.mu.Lock()
	old := p.transport
	p.transport = tr
	p.mu.Unlock()

	if old != nil {
		old.CloseIdleConnections()
	}
}

// handleRequest processes all proxy requests. It distinguishes between
// CONNECT (HTTPS tunnel) and regular HTTP proxy requests.
func (p *Proxy) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

// handleHTTP forwards a plain HTTP proxy request through the tlsgateway Transport.
func (p *Proxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// Build the upstream request. The proxy receives the full URL.
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	// Copy headers (remove hop-by-hop).
	copyHeaders(outReq.Header, r.Header)
	outReq.Header.Del("Proxy-Connection")
	outReq.Header.Del("Proxy-Authorization")

	p.mu.RLock()
	tr := p.transport
	p.mu.RUnlock()

	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	resp, err := client.Do(outReq)
	if err != nil {
		p.Logger.Printf("proxy error: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleConnect establishes a CONNECT tunnel for HTTPS traffic.
// The proxy creates a TLS connection to the upstream using the
// fingerprint profile, then blindly relays bytes between client and upstream.
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	// Connect to the upstream.
	hostPort := r.URL.Host
	if hostPort == "" {
		hostPort = r.Host
	}
	if _, _, err := net.SplitHostPort(hostPort); err != nil {
		hostPort = net.JoinHostPort(hostPort, "443")
	}

	p.mu.RLock()
	tr := p.transport
	p.mu.RUnlock()

	// Dial upstream using our fingerprint Transport.
	upstream, err := tr.DialTLS(r.Context(), "tcp", hostPort)
	if err != nil {
		p.Logger.Printf("CONNECT dial %s: %v", hostPort, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	// Hijack the client connection.
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	// Send 200 to the client.
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Bidirectional copy.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(upstream, clientConn)
	}()

	go func() {
		defer wg.Done()
		io.Copy(clientConn, upstream)
		// Signal upstream to close.
		if tcpConn, ok := upstream.(*net.TCPConn); ok {
			tcpConn.CloseWrite()
		}
	}()

	wg.Wait()
}

// handleHealth returns a simple health check response.
func (p *Proxy) handleHealth(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	profile := p.profile
	p.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","profile":"%s"}`+"\n", profile.GetClientHelloStr())
}

// handleReload triggers a profile reload from a JSON file.
// POST /reload?path=/path/to/profiles.json&profile=chrome_150
func (p *Proxy) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jsonPath := r.URL.Query().Get("path")
	profileKey := r.URL.Query().Get("profile")

	if jsonPath != "" {
		file, err := profiles.LoadProfilesFromJSONFile(jsonPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("load profiles: %v", err), http.StatusBadRequest)
			return
		}
		if err := profiles.MergeJSONProfilesIntoRegistry(file); err != nil {
			http.Error(w, fmt.Sprintf("merge profiles: %v", err), http.StatusInternalServerError)
			return
		}
		p.Logger.Printf("reloaded %d profiles from %s", len(file.Profiles), jsonPath)
	}

	if profileKey != "" {
		profile, err := profiles.ResolveClientProfileStrict(profileKey)
		if err != nil {
			http.Error(w, fmt.Sprintf("resolve profile: %v", err), http.StatusBadRequest)
			return
		}
		p.SetProfile(profile)
	}

	p.mu.RLock()
	current := p.profile.GetClientHelloStr()
	p.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"reloaded","profile":"%s"}`+"\n", current)
}

func copyHeaders(dst, src http.Header) {
	for k, vs := range src {
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}
