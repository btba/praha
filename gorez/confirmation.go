package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	texttemplate "text/template"

	"github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/charge"
)

type RiderVars struct {
	Gender string
	Height int
}

// ConfirmationVars represents the form inputs.
type ConfirmationVars struct {
	TourID      int32
	NumRiders   int
	Riders      []RiderVars
	QuotedTotal float64

	Name        string
	StripeToken string

	Email  string
	Mobile string
	Hotel  string
	Misc   string
}

// ConfirmationData is the data passed to the templates for the
// customer email, BTBA email, and web response.
type ConfirmationData struct {
	TourDetail   *TourDetail
	NumRiders    int
	DisplayTotal string

	Name   string
	Email  string
	Mobile string
	Hotel  string
	Misc   string

	Warnings     map[warning]bool
	EmailSkipped string // empty if customer email sent

	GoogleTrackingID      string
	GoogleConversionID    template.JS
	GoogleConversionLabel string
	CDATABegin            template.JS
	CDATAEnd              template.JS

	NewTotalRiders int
	Teams          []*Team
}

type ConfirmationErrorData struct {
	Error            string
	GoogleTrackingID string
}

var knownConfCodes = map[string]bool{
	"110SouthSt": true,
	"124th":      true,
	"60thSt":     true,
	"A-Roger":    true,
	"B-Carlos":   true,
	"C-Roger":    true,
	"D-Carlos":   true,
	"E-Carlos":   true,
	"F-Carlos":   true,
	"Gen-Carlos": true,
	"LIC-Carlos": true,
}

func skipEmail(w map[warning]bool) bool {
	return w[WarningTourPast] || w[WarningTourFull] || w[WarningTourCancelled] || w[WarningTourDeleted] || w[WarningTourOversubscribed] || w[WarningInvalidHeights] || w[WarningNoName] || w[WarningNoEmail]
}

