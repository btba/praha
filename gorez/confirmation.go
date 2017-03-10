package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/charge"
)

// ConfirmationVars represents the form inputs.
type ConfirmationVars struct {
	TourID       int32
	NumRiders    int
	RiderHeights []int
	QuotedTotal  float64

	Name        string
	StripeToken string

	Email  string
	Mobile string
	Hotel  string
	Misc   string
}

func (s *Server) HandleConfirmation(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method must be POST", http.StatusMethodNotAllowed)
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

	// Validate that we have non-zero RiderHeights for all NumRiders.
	var heights []int
	if len(vars.RiderHeights) < vars.NumRiders {
		http.Error(w, fmt.Sprintf("Only provided %d heights for %d riders", len(vars.RiderHeights), vars.NumRiders), http.StatusBadRequest)
		return
	}
	for _, h := range vars.RiderHeights[:vars.NumRiders] {
		if h == 0 {
			http.Error(w, "Must select heights for all riders", http.StatusBadRequest)
			return
		}
		heights = append(heights, h)
	}

	// Trim strings and validate email.
	var (
		name   = strings.TrimSpace(vars.Name)
		email  = strings.TrimSpace(vars.Email)
		mobile = strings.TrimSpace(vars.Mobile)
		hotel  = strings.TrimSpace(vars.Hotel)
		misc   = strings.TrimSpace(vars.Misc)
	)
	if email == "" {
		http.Error(w, "Must enter email address", http.StatusBadRequest)
		return
	}

	// Look up requested tour.
	tourDetail, ok, err := s.store.GetTourDetailByID(vars.TourID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, fmt.Sprintf("Invalid tour ID %d", vars.TourID), http.StatusBadRequest)
		return
	}

	// Compute totals in cents [float64(13.57) -> uint64(1357)] and validate.
	actualTotal := uint64(float64(vars.NumRiders)*tourDetail.Price*100 + 0.5)
	quotedTotal := uint64(vars.QuotedTotal*100 + 0.5)
	if actualTotal != quotedTotal {
		http.Error(w, fmt.Sprintf("Internal pricing error: actual=%d, quoted=%d", actualTotal, quotedTotal), http.StatusInternalServerError)
		return
	}

	// Add order to database.
	orderID, err := s.store.CreateOrder(vars.TourID, heights, actualTotal, name, email, mobile, hotel, misc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Charge to Stripe.
	stripe.Key = s.stripeSecretKey
	_, err = charge.New(&stripe.ChargeParams{
		Amount:   actualTotal,
		Currency: "USD",
		Source:   &stripe.SourceParams{Token: vars.StripeToken},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// At this point, checkout has succeeded.  Everything below is
	// optional.

	// Update order in database to record payment.
	if err := s.store.UpdateOrderPaymentRecorded(orderID); err != nil {
		log.Print(err)
	}

	// Email the customer.
	if err := s.emailCustomer(name, email); err != nil {
		log.Print(err)
	} else {
		// Update order in database to record confirmation email.
		if err := s.store.UpdateOrderConfirmationSent(orderID); err != nil {
			log.Print(err)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("https://www.bikethebigapple.com/thankyou.php?OrderId=%d", orderID), http.StatusSeeOther)
}

func (s *Server) emailCustomer(name, email string) error {
	from := mail.NewEmail("Bike the Big Apple Reservations", "reservations@bikethebigapple.com")
	subject := "Bike the Big Apple confirmation"
	to := mail.NewEmail(name, email)
	content := mail.NewContent("text/plain", "Testing testing 123")
	m := mail.NewV3MailInit(from, subject, to, content)

	request := sendgrid.GetRequest(s.sendgridKey, "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = mail.GetRequestBody(m)
	_, err := sendgrid.API(request)
	return err
}
