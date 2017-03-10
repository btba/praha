package main

import (
	"fmt"
	"html/template"
	"net/http"
	"path"
)

const (
	maxRiders = 12
)

// CheckoutVars represents the form inputs.
type CheckoutVars struct {
	TourID int32 `schema:"TourId"`
}

// CheckoutData is the data passed to the template.
type CheckoutData struct {
	TourDetail           *TourDetail
	NumRidersOptions     []int
	StripePublishableKey template.JSStr
}

func (s *Server) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var vars CheckoutVars
	if err := s.decoder.Decode(&vars, r.Form); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tourDetail, ok, err := s.store.GetTourDetailByID(vars.TourID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("Invalid tour ID %d", vars.TourID), http.StatusBadRequest)
		return
	}

	// There's no for-loop in templates, so we construct a list like
	// []int{1, 2, 3, 4, 5} for the user to select the number of riders.
	var numRidersOptions []int
	for i := 1; i <= maxRiders; i++ {
		numRidersOptions = append(numRidersOptions, i)
	}
	data := &CheckoutData{
		TourDetail:           tourDetail,
		NumRidersOptions:     numRidersOptions,
		StripePublishableKey: template.JSStr(s.stripePublishableKey),
	}
	tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "checkout.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
