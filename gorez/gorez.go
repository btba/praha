package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/gorilla/schema"
)

var (
	port         = flag.Int("port", 8080, "port to run web server on")
	bookingsDSN  = flag.String("bookings_dsn", "", "data source name for bookings database")
	templatesDir = flag.String("templates_dir", "templates", "directory containing templates")

	decoder = schema.NewDecoder()
)

const (
	cartIDCookieName = "BTBACartID"
)

func newCartID() int {
	// NB: Very small possibility of collision, so you would get an
	// existing cart.  It's ok.
	// TODO: Use an int64.
	return rand.Int() & 0xFFFFFFFF
}

func findTourInCart(items []*CartItem, tourID int) (*CartItem, bool) {
	for _, item := range items {
		if item.TourID == tourID {
			return item, true
		}
	}
	return nil, false
}

func readCartID(r *http.Request) (int, bool) {
	cookie, err := r.Cookie(cartIDCookieName)
	if err != nil {
		return 0, false
	}
	cartID, err := strconv.Atoi(cookie.Value)
	if err != nil {
		return 0, false
	}
	return cartID, true
}

func writeCartID(w http.ResponseWriter, cartID int) {
	http.SetCookie(w, &http.Cookie{
		Name:  cartIDCookieName,
		Value: strconv.Itoa(cartID),
		Path:  "/",
	})
}

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
	// GET /reservations/shop
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

type ShopPostVars struct {
	TourID int
}

func (s *Server) HandleShopPost(w http.ResponseWriter, r *http.Request) {
	// POST /reservations/shoppost TourID=562
	if r.Method != "POST" {
		http.Error(w, "Method must be POST", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var vars ShopPostVars
	if err := decoder.Decode(&vars, r.PostForm); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Read cartID from cookie.
	cartID, ok := readCartID(r)

	// If cookie, look up any times in this cart.
	var items []*CartItem
	if ok {
		var err error
		items, err = s.store.ListCartItems(cartID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// If no cookie or no items, create new cartID.
	if len(items) == 0 {
		cartID = newCartID()
		writeCartID(w, cartID)
	}

	// TODO: Check that tour has spots available.
	if item, ok := findTourInCart(items, vars.TourID); ok {
		if err := s.store.UpdateCartItem(cartID, item.ID, item.Quantity+1); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		if err := s.store.AddCartItem(cartID, vars.TourID, 1); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/reservations/cart", http.StatusSeeOther)
}

func (s *Server) HandleCart(w http.ResponseWriter, r *http.Request) {
	// GET /reservations/cart
	if r.Method != "GET" {
		http.Error(w, "Method must be GET", http.StatusBadRequest)
		return
	}

	cartID, ok := readCartID(r)
	if !ok {
		http.Error(w, "No cart", http.StatusBadRequest)
		return
	}
	items, err := s.store.ListCartItems(cartID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl, err := template.ParseFiles(path.Join(*templatesDir, "cart.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, items); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) HandleApiCartItems(w http.ResponseWriter, r *http.Request) {
	// PUT    /reservations/api/cartitems/<itemID> <quantity>
	// DELETE /reservations/api/cartitems/<itemID>
	itemID, err := strconv.Atoi(r.URL.Path[len("/reservations/api/cartitems/"):])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cartID, ok := readCartID(r)
	if !ok {
		http.Error(w, "No cart", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case "PUT":
		var quantity int
		if err := json.NewDecoder(r.Body).Decode(&quantity); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.UpdateCartItem(cartID, itemID, quantity); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "DELETE":
		if err := s.store.DeleteCartItem(cartID, itemID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "Method must be PUT or DELETE", http.StatusBadRequest)
		return
	}
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
