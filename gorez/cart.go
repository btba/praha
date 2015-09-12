package main

import (
	"html/template"
	"net/http"
	"path"
)

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
