package main

import (
	"html/template"
	"net/http"
	"path"
)

// ConfirmationVars represents the form inputs.
type ConfirmationVars struct {
	Name        string
	Email       string
	StripeToken string
}

func (s *Server) HandleConfirmation(w http.ResponseWriter, r *http.Request) {
	// POST /reservations/confirmation
	if r.Method != "POST" {
		http.Error(w, "Method must be POST", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var vars ConfirmationVars
	if err := s.decoder.Decode(&vars, r.PostForm); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// TODO: Validate name & email.

	tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "confirmation.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, vars); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
