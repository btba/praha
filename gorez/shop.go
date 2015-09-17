package main

import (
	"html/template"
	"net/http"
	"path"
)

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
	tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "shop.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, toursByCode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