func (s *Server) confirm(r *http.Request) (*ConfirmationData, map[warning]bool, *appError) {
	warnings := make(map[warning]bool)
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
	//
	// NOTE: These checks are racy, but the conditions are unlikely.
	// - Tour deleted by admin
	// - Tour fields changed by admin (price, time, full, cancelled, deleted)
	// - Tour number of riders changed (due to concurrent purchase)
	// We could do the reads/checks/write in a transaction, retrying
	// when the commit fails due to a change in the number of riders.
	tourDetail, ok, err := s.store.GetTourDetailByID(vars.TourID, maxRiders)
	if err != nil {
		return nil, warnings, &appError{http.StatusInternalServerError, "Server error", fmt.Errorf("GetTourDetailByID: %v", err)}
	}
	if !ok {
		return nil, warnings, &appError{http.StatusBadRequest, fmt.Sprintf("Invalid tour ID %d", vars.TourID), nil}
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
	switch {
	case vars.NumRiders < 1:
		return nil, warnings, &appError{http.StatusBadRequest, "NumRiders must be at least 1", nil}
	case vars.NumRiders > tourDetail.NumSpotsRemaining:
		warnings[WarningTourOversubscribed] = true
	}
	// Compute totals in cents [float64(13.57) -> uint64(1357)] and validate.
	actualTotal := uint64(float64(vars.NumRiders)*tourDetail.Price*100 + 0.5)
	quotedTotal := uint64(vars.QuotedTotal*100 + 0.5)
	if actualTotal != quotedTotal {
		return nil, warnings, &appError{http.StatusBadRequest, "Pricing error", fmt.Errorf("quoted=%d, actual=%d", quotedTotal, actualTotal)}
	}

	// Validate genders & heights.
	var riders []Rider
	if tourDetail.HeightsNeeded {
		if len(vars.Riders) < vars.NumRiders {
			warnings[WarningInvalidHeights] = true
		} else {
			vars.Riders = vars.Riders[:vars.NumRiders]
		}
		for _, r := range vars.Riders {
			if r.Gender != "F" && r.Gender != "M" && r.Gender != "X" {
				r.Gender = "?"
				warnings[WarningInvalidHeights] = true
			}
			switch {
			case r.Height < 0:
				warnings[WarningUnknownHeights] = true
			case r.Height == 0:
				warnings[WarningInvalidHeights] = true
			}
			riders = append(riders, Rider{r.Gender, r.Height})
		}
	}

	// Trim strings and validate email.
	var (
		name   = strings.TrimSpace(vars.Name)
		email  = strings.TrimSpace(vars.Email)
		mobile = strings.TrimSpace(vars.Mobile)
		hotel  = strings.TrimSpace(vars.Hotel)
		misc   = strings.TrimSpace(vars.Misc)
	)
	if name == "" {
		warnings[WarningNoName] = true
	}
	if email == "" {
		warnings[WarningNoEmail] = true
	}

	// Add order to database.
	orderID, err := s.store.CreateOrder(vars.TourID, vars.NumRiders, riders, actualTotal, name, email, mobile, hotel, misc)
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
		if stripeErr, ok := err.(*stripe.Error); ok && stripeErr.Code == stripe.CardDeclined {
			return nil, warnings, &appError{http.StatusPaymentRequired, stripeErr.Msg, stripeErr}
		}
		return nil, warnings, &appError{http.StatusInternalServerError, "Server error", fmt.Errorf("charge.New: %v", err)}
	}

	// At this point, checkout has succeeded.  Everything below is
	// optional.

	// Update order in database to record payment.
	if err := s.store.UpdateOrderPaymentRecorded(orderID); err != nil {
		s.log.Printf("UpdateOrderPaymentRecorded: %v", err)
		warnings[WarningPaymentRecorded] = true
	}

	// Gather data for email & web templates.
	data := &ConfirmationData{
		TourDetail:            tourDetail,
		NumRiders:             vars.NumRiders,
		DisplayTotal:          fmt.Sprintf("$%d.%02d", actualTotal/100, actualTotal%100),
		Name:                  name,
		Email:                 email,
		Mobile:                mobile,
		Hotel:                 hotel,
		Misc:                  misc,
		Warnings:              warnings,
		GoogleTrackingID:      s.googleTrackingID,
		GoogleConversionID:    template.JS(strconv.Itoa(s.googleConversionID)),
		GoogleConversionLabel: s.googleConversionLabel,
		CDATABegin:            template.JS("/* <![CDATA[ */"),
		CDATAEnd:              template.JS("/* ]]> */"),
		NewTotalRiders:        tourDetail.TotalRiders + vars.NumRiders,
	}

	// Email the customer.
	if !tourDetail.AutoConfirm {
		data.EmailSkipped = "no auto confirm"
	} else if skipEmail(warnings) {
		data.EmailSkipped = fmt.Sprintf("warnings: %v", warningsList(warnings))
	} else if err := s.emailCustomer(data); err != nil {
		data.EmailSkipped = fmt.Sprintf("email send failure: %v", err)
		s.log.Printf("Error emailing customer: %v", err)
		warnings[WarningEmailCustomer] = true
	} else {
		// Update order in database to record confirmation email.
		if err := s.store.UpdateOrderConfirmationSent(orderID); err != nil {
			s.log.Printf("UpdateOrderConfirmationSent: %v", err)
			warnings[WarningConfirmationSent] = true
		}
	}
	// Email BTBA.
	data.Teams, err = s.store.GetTeams(vars.TourID)
	if err != nil {
		s.log.Printf("GetTeams: %v", err)
		warnings[WarningGetTeams] = true
	}
	if err := s.emailBTBA(data); err != nil {
		s.log.Printf("Error emailing BTBA: %v", err)
		warnings[WarningEmailBTBA] = true
	}

	return data, warnings, nil
}

