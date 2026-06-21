package main

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed dashboard
var dashboardFiles embed.FS

func getDashboardFS() (fs.FS, error) {
	return fs.Sub(dashboardFiles, "dashboard")
}

var Version = "dev"

type APIStyle int

const (
	APIStyleOpenAI APIStyle = iota
	APIStyleAnthropic
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       string
	TLS      bool
	StartTLS bool
}

type App struct {
	config     Config
	keys       *KeyManager
	client     *http.Client
	sender     *SMTPNotifier
	requestLog *RequestLog
	configMu   sync.Mutex
}

func newApp(cfg Config) *App {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 2 * time.Second,
	}
	a := &App{
		config: cfg,
		keys:   NewKeyManager(KeyEntriesToKeys(cfg.UpstreamAPIKeys), KeyEntriesToNames(cfg.UpstreamAPIKeys)),
		client: &http.Client{Transport: tr},
		sender: NewSMTPNotifier(SMTPConfig{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort,
			Username: cfg.SMTPUsername, Password: cfg.SMTPPassword,
			From: cfg.SMTPFrom, To: cfg.SMTPTo,
			TLS: cfg.SMTPTLS, StartTLS: cfg.SMTPStartTLS,
		}),
	}
	if cfg.RequestLogSize > 0 {
		a.requestLog = NewRequestLog(cfg.RequestLogSize)
	}
	return a
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setupMux(a).ServeHTTP(w, r)
}

type SMTPNotifier struct{ cfg SMTPConfig }

func NewSMTPNotifier(cfg SMTPConfig) *SMTPNotifier { return &SMTPNotifier{cfg: cfg} }
func (n *SMTPNotifier) NotifySwitch(idx int, st StatusResponse) {
	go n.send("Switchboard Go switched upstream key", fmt.Sprintf("Switched away from key %d\n\n%+v", idx, st))
}
func (n *SMTPNotifier) NotifyAllExhausted(st StatusResponse) {
	go n.send("Switchboard Go exhausted all upstream keys", fmt.Sprintf("All keys exhausted\n\n%+v", st))
}
func (n *SMTPNotifier) send(subject, body string) {
	if n.cfg.Host == "" || n.cfg.To == "" || n.cfg.From == "" {
		return
	}
	addr := net.JoinHostPort(n.cfg.Host, strconv.Itoa(n.cfg.Port))
	auth := smtp.Auth(nil)
	if strings.TrimSpace(n.cfg.Username) != "" {
		auth = smtp.PlainAuth("", n.cfg.Username, n.cfg.Password, n.cfg.Host)
	}
	msg := []byte("To: " + n.cfg.To + "\r\nSubject: " + subject + "\r\n\r\n" + body + "\r\n")
	if err := sendMail(addr, auth, n.cfg.From, []string{n.cfg.To}, msg, n.cfg.TLS, n.cfg.StartTLS); err != nil {
		log.Printf("smtp notification failed: %v", err)
	}
}

var sendMail = func(addr string, auth smtp.Auth, from string, to []string, msg []byte, useTLS, useStartTLS bool) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	host, _, _ := net.SplitHostPort(addr)
	if useTLS {
		conn = tls.Client(conn, &tls.Config{ServerName: host})
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Quit()
	if useStartTLS {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return err
			}
		}
	}
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "validate-config" {
		cfg, err := LoadConfig()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("config valid: listen=%s upstream=%s keys=%d",
			cfg.ListenAddr, cfg.UpstreamBaseURL, len(cfg.UpstreamAPIKeys))
		return
	}

	cfg, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	a := newApp(cfg)
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: a, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 65 * time.Second, WriteTimeout: 0, IdleTimeout: 120 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()

	log.Printf("startup listen_addr=%s upstream_base_url=%s upstream_keys=%d request_log=%d",
		cfg.ListenAddr, cfg.UpstreamBaseURL, len(cfg.UpstreamAPIKeys),
		cfg.RequestLogSize)
	if len(cfg.UpstreamAPIKeys) == 0 {
		log.Print("no upstream API keys configured — add them via the dashboard at /dashboard/")
	}

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
