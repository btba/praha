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

type NumRidersOption struct {
	Index   int
	Display int
}

// CheckoutData is the data passed to the template.
type CheckoutData struct {
	TourDetail           *TourDetail
	NumRidersOptions     []*NumRidersOption
	ExpiryYearOptions    []int
	StripePublishableKey template.JSStr
	Warnings             map[warning]bool
	GoogleTrackingID     string
}

type CheckoutErrorData struct {
	Error            string
	GoogleTrackingID string
}

func (s *Server) checkout(r *http.Request) (*CheckoutData, map[warning]bool, *appError) {
	warnings := make(map[warning]bool)
	if err := r.ParseForm(); err != nil {
		return nil, warnings, &appError{http.StatusBadRequest, "Error parsing form", err}
	}
	var vars CheckoutVars
	if err := s.decoder.Decode(&vars, r.Form); err != nil {
		return nil, warnings, &appError{http.StatusBadRequest, "Error decoding form values", err}
	}
	if vars.TourID <= 0 { // also handles missing TourID in vars
		return nil, warnings, &appError{http.StatusBadRequest, "Please return to the previous page and select a date. Thank you.", nil}
	}

	tourDetail, ok, err := s.store.GetTourDetailByID(vars.TourID, maxRiders)
	if err != nil {
		return nil, warnings, &appError{http.StatusInternalServerError, "Server error", fmt.Errorf("GetTourDetailByID: %v", err)}
	}
	if !ok {
		return nil, warnings, &appError{http.StatusBadRequest, fmt.Sprintf("Invalid tour ID %d", vars.TourID), nil}
	}
	if tourDetail.NumSpotsRemaining == 0 {
		return nil, warnings, &appError{http.StatusNotFound, fmt.Sprintf("Tour %d has no availability", vars.TourID), nil}
	}

	if tourDetail.Time.Before(time.Now()) {
		warnings[WarningTourPast] = true
	}
	if tourDetail.Full {
		warnings[WarningTourFull] = true
	}
	if tourDetail.Cancelled {
		warnings[WarningTourCancelled] = true
	}
	if tourDetail.Deleted {
		warnings[WarningTourDeleted] = true
	}

	// There's no for-loop in templates, so we construct a list like
	// []*NumRidersOption{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}} for
	// the user to select the number of riders.
	var numRidersOptions []*NumRidersOption
	for i := 0; i < tourDetail.NumSpotsRemaining; i++ {
		numRidersOptions = append(numRidersOptions, &NumRidersOption{i, i + 1})
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
		Warnings:             warnings,
		GoogleTrackingID:     s.googleTrackingID,
	}
	return data, warnings, nil
}

func (s *Server) HandleCheckout(w http.ResponseWriter, r *http.Request) (code int, warnings map[warning]bool, summary string) {
	data, warnings, e := s.checkout(r)
	if e != nil {
		if e.Error != nil {
			s.log.Printf("%s: %v", e.Message, e.Error)
		}
		tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "checkout_error.html"))
		if err != nil {
			s.log.Printf("%v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return http.StatusInternalServerError, warnings, "Error parsing checkout error template"
		}
		w.WriteHeader(e.Code)
		if err := tmpl.Execute(w, &CheckoutErrorData{e.Message, s.googleTrackingID}); err != nil {
			s.log.Printf("%v", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return http.StatusInternalServerError, warnings, "Error executing checkout error template"
		}
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
