package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/schema"
)

var (
	port                 = flag.Int("port", 8080, "port to run web server on")
	bookingsDSN          = flag.String("bookings_dsn", "", "data source name for bookings database")
	sendgridKey          = flag.String("sendgrid_key", "", "SendGrid API key")
	stripeSecretKey      = flag.String("stripe_secret_key", "", "Stripe key used by server")
	stripePublishableKey = flag.String("stripe_publishable_key", "", "Stripe key to embed in Javascript")
	staticDir            = flag.String("static_dir", "", "if provided, directory for static files")
	templatesDir         = flag.String("templates_dir", "templates", "directory containing templates")
	requestLog           = flag.String("request_log", "", "file for request logs (empty means stdout)")
	debugLog             = flag.String("debug_log", "", "file for debug logs (empty means stdout)")
)

const (
	maxRiders = 14
)

type Server struct {
	store                Store
	sendgridKey          string
	stripeSecretKey      string
	stripePublishableKey string
	templatesDir         string
	decoder              *schema.Decoder
	log                  *log.Logger
}

func NewServer(dsn, sendgridKey, stripeSecretKey, stripePublishableKey, templatesDir string, log *log.Logger) (*Server, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Server{
		store:                &RemoteStore{db},
		sendgridKey:          sendgridKey,
		stripeSecretKey:      stripeSecretKey,
		stripePublishableKey: stripePublishableKey,
		templatesDir:         templatesDir,
		decoder:              schema.NewDecoder(),
		log:                  log,
	}, nil
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

	debugLogWriter := os.Stdout
	if *debugLog != "" {
		var err error
		debugLogWriter, err = os.OpenFile(*debugLog, os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			log.Fatal(err)
		}
	}

	server, err := NewServer(*bookingsDSN, *sendgridKey, *stripeSecretKey, *stripePublishableKey, *templatesDir, log.New(debugLogWriter, "", log.LstdFlags))
	if err != nil {
		log.Fatal(err)
	}

	rand.Seed(time.Now().UnixNano())
	m := http.NewServeMux()
	m.HandleFunc("/checkout", server.HandleCheckout)
	m.HandleFunc("/checkout/confirmation", server.HandleConfirmation)
	if *staticDir != "" {
		m.Handle("/", http.FileServer(http.Dir(*staticDir)))
	}
	http.ListenAndServe(fmt.Sprintf(":%d", *port), handlers.CombinedLoggingHandler(requestLogWriter, m))
}
