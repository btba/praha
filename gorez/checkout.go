package main

import (
	"fmt"
	"html/template"
	"net/http"
	"path"
	"time"
)

const (
	futureYears = 10 // present 2017..2026 as options for credit card expiration year
)

// CheckoutVars represents the form inputs.
type CheckoutVars struct {
	TourID int32 `schema:"TourId"`
}

// CheckoutData is the data passed to the template.
type CheckoutData struct {
	TourDetail           *TourDetail
	NumRidersOptions     []int
	ExpiryYearOptions    []int
	StripePublishableKey template.JSStr
	WarnOnLoad           bool
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

	tourDetail, ok, err := s.store.GetTourDetailByID(vars.TourID, maxRiders)
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
	for i := 1; i <= tourDetail.NumSpotsRemaining; i++ {
		numRidersOptions = append(numRidersOptions, i)
	}
	// Same with number of years: []int{2017, 2018, ..., 2026}
	thisYear := time.Now().Year()
	var expiryYearOptions []int
	for y := thisYear; y < thisYear+futureYears; y++ {
		expiryYearOptions = append(expiryYearOptions, y)
	}
	data := &CheckoutData{
		TourDetail:           tourDetail,
		NumRidersOptions:     numRidersOptions,
		ExpiryYearOptions:    expiryYearOptions,
		StripePublishableKey: template.JSStr(s.stripePublishableKey),
		WarnOnLoad:           tourDetail.Time.Before(time.Now()) || tourDetail.Full || tourDetail.Cancelled || tourDetail.Deleted,
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
