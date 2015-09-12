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
	port         = flag.Int("port", 8080, "port to run web server on")
	bookingsDSN  = flag.String("bookings_dsn", "", "data source name for bookings database")
	templatesDir = flag.String("templates_dir", "templates", "directory containing templates")

	decoder = schema.NewDecoder()
)

type Server struct {
	store Store
}

func NewServer(dsn string) (*Server, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Server{store: &RemoteStore{db}}, nil
}

func main() {
	flag.Parse()
	server, err := NewServer(*bookingsDSN)
	if err != nil {
		log.Fatal(err)
	}
	rand.Seed(time.Now().UnixNano())
	http.HandleFunc("/reservations/shop", server.HandleShop)
	http.HandleFunc("/reservations/shoppost", server.HandleShopPost)
	http.HandleFunc("/reservations/cart", server.HandleCart)
	http.HandleFunc("/reservations/api/cartitems/", server.HandleApiCartItems)
	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
