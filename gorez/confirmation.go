package main

import (
	"fmt"
	"html/template"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/charge"
)

// ConfirmationVars represents the form inputs.
type ConfirmationVars struct {
	TourID       int32
	NumRiders    int
	RiderGenders []string
	RiderHeights []int
	QuotedTotal  float64

	Name        string
	StripeToken string

	Email  string
	Mobile string
	Hotel  string
	Misc   string
}

// ConfirmationData is the data passed to the template.
type ConfirmationData struct {
	TourDetail   *TourDetail
	NumRiders    int
	DisplayTotal string

	Name   string
	Email  string
	Mobile string
	Hotel  string
	Misc   string

	Warn bool
}

func (s *Server) confirm(r *http.Request) (data *ConfirmationData, warnings []string, appErr *appError) {
	if r.Method != "POST" {
		return nil, warnings, &appError{http.StatusMethodNotAllowed, "Method must be POST", nil}
	}
	if err := r.ParseForm(); err != nil {
		return nil, warnings, &appError{http.StatusBadRequest, "Error parsing form", err}
	}
	var vars ConfirmationVars
	if err := s.decoder.Decode(&vars, r.PostForm); err != nil {
		return nil, warnings, &appError{http.StatusBadRequest, "Error decoding form values", err}
	}

	// Look up requested tour.
	tourDetail, ok, err := s.store.GetTourDetailByID(vars.TourID, maxRiders)
	if err != nil {
		return nil, warnings, &appError{http.StatusInternalServerError, "Server error", fmt.Errorf("GetTourDetailByID: %v", err)}
	}
	if !ok {
		return nil, warnings, &appError{http.StatusBadRequest, fmt.Sprintf("Invalid tour ID %d", vars.TourID), nil}
	}

	// NOTE: These checks are racy, but the conditions are unlikely.
	// - Tour deleted by admin
	// - Tour fields changed by admin (price, time, full, cancelled, deleted)
	// - Tour number of riders changed (due to concurrent purchase)
	// We could do the reads/checks/write in a transaction, retrying
	// when the commit fails due to a change in the number of riders.
	//
	// Compute totals in cents [float64(13.57) -> uint64(1357)] and validate.
	actualTotal := uint64(float64(vars.NumRiders)*tourDetail.Price*100 + 0.5)
	quotedTotal := uint64(vars.QuotedTotal*100 + 0.5)
	if actualTotal != quotedTotal {
		return nil, warnings, &appError{http.StatusBadRequest, "Pricing error", fmt.Errorf("quoted=%d, actual=%d", quotedTotal, actualTotal)}
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
	if vars.NumRiders > tourDetail.NumSpotsRemaining {
		warnings = append(warnings, fmt.Sprintf("riders(%d)>spots(%d)", vars.NumRiders, tourDetail.NumSpotsRemaining))
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
		return nil, warnings, &appError{http.StatusBadRequest, "Must enter email address", nil}
	}

	// Validate that we have RiderGenders & RiderHeights for all NumRiders.
	var genders []string
	var heights []int
	if tourDetail.HeightsNeeded {
		if len(vars.RiderGenders) < vars.NumRiders {
			return nil, warnings, &appError{http.StatusBadRequest, fmt.Sprintf("Only provided %d genders for %d riders", len(vars.RiderGenders), vars.NumRiders), nil}
		}
		for _, g := range vars.RiderGenders[:vars.NumRiders] {
			if g == "" {
				return nil, warnings, &appError{http.StatusBadRequest, "Must select genders for all riders", nil}
			}
			genders = append(genders, g)
		}
		if len(vars.RiderHeights) < vars.NumRiders {
			return nil, warnings, &appError{http.StatusBadRequest, fmt.Sprintf("Only provided %d heights for %d riders", len(vars.RiderHeights), vars.NumRiders), nil}
		}
		for _, h := range vars.RiderHeights[:vars.NumRiders] {
			if h == 0 {
				return nil, warnings, &appError{http.StatusBadRequest, "Must select heights for all riders", nil}
			}
			heights = append(heights, h)
		}
	}

	// Add order to database.
	orderID, err := s.store.CreateOrder(vars.TourID, vars.NumRiders, genders, heights, actualTotal, name, email, mobile, hotel, misc)
	if err != nil {
		return nil, warnings, &appError{http.StatusInternalServerError, "Server error", fmt.Errorf("CreateOrder: %v", err)}
	}

	// Charge to Stripe.
	stripe.Key = s.stripeSecretKey
	_, err = charge.New(&stripe.ChargeParams{
		Amount:   actualTotal,
		Currency: "USD",
		Source:   &stripe.SourceParams{Token: vars.StripeToken},
	})
	if err != nil {
		return nil, warnings, &appError{http.StatusInternalServerError, "Server error", fmt.Errorf("charge.New: %v", err)}
	}

	// At this point, checkout has succeeded.  Everything below is
	// optional.

	// Update order in database to record payment.
	if err := s.store.UpdateOrderPaymentRecorded(orderID); err != nil {
		s.log.Printf("UpdateOrderPaymentRecorded: %v", err)
		warnings = append(warnings, "updateorder:paymentrecorded")
	}

	// Email the customer.
	if tourDetail.AutoConfirm && len(warnings) == 0 {
		if err := s.emailCustomer(name, email); err != nil {
			s.log.Printf("emailCustomer: %v", err)
			warnings = append(warnings, "emailcustomer")
		} else {
			// Update order in database to record confirmation email.
			if err := s.store.UpdateOrderConfirmationSent(orderID); err != nil {
				s.log.Printf("UpdateOrderConfirmationSent: %v", err)
				warnings = append(warnings, "updateorder:confirmationsent")
			}
		}
	}

	data = &ConfirmationData{
		TourDetail:   tourDetail,
		NumRiders:    vars.NumRiders,
		DisplayTotal: fmt.Sprintf("$%d.%02d", actualTotal/100, actualTotal%100),
		Name:         name,
		Email:        email,
		Mobile:       mobile,
		Hotel:        hotel,
		Misc:         misc,
		Warn:         len(warnings) > 0,
	}
	return data, warnings, nil
}

func (s *Server) HandleConfirmation(w http.ResponseWriter, r *http.Request) (code int, warnings []string, summary string) {
	data, warnings, e := s.confirm(r)
	if e != nil {
		s.log.Printf("%s: %v", e.Message, e.Error)
		http.Error(w, e.Message, e.Code)
		return e.Code, warnings, e.Message
	}
	summary = fmt.Sprintf("tour:%d riders:%d %s '%s' <%s>", data.TourDetail.ID, data.NumRiders, data.DisplayTotal, data.Name, data.Email)
	tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "confirmation.html"))
	if err != nil {
		s.log.Printf("%v", err)
		fmt.Fprint(w, "Reservation accepted") // fallback message
		return http.StatusOK, append(warnings, "parsetemplate"), summary
	}
	if err := tmpl.Execute(w, data); err != nil {
		s.log.Printf("%v", err)
		fmt.Fprint(w, "Reservation accepted") // fallback message
		return http.StatusOK, append(warnings, "executetemplate"), summary
	}
	return http.StatusOK, warnings, summary
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
