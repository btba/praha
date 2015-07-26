package main

import (
	"database/sql"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
)

var (
	port         = flag.Int("port", 8080, "port to run web server on")
	bookingsDSN  = flag.String("bookings_dsn", "", "data source name for bookings database")
	templatesDir = flag.String("templates_dir", "templates", "directory containing templates")
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

func (s *Server) HandleShop(w http.ResponseWriter, r *http.Request) {
	// GET /shop
	if r.Method != "GET" {
		http.Error(w, "Method must be GET", http.StatusBadRequest)
		return
	}

	// TODO: Display "(only 3 spots left)" annotations.
	toursByCode, err := s.store.ListOpenToursByCode()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl, err := template.ParseFiles(path.Join(*templatesDir, "shop.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, toursByCode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func main() {
	flag.Parse()
	server, err := NewServer(*bookingsDSN)
	if err != nil {
		log.Fatal(err)
	}
	http.HandleFunc("/shop", server.HandleShop)
	http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
}