func (s *Server) emailCustomer(data *ConfirmationData) error {
	if !knownConfCodes[data.TourDetail.ConfCode] {
		return fmt.Errorf("unknown conf code: %s", data.TourDetail.ConfCode)
	}
	tmpl, err := texttemplate.ParseFiles(path.Join(s.emailTemplatesDir, "customer.txt"))
	if err != nil {
		return fmt.Errorf("parse customer email template: %v", err)
	}
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("execute customer email template: %v", err)
	}
	from := mail.NewEmail("Bike the Big Apple reservations", "reservations@bikethebigapple.com")
	to := mail.NewEmail(data.Name, data.Email)
	bcc := mail.NewEmail("Bike the Big Apple reservations", "reservations@bikethebigapple.com")
	subject := fmt.Sprintf("%s Tour %s Bike Tour Confirmation", data.TourDetail.Time.Format("January 2"), data.TourDetail.Code)
	if err := s.sendEmail(from, to, bcc, subject, body.String()); err != nil {
		return fmt.Errorf("send customer email: %v", err)
	}
	return nil
}

func (s *Server) emailBTBA(data *ConfirmationData) error {
	tmpl, err := texttemplate.ParseFiles(path.Join(s.emailTemplatesDir, "btba.txt"))
	if err != nil {
		return fmt.Errorf("parse BTBA email template: %v", err)
	}
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("execute BTBA email template: %v", err)
	}
	from := mail.NewEmail("Bike the Big Apple reservations", "reservations@bikethebigapple.com")
	to := mail.NewEmail("Bike the Big Apple reservations", "reservations@bikethebigapple.com")
	subject := fmt.Sprintf("%s-%s | %dpax -> %dpax", data.TourDetail.Time.Format("Jan2"), data.TourDetail.Code, data.NumRiders, data.NewTotalRiders)
	if len(data.Misc) > 0 {
		subject += " | MSG"
	}
	if data.EmailSkipped != "" {
		subject += " | NO CONF SENT"
	}
	if data.Warnings[WarningUnknownHeights] {
		subject += " | NOHEIGHTS"
	}
	if err := s.sendEmail(from, to, nil /*bcc*/, subject, body.String()); err != nil {
		return fmt.Errorf("send BTBA email: %v", err)
	}
	return nil
}

func (s *Server) sendEmail(from, to, bcc *mail.Email, subject, body string) error {
	content := mail.NewContent("text/plain", body)
	m := mail.NewV3MailInit(from, subject, to, content)
	if bcc != nil {
		m.Personalizations[0].AddBCCs(bcc)
	}
	request := sendgrid.GetRequest(s.sendgridKey, "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = mail.GetRequestBody(m)
	_, err := sendgrid.API(request)
	return err
}

func (s *Server) HandleConfirmation(w http.ResponseWriter, r *http.Request) (code int, warnings map[warning]bool, summary string) {
	data, warnings, e := s.confirm(r)
	if e != nil {
		if e.Error != nil {
			s.log.Printf("%s: %v", e.Message, e.Error)
		}
		tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "confirmation_error.html"))
		if err != nil {
			s.log.Printf("%v", err)
			http.Error(w, e.Message, e.Code)
			return e.Code, warnings, e.Message
		}
		w.WriteHeader(e.Code)
		if err := tmpl.Execute(w, &ConfirmationErrorData{e.Message, s.googleTrackingID}); err != nil {
			s.log.Printf("%v", err)
			http.Error(w, e.Message, e.Code)
			return e.Code, warnings, e.Message
		}
		return e.Code, warnings, e.Message
	}
	summary = fmt.Sprintf("tour:%d riders:%d %s %q <%s>", data.TourDetail.ID, data.NumRiders, data.DisplayTotal, data.Name, data.Email)
	tmpl, err := template.ParseFiles(path.Join(s.templatesDir, "confirmation.html"))
	if err != nil {
		s.log.Printf("%v", err)
		fmt.Fprint(w, "Reservation accepted") // fallback message
		return http.StatusOK, warnings, summary
	}
	if err := tmpl.Execute(w, data); err != nil {
		s.log.Printf("%v", err)
		fmt.Fprint(w, "Reservation accepted") // fallback message
		return http.StatusOK, warnings, summary
	}
	return http.StatusOK, warnings, summary
}
