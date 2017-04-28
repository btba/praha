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
	Warn                 bool
}

func (s *Server) checkout(r *http.Request) (data *CheckoutData, warnings []string, appErr *appError) {
	if err := r.ParseForm(); err != nil {
		return nil, warnings, &appError{http.StatusBadRequest, "Error parsing form", err}
	}
	var vars CheckoutVars
	if err := s.decoder.Decode(&vars, r.Form); err != nil {
		return nil, warnings, &appError{http.StatusBadRequest, "Error decoding form values", err}
	}

	tourDetail, ok, err := s.store.GetTourDetailByID(vars.TourID, maxRiders)
	if err != nil {
		return nil, warnings, &appError{http.StatusInternalServerError, "Server error", fmt.Errorf("GetTourDetailByID: %v", err)}
		return
	}
	if !ok {
		return nil, warnings, &appError{http.StatusBadRequest, fmt.Sprintf("Invalid tour ID %d", vars.TourID), nil}
	}
	if tourDetail.NumSpotsRemaining == 0 {
		return nil, warnings, &appError{http.StatusNotFound, fmt.Sprintf("Tour %d has no availability", vars.TourID), nil}
	}

	if tourDetail.Time.Before(time.Now()) {
		warnings = append(warnings, tourDetail.Time.Format("past:2006/01/02"))
	}
	if tourDetail.Full {
		warnings = append(warnings, "full")
	}
	if tourDetail.Cancelled {
		warnings = append(warnings, "cancelled")
	}
	if tourDetail.Deleted {
		warnings = append(warnings, "deleted")
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
	data = &CheckoutData{
		TourDetail:           tourDetail,
		NumRidersOptions:     numRidersOptions,
		ExpiryYearOptions:    expiryYearOptions,
		StripePublishableKey: template.JSStr(s.stripePublishableKey),
		Warn:                 len(warnings) > 0,
	}
	return data, warnings, nil
}

func (s *Server) HandleCheckout(w http.ResponseWriter, r *http.Request) (code int, warnings []string, summary string) {
	data, warnings, e := s.checkout(r)
	if e != nil {
		s.log.Printf("%v", e.Error)
		http.Error(w, e.Message, e.Code)
		return e.Code, warnings, e.Message
	}
	tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "checkout.html"))
	if err != nil {
		s.log.Printf("%v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return http.StatusInternalServerError, warnings, "Error parsing checkout template"
	}
	if err := tmpl.Execute(w, data); err != nil {
		s.log.Printf("%v", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return http.StatusInternalServerError, warnings, "Error executing checkout template"
	}
	return http.StatusOK, warnings, fmt.Sprintf("tour:%d", data.TourDetail.ID)
}
