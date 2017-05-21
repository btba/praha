package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/schema"
)

var (
	port                  = flag.Int("port", 8080, "port to run web server on")
	bookingsDSN           = flag.String("bookings_dsn", "", "data source name for bookings database")
	sendgridKey           = flag.String("sendgrid_key", "", "SendGrid API key")
	stripeSecretKey       = flag.String("stripe_secret_key", "", "Stripe key used by server")
	stripePublishableKey  = flag.String("stripe_publishable_key", "", "Stripe key to embed in Javascript")
	staticDir             = flag.String("static_dir", "", "if provided, directory for static files")
	templatesDir          = flag.String("templates_dir", "templates", "directory containing templates")
	emailTemplatesDir     = flag.String("email_templates_dir", "", "directory containing email templates")
	requestLog            = flag.String("request_log", "", "file for request logs (empty means stdout)")
	debugLog              = flag.String("debug_log", "", "file for debug logs (empty means stdout)")
	googleTrackingID      = flag.String("google_tracking_id", "", "Google Analytics tracking ID")
	googleConversionID    = flag.Int("google_conversion_id", 0, "Google AdWords conversion ID")
	googleConversionLabel = flag.String("google_conversion_label", "", "Google AdWords conversion label")
)

const (
	maxRiders = 14
)

type Server struct {
	store                 Store
	sendgridKey           string
	stripeSecretKey       string
	stripePublishableKey  string
	templatesDir          string
	emailTemplatesDir     string
	googleTrackingID      string
	googleConversionID    int
	googleConversionLabel string
	decoder               *schema.Decoder
	log                   *log.Logger
}

func NewServer(dsn, sendgridKey, stripeSecretKey, stripePublishableKey, templatesDir, emailTemplatesDir, googleTrackingID string, googleConversionID int, googleConversionLabel string, log *log.Logger) (*Server, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Server{
		store:                 &RemoteStore{db},
		sendgridKey:           sendgridKey,
		stripeSecretKey:       stripeSecretKey,
		stripePublishableKey:  stripePublishableKey,
		templatesDir:          templatesDir,
		emailTemplatesDir:     emailTemplatesDir,
		googleTrackingID:      googleTrackingID,
		googleConversionID:    googleConversionID,
		googleConversionLabel: googleConversionLabel,
		decoder:               schema.NewDecoder(),
		log:                   log,
	}, nil
}

// See also https://blog.golang.org/error-handling-and-go, although
// this code does something slightly different.
type appError struct {
	Code    int
	Message string // for client
	Error   error  // for server debug logs
}

type logHandler struct {
	log    *log.Logger
	handle func(http.ResponseWriter, *http.Request) (code int, warnings []string, summary string)
}

func (h *logHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	code, warnings, summary := h.handle(w, r)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	h.log.Printf("%s %s %s %s code:%d warnings:%v %s\n", host, r.Method, r.URL.Path, r.Form.Encode(), code, warnings, summary)
}

func main() {
	flag.Parse()

	requestLogWriter := os.Stdout
	if *requestLog != "" {
		var err error
		requestLogWriter, err = os.OpenFile(*requestLog, os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}
	requestLog := log.New(requestLogWriter, "", log.LstdFlags)

	debugLogWriter := os.Stdout
	if *debugLog != "" {
		var err error
		debugLogWriter, err = os.OpenFile(*debugLog, os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}
	debugLog := log.New(debugLogWriter, "", log.LstdFlags|log.Lshortfile)

	server, err := NewServer(*bookingsDSN, *sendgridKey, *stripeSecretKey, *stripePublishableKey, *templatesDir, *emailTemplatesDir, *googleTrackingID, *googleConversionID, *googleConversionLabel, debugLog)
	if err != nil {
		log.Fatal(err)
	}

	rand.Seed(time.Now().UnixNano())
	m := http.NewServeMux()
	m.Handle("/checkout", &logHandler{requestLog, server.HandleCheckout})
	m.Handle("/checkout/confirmation", &logHandler{requestLog, server.HandleConfirmation})
	if *staticDir != "" {
		m.Handle("/", http.FileServer(http.Dir(*staticDir)))
	}
	http.ListenAndServe(fmt.Sprintf(":%d", *port), m)
}
