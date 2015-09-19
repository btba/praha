package main

import (
	"fmt"
	"html/template"
	"net/http"
	"path"

	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/charge"
)

// ConfirmationVars represents the form inputs.
type ConfirmationVars struct {
	Items       []*OrderItem
	Name        string
	Email       string
	Hotel       string
	Mobile      string
	StripeToken string
}

// ConfirmationData is the data passed to the template.
type ConfirmationData struct {
	Charge        *stripe.Charge
	DisplayAmount string
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

	// Look up requested tours.
	var tourIDs []int32
	for _, item := range vars.Items {
		tourIDs = append(tourIDs, item.TourID)
	}
	tourDetails, err := s.store.GetTourDetailsByID(tourIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Compute total.
	var total float64 // TODO: uint32
	for _, item := range vars.Items {
		tourDetail, ok := tourDetails[item.TourID]
		if !ok {
			http.Error(w, fmt.Sprintf("Invalid tour ID: %d", item.TourID), http.StatusBadRequest)
			return
		}
		total += float64(item.Quantity) * tourDetail.Price
	}
	// Poor man's rounding computation.  Converts float64(13.57) to uint64(1357).
	stripeAmount := uint64(total*100 + 0.1)

	// Add order to database.
	orderID, err := s.store.CreateOrder(vars.Name, vars.Email, vars.Hotel, vars.Mobile, vars.Items)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Charge to Stripe.
	stripe.Key = s.stripeSecretKey
	ch, err := charge.New(&stripe.ChargeParams{
		Amount:   stripeAmount,
		Currency: "USD",
		Source:   &stripe.SourceParams{Token: vars.StripeToken},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update order in database to record payment.
	if err := s.store.UpdateOrderPaymentRecorded(orderID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := &ConfirmationData{
		Charge:        ch,
		DisplayAmount: fmt.Sprintf("%d.%02d", ch.Amount/100, ch.Amount%100),
	}
	tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "confirmation.html"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
