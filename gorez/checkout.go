package main

import (
	"fmt"
	"html/template"
	"net/http"
	"path"
)

// CheckoutVars represents the form inputs.
type CheckoutVars struct {
	Items []*TourQuantity
}

type TourQuantity struct {
	TourID   int32
	Quantity int32
}

// CheckoutData is the data passed to the template.
type CheckoutData struct {
	Items     []*CheckoutItem
	StripeKey template.JSStr
}

func (c *CheckoutData) Total() float64 { // TODO: int32
	total := 0.0
	for _, item := range c.Items {
		total += item.Amount()
	}
	return total
}

type CheckoutItem struct {
	Quantity   int32
	TourDetail *TourDetail
}

func (c *CheckoutItem) Amount() float64 { // TODO: int32
	return float64(c.Quantity) * c.TourDetail.Price
}

func (s *Server) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	// POST /reservations/checkout
	if r.Method != "POST" {
		http.Error(w, "Method must be POST", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var vars CheckoutVars
	if err := s.decoder.Decode(&vars, r.PostForm); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var tourIDs []int32
	for _, item := range vars.Items {
		tourIDs = append(tourIDs, item.TourID)
	}
	tourDetails, err := s.store.GetTourDetailsByID(tourIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var checkoutItems []*CheckoutItem
	for _, item := range vars.Items {
		tourDetail, ok := tourDetails[item.TourID]
		if !ok {
			http.Error(w, fmt.Sprintf("Invalid tour ID: %d", item.TourID), http.StatusBadRequest)
			return
		}
		checkoutItems = append(checkoutItems, &CheckoutItem{
			Quantity:   item.Quantity,
			TourDetail: tourDetail,
		})
	}

	data := &CheckoutData{
		Items:     checkoutItems,
		StripeKey: template.JSStr(s.stripeKey),
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
