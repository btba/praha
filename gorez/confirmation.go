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

	Warn                  bool
	GoogleTrackingID      string
	GoogleConversionID    template.JS
	GoogleConversionLabel string
	CDATABegin            template.JS
	CDATAEnd              template.JS
}

var knownConfCodes = map[string]bool{
	"110SouthSt": true,
	"124th":      true,
	"60thSt":     true,
	"A-Roger":    true,
	"C-Roger":    true,
	"D-Carlos":   true,
	"E-Carlos":   true,
	"F-Carlos":   true,
	"Gen-Carlos": true,
	"LIC-Carlos": true,
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
	switch {
	case vars.NumRiders < 1:
		return nil, warnings, &appError{http.StatusBadRequest, "NumRiders must be at least 1", nil}
	case vars.NumRiders > tourDetail.NumSpotsRemaining:
		warnings = append(warnings, fmt.Sprintf("riders(%d)>spots(%d)", vars.NumRiders, tourDetail.NumSpotsRemaining))
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
		warn := false
		if len(vars.Riders) < vars.NumRiders {
			warn = true
		} else {
			vars.Riders = vars.Riders[:vars.NumRiders]
		}
		for _, r := range vars.Riders {
			if r.Gender != "F" && r.Gender != "M" && r.Gender != "X" {
				r.Gender = "?"
				warn = true
			}
			if r.Height <= 0 {
				warn = true
			}
			riders = append(riders, Rider{r.Gender, r.Height})
		}
		if warn {
			warnings = append(warnings, "badheights")
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
		warnings = append(warnings, "noname")
	}
	if email == "" {
		warnings = append(warnings, "noemail")
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
		return nil, warnings, &appError{http.StatusInternalServerError, "Server error", fmt.Errorf("charge.New: %v", err)}
	}

	// At this point, checkout has succeeded.  Everything below is
	// optional.

	// Update order in database to record payment.
	if err := s.store.UpdateOrderPaymentRecorded(orderID); err != nil {
		s.log.Printf("UpdateOrderPaymentRecorded: %v", err)
		warnings = append(warnings, "updateorder:paymentrecorded")
	}

	// Gather data for email & web templates.
	data = &ConfirmationData{
		TourDetail:            tourDetail,
		NumRiders:             vars.NumRiders,
		DisplayTotal:          fmt.Sprintf("$%d.%02d", actualTotal/100, actualTotal%100),
		Name:                  name,
		Email:                 email,
		Mobile:                mobile,
		Hotel:                 hotel,
		Misc:                  misc,
		Warn:                  len(warnings) > 0,
		GoogleTrackingID:      s.googleTrackingID,
		GoogleConversionID:    template.JS(strconv.Itoa(s.googleConversionID)),
		GoogleConversionLabel: s.googleConversionLabel,
		CDATABegin:            template.JS("/* <![CDATA[ */"),
		CDATAEnd:              template.JS("/* ]]> */"),
	}

	// Email the customer.
	if s.emailTemplatesDir == "" {
		warnings = append(warnings, "noemailtemplate")
		return data, warnings, nil
	}
	tmpl, err := template.ParseFiles(path.Join(s.emailTemplatesDir, "confirmation.txt"))
	if err != nil {
		s.log.Printf("ParseFiles for email template: %v", err)
		warnings = append(warnings, "parseemailtemplate")
		return data, warnings, nil
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		s.log.Printf("Execute email template: %v", err)
		warnings = append(warnings, "executeemailtemplate")
		return data, warnings, nil
	}
	if !knownConfCodes[tourDetail.ConfCode] {
		warnings = append(warnings, fmt.Sprintf("unknownconfcode:%s", tourDetail.ConfCode))
	}
	emailCustomer := tourDetail.AutoConfirm && len(warnings) == 0
	subject := fmt.Sprintf("%s Tour %s Bike Tour Confirmation", tourDetail.Time.Format("January 2"), tourDetail.Code)
	if err := s.sendEmail(emailCustomer, name, email, subject, buf.String()); err != nil {
		s.log.Printf("sendemail: %v", err)
		warnings = append(warnings, "sendemail")
		return data, warnings, nil
	}
	if emailCustomer {
		// Update order in database to record confirmation email.
		if err := s.store.UpdateOrderConfirmationSent(orderID); err != nil {
			s.log.Printf("UpdateOrderConfirmationSent: %v", err)
			warnings = append(warnings, "updateorder:confirmationsent")
		}
	}
	return data, warnings, nil
}

func (s *Server) HandleConfirmation(w http.ResponseWriter, r *http.Request) (code int, warnings []string, summary string) {
	data, warnings, e := s.confirm(r)
	if e != nil {
		if e.Error != nil {
			s.log.Printf("%s: %v", e.Message, e.Error)
		}
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

func (s *Server) sendEmail(emailCustomer bool, name, email, subject, body string) error {
	var m *mail.SGMailV3
	if emailCustomer {
		from := mail.NewEmail("Bike the Big Apple reservations", "reservations@bikethebigapple.com")
		to := mail.NewEmail(name, email)
		bcc := mail.NewEmail("", "reservations@bikethebigapple.com")
		content := mail.NewContent("text/plain", body)
		m = mail.NewV3MailInit(from, subject, to, content)
		m.Personalizations[0].AddBCCs(bcc)
	} else {
		from := mail.NewEmail("Bike the Big Apple reservations", "reservations@bikethebigapple.com")
		to := mail.NewEmail("Bike the Big Apple reservations", "reservations@bikethebigapple.com")
		subject = "NO CONF SENT | " + subject
		content := mail.NewContent("text/plain", body)
		m = mail.NewV3MailInit(from, subject, to, content)
	}
	request := sendgrid.GetRequest(s.sendgridKey, "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = mail.GetRequestBody(m)
	_, err := sendgrid.API(request)
	return err
}
