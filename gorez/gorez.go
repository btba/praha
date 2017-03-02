package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/schema"
)

var (
	port                 = flag.Int("port", 8080, "port to run web server on")
	bookingsDSN          = flag.String("bookings_dsn", "", "data source name for bookings database")
	sendgridKey          = flag.String("sendgrid_key", "", "SendGrid API key")
	stripeSecretKey      = flag.String("stripe_secret_key", "", "Stripe key used by server")
	stripePublishableKey = flag.String("stripe_publishable_key", "", "Stripe key to embed in Javascript")
	templatesDir         = flag.String("templates_dir", "templates", "directory containing templates")
)

type Server struct {
	store                Store
	sendgridKey          string
	stripeSecretKey      string
	stripePublishableKey string
	templatesDir         string
	decoder              *schema.Decoder
}

func NewServer(dsn, sendgridKey, stripeSecretKey, stripePublishableKey, templatesDir string) (*Server, error) {
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
	}, nil
}

func main() {
	flag.Parse()
	server, err := NewServer(*bookingsDSN, *sendgridKey, *stripeSecretKey, *stripePublishableKey, *templatesDir)
	if err != nil {
		log.Fatal(err)
	}
	rand.Seed(time.Now().UnixNano())
	http.HandleFunc("/reservations/shop", server.HandleShop)
	http.HandleFunc("/reservations/shoppost", server.HandleShopPost)
	http.HandleFunc("/reservations/cart", server.HandleCart)
	http.HandleFunc("/reservations/api/cartitems/", server.HandleApiCartItems)
	http.HandleFunc("/reservations/checkout", server.HandleCheckout)
	http.HandleFunc("/reservations/confirmation", server.HandleConfirmation)
	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
